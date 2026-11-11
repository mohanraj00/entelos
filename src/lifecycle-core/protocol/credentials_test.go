package protocol

// Go tests for the credentials file of approval/APPROVAL.md: the file checks
// at start, the check for each frame and the 1.0 decide result.

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

const credentialsPath = "credentials.json"

func digestOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func credentialsBody(entries ...lifecycle.Object) []byte {
	list := lifecycle.Array{}
	for _, e := range entries {
		list = append(list, e)
	}
	return lifecycle.Encode(lifecycle.Object{"format": CredentialsFormat, "principals": list})
}

func entry(token, actor string, roles ...string) lifecycle.Object {
	list := lifecycle.Array{}
	for _, r := range roles {
		list = append(list, r)
	}
	return lifecycle.Object{"token_sha256": digestOf(token), "actor": actor, "roles": list}
}

var tokenA, tokenB = strings.Repeat("a", tokenSize), strings.Repeat("b", tokenSize)

func TestNewCredentialServerRejectsInvalidFiles(t *testing.T) {
	inTempDir(t)
	valid := credentialsBody(entry(tokenA, "reviewer-a", "reviewer"))
	bodies := map[string][]byte{
		"wrong-format":     []byte(strings.Replace(string(valid), CredentialsFormat, "entelos.credentials/2", 1)),
		"no-principals":    credentialsBody(),
		"duplicate-digest": credentialsBody(entry(tokenA, "reviewer-a", "reviewer"), entry(tokenA, "reviewer-b", "reviewer")),
		"duplicate-actor":  credentialsBody(entry(tokenA, "reviewer-a", "reviewer"), entry(tokenB, "reviewer-a", "caller")),
		"unknown-role":     credentialsBody(entry(tokenA, "reviewer-a", "admin")),
		"no-roles":         credentialsBody(entry(tokenA, "reviewer-a")),
		"empty-actor":      credentialsBody(entry(tokenA, "", "reviewer")),
		"extra-field":      credentialsBody(lifecycle.Object{"token_sha256": digestOf(tokenA), "actor": "a", "roles": lifecycle.Array{"reviewer"}, "token": tokenA}),
		"upper-digest":     credentialsBody(lifecycle.Object{"token_sha256": strings.ToUpper(digestOf(tokenA)), "actor": "a", "roles": lifecycle.Array{"reviewer"}}),
		"not-json":         []byte("{"),
	}
	for name, body := range bodies {
		if err := os.WriteFile(name, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile("broad-mode", valid, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialsPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(credentialsPath, "link"); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo("fifo", 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"missing", "broad-mode", "link", "fifo"}, sortedKeys(bodies)...) {
		if _, err := NewCredentialServer(nil, name); err == nil {
			t.Errorf("%s: accepted an invalid credentials file", name)
		}
	}
	if _, err := NewCredentialServer(nil, credentialsPath); err != nil {
		t.Fatalf("refused a valid credentials file: %v", err)
	}
}

func sortedKeys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The owner check needs a file of another user. Only root can make one.
func TestCredentialsFileOfAnotherOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("only root can give a file to another user")
	}
	inTempDir(t)
	if err := os.WriteFile(credentialsPath, credentialsBody(entry(tokenA, "reviewer-a", "reviewer")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(credentialsPath, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCredentialServer(nil, credentialsPath); err == nil {
		t.Fatal("accepted a credentials file of another user")
	}
}

// principalHost gives the actor and roles of each request.
type principalHost struct{}

func (principalHost) Handle(any) (any, error) {
	return lifecycle.Object{"cursor": "", "state": lifecycle.Object{"actor": "host-context"}}, nil
}

func (principalHost) HandleAs(actor string, roles []string, _ any) (any, error) {
	list := lifecycle.Array{}
	for _, r := range roles {
		list = append(list, r)
	}
	return lifecycle.Object{"cursor": "", "state": lifecycle.Object{"actor": actor, "roles": list}}, nil
}

// Each frame runs as the principal of its own token. A principal that the
// operator removes gets unauthenticated on its next frame, also on a
// connection that is already open.
func TestCredentialRemovedOnOpenConnection(t *testing.T) {
	inTempDir(t)
	if err := os.WriteFile(credentialsPath, credentialsBody(entry(tokenA, "reviewer-a", "reviewer"), entry(tokenB, "reader-b", "reader", "caller")), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewCredentialServer(NewAdapter(principalHost{}, loadTestSchemas(t)), credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve(l)
	defer s.Shutdown(l)
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	inspect := []byte(`{"body":{},"operation":"inspect","protocol":"1.1","request_id":"r"}`)
	ask := func(token string) lifecycle.Object {
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		return socketReply(t, conn, authenticatedFrame(token, inspect))
	}
	for token, actor := range map[string]string{tokenA: "reviewer-a", tokenB: "reader-b"} {
		got := ask(token)
		if at(got, "result", "state", "actor") != actor {
			t.Fatalf("token of %s ran as %v", actor, lifecycle.Encode(got))
		}
	}
	// The operator removes reviewer-a. The next frame of reviewer-a on the
	// same connection gets unauthenticated, and the Host gets no request.
	if err := os.WriteFile(credentialsPath+".new", credentialsBody(entry(tokenB, "reader-b", "reader")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(credentialsPath+".new", credentialsPath); err != nil {
		t.Fatal(err)
	}
	got := ask(tokenA)
	if got["status"] != "unauthenticated" || got["protocol"] != "1.0" || got["request_id"] != "" {
		t.Fatalf("removed principal got %v", lifecycle.Encode(got))
	}
}

// A file that becomes invalid gives unauthenticated, not the last valid
// principals.
func TestInvalidCredentialsFileRefusesEachFrame(t *testing.T) {
	inTempDir(t)
	if err := os.WriteFile(credentialsPath, credentialsBody(entry(tokenA, "reviewer-a", "reviewer")), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewCredentialServer(NewAdapter(principalHost{}, loadTestSchemas(t)), credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve(l)
	defer s.Shutdown(l)
	if err := os.Chmod(credentialsPath, 0o644); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	got := socketReply(t, conn, authenticatedFrame(tokenA, []byte(`{"body":{},"operation":"inspect","protocol":"1.1","request_id":"r"}`)))
	if got["status"] != "unauthenticated" {
		t.Fatalf("a file with mode 0644 gave %v", lifecycle.Encode(got))
	}
}

func at(value any, path ...string) any {
	for _, k := range path {
		o, ok := value.(lifecycle.Object)
		if !ok {
			return nil
		}
		value = o[k]
	}
	return value
}

// decideHost gives a 1.1 decide result.
type decideHost struct{}

func (decideHost) Handle(any) (any, error) {
	return lifecycle.Object{"cursor": "c", "reused": false, "result": lifecycle.Object{"decision_ref": "sha256:" + strings.Repeat("d", 64),
		"decision": lifecycle.Object{"format": "entelos.record/1"}}}, nil
}

// A 1.0 decide request gets only decision_ref, so the 1.0 result does not
// change. A 1.1 request also gets the decision record.
func TestDecideResultByVersion(t *testing.T) {
	a := NewAdapter(decideHost{}, loadTestSchemas(t))
	body := lifecycle.Object{"report_ref": "sha256:" + strings.Repeat("0", 64), "reason": "r", "expected_revision": "", "request_key": "k"}
	for version, keys := range map[string]int{"1.0": 1, "1.1": 2} {
		got := a.Message(lifecycle.Object{"protocol": version, "request_id": "r", "operation": "decide", "body": body})
		result, _ := at(got, "result", "result").(lifecycle.Object)
		if got["protocol"] != version || len(result) != keys {
			t.Errorf("%s: %s", version, lifecycle.Encode(got))
		}
		// The stub decision has only a format, so only the 1.0 result is a
		// complete result for its schema.
		if errs := shape(a.Schemas, normal(t, got), "decide"); version == "1.0" && len(errs) > 0 {
			t.Errorf("%s: %v", version, errs)
		}
	}
}
