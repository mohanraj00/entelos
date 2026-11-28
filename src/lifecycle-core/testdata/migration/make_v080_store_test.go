package lifecycle

// This file is not a test of the current code. make_v080_fixture.sh copies
// it into the source tree of tag v0.8.0 and runs it there. It makes a store
// with the v0.8.0 Host and writes the store to the JSON file in
// V080_FIXTURE_OUT. The go tool ignores the testdata directory.

import (
	"encoding/base64"
	"os"
	"runtime"
	"strings"
	"testing"
)

// v080Engine describes and runs a bundle. The fixed file gives the
// function name.
type v080Engine struct{}

func (v080Engine) Build() (string, error) { return engineBuild, nil }

func (v080Engine) Execute(files map[string][]byte, command string, _ []byte, _ any) (any, error) {
	bundle := ContentRef(Envelope(Mimes["manifest"], files["manifest"]))
	switch command {
	case "describe":
		return Object{"function_id": string(files["fixed"]), "bundle_ref": bundle, "implementation_ref": engineBuild}, nil
	case "run":
		return Object{"status": "ok", "bundle_ref": bundle, "implementation_ref": engineBuild, "result": true}, nil
	}
	return nil, NewFault("unexpected_command", "the fixture engine does not run "+command, "runtime")
}

