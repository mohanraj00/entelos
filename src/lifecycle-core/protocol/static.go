package protocol

// The static file option of the HTTP binding. A browser client, for example
// the approval dashboard, can then load its page from the same origin as the
// API. The binding sends no CORS headers, so this is the only way for a page
// to read a response. The files are public: a GET needs no credential, and a
// file gives no access to the API.

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

// maxStaticFile is the byte bound of one static file.
const maxStaticFile = 4 << 20

// staticTypes is the list of the permitted file extensions and their media
// types. A file with another extension gets 404.
var staticTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".mjs":  "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".json": "application/json",
	".svg":  "image/svg+xml",
}

// staticPolicy is the Content-Security-Policy of each static file. A page
// can load scripts, styles and data only from its own origin, and it cannot
// show markup from another origin.
const staticPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; " +
	"object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// StaticHandler serves the files of one directory with GET and HEAD, and
// sends each other request to the API handler. It reads the directory
// through os.Root, so a symbolic link cannot point out of the directory.
type StaticHandler struct {
	root     *os.Root
	api      http.Handler
	loopback bool
}

// NewStaticHandler opens dir. loopback must be the value that ListenHTTP
// gave, so the static files get the same Host header check as the API.
func NewStaticHandler(dir string, api http.Handler, loopback bool) (*StaticHandler, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("the static path must be a directory")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &StaticHandler{root: root, api: api, loopback: loopback}, nil
}

// Close closes the directory.
func (s *StaticHandler) Close() error { return s.root.Close() }

// staticName gives the file name of a URL path, or false if the path is not
// a permitted static path. "/" is index.html. A name segment that starts
// with a dot, an empty segment and a backslash are not permitted.
func staticName(p string) (string, bool) {
	if p == "/" {
		return "index.html", true
	}
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") || path.Clean(p) != p {
		return "", false
	}
	name := p[1:]
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return "", false
		}
	}
	if _, ok := staticTypes[path.Ext(name)]; !ok {
		return "", false
	}
	return name, true
}

func (s *StaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == HTTPPath || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		s.api.ServeHTTP(w, r)
		return
	}
	if s.loopback && !loopbackHost(r.Host) {
		http.Error(w, "the Host header must name a loopback host", http.StatusMisdirectedRequest)
		return
	}
	name, ok := staticName(r.URL.Path)
	if !ok || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	f, err := s.root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxStaticFile {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(f, maxStaticFile+1))
	if err != nil || len(body) > maxStaticFile {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Content-Type", staticTypes[path.Ext(name)])
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", staticPolicy)
	header.Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		w.Write(body)
	}
}
