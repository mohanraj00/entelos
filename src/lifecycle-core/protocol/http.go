package protocol

// The HTTP binding of approval/APPROVAL.md. It is a second transport for the
// same request and response objects as the socket. The Host and its role
// checks do not change.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// HTTPPath is the one path of the HTTP binding.
const HTTPPath = "/v1/request"

const (
	maxHeaderBytes   = 16384
	readHeaderWait   = 10 * time.Second
	httpReadWait     = 30 * time.Second
	httpWriteWait    = 5 * time.Minute
	httpIdleWait     = 60 * time.Second
	httpShutdownWait = 10 * time.Second
)

// HTTPConfig selects one credential mode of the HTTP binding. IssuerFile
// selects signed claims. LocalTokens selects the static tokens of
// CredentialsFile, on a loopback address only.
type HTTPConfig struct {
	IssuerFile      string
	CredentialsFile string
	LocalTokens     bool
	// Loopback makes the handler refuse a Host header that is not a
	// loopback name. ListenHTTP gives this value.
	Loopback bool
	// Tenant is the scope of the Host context. A claim must name it.
	Tenant string
	// Now is the clock of the claim checks. It is the system clock, not the
	// test clock of the Host. Nil means time.Now.
	Now func() time.Time
	// Log gets the reason of each refused credential. It never gets a token.
	Log func(string)
}

// HTTPHandler serves an Adapter over HTTP.
type HTTPHandler struct {
	adapter *Adapter
	config  HTTPConfig
}

// NewHTTPHandler checks the configuration and the credential file of its
// mode. Each request reads that file again, so a removed principal or key
// gets unauthenticated on its next request.
func NewHTTPHandler(adapter *Adapter, config HTTPConfig) (*HTTPHandler, error) {
	switch {
	case config.LocalTokens == (config.IssuerFile != ""):
		return nil, errors.New("the HTTP binding needs one of an issuer file or local tokens")
	case config.LocalTokens && config.CredentialsFile == "":
		return nil, errors.New("local tokens need a credentials file")
	case config.LocalTokens && !config.Loopback:
		return nil, errors.New("local tokens are permitted only on a loopback address")
	}
	if config.LocalTokens {
		if _, err := readCredentials(config.CredentialsFile); err != nil {
			return nil, fmt.Errorf("credentials file: %w", err)
		}
	} else {
		is, err := readIssuer(config.IssuerFile)
		if err != nil {
			return nil, fmt.Errorf("issuer file: %w", err)
		}
		if !validName(config.Tenant) {
			return nil, errors.New("signed claims need the scope of the host context as the tenant")
		}
		if config.CredentialsFile != "" {
			if err := sameRoles(is, config.CredentialsFile); err != nil {
				return nil, err
			}
		}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Log == nil {
		config.Log = func(string) {}
	}
	return &HTTPHandler{adapter: adapter, config: config}, nil
}

// sameRoles refuses an actor that the issuer file and the credentials file
// both name with different roles. Thus the actor has one set of roles on
// both transports.
func sameRoles(is *issuer, credentialsFile string) error {
	entries, err := readCredentials(credentialsFile)
	if err != nil {
		return fmt.Errorf("credentials file: %w", err)
	}
	for _, e := range entries {
		roles, has := is.roles[e.actor]
		if has && strings.Join(sortedCopy(roles), ",") != strings.Join(sortedCopy(e.roles), ",") {
			return fmt.Errorf("actor %q has other roles in the issuer file than in the credentials file", e.actor)
		}
	}
	return nil
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func (h *HTTPHandler) principal(r *http.Request) (*Principal, error) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return nil, errors.New("request needs exactly one Authorization header")
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t") {
		return nil, errors.New("Authorization must be Bearer and one credential")
	}
	if h.config.LocalTokens {
		p, err := staticPrincipal(token, h.config.CredentialsFile)
		if err != nil {
			return nil, errors.New("static token is not in the credentials file")
		}
		return p, nil
	}
	is, err := readIssuer(h.config.IssuerFile)
	if err != nil {
		return nil, fmt.Errorf("issuer file: %w", err)
	}
	return is.verifyClaim(token, h.config.Now(), h.config.Tenant)
}

