package protocol

// TestSidecarAuthMatrix sends each wire operation of each protocol version
// with no credential, a wrong credential and an ended credential, on each
// transport mode of the sidecar (#237). The table in
// cmd/entelos-sidecar/AUTH.md lists the same operations and versions.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// spyHost counts the requests that get to the Host.
type spyHost struct {
	host  *lifecycle.Host
	calls *atomic.Int64
}

func (s spyHost) Handle(q any) (any, error) {
	s.calls.Add(1)
	return s.host.Handle(q)
}

func (s spyHost) HandleAs(actor string, roles []string, q any) (any, error) {
	s.calls.Add(1)
	return s.host.HandleAs(actor, roles, q)
}

// wireOperations gives the first minor version of each operation that the
// loaded schemas define.
func wireOperations(s *Schemas) map[string]int {
	names := map[string]bool{}
	for _, set := range []map[string]lifecycle.Object{s.Operations, s.Approval, s.approval12, s.approval13} {
		for name := range set {
			names[name] = true
		}
	}
	first := map[string]int{}
	for name := range names {
		for minor := 0; minor <= 3; minor++ {
			if _, ok := s.Operation(name, minor); ok {
				first[name] = minor
				break
			}
		}
	}
	return first
}

var authRow = regexp.MustCompile("^\\| `([a-z_]+)` \\| 1\\.([0-3])(?: to 1\\.3)? \\| (read|write) \\| `[a-z_]+` \\|$")

func authTable(t *testing.T, path string) map[string]int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]int{}
	for _, line := range regexp.MustCompile("\r?\n").Split(string(raw), -1) {
		if m := authRow.FindStringSubmatch(line); m != nil {
			if _, dup := rows[m[1]]; dup {
				t.Errorf("AUTH.md: duplicate row %s", m[1])
			}
			minor, _ := strconv.Atoi(m[2])
			rows[m[1]] = minor
			if (m[3] == "read") != lifecycle.Reads[m[1]] {
				t.Errorf("AUTH.md: access of %s", m[1])
			}
		}
	}
	return rows
}

