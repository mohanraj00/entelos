package protocol

// A Go runner for the approval API vectors in src/protocol/approval/vectors.
// It follows the vector rules of APPROVAL.md: a credentials file with a random
// token for each principal, a clock file for each step, the record-filter
// bundle of prepare.py for $record_bundle, and the markers $subset,
// $type object and $wait. The expected values come from the vector files,
// not from this Host.
//
// The steps go through the Unix socket of a Server, so each step runs as the
// principal of its own token. The parity steps also go to the Host directly,
// as the JSONL host sends them. qualify needs the CEL executable, so the test
// needs LIFECYCLE_CEL_BINARY and LIFECYCLE_CEL_BUNDLE, as the lifecycle
// scenario does. With LIFECYCLE_REQUIRE_SCENARIO, a missing value is a failure.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

var bundleFiles = map[string]string{"manifest": "manifest.json", "environment": "env.json", "source": "rule.cel", "fixed": "fixed.json"}

var bundleMimes = map[string]string{"manifest": "application/vnd.cel-handoff.bundle+json", "environment": "application/json",
	"source": "text/plain", "fixed": "application/json"}

func celRuntime(t *testing.T) (string, string) {
	t.Helper()
	binary, bundle := os.Getenv("LIFECYCLE_CEL_BINARY"), os.Getenv("LIFECYCLE_CEL_BUNDLE")
	if binary == "" || bundle == "" {
		if os.Getenv("LIFECYCLE_REQUIRE_SCENARIO") != "" {
			t.Fatal("LIFECYCLE_CEL_BINARY and LIFECYCLE_CEL_BUNDLE are not set")
		}
		t.Skip("LIFECYCLE_CEL_BINARY and LIFECYCLE_CEL_BUNDLE are not set")
	}
	return binary, bundle
}

