package lifecycle

// This file is not a test of the current code. make_fixture.sh copies it
// into the source tree of tag v0.3.0 and runs it there. It makes a store
// with the v0.3.0 Host and writes the store to the JSON file in
// V030_FIXTURE_OUT. The go tool ignores the testdata directory.

import (
	"encoding/base64"
	"os"
	"runtime"
	"strings"
	"testing"
)

// v030Engine describes a bundle. The fixed file gives the function name.
type v030Engine struct{}

func (v030Engine) Build() (string, error) { return engineBuild, nil }

func (v030Engine) Execute(files map[string][]byte, command string, _ []byte, _ any) (any, error) {
	return Object{"function_id": string(files["fixed"]), "bundle_ref": ContentRef(Envelope(Mimes["manifest"], files["manifest"])),
		"implementation_ref": engineBuild}, nil
}

func TestMakeV030Store(t *testing.T) {
	out := os.Getenv("V030_FIXTURE_OUT")
	if out == "" {
		t.Skip("V030_FIXTURE_OUT is not set")
	}
	store, err := OpenGitStore(t.TempDir() + "/state.git")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("visible"): "passed", bundleRefOf("archived"): "passed"}}
	keys := 0
	write := func(actor, role, op string, fields Object) Object {
		t.Helper()
		root, err := store.Read(nil)
		if err != nil {
			t.Fatal(err)
		}
		keys++
		q := Object{"op": op, "expected_revision": root.Cursor, "request_key": "v030-" + op + "-" + string(rune('a'+keys))}
		for k, v := range fields {
			q[k] = v
		}
		host, err := NewHostWithRunner(store, v030Engine{}, runner, Object{"actor": actor, "roles": Array{role}, "scope": "demo",
			"runner_actor": "fixture-runner", "worker": "worker-1", "functions": Array{"record.visible", "record.archived"}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := host.Handle(q)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		return result.(Object)["result"].(Object)
	}
	refs := Object{}
	proposal := func(function, rule string) (string, string) {
		window := Object{"function": function, "from": "2026-11-01", "until": "2027-01-01"}
		source := write("reviewer-a", "reviewer", "source", with(window, Object{"text": rule + " rule", "reason": "first", "supersedes": Array{}}))["source_ref"]
		plan := write("reviewer-a", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}},
			"origin": "independent", "supersedes": Array{}})["plan_ref"].(string)
		b := bundle(rule)
		b["fixed"] = function
		snapshot := write("author-a", "author", "propose", with(window, Object{"base_snapshot": "", "source_ref": source, "bundle": b,
			"reason": "first"}))["snapshot_ref"].(string)
		return snapshot, plan
	}
	// The activated proposal: qualify, decide and activate.
	snapshot, plan := proposal("record.visible", "visible")
	report := write("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
	decision := write("reviewer-a", "reviewer", "decide", Object{"report_ref": report, "reason": "fixture verdict"})["decision_ref"]
	event := write("reviewer-a", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})["event_ref"]
	refs["activated_snapshot"], refs["activated_report"], refs["activated_decision"], refs["activation_event"] = snapshot, report, decision, event
	// The open proposal: qualified, with no verdict.
	snapshot, plan = proposal("record.archived", "archived")
	report = write("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
	refs["open_snapshot"], refs["open_plan"], refs["open_report"] = snapshot, plan, report

	// Export the first-parent history with Git plumbing only.
	lines := func(raw []byte) []string { return strings.Fields(strings.TrimSpace(string(raw))) }
	commits := Array{}
	blobs := Object{}
	list, err := store.Git(nil, "rev-list", "--first-parent", "--reverse", RootRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, commit := range lines(list) {
		message, err := store.Git(nil, "log", "-1", "--format=%B", commit)
		if err != nil {
			t.Fatal(err)
		}
		listing, err := store.Git(nil, "ls-tree", commit)
		if err != nil {
			t.Fatal(err)
		}
		tree := Object{}
		for _, row := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
			meta, name, _ := strings.Cut(row, "\t")
			oid := strings.Fields(meta)[2]
			tree[name] = oid
			if _, has := blobs[oid]; !has {
				body, err := store.Git(nil, "cat-file", "blob", oid)
				if err != nil {
					t.Fatal(err)
				}
				blobs[oid] = base64.StdEncoding.EncodeToString(body)
			}
		}
		commits = append(commits, Object{"message": strings.TrimSpace(string(message)), "tree": tree})
	}
	fixture := Object{"format": "entelos.migration-fixture/1",
		"made_by": Object{"tag": "v0.3.0", "commit": os.Getenv("V030_COMMIT"), "go": runtime.Version(), "file": "testdata/migration/make_v030_store_test.go"},
		"refs":    refs, "commits": commits, "blobs": blobs}
	if err := os.WriteFile(out, Encode(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
}
