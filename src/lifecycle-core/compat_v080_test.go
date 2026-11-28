package lifecycle

// Compatibility tests for a store of v0.8.0. They start from
// testdata/migration/v080-store.json. The v0.8.0 Host made that store and
// the expected reads. testdata/migration/make_v080_fixture.sh states how.
// See "Compatibility promise for 1.x releases" in docs/STABILITY.md.

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

type v080Fixture struct {
	store *GitStore
	doc   Object
	refs  Object
}

// readV080Fixture reads the fixture file and checks its header.
func readV080Fixture(t *testing.T) Object {
	t.Helper()
	raw, err := os.ReadFile("testdata/migration/v080-store.json")
	if err != nil {
		t.Fatal(err)
	}
	value, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(Object)
	if doc["format"] != "entelos.migration-fixture/1" || doc["made_by"].(Object)["tag"] != "v0.8.0" {
		t.Fatalf("unexpected fixture header %v", doc["made_by"])
	}
	return doc
}

// loadV080Store makes a Git store with the blobs and the raw commits of the
// fixture. Each blob, tree and commit must get the object ID that the
// fixture states. Thus each cursor of the v0.8.0 store stays.
func loadV080Store(t *testing.T) v080Fixture {
	t.Helper()
	doc := readV080Fixture(t)
	store, err := OpenGitStore(t.TempDir() + "/state.git")
	if err != nil {
		t.Fatal(err)
	}
	body := func(encoded any) []byte {
		raw, err := base64.StdEncoding.DecodeString(encoded.(string))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for oid, encoded := range doc["blobs"].(Object) {
		if got := gitWrite(t, store, body(encoded), "hash-object", "-w", "--stdin"); got != oid {
			t.Fatalf("blob %s gives object ID %s", oid, got)
		}
	}
	last := ""
	for _, c := range doc["commits"].(Array) {
		commit := c.(Object)
		var rows strings.Builder
		tree := commit["tree"].(Object)
		for _, name := range sortedKeys(tree) {
			rows.WriteString("100644 blob " + tree[name].(string) + "\t" + name + "\n")
		}
		raw := body(commit["raw"])
		if want := "tree " + gitWrite(t, store, []byte(rows.String()), "mktree") + "\n"; !strings.HasPrefix(string(raw), want) {
			t.Fatalf("commit %s does not have the %s", commit["oid"], want)
		}
		if last = gitWrite(t, store, raw, "hash-object", "-t", "commit", "-w", "--stdin"); last != commit["oid"] {
			t.Fatalf("commit %s gives object ID %s", commit["oid"], last)
		}
	}
	layout := doc["layout"].(Object)["refs"].(Object)
	if len(layout) != 1 || layout[RootRef] != last {
		t.Fatalf("the fixture has the refs %v, and the last commit is %s", layout, last)
	}
	gitWrite(t, store, nil, "update-ref", RootRef, last)
	return v080Fixture{store: store, doc: doc, refs: doc["refs"].(Object)}
}

// compatEngine is the engine of the fixture. The fixed file gives the
// function name.
type compatEngine struct{}

func (compatEngine) Build() (string, error) { return engineBuild, nil }

func (compatEngine) Execute(files map[string][]byte, command string, _ []byte, _ any) (any, error) {
	bundle := ContentRef(Envelope(Mimes["manifest"], files["manifest"]))
	switch command {
	case "describe":
		return Object{"function_id": string(files["fixed"]), "bundle_ref": bundle, "implementation_ref": engineBuild}, nil
	case "run":
		return Object{"status": "ok", "bundle_ref": bundle, "implementation_ref": engineBuild, "result": true}, nil
	}
	return nil, NewFault("unexpected_command", "the fixture engine does not run "+command, "runtime")
}

// compatWorld is a world on the store of the fixture, with the runner, the
// engine and the clock of the fixture. build is the lifecycle build of each
// Host. An empty value is the build of the current source.
type compatWorld struct {
	*world
	build string
}

func (f v080Fixture) world(t *testing.T, build string) compatWorld {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{
		bundleRefOf("visible"): "passed", bundleRefOf("hidden"): "passed", bundleRefOf("archived"): "passed"}})
	w.store, w.engine = f.store, compatEngine{}
	w.functions = Array{"record.visible", "record.hidden", "record.archived"}
	now := f.doc["clock"].(string)
	w.now = &now
	return compatWorld{w, build}
}