// storeDigest gives one digest of the names and the bytes of each file of a
// directory tree.
func storeDigest(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(h, "%s\x00%v\x00", rel, d.IsDir())
		if d.Type().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(raw)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func credentialsOf(t *testing.T, path string, tokens map[string]string) {
	t.Helper()
	entries := lifecycle.Array{}
	names := make([]string, 0, len(tokens))
	for actor := range tokens {
		names = append(names, actor)
	}
	sort.Strings(names)
	for _, actor := range names {
		sum := sha256.Sum256([]byte(tokens[actor]))
		entries = append(entries, lifecycle.Object{"token_sha256": hex.EncodeToString(sum[:]), "actor": actor,
			"roles": lifecycle.Array{"reviewer", "author", "reader"}})
	}
	writePrivate(t, path, lifecycle.Encode(lifecycle.Object{"format": CredentialsFormat, "principals": entries}))
}

// authMode is one transport mode. send gives the HTTP status (0 on the
// socket) and the response of one request with one credential case.
type authMode struct {
	name  string
	calls *atomic.Int64
	store string
	send  func(credential string, payload []byte) (int, lifecycle.Object)
	// cases maps each refused credential case to its credential. The
	// empty credential is no credential.
	cases map[string]string
	valid string
}

func authHost(t *testing.T, store string, context lifecycle.Object) (spyHost, *atomic.Int64) {
	t.Helper()
	repo, err := lifecycle.OpenGitStore(store)
	if err != nil {
		t.Fatal(err)
	}
	host, err := lifecycle.NewHost(repo, noRuntime{}, context)
	if err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int64{}
	return spyHost{host, calls}, calls
}

func authContext(principal bool) lifecycle.Object {
	context := lifecycle.Object{"scope": testTenant, "runner_actor": "fixture-runner", "worker": "local-worker",
		"functions": lifecycle.Array{"demo-function"}}
	if principal {
		context["actor"], context["roles"] = "reviewer-a", lifecycle.Array{"reviewer", "author", "reader"}
	}
	return context
}

func socketSend(t *testing.T, path string) func(string, []byte) (int, lifecycle.Object) {
	return func(token string, payload []byte) (int, lifecycle.Object) {
		t.Helper()
		conn, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		wire := frameOf(payload)
		if token != "" {
			wire = authenticatedFrame(token, payload)
		}
		return 0, socketReply(t, conn, wire)
	}
}

func TestSidecarAuthMatrix(t *testing.T) {
	table, err := filepath.Abs(filepath.Join("..", "cmd", "entelos-sidecar", "AUTH.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := loadTestSchemas(t)
	operations := wireOperations(s)
	if documented := authTable(t, table); fmt.Sprint(documented) != fmt.Sprint(operations) {
		t.Fatalf("AUTH.md lists %v; the schemas define %v", documented, operations)
	}
	if max := NewAdapter(nil, s).maxMinor(); max != 3 {
		t.Fatalf("the table ends at 1.3; the adapter supports 1.%d", max)
	}
	inTempDir(t)
	var modes []authMode

	// Socket with the operator token file. A rotation replaces the old token.
	{
		host, calls := authHost(t, "token.git", authContext(true))
		old, current := randomToken(t), randomToken(t)
		writePrivate(t, "operator.token", []byte(old+"\n"))
		server, err := NewServer(NewAdapter(host, s), "operator.token")
		if err != nil {
			t.Fatal(err)
		}
		l, err := Listen("token.sock")
		if err != nil {
			t.Fatal(err)
		}
		go server.Serve(l)
		t.Cleanup(func() { server.Shutdown(l) })
		writePrivate(t, "operator.token", []byte(current+"\n"))
		modes = append(modes, authMode{"socket-token", calls, "token.git", socketSend(t, "token.sock"),
			map[string]string{"none": "", "wrong": randomToken(t), "ended": old}, current})
	}

	// Socket and HTTP with the credentials file. The operator removes one
	// principal after the start.
	tokens := map[string]string{"reviewer-a": randomToken(t), "removed-b": randomToken(t)}
	credentialsOf(t, "credentials.json", tokens)
	{
		host, calls := authHost(t, "credentials.git", authContext(false))
		server, err := NewCredentialServer(NewAdapter(host, s), "credentials.json")
		if err != nil {
			t.Fatal(err)
		}
		l, err := Listen("credentials.sock")
		if err != nil {
			t.Fatal(err)
		}
		go server.Serve(l)
		t.Cleanup(func() { server.Shutdown(l) })
		modes = append(modes, authMode{"socket-credentials", calls, "credentials.git", socketSend(t, "credentials.sock"),
			map[string]string{"none": "", "wrong": randomToken(t), "ended": tokens["removed-b"]}, tokens["reviewer-a"]})
	}
	httpSend := func(url string) func(string, []byte) (int, lifecycle.Object) {
		return func(bearer string, payload []byte) (int, lifecycle.Object) { return post(t, url, bearer, payload, nil) }
	}
	{
		host, calls := authHost(t, "static.git", authContext(false))
		url := startHTTP(t, HTTPConfig{CredentialsFile: "credentials.json", LocalTokens: true}, NewAdapter(host, s))
		modes = append(modes, authMode{"http-static-token", calls, "static.git", httpSend(url),
			map[string]string{"none": "", "wrong": randomToken(t), "ended": tokens["removed-b"]}, tokens["reviewer-a"]})
	}
	{
		host, calls := authHost(t, "claim.git", authContext(false))
		key, other := newKey(t, "k1"), newKey(t, "k1")
		writeIssuer(t, "issuer.json", []testKey{key}, lifecycle.Array{
			lifecycle.Object{"subject": "reviewer-a", "roles": lifecycle.Array{"reviewer", "author", "reader"}}})
		url := startHTTP(t, HTTPConfig{IssuerFile: "issuer.json", CredentialsFile: "credentials.json", Tenant: testTenant,
			Now: func() time.Time { return testNow }, Log: func(string) {}}, NewAdapter(host, s))
		modes = append(modes, authMode{"http-signed-claim", calls, "claim.git", httpSend(url),
			map[string]string{"none": "", "wrong": mint(other, claimBody("reviewer-a", testNow)),
				"ended": mint(key, claimBody("reviewer-a", testNow.Add(-time.Hour)))},
			mint(key, claimBody("reviewer-a", testNow))})
	}
	delete(tokens, "removed-b")
	credentialsOf(t, "credentials.json", tokens)

	names := make([]string, 0, len(operations))
	for name := range operations {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			before := storeDigest(t, mode.store)
			sent := 0
			for _, op := range names {
				for minor := operations[op]; minor <= 3; minor++ {
					for _, kind := range []string{"none", "wrong", "ended"} {
						version := "1." + strconv.Itoa(minor)
						payload, err := Canonical(lifecycle.Object{"protocol": version, "request_id": "auth-1", "operation": op,
							"body": lifecycle.Object{}})
						if err != nil {
							t.Fatal(err)
						}
						status, got := mode.send(mode.cases[kind], payload)
						sent++
						d, _ := got["diagnostic"].(lifecycle.Object)
						wantStatus := 0
						if mode.name[:4] == "http" {
							wantStatus = http.StatusUnauthorized
						}
						if status != wantStatus || got["status"] != "unauthenticated" || d["code"] != "unauthenticated" ||
							d["stage"] != "authentication" || got["protocol"] != "1.0" {
							t.Errorf("%s %s %s: got %d %s", op, version, kind, status, lifecycle.Encode(got))
						}
					}
				}
			}
			if n := mode.calls.Load(); n != 0 {
				t.Fatalf("the Host got %d of %d refused requests", n, sent)
			}
			if after := storeDigest(t, mode.store); after != before {
				t.Fatal("a refused request changed the store")
			}
			if sent != 3*99 {
				t.Fatalf("sent %d requests; 99 operation versions need %d", sent, 3*99)
			}
			// A valid credential gets to the Host, so the setup is correct.
			payload, _ := Canonical(lifecycle.Object{"protocol": "1.1", "request_id": "auth-2", "operation": "inspect",
				"body": lifecycle.Object{}})
			_, got := mode.send(mode.valid, payload)
			if got["status"] != "ok" || mode.calls.Load() != 1 {
				t.Fatalf("valid credential: %s, %d Host calls", lifecycle.Encode(got), mode.calls.Load())
			}
		})
	}
}
