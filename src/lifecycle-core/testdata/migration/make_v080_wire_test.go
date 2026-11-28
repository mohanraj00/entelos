package protocol

// This file is not a test of the current code. make_v080_fixture.sh copies
// it into the source tree of tag v0.8.0 and runs it there. It loads the
// store of V080_STORE and sends the same requests with each protocol
// version through the v0.8.0 adapter. It writes the requests and the
// responses to the JSON file in V080_WIRE_OUT. The go tool ignores the
// testdata directory.

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// v080Load makes a Git store with the blobs and the raw commits of the
// store fixture. Each object must get the object ID that the fixture states.
func v080Load(t *testing.T, fixture lifecycle.Object) *lifecycle.GitStore {
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

// v080WireEngine has the engine build of the store fixture. It runs
// nothing: no step of this file needs an execution.
type v080WireEngine struct{}

func (v080WireEngine) Build() (string, error) { return "sha256:" + strings.Repeat("e", 64), nil }

func (v080WireEngine) Execute(map[string][]byte, string, []byte, any) (any, error) {
	return nil, lifecycle.NewFault("implementation_unavailable", "no execution in the wire fixture", "runtime")
}

// v080WireRunner has the runner build of the store fixture. A verdict needs
// the runner of the report. No step of this file qualifies.
type v080WireRunner struct{ build string }

func (r v080WireRunner) Identity() (string, error) { return r.build, nil }

func (r v080WireRunner) Expected(lifecycle.Candidate) string { return r.build }

func (v080WireRunner) Qualify(lifecycle.QualificationInput) (lifecycle.Verdict, error) {
	return lifecycle.Verdict{}, lifecycle.NewFault("runner_unavailable", "no qualification in the wire fixture", "runner")
}

func TestMakeV080Wire(t *testing.T) {
	out := os.Getenv("V080_WIRE_OUT")
	if out == "" {
		t.Skip("V080_WIRE_OUT is not set")
	}
	raw, err := os.ReadFile(os.Getenv("V080_STORE"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	fixture := value.(lifecycle.Object)
	refs := fixture["refs"].(lifecycle.Object)
	builds := fixture["builds"].(lifecycle.Object)
	root := fixture["layout"].(lifecycle.Object)["refs"].(lifecycle.Object)[lifecycle.RootRef]
	// The actor is a second reviewer. The producer of a proposal cannot
	// give its verdict.
	context := lifecycle.Object{"actor": "reviewer-b", "roles": lifecycle.Array{"reviewer", "reader", "event_reader"}, "scope": "demo",
		"runner_actor": "fixture-runner", "worker": "worker-1",
		"functions": lifecycle.Array{"record.visible", "record.hidden", "record.archived"}}
	clock := "2026-11-27T13:00:00Z"
	clockFile := filepath.Join(t.TempDir(), "clock")
	if err := os.WriteFile(clockFile, []byte(clock+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The reads come first, on the state of the store fixture. The commits
	// of the writes have the time of the run, so each cursor after a write
	// is different in each run.
	type step struct {
		name, operation string
		body            lifecycle.Object
	}
	steps := []step{
		{"inspect", "inspect", lifecycle.Object{}},
		{"resolve-active", "resolve", lifecycle.Object{"function": "record.visible", "date": "2026-11-15"}},
		{"resolve-suspended", "resolve", lifecycle.Object{"function": "record.hidden", "date": "2026-11-15"}},
		{"receipt", "receipt", lifecycle.Object{"receipt_ref": refs["visible_receipt"]}},
		{"record-decision", "record", lifecycle.Object{"kind": "decision", "ref": refs["visible_decision"]}},
		{"record-wrong-kind", "record", lifecycle.Object{"kind": "decision", "ref": refs["visible_report"]}},
		{"proposals-before", "proposals", lifecycle.Object{}},
		{"proposal-open", "proposal", lifecycle.Object{"snapshot_ref": refs["archived_snapshot"]}},
		{"decision-events", "decision_events", lifecycle.Object{"limit": 10}},
		{"approval", "approval", lifecycle.Object{"function": "record.visible",
			"candidate": lifecycle.Object{"kind": "cel-bundle", "ref": refs["visible_bundle"]}}},
		{"evidence", "evidence", lifecycle.Object{"snapshot_ref": refs["visible_snapshot"]}},
		{"unknown-operation", "no_such_operation", lifecycle.Object{}},
		{"decide-stale-revision", "decide", lifecycle.Object{"report_ref": refs["archived_report"], "reason": "compatibility verdict",
			"expected_revision": "", "request_key": "compat-stale"}},
		{"decide", "decide", lifecycle.Object{"report_ref": refs["archived_report"], "reason": "compatibility verdict",
			"expected_revision": root, "request_key": "compat-decide"}},
		{"decide-retry", "decide", lifecycle.Object{"report_ref": refs["archived_report"], "reason": "compatibility verdict",
			"expected_revision": root, "request_key": "compat-decide"}},
		{"proposals-after", "proposals", lifecycle.Object{}},
		{"proposal-decided", "proposal", lifecycle.Object{"snapshot_ref": refs["archived_snapshot"]}},
	}
	schemas := loadTestSchemas(t)
	versions := lifecycle.Array{}
	for _, version := range []string{"1.0", "1.1", "1.2", "1.3"} {
		host, err := lifecycle.NewHostWithRunner(v080Load(t, fixture), v080WireEngine{}, v080WireRunner{builds["runner_build"].(string)}, context)
		if err != nil {
			t.Fatal(err)
		}
		if host.BuildRef != builds["lifecycle_build"] {
			t.Fatalf("the lifecycle build is %s, and the store fixture has %s", host.BuildRef, builds["lifecycle_build"])
		}
		host.Clock = lifecycle.FileClock{Path: clockFile}
		a := NewAdapter(host, schemas)
		results := lifecycle.Array{}
		for i, s := range steps {
			request := lifecycle.Object{"protocol": version, "request_id": fmt.Sprintf("c-%d", i+1), "operation": s.operation, "body": s.body}
			payload, err := Canonical(request)
			if err != nil {
				t.Fatal(err)
			}
			response, closing := a.Frame(payload)
			results = append(results, lifecycle.Object{"name": s.name, "request": request, "response": normal(t, response), "close": closing})
		}
		versions = append(versions, lifecycle.Object{"protocol": version, "steps": results})
	}
	doc := lifecycle.Object{"format": "entelos.wire-fixture/1",
		"made_by": lifecycle.Object{"tag": "v0.8.0", "commit": os.Getenv("V080_COMMIT"), "go": runtime.Version(),
			"file": "testdata/migration/make_v080_wire_test.go"},
		"store": "v080-store.json", "context": context, "clock": clock, "versions": versions}
	if err := os.WriteFile(out, lifecycle.Encode(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}