func (c compatWorld) handle(actor, role string, q Object) (any, error) {
	h := c.host(actor, role, c.runner)
	if c.build != "" {
		h.BuildRef = c.build
	}
	return h.Handle(q)
}

// write sends one write request at the current root.
func (c compatWorld) write(actor, role, op string, fields Object) (Object, error) {
	c.t.Helper()
	root, err := c.store.Read(nil)
	if err != nil {
		c.t.Fatal(err)
	}
	c.keys++
	q := merge(Object{"op": op, "expected_revision": root.Cursor, "request_key": "compat-" + op + "-" + string(rune('a'+c.keys))}, fields)
	out, err := c.handle(actor, role, q)
	if err != nil {
		return nil, err
	}
	return out.(Object)["result"].(Object), nil
}

func (c compatWorld) mustWrite(actor, role, op string, fields Object) Object {
	c.t.Helper()
	out, err := c.write(actor, role, op, fields)
	if err != nil {
		c.t.Fatalf("%s: %v", op, err)
	}
	return out
}

// checkReads sends each read of the fixture and compares the bytes with
// the answer of the v0.8.0 Host. skip names the reads that depend on the
// lifecycle build of the Host.
func (f v080Fixture) checkReads(t *testing.T, c compatWorld, skip func(name string) bool) int {
	t.Helper()
	count := 0
	for _, x := range f.doc["reads"].(Array) {
		read := x.(Object)
		name := read["name"].(string)
		if skip != nil && skip(name) {
			continue
		}
		count++
		response, err := c.handle(read["actor"].(string), read["role"].(string), copyObject(read["request"].(Object)))
		want, got := read["response"], response
		if fault, ok := err.(*Fault); ok {
			want, got = read["fault"], fault.Record()
		} else if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if fault, ok := want.(Object); ok && fault["diagnostic"] != nil {
			// The text of a diagnostic message is not a stable surface.
			want, got = withoutMessage(want), withoutMessage(got)
		}
		if want == nil || !bytes.Equal(Encode(got), Encode(want)) {
			t.Errorf("%s: the answer is not the answer of v0.8.0:\n got %s\nwant %s", name, Encode(got), Encode(want))
		}
	}
	return count
}

// withoutMessage gives a fault record without the text of its message.
func withoutMessage(record any) any {
	out := copyObject(record.(Object))
	diagnostic := copyObject(out["diagnostic"].(Object))
	delete(diagnostic, "message")
	out["diagnostic"] = diagnostic
	return out
}

// checkObjects checks that the current root has each object of the last
// commit of the fixture, with the same Git object ID. The state file is the
// one entry that a write changes.
func (f v080Fixture) checkObjects(t *testing.T) int {
	t.Helper()
	commits := f.doc["commits"].(Array)
	tree := commits[len(commits)-1].(Object)["tree"].(Object)
	entries := must(f.store.Read(nil)).Entries
	listing := gitWrite(t, f.store, nil, "ls-tree", RootRef)
	count := 0
	for name, oid := range tree {
		if !strings.HasPrefix(name, "o-") {
			continue
		}
		count++
		if entries[name] != oid || !strings.Contains(listing, "100644 blob "+oid.(string)+"\t"+name) {
			t.Errorf("object %s has the ID %s, want %s", name, entries[name], oid)
		}
	}
	return count
}

func TestV080StoreNeedsNoMigration(t *testing.T) {
	f := loadV080Store(t)
	before := must(f.store.Read(nil)).Cursor
	for _, dry := range []bool{true, false} {
		result, err := MigrateReviewIndex(f.store, dry)
		if err != nil || result["action"] != "current" {
			t.Fatalf("migration with dry run %v: %v, %v", dry, result, err)
		}
	}
	if must(f.store.Read(nil)).Cursor != before {
		t.Fatal("the migration changed the root")
	}
}

