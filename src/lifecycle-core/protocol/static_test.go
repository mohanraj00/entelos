package protocol

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticHandler(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.json"), []byte(`{"secret":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"index.html":       "<!doctype html><title>x</title>",
		"app/main.js":      "export {};",
		"style.css":        "body{}",
		"notes.txt":        "not a permitted type",
		".hidden.json":     "{}",
		"big.json":         strings.Repeat(" ", maxStaticFile+1),
		"kit/catalog.json": `{"a":1}`,
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API", "1")
		w.WriteHeader(http.StatusTeapot)
	})
	h, err := NewStaticHandler(dir, api, true)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := NewStaticHandler(filepath.Join(dir, "index.html"), api, true); err == nil {
		t.Fatal("a file as the static path was accepted")
	}

	cases := []struct {
		name, method, target, host string
		status                     int
		api                        bool
		media, body                string
	}{
		{"root is index.html", "GET", "/", "127.0.0.1:1", 200, false, "text/html; charset=utf-8", "<!doctype html><title>x</title>"},
		{"nested module", "GET", "/app/main.js", "localhost:1", 200, false, "text/javascript; charset=utf-8", "export {};"},
		{"head has no body", "HEAD", "/style.css", "127.0.0.1:1", 200, false, "text/css; charset=utf-8", ""},
		{"json", "GET", "/kit/catalog.json", "127.0.0.1:1", 200, false, "application/json", `{"a":1}`},
		{"API path goes to the API", "GET", HTTPPath, "127.0.0.1:1", 418, true, "", ""},
		{"POST goes to the API", "POST", "/index.html", "127.0.0.1:1", 418, true, "", ""},
		{"other host", "GET", "/", "example.com", 421, false, "", ""},
		{"type not permitted", "GET", "/notes.txt", "127.0.0.1:1", 404, false, "", ""},
		{"hidden file", "GET", "/.hidden.json", "127.0.0.1:1", 404, false, "", ""},
		{"dot segment", "GET", "/app/../index.html", "127.0.0.1:1", 404, false, "", ""},
		{"encoded dot segment", "GET", "/app/%2e%2e/index.html", "127.0.0.1:1", 404, false, "", ""},
		{"double slash", "GET", "//index.html", "127.0.0.1:1", 404, false, "", ""},
		{"query", "GET", "/index.html?x=1", "127.0.0.1:1", 404, false, "", ""},
		{"missing", "GET", "/missing.js", "127.0.0.1:1", 404, false, "", ""},
		{"symlink out of the root", "GET", "/link.json", "127.0.0.1:1", 404, false, "", ""},
		{"directory", "GET", "/folder.json", "127.0.0.1:1", 404, false, "", ""},
		{"file over the bound", "GET", "/big.json", "127.0.0.1:1", 404, false, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "http://127.0.0.1:1"+c.target, nil)
			r.Host = c.host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.status {
				t.Fatalf("status %d, want %d", w.Code, c.status)
			}
			if (w.Header().Get("X-API") == "1") != c.api {
				t.Fatalf("API handler use %v, want %v", w.Header().Get("X-API") == "1", c.api)
			}
			if c.status != 200 {
				if strings.Contains(w.Body.String(), "secret") {
					t.Fatal("the response contains a file from outside the root")
				}
				return
			}
			got := w.Header()
			if got.Get("Content-Type") != c.media || got.Get("Content-Security-Policy") != staticPolicy ||
				got.Get("X-Content-Type-Options") != "nosniff" || got.Get("Cache-Control") != "no-store" {
				t.Fatalf("headers %v", got)
			}
			if w.Body.String() != c.body {
				t.Fatalf("body %q, want %q", w.Body.String(), c.body)
			}
		})
	}

	remote, err := NewStaticHandler(dir, api, false)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()
	remote.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("a remote listener refused its own host: %d", w.Code)
	}
}