func contentRef(mime string, raw []byte) string {
	sum := sha256.Sum256(append(append([]byte(mime), 0), raw...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// recordBundle is the bundle of examples/sidecar-client/run.py: the prepared
// record bundle with one fixed allowed_state.
func recordBundle(t *testing.T, dir string, fixed lifecycle.Object) lifecycle.Object {
	t.Helper()
	data := map[string][]byte{}
	for key, name := range bundleFiles {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		data[key] = raw
	}
	data["fixed"] = lifecycle.Encode(fixed)
	value, err := lifecycle.Decode(data["manifest"])
	if err != nil {
		t.Fatal(err)
	}
	manifest := value.(lifecycle.Object)
	for key, field := range map[string]string{"environment": "environment_ref", "source": "source_ref", "fixed": "fixed_ref"} {
		manifest[field] = contentRef(bundleMimes[key], data[key])
	}
	data["manifest"] = lifecycle.Encode(manifest)
	out := lifecycle.Object{}
	for k, v := range data {
		out[k] = string(v)
	}
	return out
}

// approvalSubstitute replaces $use and $record_bundle in a request.
func approvalSubstitute(t *testing.T, value any, bound map[string]string, bundleDir string) any {
	switch v := value.(type) {
	case lifecycle.Object:
		if len(v) == 1 {
			if name, ok := v["$use"].(string); ok {
				s, has := bound[name]
				if !has {
					t.Fatalf("$use of %s before its $capture", name)
				}
				return s
			}
			if fixed, ok := v["$record_bundle"].(lifecycle.Object); ok {
				return recordBundle(t, bundleDir, fixed)
			}
		}
		out := lifecycle.Object{}
		for k, x := range v {
			out[k] = approvalSubstitute(t, x, bound, bundleDir)
		}
		return out
	case lifecycle.Array:
		out := lifecycle.Array{}
		for _, x := range v {
			out = append(out, approvalSubstitute(t, x, bound, bundleDir))
		}
		return out
	}
	return value
}

// approvalMatch is match with the markers of APPROVAL.md. $use in an
// expected value must equal the captured value.
func approvalMatch(expected, actual any, bound map[string]string, where string) []string {
	if e, ok := expected.(lifecycle.Object); ok && len(e) == 1 {
		if name, ok := e["$capture"].(string); ok {
			s, isString := actual.(string)
			if !isString || s == "" {
				return []string{where + ": capture needs a nonempty string"}
			}
			if old, has := bound[name]; has && old != s {
				return []string{where + ": differs from captured " + name}
			}
			bound[name] = s
			return nil
		}
		if name, ok := e["$use"].(string); ok {
			if s, isString := actual.(string); !isString || s != bound[name] || s == "" {
				return []string{fmt.Sprintf("%s: %s != captured %s", where, lifecycle.Encode(actual), name)}
			}
			return nil
		}
		if kind, ok := e["$type"].(string); ok {
			_, isString := actual.(string)
			_, isObject := actual.(lifecycle.Object)
			if (kind == "string" && isString) || (kind == "object" && isObject) {
				return nil
			}
			return []string{where + ": expected a " + kind}
		}
		if fields, ok := e["$subset"].(lifecycle.Object); ok {
			a, isObject := actual.(lifecycle.Object)
			if !isObject {
				return []string{where + ": expected an object"}
			}
			var out []string
			for k, v := range fields {
				w, has := a[k]
				if !has {
					out = append(out, where+": missing key "+k)
					continue
				}
				out = append(out, approvalMatch(v, w, bound, where+"."+k)...)
			}
			return out
		}
		if wait, ok := e["$wait"].(lifecycle.Object); ok {
			if _, isInt := actual.(lifecycle.Int); !isInt || !sameValue(wait["seconds"], actual) {
				return []string{fmt.Sprintf("%s: wait %s != %s", where, lifecycle.Encode(actual), lifecycle.Encode(wait["seconds"]))}
			}
			return nil
		}
	}
	switch e := expected.(type) {
	case lifecycle.Object:
		a, ok := actual.(lifecycle.Object)
		if !ok || len(a) != len(e) {
			return []string{fmt.Sprintf("%s: keys differ: %s != %s", where, lifecycle.Encode(expected), lifecycle.Encode(actual))}
		}
		var out []string
		for _, k := range sortedNames(e) {
			w, has := a[k]
			if !has {
				return []string{where + ": missing key " + k}
			}
			out = append(out, approvalMatch(e[k], w, bound, where+"."+k)...)
		}
		return out
	case lifecycle.Array:
		a, ok := actual.(lifecycle.Array)
		if !ok || len(a) != len(e) {
			return []string{fmt.Sprintf("%s: list differs: %s != %s", where, lifecycle.Encode(expected), lifecycle.Encode(actual))}
		}
		var out []string
		for i := range e {
			out = append(out, approvalMatch(e[i], a[i], bound, fmt.Sprintf("%s[%d]", where, i))...)
		}
		return out
	}
	if !sameValue(expected, actual) || fmt.Sprintf("%T", expected) != fmt.Sprintf("%T", actual) {
		return []string{fmt.Sprintf("%s: %s != %s", where, lifecycle.Encode(expected), lifecycle.Encode(actual))}
	}
	return nil
}

// sortedNames gives the keys in order, so that each $capture comes before a
// $use of it in the same object when the names sort that way, and so that
// the messages have a stable order.
func sortedNames(o lifecycle.Object) []string {
	out := make([]string, 0, len(o))
	for k := range o {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func randomToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func writePrivate(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path+".new", raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
}

// writeCredentials writes the credentials file and gives the token of each
// principal name.
func writeCredentials(t *testing.T, path string, principals lifecycle.Object) map[string]string {
	t.Helper()
	tokens := map[string]string{}
	entries := lifecycle.Array{}
	for _, name := range sortedNames(principals) {
		p := principals[name].(lifecycle.Object)
		token := randomToken(t)
		tokens[name] = token
		sum := sha256.Sum256([]byte(token))
		entries = append(entries, lifecycle.Object{"token_sha256": hex.EncodeToString(sum[:]), "actor": p["actor"], "roles": p["roles"]})
	}
	writePrivate(t, path, lifecycle.Encode(lifecycle.Object{"format": CredentialsFormat, "principals": entries}))
	return tokens
}

func socketRequest(t *testing.T, token string, payload []byte) lifecycle.Object {
	t.Helper()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(60 * time.Second))
	return socketReply(t, conn, authenticatedFrame(token, payload))
}

func approvalHost(t *testing.T, store string, context any, clock string) *lifecycle.Host {
	t.Helper()
	binary, _ := celRuntime(t)
	repo, err := lifecycle.OpenGitStore(store)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := lifecycle.NewCELRuntime(binary, filepath.Join(filepath.Dir(store), "tmp-"+filepath.Base(store)))
	if err != nil {
		t.Fatal(err)
	}
	host, err := lifecycle.NewHost(repo, rt, context)
	if err != nil {
		t.Fatal(err)
	}
	host.Clock = lifecycle.FileClock{Path: clock}
	return host
}

// stepClocks gives the clock value of each step: its own clock, else the
// clock of the previous step, else the start clock of the file.
func stepClocks(doc lifecycle.Object) []string {
	now := doc["clock"].(string)
	var out []string
	for _, raw := range doc["steps"].(lifecycle.Array) {
		if c, ok := raw.(lifecycle.Object)["clock"].(string); ok {
			now = c
		}
		out = append(out, now)
	}
	return out
}

func readApprovalVector(t *testing.T, path string) lifecycle.Object {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(lifecycle.Object)
	if doc["protocol_version"] != "1.1" {
		t.Fatalf("%s: protocol_version is not 1.1", path)
	}
	return doc
}

func TestApprovalVectors(t *testing.T) {
	_, bundleDir := celRuntime(t)
	bundleDir, _ = filepath.Abs(bundleDir)
	s := loadTestSchemas(t)
	paths, _ := filepath.Glob(filepath.Join(protocolDir, "approval", "vectors", "approval-*.json"))
	if len(paths) != 3 {
		t.Fatalf("found %d approval vector files, want 3", len(paths))
	}
	total := 0
	for _, path := range paths {
		doc := readApprovalVector(t, path)
		t.Run(filepath.Base(path), func(t *testing.T) {
			inTempDir(t)
			dir, _ := os.Getwd()
			host := approvalHost(t, filepath.Join(dir, "socket.git"), doc["context"], filepath.Join(dir, "socket.clock"))
			tokens := writeCredentials(t, "credentials.json", doc["principals"].(lifecycle.Object))
			server, err := NewCredentialServer(NewAdapter(host, s), "credentials.json")
			if err != nil {
				t.Fatal(err)
			}
			l, err := Listen(sock)
			if err != nil {
				t.Fatal(err)
			}
			go server.Serve(l)
			defer server.Shutdown(l)
			clocks := stepClocks(doc)
			bound := map[string]string{}
			for i, raw := range doc["steps"].(lifecycle.Array) {
				step := raw.(lifecycle.Object)
				total++
				writePrivate(t, filepath.Join(dir, "socket.clock"), []byte(clocks[i]+"\n"))
				token := tokens[step["principal"].(string)]
				if step["principal"] == "$unknown" {
					token = randomToken(t)
				} else if token == "" {
					t.Fatalf("%s: unknown principal %v", step["name"], step["principal"])
				}
				request := approvalSubstitute(t, step["request"], bound, bundleDir).(lifecycle.Object)
				payload, err := Canonical(request)
				if err != nil {
					t.Fatal(err)
				}
				actual := normal(t, socketRequest(t, token, payload))
				expected := step["expect"].(lifecycle.Object)["response"]
				for _, e := range approvalMatch(expected, actual, bound, "response") {
					t.Errorf("%s: %s", step["name"], e)
				}
				for _, e := range shape(s, actual, request["operation"]) {
					t.Errorf("%s: %s", step["name"], e)
				}
			}
			if parity, ok := doc["parity"].(lifecycle.Array); ok {
				checkParity(t, doc, parity, bound, bundleDir, clocks, filepath.Join(dir, "jsonl.git"), filepath.Join(dir, "jsonl.clock"))
			}
		})
	}
	if total != 46 {
		t.Fatalf("ran %d approval steps; the vector files have 46", total)
	}
}

// checkParity sends each step with an expected success to the Host
// directly, as the JSONL host does, with the same principal and clock. The
// captured record references of each parity step must equal the socket
// values.
func checkParity(t *testing.T, doc lifecycle.Object, parity lifecycle.Array, socketBound map[string]string, bundleDir string, clocks []string, store, clock string) {
	t.Helper()
	host := approvalHost(t, store, doc["context"], clock)
	principals := doc["principals"].(lifecycle.Object)
	bound := map[string]string{}
	names := map[string]bool{}
	for _, n := range parity {
		names[n.(string)] = true
	}
	seen := 0
	for i, raw := range doc["steps"].(lifecycle.Array) {
		step := raw.(lifecycle.Object)
		response := step["expect"].(lifecycle.Object)["response"].(lifecycle.Object)
		if response["status"] != "ok" {
			continue
		}
		writePrivate(t, clock, []byte(clocks[i]+"\n"))
		request := approvalSubstitute(t, step["request"], bound, bundleDir).(lifecycle.Object)
		q := lifecycle.Object{"op": request["operation"]}
		for k, v := range request["body"].(lifecycle.Object) {
			q[k] = v
		}
		p := principals[step["principal"].(string)].(lifecycle.Object)
		var roles []string
		for _, r := range p["roles"].(lifecycle.Array) {
			roles = append(roles, r.(string))
		}
		result, err := host.HandleAs(p["actor"].(string), roles, q)
		if err != nil {
			t.Fatalf("parity %s: %v", step["name"], err)
		}
		for _, e := range approvalMatch(response["result"], normal(t, result), bound, "data") {
			t.Errorf("parity %s: %s", step["name"], e)
		}
		if !names[step["name"].(string)] {
			continue
		}
		seen++
		captures := map[string]bool{}
		collectCaptures(response, captures)
		for name := range captures {
			// A cursor is a Git commit of one store, with its own commit
			// time, so only record references must be equal.
			if !strings.HasPrefix(socketBound[name], "sha256:") {
				continue
			}
			if bound[name] != socketBound[name] {
				t.Errorf("parity %s: %s differs: socket %s, JSONL %s", step["name"], name, socketBound[name], bound[name])
			}
		}
	}
	if seen != len(parity) {
		t.Errorf("parity: ran %d of %d named steps", seen, len(parity))
	}
}

func collectCaptures(value any, out map[string]bool) {
	switch v := value.(type) {
	case lifecycle.Object:
		if name, ok := v["$capture"].(string); ok && len(v) == 1 {
			out[name] = true
			return
		}
		for _, x := range v {
			collectCaptures(x, out)
		}
	case lifecycle.Array:
		for _, x := range v {
			collectCaptures(x, out)
		}
	}
}
