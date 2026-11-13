package lifecycle

// Tests of MigrateReviewIndex. They start from testdata/migration/v030-store.json.
// The v0.3.0 Host made that store. testdata/migration/make_fixture.sh states how.

import (
	"bytes"
	"encoding/base64"
	"os"
	"sort"
	"strings"
	"testing"
)

type v030Fixture struct {
	store *GitStore
	refs  Object
}

// loadV030Store makes a Git store with the commits and blobs of the fixture.
// Each blob must get the object ID that the fixture states.
func loadV030Store(t *testing.T) v030Fixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/migration/v030-store.json")
	if err != nil {
		t.Fatal(err)
	}
	value, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	fixture := value.(Object)
	if fixture["format"] != "entelos.migration-fixture/1" || fixture["made_by"].(Object)["tag"] != "v0.3.0" {
		t.Fatalf("unexpected fixture header %v", fixture["made_by"])
	}
	store, err := OpenGitStore(t.TempDir() + "/state.git")
	if err != nil {
		t.Fatal(err)
	}
	for oid, encoded := range fixture["blobs"].(Object) {
		body, err := base64.StdEncoding.DecodeString(encoded.(string))
		if err != nil {
			t.Fatal(err)
		}
		if got := gitWrite(t, store, body, "hash-object", "-w", "--stdin"); got != oid {
			t.Fatalf("blob %s gives object ID %s", oid, got)
		}
	}
	parent := ""
	for _, c := range fixture["commits"].(Array) {
		commit := c.(Object)
		entries := map[string]string{}
		for name, oid := range commit["tree"].(Object) {
			entries[name] = oid.(string)
		}
		parent = gitCommit(t, store, entries, parent, commit["message"].(string))
	}
	if _, err := store.Git(nil, "update-ref", RootRef, parent); err != nil {
		t.Fatal(err)
	}
	return v030Fixture{store: store, refs: fixture["refs"].(Object)}
}

func gitWrite(t *testing.T, store *GitStore, data []byte, args ...string) string {
	t.Helper()
	out, err := store.Git(data, args...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// gitCommit writes a tree of blob entries and a commit on parent.
func gitCommit(t *testing.T, store *GitStore, entries map[string]string, parent, message string) string {
	t.Helper()
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var rows strings.Builder
	for _, name := range names {
		rows.WriteString("100644 blob " + entries[name] + "\t" + name + "\n")
	}
	args := []string{"commit-tree", gitWrite(t, store, []byte(rows.String()), "mktree")}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	return gitWrite(t, store, []byte(message+"\n"), args...)
}

// craft adds one commit on the root by Git plumbing, not by the Host. The
// mutate function changes the state and can add records.
func craft(t *testing.T, store *GitStore, mutate func(state Object, put func(kind string, record Object) string)) {
	t.Helper()
	view := must(store.Read(nil))
	entries := map[string]string{}
	for name, oid := range view.Entries {
		entries[name] = oid
	}
	put := func(kind string, record Object) string {
		body := Envelope(Media(kind), Encode(record))
		ref := ContentRef(body)
		entries["o-"+ref[7:]] = gitWrite(t, store, body, "hash-object", "-w", "--stdin")
		return ref
	}
	mutate(view.State, put)
	entries["state.json"] = gitWrite(t, store, Encode(view.State), "hash-object", "-w", "--stdin")
	commit := gitCommit(t, store, entries, view.Cursor, "crafted")
	if _, err := store.Git(nil, "update-ref", RootRef, commit, view.Cursor); err != nil {
		t.Fatal(err)
	}
}

// objectIDs gives the Git object ID of each record at the root.
func objectIDs(t *testing.T, store Store) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, oid := range must(store.Read(nil)).Entries {
		if name != "state.json" {
			out[name] = oid
		}
	}
	return out
}

func migrationHost(t *testing.T, store Store, now *string) *Host {
	t.Helper()
	runner := &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("visible"): "passed", bundleRefOf("archived"): "passed"}}
	host, err := NewHostWithRunner(store, functionEngine{}, runner, Object{"scope": "demo", "runner_actor": "fixture-runner",
		"worker": "worker-1", "functions": Array{"record.visible", "record.archived"}})
	if err != nil {
		t.Fatal(err)
	}
	host.Clock = fixedClock{now}
	return host
}