// A Host with the lifecycle build of v0.8.0 gives each answer of the
// v0.8.0 Host, accepts a new verdict on the report of v0.8.0, and keeps
// each record with its Git object ID.
func TestV080StoreWithTheSameLifecycleBuild(t *testing.T) {
	f := loadV080Store(t)
	builds := f.doc["builds"].(Object)
	if builds["engine_build"] != engineBuild || builds["runner_build"] != fixtureIdentity("fixture-a") {
		t.Fatalf("the fixture has the builds %v", builds)
	}
	c := f.world(t, builds["lifecycle_build"].(string))
	if n := f.checkReads(t, c, nil); n < 30 {
		t.Fatalf("checked %d reads; the fixture has at least 30", n)
	}
	// The list has the open proposal.
	list := must(c.handle("reviewer", "reviewer", Object{"op": "proposals"})).(Object)
	proposals := list["proposals"].(Array)
	if len(proposals) != 1 || proposals[0].(Object)["snapshot_ref"] != f.refs["archived_snapshot"] {
		t.Fatalf("proposals %s", Encode(proposals))
	}
	// A new verdict on the report of v0.8.0, from a second reviewer.
	decision := c.mustWrite("reviewer-b", "reviewer", "decide", Object{"report_ref": f.refs["archived_report"], "reason": "compatibility verdict"})
	record := decision["decision"].(Object)
	if record["disposition"] != "accept" || record["report_ref"] != f.refs["archived_report"] || record["format"] != RecordFormat {
		t.Fatalf("decision %s", Encode(record))
	}
	if n := f.checkObjects(t); n < 40 {
		t.Fatalf("checked %d objects; the fixture has at least 40", n)
	}
	// The records of v0.8.0 give the same bytes after the write, and the
	// activation record is the same record.
	n := f.checkReads(t, c, func(name string) bool {
		return !strings.HasPrefix(name, "record-") && !strings.HasPrefix(name, "receipt")
	})
	if n < 14 {
		t.Fatalf("checked %d record reads after the write; the fixture has at least 14", n)
	}
	// The answer of v0.8.0 replays with the same result.
	replay := c.mustWrite("reader", "reader", "replay", Object{"receipt_ref": f.refs["visible_receipt"], "reason": "audit"})
	if again := replay["receipt"].(Object); again["matches_original"] != true || again["previous_receipt"] != f.refs["visible_receipt"] {
		t.Fatalf("replay %s", Encode(again))
	}
}

