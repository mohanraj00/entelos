package protocol

// Compatibility test for the wire protocol. It starts from
// testdata/migration/v080-store.json and v080-wire.json. The v0.8.0 code
// made the store and answered the requests of each protocol version.
// testdata/migration/make_v080_fixture.sh states how. See "Compatibility
// promise for 1.x releases" in docs/STABILITY.md.

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

func readCompatFixture(t *testing.T, name, format string) lifecycle.Object {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "testdata", "migration", name))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(lifecycle.Object)
	if doc["format"] != format || doc["made_by"].(lifecycle.Object)["tag"] != "v0.8.0" {
		t.Fatalf("unexpected header of %s: %v", name, doc["made_by"])
	}
	return doc
}

// loadCompatStore makes a Git store with the blobs and the raw commits of
// the store fixture. Each object must get the object ID that the fixture
// states.
func loadCompatStore(t *testing.T, fixture lifecycle.Object) *lifecycle.GitStore {
	t.Helper()
	store, err := lifecycle.OpenGitStore(filepath.Join(t.TempDir(), "state.git"))
	if err != nil {
		t.Fatal(err)
	}
	git := func(data []byte, args ...string) string {
		out, err := store.Git(data, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	body := func(encoded any) []byte {
		raw, err := base64.StdEncoding.DecodeString(encoded.(string))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for oid, encoded := range fixture["blobs"].(lifecycle.Object) {
		if got := git(body(encoded), "hash-object", "-w", "--stdin"); got != oid {
			t.Fatalf("blob %s gives object ID %s", oid, got)
		}
	}
	last := ""
	for _, c := range fixture["commits"].(lifecycle.Array) {
		commit := c.(lifecycle.Object)
		tree := commit["tree"].(lifecycle.Object)
		names := make([]string, 0, len(tree))
		for name := range tree {
			names = append(names, name)
		}
		sort.Strings(names)
		var rows strings.Builder
		for _, name := range names {
			rows.WriteString("100644 blob " + tree[name].(string) + "\t" + name + "\n")
		}
		raw := body(commit["raw"])
		if want := "tree " + git([]byte(rows.String()), "mktree") + "\n"; !strings.HasPrefix(string(raw), want) {
			t.Fatalf("commit %s does not have the %s", commit["oid"], want)
		}
		if last = git(raw, "hash-object", "-t", "commit", "-w", "--stdin"); last != commit["oid"] {
			t.Fatalf("commit %s gives object ID %s", commit["oid"], last)
		}
	}
	git(nil, "update-ref", lifecycle.RootRef, last)
	return store
}

// compatEngine and compatRunner have the builds of the store fixture. They
// run nothing: no step of the wire fixture needs an execution or a
// qualification.
type compatEngine struct{ build string }

func (e compatEngine) Build() (string, error) { return e.build, nil }

func (compatEngine) Execute(map[string][]byte, string, []byte, any) (any, error) {
	return nil, lifecycle.NewFault("implementation_unavailable", "no execution in the wire fixture", "runtime")
}

type compatRunner struct{ build string }

func (r compatRunner) Identity() (string, error) { return r.build, nil }

func (r compatRunner) Expected(lifecycle.Candidate) string { return r.build }

func (compatRunner) Qualify(lifecycle.QualificationInput) (lifecycle.Verdict, error) {
	return lifecycle.Verdict{}, lifecycle.NewFault("runner_unavailable", "no qualification in the wire fixture", "runner")
}

var commitID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// sameAnswer compares an answer with the answer of v0.8.0. The commit of a
// write has the time of its run, so its ID is different in each run. old
// has each commit of the store fixture. A commit ID that is not in old
// must agree with one new commit ID, the same one at each place.
func sameAnswer(want, got any, old map[string]bool, bound map[string]string, where string) []string {
	switch w := want.(type) {
	case lifecycle.Object:
		g, ok := got.(lifecycle.Object)
		if !ok || len(g) != len(w) {
			return []string{fmt.Sprintf("%s: got %s, want %s", where, lifecycle.Encode(got), lifecycle.Encode(want))}
		}
		var errs []string
		for k, v := range w {
			if k == "message" && strings.HasSuffix(where, ".diagnostic") {
				// The text of a diagnostic message is not a stable surface.
				continue
			}
			if _, has := g[k]; !has {
				errs = append(errs, where+"."+k+": missing")
				continue
			}
			errs = append(errs, sameAnswer(v, g[k], old, bound, where+"."+k)...)
		}
		return errs
	case lifecycle.Array:
		g, ok := got.(lifecycle.Array)
		if !ok || len(g) != len(w) {
			return []string{fmt.Sprintf("%s: got %s, want %s", where, lifecycle.Encode(got), lifecycle.Encode(want))}
		}
		var errs []string
		for i := range w {
			errs = append(errs, sameAnswer(w[i], g[i], old, bound, fmt.Sprintf("%s[%d]", where, i))...)
		}
		return errs
	case string:
		g, ok := got.(string)
		if ok && commitID.MatchString(w) && !old[w] && commitID.MatchString(g) && !old[g] {
			if seen, has := bound[w]; has && seen != g {
				return []string{fmt.Sprintf("%s: the new commit is %s, and it was %s before", where, g, seen)}
			}
			bound[w] = g
			return nil
		}
	}
	if string(lifecycle.Encode(want)) != string(lifecycle.Encode(got)) {
		return []string{fmt.Sprintf("%s: got %s, want %s", where, lifecycle.Encode(got), lifecycle.Encode(want))}
	}
	return nil
}

// Each protocol version from 1.0 to 1.3 gives the answers of v0.8.0 for the
// same requests on the same store. The Host has the lifecycle build of the
// store fixture, because the verdict step needs the build of the report.
func TestV080WireAnswers(t *testing.T) {
	store := readCompatFixture(t, "v080-store.json", "entelos.migration-fixture/1")
	wire := readCompatFixture(t, "v080-wire.json", "entelos.wire-fixture/1")
	builds := store["builds"].(lifecycle.Object)
	old := map[string]bool{}
	for _, c := range store["commits"].(lifecycle.Array) {
		old[c.(lifecycle.Object)["oid"].(string)] = true
	}
	clockFile := filepath.Join(t.TempDir(), "clock")
	if err := os.WriteFile(clockFile, []byte(wire["clock"].(string)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	schemas := loadTestSchemas(t)
	seen, total, writes := "", 0, 0
	for _, v := range wire["versions"].(lifecycle.Array) {
		version := v.(lifecycle.Object)
		name := version["protocol"].(string)
		seen += name + " "
		t.Run(name, func(t *testing.T) {
			host, err := lifecycle.NewHostWithRunner(loadCompatStore(t, store), compatEngine{builds["engine_build"].(string)},
				compatRunner{builds["runner_build"].(string)}, wire["context"])
			if err != nil {
				t.Fatal(err)
			}
			host.BuildRef = builds["lifecycle_build"].(string)
			host.Clock = lifecycle.FileClock{Path: clockFile}
			a := NewAdapter(host, schemas)
			bound := map[string]string{}
			for _, x := range version["steps"].(lifecycle.Array) {
				step := x.(lifecycle.Object)
				request := step["request"].(lifecycle.Object)
				if request["protocol"] != name {
					t.Fatalf("%s: the request has the protocol %v", step["name"], request["protocol"])
				}
				payload, err := Canonical(request)
				if err != nil {
					t.Fatal(err)
				}
				response, closing := a.Frame(payload)
				total++
				for _, e := range sameAnswer(step["response"], normal(t, response), old, bound, "response") {
					t.Errorf("%s: %s", step["name"], e)
				}
				if closing != step["close"] {
					t.Errorf("%s: close=%v", step["name"], closing)
				}
				if step["name"] == "decide" && step["response"].(lifecycle.Object)["status"] == "ok" {
					writes++
				}
			}
			if len(bound) > 1 {
				t.Errorf("the steps made %d new commits, want one at most", len(bound))
			}
		})
	}
	if seen != "1.0 1.1 1.2 1.3 " || total != 4*17 || writes != 4 {
		t.Fatalf("ran the versions %q with %d steps and %d verdict writes", seen, total, writes)
	}
}
