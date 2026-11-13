// Command entelos-sidecar serves the wire protocol 1.0 of src/protocol and
// the approval API 1.1 of src/protocol/approval on a local Unix domain
// socket, in front of the Go lifecycle Host. The host context comes from a
// file that the operator selects; a request cannot supply it. Each socket
// request requires a credential of the operator: the one token of
// --token-file, or one principal of --credentials-file.
//
// --clock-file is for conformance tests only. The Host then reads its time
// from that file for each request, not from the system clock.
//
// --http-listen adds the HTTP binding of APPROVAL.md as a second transport.
// It needs --credentials-file for the socket and one HTTP credential mode:
// --issuer-file for signed claims, or --http-local-tokens for the static
// tokens of the credentials file on a loopback address. The address must be
// loopback unless --http-allow-remote is given. The sidecar has no TLS; a
// remote address needs a TLS front proxy. --http-static adds a directory of
// public files that a GET request reads, so a browser client loads its page
// from the same origin as the API.
//
// When the socket listens, the command writes one JSON line to stdout:
// {"socket":"<path>","status":"listening"}. With --http-listen, the line
// also has "http":"<address>". On SIGINT or SIGTERM, it closes
// the socket, removes the socket file and exits with status 0.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
	"github.com/mohanraj00/entelos/src/lifecycle-core/protocol"
)

func main() { os.Exit(run()) }

func run() int {
	socket := flag.String("socket", "", "socket path; it must not exist, or it must be a stale socket")
	repo := flag.String("repo", "", "dedicated bare Git repository")
	contextPath := flag.String("context", "", "trusted host context, not caller input")
	runtime := flag.String("runtime", "", "pinned CEL executable")
	work := flag.String("work", "", "work directory")
	schema := flag.String("schema", "", "the src/protocol/schema directory; the approval schemas are in ../approval/schema")
	tokenFile := flag.String("token-file", "", "private file containing the sidecar token")
	credentialsFile := flag.String("credentials-file", "", "private file with one token digest, actor and roles for each principal")
	clockFile := flag.String("clock-file", "", "for tests only: a file with the time of the next request")
	httpListen := flag.String("http-listen", "", "IP literal and port of the HTTP binding, for example 127.0.0.1:8740")
	issuerFile := flag.String("issuer-file", "", "private file with the issuer, the audience, the key set and the principals of signed claims")
	localTokens := flag.Bool("http-local-tokens", false, "local mode: accept the static tokens of --credentials-file over HTTP on loopback")
	allowRemote := flag.Bool("http-allow-remote", false, "permit an HTTP address that is not loopback; a TLS front proxy is necessary")
	httpStatic := flag.String("http-static", "", "directory of public static files to serve with GET on the HTTP binding, for example the approval dashboard")
	flag.Parse()
	if *socket == "" || *repo == "" || *contextPath == "" || *runtime == "" || *work == "" || *schema == "" || (*tokenFile == "") == (*credentialsFile == "") {
		fmt.Fprintln(os.Stderr, "entelos-sidecar: --socket, --repo, --context, --runtime, --work, --schema and one of --token-file or --credentials-file are required")
		return 2
	}
	schemas, err := protocol.LoadSchemas(*schema)
	if err != nil {
		return stop(err)
	}
	if err := schemas.LoadApproval(protocol.ApprovalDir(*schema)); err != nil {
		return stop(err)
	}
	httpFlags := *issuerFile != "" || *localTokens || *allowRemote || *httpStatic != ""
	if httpFlags && *httpListen == "" {
		return stop(fmt.Errorf("--issuer-file, --http-local-tokens, --http-allow-remote and --http-static need --http-listen"))
	}
	if *httpListen != "" && *credentialsFile == "" {
		return stop(fmt.Errorf("--http-listen needs --credentials-file"))
	}
	host, err := open(*repo, *contextPath, *runtime, *work, *credentialsFile != "")
	if err != nil {
		return stop(err)
	}
	if *clockFile != "" {
		host.Clock = lifecycle.FileClock{Path: *clockFile}
	}
	adapter := protocol.NewAdapter(host, schemas)
	var server *protocol.Server
	if *credentialsFile != "" {
		server, err = protocol.NewCredentialServer(adapter, *credentialsFile)
	} else {
		server, err = protocol.NewServer(adapter, *tokenFile)
	}
	if err != nil {
		return stop(err)
	}
	done := make(chan error, 2)
	var httpServer *protocol.HTTPServer
	ready := lifecycle.Object{"status": "listening", "socket": *socket}
	if *httpListen != "" {
		l, loopback, err := protocol.ListenHTTP(*httpListen, *allowRemote)
		if err != nil {
			return stop(err)
		}
		scope, _ := host.Context["scope"].(string)
		handler, err := protocol.NewHTTPHandler(adapter, protocol.HTTPConfig{
			IssuerFile: *issuerFile, CredentialsFile: *credentialsFile, LocalTokens: *localTokens,
			Loopback: loopback, Tenant: scope,
			Log: func(s string) { fmt.Fprintln(os.Stderr, "entelos-sidecar:", s) },
		})
		if err != nil {
			l.Close()
			return stop(err)
		}
		var served http.Handler = handler
		if *httpStatic != "" {
			static, err := protocol.NewStaticHandler(*httpStatic, handler, loopback)
			if err != nil {
				l.Close()
				return stop(err)
			}
			defer static.Close()
			served = static
		}
		httpServer = protocol.NewHTTPServer(served)
		ready["http"] = l.Addr().String()
		// The deferred Shutdown runs on each return after this point.
		defer httpServer.Shutdown()
		// If the HTTP server stops with an error, the sidecar stops, so a
		// supervisor sees the failure.
		go func() {
			if err := httpServer.Serve(l); err != nil {
				done <- fmt.Errorf("http: %w", err)
			}
		}()
	}
	listener, err := protocol.Listen(*socket)
	if err != nil {
		return stop(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() { done <- server.Serve(listener) }()
	os.Stdout.Write(lifecycle.Encode(ready))
	select {
	case <-signals:
	case err = <-done:
		if err != nil {
			fmt.Fprintln(os.Stderr, "entelos-sidecar:", err)
		}
	}
	if shutdownErr := server.Shutdown(listener); shutdownErr != nil {
		return stop(shutdownErr)
	}
	if err != nil {
		return 1
	}
	return 0
}

func stop(err error) int {
	fmt.Fprintln(os.Stderr, "entelos-sidecar:", err)
	return 1
}

// open makes the Host. With principals, the context must not contain actor
// or roles, so each actor comes from one source. Without principals, the
// context must contain both.
func open(repo, contextPath, binary, work string, principals bool) (*lifecycle.Host, error) {
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		return nil, err
	}
	context, err := lifecycle.Decode(raw)
	if err != nil {
		return nil, err
	}
	ctx, _ := context.(lifecycle.Object)
	_, hasActor := ctx["actor"]
	_, hasRoles := ctx["roles"]
	switch {
	case principals && (hasActor || hasRoles):
		return nil, lifecycle.NewFault("configuration_error", "with --credentials-file, the context must not contain actor or roles", "")
	case !principals && (!hasActor || !hasRoles):
		return nil, lifecycle.NewFault("configuration_error", "with --token-file, the context must contain actor and roles", "")
	}
	store, err := lifecycle.OpenGitStore(repo)
	if err != nil {
		return nil, err
	}
	rt, err := lifecycle.NewCELRuntime(binary, filepath.Join(work, "tmp"))
	if err != nil {
		return nil, err
	}
	return lifecycle.NewHost(store, rt, context)
}