func TestMakeV080Store(t *testing.T) {
	out := os.Getenv("V080_FIXTURE_OUT")
	if out == "" {
		t.Skip("V080_FIXTURE_OUT is not set")
	}
	store, err := OpenGitStore(t.TempDir() + "/state.git")
	if err != nil {
		t.Fatal(err)
	}
	functions := Array{"record.visible", "record.hidden", "record.archived"}
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{
		bundleRefOf("visible"): "passed", bundleRefOf("hidden"): "passed", bundleRefOf("archived"): "passed"}})
	w.store, w.functions, w.engine = store, functions, v080Engine{}
	now := "2026-11-27T12:00:00Z"
	w.now = &now
	signer, keys := newTestSigner(t)
	w.signer = signer

	refs := Object{}
	proposal := func(function, rule string) (string, string) {
		at := Object{"function": function, "from": "2026-11-01", "until": "2027-01-01"}
		source := w.must("reviewer", "reviewer", "source", with(at, Object{"text": rule + " rule", "reason": "first", "supersedes": Array{}}))["source_ref"]
		plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}},
			"origin": "independent", "supersedes": Array{}})["plan_ref"].(string)
		b := bundle(rule)
		b["fixed"] = function
		snapshot := w.must("author", "author", "propose", with(at, Object{"base_snapshot": "", "source_ref": source, "bundle": b,
			"reason": "first"}))["snapshot_ref"].(string)
		refs[rule+"_source"], refs[rule+"_plan"], refs[rule+"_snapshot"] = source, plan, snapshot
		refs[rule+"_bundle"] = bundleRefOf(rule)
		return snapshot, plan
	}
	approve := func(function, rule string) string {
		snapshot, plan := proposal(function, rule)
		report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
		decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "fixture verdict"})["decision_ref"]
		event := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})["event_ref"].(string)
		refs[rule+"_report"], refs[rule+"_decision"], refs[rule+"_event"] = report, decision, event
		return event
	}
	// record.visible: an accepted verdict, a signed activation record, an
	// answer and a replay.
	approve("record.visible", "visible")
	w.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
	answer := w.must("caller", "caller", "invoke", Object{"function": "record.visible", "date": "2026-11-15",
		"expected_bundle_ref": bundleRefOf("visible"), "inputs": Object{}})
	replay := w.must("reader", "reader", "replay", Object{"receipt_ref": answer["receipt_ref"], "reason": "audit"})
	refs["visible_receipt"], refs["visible_replay_receipt"] = answer["receipt_ref"], replay["receipt_ref"]
	// record.hidden: an activation, then a signed suspension record.
	hidden := approve("record.hidden", "hidden")
	refs["hidden_suspension_event"] = w.must("reviewer", "reviewer", "suspend", Object{"function": "record.hidden",
		"from": "2026-11-01", "until": "2027-01-01", "targets": Array{hidden}, "reason": "stop"})["event_ref"]
	// record.archived: the open proposal. It has a report and no verdict.
	snapshot, plan := proposal("record.archived", "archived")
	refs["archived_report"] = w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]

	// Each signed record, as the file that entelos-verify reads. The
	// v0.8.0 code must accept each one.
	signed := Array{}
	signatures := w.signatures()
	for _, ref := range sortedNames(signatures) {
		doc, err := w.read(Object{"op": "record", "kind": "signed", "ref": signatures[ref]})
		if err != nil {
			t.Fatal(err)
		}
		result, err := VerifySigned(Encode(doc), keys)
		if err != nil {
			t.Fatalf("signed record of %s: %v", ref, err)
		}
		signed = append(signed, Object{"record_ref": ref, "signed_ref": signatures[ref], "record_kind": result["record_kind"], "document": doc})
	}

	// The reads of the v0.8.0 Host on the final store. A later Host must
	// give the same bytes for each one.
	reads := Array{}
	read := func(name, actor, role string, q Object) {
		response, err := w.host(actor, role, w.runner).Handle(q)
		entry := Object{"name": name, "actor": actor, "role": role, "request": q}
		if fault, ok := err.(*Fault); ok {
			entry["fault"] = fault.Record()
		} else if err != nil {
			t.Fatalf("%s: %v", name, err)
		} else {
			entry["response"] = response
		}
		reads = append(reads, entry)
	}
	read("inspect", "reviewer", "reviewer", Object{"op": "inspect"})
	read("inspect-forbidden", "reader", "reader", Object{"op": "inspect"})
	read("proposals", "reviewer", "reviewer", Object{"op": "proposals"})
	for _, rule := range []string{"visible", "hidden", "archived"} {
		read("proposal-"+rule, "reviewer", "reviewer", Object{"op": "proposal", "snapshot_ref": refs[rule+"_snapshot"]})
		read("resolve-"+rule, "reader", "reader", Object{"op": "resolve", "function": "record." + rule, "date": "2026-11-15"})
		read("approval-"+rule, "reader", "reader", Object{"op": "approval", "function": "record." + rule,
			"candidate": Object{"kind": "cel-bundle", "ref": bundleRefOf(rule)}})
		read("evidence-"+rule, "reviewer", "reviewer", Object{"op": "evidence", "snapshot_ref": refs[rule+"_snapshot"]})
	}
	read("receipt", "reader", "reader", Object{"op": "receipt", "receipt_ref": refs["visible_receipt"]})
	read("receipt-replay", "reader", "reader", Object{"op": "receipt", "receipt_ref": refs["visible_replay_receipt"]})
	for _, name := range []string{"visible_source", "visible_plan", "visible_snapshot", "visible_report", "visible_decision", "visible_event",
		"hidden_suspension_event", "archived_report"} {
		kind := name[strings.LastIndex(name, "_")+1:]
		if kind == "report" {
			kind = "qualification"
		}
		read("record-"+name, "reviewer", "reviewer", Object{"op": "record", "kind": kind, "ref": refs[name]})
	}
	read("record-wrong-kind", "reviewer", "reviewer", Object{"op": "record", "kind": "decision", "ref": refs["visible_report"]})
	read("record-forbidden", "reader", "reader", Object{"op": "record", "kind": "decision", "ref": refs["visible_decision"]})
	activations, suspensions := w.activations(), w.suspensions()
	for _, ref := range sortedNames(activations) {
		read("record-activation-"+ref, "reviewer", "reviewer", Object{"op": "record", "kind": "activation", "ref": activations[ref]})
	}
	for _, ref := range sortedNames(suspensions) {
		read("record-suspension-"+ref, "reviewer", "reviewer", Object{"op": "record", "kind": "suspension", "ref": suspensions[ref]})
	}

	// Export the first-parent history with Git plumbing only. The raw
	// commit objects keep the cursors.
	lines := func(raw []byte) []string { return strings.Fields(strings.TrimSpace(string(raw))) }
	git := func(args ...string) []byte {
		raw, err := store.Git(nil, args...)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	commits := Array{}
	blobs := Object{}
	for _, commit := range lines(git("rev-list", "--first-parent", "--reverse", RootRef)) {
		tree := Object{}
		for _, row := range strings.Split(strings.TrimSpace(string(git("ls-tree", commit))), "\n") {
			meta, name, _ := strings.Cut(row, "\t")
			fields := strings.Fields(meta)
			if fields[0] != "100644" || fields[1] != "blob" {
				t.Fatalf("the tree of %s has the entry %q", commit, row)
			}
			tree[name] = fields[2]
			if _, has := blobs[fields[2]]; !has {
				blobs[fields[2]] = base64.StdEncoding.EncodeToString(git("cat-file", "blob", fields[2]))
			}
		}
		commits = append(commits, Object{"oid": commit, "raw": base64.StdEncoding.EncodeToString(git("cat-file", "commit", commit)), "tree": tree})
	}
	layout := Object{}
	for _, row := range strings.Split(strings.TrimSpace(string(git("for-each-ref", "--format=%(refname) %(objectname)"))), "\n") {
		name, oid, _ := strings.Cut(row, " ")
		layout[name] = oid
	}
	if info, err := os.Stat(store.Path + "/lifecycle-root-update.flock"); err != nil || info.Size() != 0 {
		t.Fatalf("the update marker: %v", err)
	}
	lifecycleBuild, err := Implementation()
	if err != nil {
		t.Fatal(err)
	}
	fixture := Object{"format": "entelos.migration-fixture/1",
		"builds": Object{"lifecycle_build": lifecycleBuild, "engine_build": engineBuild, "runner_build": fixtureIdentity("fixture-a")},
		"made_by": Object{"tag": "v0.8.0", "commit": os.Getenv("V080_COMMIT"), "go": runtime.Version(),
			"git": strings.TrimSpace(string(git("version"))), "file": "testdata/migration/make_v080_store_test.go"},
		"clock": now, "keys": keysDocument(signer), "refs": refs, "layout": Object{"refs": layout, "marker": "lifecycle-root-update.flock"},
		"signed": signed, "reads": reads, "commits": commits, "blobs": blobs}
	if err := os.WriteFile(out, Encode(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
}

// keysDocument is the public key set file of the signer. The fixture has
// no private key.
func keysDocument(signer *Signer) Object {
	return Object{"format": KeySetFormat, "signer": "test-host", "keys": Array{signer.PublicKey()}}
}

// sortedNames gives the names of an object in order.
func sortedNames(o Object) []string {
	list := make([]string, 0, len(o))
	for name := range o {
		list = append(list, name)
	}
	for i := range list {
		for j := i + 1; j < len(list); j++ {
			if list[j] < list[i] {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list
}