func migrate(t *testing.T, store Store, dryRun bool, action string) Object {
	t.Helper()
	out, err := MigrateReviewIndex(store, dryRun)
	if err != nil {
		t.Fatalf("migration (dry run %v): %v", dryRun, err)
	}
	if out["action"] != action {
		t.Fatalf("action %v, want %s: %s", out["action"], action, Encode(out))
	}
	return out
}

// A v0.3.0 store gets the review index. After the migration, the approval
// API lists the open proposal and accepts a verdict. The records of the
// activated proposal keep their bytes. A second run writes nothing.
func TestMigrationOfAV030Store(t *testing.T) {
	f := loadV030Store(t)
	now := "2026-11-20T09:00:00Z"
	host := migrationHost(t, f.store, &now)
	if _, err := host.HandleAs("reviewer-b", []string{"reviewer"}, Object{"op": "proposals"}); faultCode(t, err) != "index_unavailable" {
		t.Fatal("the v0.3.0 store must refuse the list before the migration")
	}
	before := must(f.store.Read(nil))
	records := objectIDs(t, f.store)
	activated := map[string][]byte{}
	for _, key := range []string{"activated_snapshot", "activated_report", "activated_decision", "activation_event"} {
		ref := f.refs[key].(string)
		activated[ref] = must(before.GetRaw(ref, ""))
	}

	// The dry run lists the index and writes nothing.
	dry := migrate(t, f.store, true, "dry_run")
	if root := must(f.store.Read(nil)).Cursor; root != before.Cursor {
		t.Fatalf("the dry run moved the root to %s", root)
	}
	want := Object{
		f.refs["activated_snapshot"].(string): Object{"reports": Array{f.refs["activated_report"]}, "decisions": Array{f.refs["activated_decision"]}},
		f.refs["open_snapshot"].(string):      Object{"reports": Array{f.refs["open_report"]}, "decisions": Array{}},
	}
	writes := dry["writes"].(Array)
	if len(writes) != 2 || !equal(writes[0].(Object)["value"], want) {
		t.Fatalf("dry run writes %s", Encode(writes))
	}

	out := migrate(t, f.store, false, "migrated")
	after := must(f.store.Read(nil))
	if out["cursor"] != after.Cursor || must(f.store.History(after.Cursor))[1] != before.Cursor {
		t.Fatal("the migration must add one commit on the old root")
	}
	if !equal(after.State["reviews"], want) {
		t.Fatalf("index %s", Encode(after.State["reviews"]))
	}
	// The state differs only in reviews and in the request entry of the migration.
	rest := copyObject(after.State)
	delete(rest, "reviews")
	requests := copyObject(rest["requests"].(Object))
	if _, has := requests[migrationKey]; !has {
		t.Fatal("the migration request entry is missing")
	}
	delete(requests, migrationKey)
	rest["requests"] = requests
	if !equal(rest, before.State) {
		t.Fatal("the migration changed a state field other than reviews")
	}
	// Each record keeps its object ID, and the migration adds no record.
	if !equal(toObject(objectIDs(t, f.store)), toObject(records)) {
		t.Fatal("the migration changed, added or removed a record")
	}
	for ref, raw := range activated {
		if got := must(after.GetRaw(ref, "")); !bytes.Equal(got, raw) {
			t.Fatalf("the bytes of %s changed", ref)
		}
	}

	// A second run and a dry run write nothing.
	migrate(t, f.store, false, "current")
	migrate(t, f.store, true, "current")
	if root := must(f.store.Read(nil)).Cursor; root != after.Cursor {
		t.Fatal("a second run wrote a commit")
	}

	// The API lists the open proposal. Its v0.3.0 report names the v0.3.0
	// lifecycle build, so a new report is necessary for an accept.
	list := must(host.HandleAs("reviewer-b", []string{"reviewer"}, Object{"op": "proposals"})).(Object)["proposals"].(Array)
	if len(list) != 1 || list[0].(Object)["snapshot_ref"] != f.refs["open_snapshot"] || list[0].(Object)["state"] != "not_approvable" ||
		list[0].(Object)["refusal"] != "implementation_unavailable" {
		t.Fatalf("proposals %s", Encode(list))
	}
	write := func(actor, role, op string, fields Object) Object {
		t.Helper()
		q := Object{"op": op, "expected_revision": must(f.store.Read(nil)).Cursor, "request_key": "after-" + op}
		for k, v := range fields {
			q[k] = v
		}
		res, err := host.HandleAs(actor, []string{role}, q)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		return res.(Object)["result"].(Object)
	}
	report := write("fixture-runner", "runner", "qualify", Object{"snapshot_ref": f.refs["open_snapshot"], "plan_ref": f.refs["open_plan"]})["report_ref"]
	list = must(host.HandleAs("reviewer-b", []string{"reviewer"}, Object{"op": "proposals"})).(Object)["proposals"].(Array)
	if len(list) != 1 || list[0].(Object)["state"] != "awaiting_verdict" {
		t.Fatalf("proposals after the new report %s", Encode(list))
	}
	decision := write("reviewer-b", "reviewer", "decide", Object{"report_ref": report, "reason": "accept after the migration"})
	if decision["decision"].(Object)["disposition"] != "accept" {
		t.Fatalf("decision %s", Encode(decision))
	}
	list = must(host.HandleAs("reviewer-b", []string{"reviewer"}, Object{"op": "proposals"})).(Object)["proposals"].(Array)
	if len(list) != 0 {
		t.Fatalf("an accepted proposal is still open: %s", Encode(list))
	}
	// The index that the Host keeps after the migration agrees with the history.
	migrate(t, f.store, false, "current")
}