// A Host with a later lifecycle build reads each record of v0.8.0. It
// refuses each operation that needs the build of a v0.8.0 report, and it
// serves again after a new qualification, verdict and activation. These are
// the upgrade steps of the CHANGELOG.
func TestV080StoreWithALaterLifecycleBuild(t *testing.T) {
	f := loadV080Store(t)
	later := "sha256:" + strings.Repeat("1", 64)
	if later == f.doc["builds"].(Object)["lifecycle_build"] {
		t.Fatal("the later build is the build of the fixture")
	}
	c := f.world(t, later)
	// The list and the proposal read name the refusal of the old report.
	// Each other read gives the bytes of v0.8.0.
	buildReads := func(name string) bool { return name == "proposals" || strings.HasPrefix(name, "proposal-") }
	if n := f.checkReads(t, c, buildReads); n < 26 {
		t.Fatalf("checked %d reads; the fixture has at least 26", n)
	}
	list := must(c.handle("reviewer", "reviewer", Object{"op": "proposals"})).(Object)
	open := list["proposals"].(Array)
	if len(open) != 1 || open[0].(Object)["snapshot_ref"] != f.refs["archived_snapshot"] || open[0].(Object)["refusal"] != "implementation_unavailable" {
		t.Fatalf("proposals %s", Encode(open))
	}
	before := must(f.store.Read(nil)).Cursor
	refused := func(what string, err error) {
		t.Helper()
		if faultCode(t, err) != "implementation_unavailable" {
			t.Fatalf("%s: %v", what, err)
		}
		if must(f.store.Read(nil)).Cursor != before {
			t.Fatalf("%s changed the root", what)
		}
	}
	_, err := c.write("reviewer-b", "reviewer", "decide", Object{"report_ref": f.refs["archived_report"], "reason": "compatibility verdict"})
	refused("a verdict on the report of v0.8.0", err)
	_, err = c.write("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
	refused("a load of the activation of v0.8.0", err)
	_, err = c.write("caller", "caller", "invoke", Object{"function": "record.visible", "date": "2026-11-15",
		"expected_bundle_ref": f.refs["visible_bundle"], "inputs": Object{}})
	refused("an answer from the activation of v0.8.0", err)
	_, err = c.write("reader", "reader", "replay", Object{"receipt_ref": f.refs["visible_receipt"], "reason": "audit"})
	refused("a replay of the answer of v0.8.0", err)

	// The upgrade steps for the open proposal: qualify again, then decide.
	report := c.mustWrite("fixture-runner", "runner", "qualify", Object{"snapshot_ref": f.refs["archived_snapshot"], "plan_ref": f.refs["archived_plan"]})
	if report["report"].(Object)["lifecycle_build"] != later || report["report_ref"] == f.refs["archived_report"] {
		t.Fatalf("report %s", Encode(report))
	}
	c.mustWrite("reviewer-b", "reviewer", "decide", Object{"report_ref": report["report_ref"], "reason": "compatibility verdict"})
	// The upgrade steps for the active function: qualify, decide, activate
	// with supersedes, load. Then the Host answers again.
	report = c.mustWrite("fixture-runner", "runner", "qualify", Object{"snapshot_ref": f.refs["visible_snapshot"], "plan_ref": f.refs["visible_plan"]})
	decision := c.mustWrite("reviewer-b", "reviewer", "decide", Object{"report_ref": report["report_ref"], "reason": "the same rule on the new build"})
	c.mustWrite("reviewer-b", "reviewer", "activate", Object{"decision_ref": decision["decision_ref"], "targets": Array{},
		"supersedes": Array{f.refs["visible_event"]}})
	c.mustWrite("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
	answer := c.mustWrite("caller", "caller", "invoke", Object{"function": "record.visible", "date": "2026-11-15",
		"expected_bundle_ref": f.refs["visible_bundle"], "inputs": Object{}})
	if answer["actual"].(Object)["result"] != true || answer["receipt"].(Object)["lifecycle_build"] != later {
		t.Fatalf("answer %s", Encode(answer))
	}
	// Each record of v0.8.0 stays, with its Git object ID and its bytes.
	if n := f.checkObjects(t); n < 40 {
		t.Fatalf("checked %d objects; the fixture has at least 40", n)
	}
	n := f.checkReads(t, c, func(name string) bool {
		return !strings.HasPrefix(name, "record-") && !strings.HasPrefix(name, "receipt")
	})
	if n < 14 {
		t.Fatalf("checked %d record reads after the writes; the fixture has at least 14", n)
	}
}

// Each signed record of v0.8.0 verifies with the current code and the key
// set of the fixture. The fixture has no private key.
func TestV080SignedRecordsVerify(t *testing.T) {
	doc := readV080Fixture(t)
	keys, err := ParseKeySet(Encode(doc["keys"]))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, x := range doc["signed"].(Array) {
		signed := x.(Object)
		result, err := VerifySigned(Encode(signed["document"]), keys)
		if err != nil {
			t.Errorf("signed record of %s: %v", signed["record_ref"], err)
			continue
		}
		if result["record_kind"] != signed["record_kind"] {
			t.Errorf("signed record of %s has the kind %v, want %v", signed["record_ref"], result["record_kind"], signed["record_kind"])
		}
		kind := signed["record_kind"].(string)
		kinds[kind]++
		switch kind {
		case "activation":
			_, err = VerifyActivation(Encode(signed["document"]), keys)
		case "suspension":
			_, err = VerifySuspension(Encode(signed["document"]), keys)
		}
		if err != nil {
			t.Errorf("%s record of %s: %v", kind, signed["record_ref"], err)
		}
	}
	if kinds["decision"] != 2 || kinds["event"] != 3 || kinds["activation"] != 2 || kinds["suspension"] != 1 {
		t.Fatalf("signed record kinds %v", kinds)
	}
}