// loopbackHost tells if the Host header names a loopback host. This refuses
// a request that a DNS rebinding sends to a loopback listener.
func loopbackHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case h.config.Loopback && !loopbackHost(r.Host):
		h.reply(w, http.StatusMisdirectedRequest, failure("", "invalid_request", "protocol", "the Host header must name a loopback host"))
		return
	case r.URL.Path != HTTPPath || r.URL.RawQuery != "":
		h.reply(w, http.StatusNotFound, failure("", "invalid_request", "protocol", "the only path is POST "+HTTPPath))
		return
	case r.Method != http.MethodPost:
		w.Header().Set("Allow", http.MethodPost)
		h.reply(w, http.StatusMethodNotAllowed, failure("", "invalid_request", "protocol", "the only method is POST"))
		return
	}
	// The credential check comes before the body is read, as the socket
	// checks the prelude before it reads the frame length.
	p, err := h.principal(r)
	if err != nil {
		h.config.Log("unauthenticated HTTP request: " + err.Error())
		w.Header().Set("WWW-Authenticate", "Bearer")
		message := errToken.Error()
		if !h.config.LocalTokens {
			message = errClaim.Error()
		}
		h.reply(w, http.StatusUnauthorized, failure("", "unauthenticated", "authentication", message))
		return
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	charset, hasCharset := params["charset"]
	if err != nil || media != "application/json" || len(params) > 1 || (len(params) == 1 && (!hasCharset || !strings.EqualFold(charset, "utf-8"))) {
		h.reply(w, http.StatusUnsupportedMediaType, failure("", "invalid_request", "protocol", "Content-Type must be application/json"))
		return
	}
	if r.ContentLength > MaxFrame {
		h.reply(w, http.StatusRequestEntityTooLarge, failure("", "invalid_frame", "protocol", "frame exceeds the byte bound"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxFrame+1))
	switch {
	case err != nil:
		h.reply(w, http.StatusBadRequest, failure("", "invalid_frame", "protocol", "truncated frame payload"))
		return
	case len(body) > MaxFrame:
		h.reply(w, http.StatusRequestEntityTooLarge, failure("", "invalid_frame", "protocol", "frame exceeds the byte bound"))
		return
	}
	response, _ := h.adapter.FrameAs(p, body)
	code, _ := response["status"].(string)
	h.reply(w, httpStatus(code), response)
}

// httpStatus maps a response status to an HTTP status. The body is the
// record; the HTTP status is only a hint for clients and proxies.
func httpStatus(code string) int {
	switch code {
	case "ok":
		return http.StatusOK
	case "unauthenticated":
		return http.StatusUnauthorized
	case "forbidden":
		return http.StatusForbidden
	case "invalid_request", "invalid_frame", "unsupported_version":
		return http.StatusBadRequest
	case "internal_error", "response_too_large":
		return http.StatusInternalServerError
	}
	return http.StatusConflict
}

func (h *HTTPHandler) reply(w http.ResponseWriter, status int, response map[string]any) {
	body, err := Canonical(limit(response))
	if err != nil {
		status = http.StatusInternalServerError
		body, _ = Canonical(internal(""))
	}
	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	w.Write(body)
}

// ListenHTTP listens on addr, an IP literal and a port. It refuses an
// address that is not loopback unless allowRemote is true. It gives true if
// the address is loopback.
func ListenHTTP(addr string, allowRemote bool) (net.Listener, bool, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, false, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, false, errors.New("the HTTP address must be an IP literal and a port")
	}
	loopback := ip.IsLoopback()
	if !loopback && !allowRemote {
		return nil, false, errors.New("the HTTP address must be loopback; a remote address needs --http-allow-remote and a TLS front proxy")
	}
	l, err := net.Listen("tcp", addr)
	return l, loopback, err
}

// HTTPServer is an http.Server with the bounds of the HTTP binding.
type HTTPServer struct {
	server *http.Server
}

// NewHTTPServer makes the server for one handler.
func NewHTTPServer(handler http.Handler) *HTTPServer {
	return &HTTPServer{server: &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderWait,
		ReadTimeout:       httpReadWait,
		WriteTimeout:      httpWriteWait,
		IdleTimeout:       httpIdleWait,
		MaxHeaderBytes:    maxHeaderBytes,
	}}
}

// Serve accepts connections until Shutdown.
func (s *HTTPServer) Serve(l net.Listener) error {
	err := s.server.Serve(l)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops new requests and waits a bounded time for open requests.
func (s *HTTPServer) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), httpShutdownWait)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		return s.server.Close()
	}
	return nil
}