func toObject(m map[string]string) Object {
	out := Object{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// An empty store needs no migration. The Host makes the index on the first write.
func TestMigrationOfAnEmptyStore(t *testing.T) {
	store := NewMemoryStore()
	migrate(t, store, false, "empty")
	if must(store.Read(nil)).Cursor != "" {
		t.Fatal("the migration wrote to an empty store")
	}
}

// Each refusal reason stops the migration and the dry run, and the root
// stays the same.
func TestMigrationRefusals(t *testing.T) {
	cases := []struct {
		name, code string
		migrated   bool
		mutate     func(f v030Fixture, state Object, put func(string, Object) string)
	}{
		{"index-differs", "index_inconsistent", true, func(f v030Fixture, state Object, _ func(string, Object) string) {
			state["reviews"].(Object)[f.refs["activated_snapshot"].(string)].(Object)["decisions"] = Array{}
			state["requests"].(Object)["crafted"] = Object{"payload_ref": "x", "result": Object{}}
		}},
		{"index-not-an-object", "index_inconsistent", false, func(_ v030Fixture, state Object, _ func(string, Object) string) {
			state["reviews"] = Array{}
			state["requests"].(Object)["crafted"] = Object{"payload_ref": "x", "result": Object{}}
		}},
		{"key-without-index", "index_inconsistent", false, func(_ v030Fixture, state Object, _ func(string, Object) string) {
			state["requests"].(Object)[migrationKey] = Object{"payload_ref": "x", "result": Object{}}
		}},
		{"record-without-request", "index_incomplete", false, func(f v030Fixture, state Object, put func(string, Object) string) {
			v := must(f.store.Read(nil))
			d := copyObject(get(v, f.refs["activated_decision"], "decision"))
			d["reason"] = "a decision that no request names"
			put("decision", d)
			state["requests"].(Object)["crafted"] = Object{"payload_ref": "x", "result": Object{}}
		}},
		{"two-keys-in-one-commit", "unexpected_history", false, func(_ v030Fixture, state Object, _ func(string, Object) string) {
			state["requests"].(Object)["crafted-1"] = Object{"payload_ref": "x", "result": Object{}}
			state["requests"].(Object)["crafted-2"] = Object{"payload_ref": "x", "result": Object{}}
		}},
		{"key-removed", "unexpected_history", false, func(_ v030Fixture, state Object, _ func(string, Object) string) {
			requests := state["requests"].(Object)
			for key := range requests {
				delete(requests, key)
				break
			}
			requests["crafted"] = Object{"payload_ref": "x", "result": Object{}}
		}},
		{"newer-record-format", "unsupported_record_format", false, func(f v030Fixture, state Object, put func(string, Object) string) {
			v := must(f.store.Read(nil))
			d := copyObject(get(v, f.refs["activated_decision"], "decision"))
			d["reason"] = "a decision of a later format"
			d["format"] = "entelos.record/2"
			ref := put("decision", d)
			state["requests"].(Object)["crafted"] = Object{"payload_ref": "x", "result": Object{"decision_ref": ref}}
		}},
		{"decision-snapshot-differs", "invalid_record", false, func(f v030Fixture, state Object, put func(string, Object) string) {
			v := must(f.store.Read(nil))
			d := copyObject(get(v, f.refs["activated_decision"], "decision"))
			d["snapshot_ref"] = f.refs["open_snapshot"]
			ref := put("decision", d)
			state["requests"].(Object)["crafted"] = Object{"payload_ref": "x", "result": Object{"decision_ref": ref}}
		}},
		{"unknown-state-schema", "invalid_store", false, func(_ v030Fixture, state Object, _ func(string, Object) string) {
			state["schema"] = "other-lifecycle/v9"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Each case makes about 150 git processes, so the cases run in parallel.
			t.Parallel()
			f := loadV030Store(t)
			if c.migrated {
				migrate(t, f.store, false, "migrated")
			}
			craft(t, f.store, func(state Object, put func(string, Object) string) { c.mutate(f, state, put) })
			root, err := f.store.Root()
			if err != nil {
				t.Fatal(err)
			}
			for _, dry := range []bool{true, false} {
				_, err := MigrateReviewIndex(f.store, dry)
				if code := faultCode(t, err); code != c.code {
					t.Errorf("dry run %v: %s (%v), want %s", dry, code, err, c.code)
				}
			}
			if now, _ := f.store.Root(); now != root {
				t.Fatal("a refused migration moved the root")
			}
		})
	}
}

// racingStore runs a write of another client before each Transact.
type racingStore struct {
	*GitStore
	race func()
}

func (s racingStore) Transact(expected, key, payload any, apply func(*View) (any, error)) (Object, error) {
	s.race()
	return s.GitStore.Transact(expected, key, payload, apply)
}

// If the root moves after the read, the migration writes nothing.
func TestMigrationLosesARace(t *testing.T) {
	f := loadV030Store(t)
	var moved string
	store := racingStore{f.store, func() {
		craft(t, f.store, func(state Object, _ func(string, Object) string) {
			state["requests"].(Object)["other-client"] = Object{"payload_ref": "x", "result": Object{}}
		})
		moved = must(f.store.Root())
	}}
	if _, err := MigrateReviewIndex(store, false); faultCode(t, err) != "stale_revision" {
		t.Fatalf("got %v, want stale_revision", err)
	}
	after := must(f.store.Read(nil))
	if after.Cursor != moved {
		t.Fatal("the migration wrote after a lost race")
	}
	if _, has := after.State["reviews"]; has {
		t.Fatal("the migration wrote an index after a lost race")
	}
}
