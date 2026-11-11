package lifecycle

// Go tests for the coverage limits of src/protocol/approval/APPROVAL.md that
// the approval vectors do not reach.

import (
	"os"
	"strings"
	"testing"
)

type fixedClock struct{ now *string }

func (c fixedClock) Now() (string, error) { return *c.now, nil }

// legacyStore is a store with one accepted transaction and no review index,
// as a v0.3.0 store has.
func legacyStore(t *testing.T) Store {
	t.Helper()
	store := NewMemoryStore()
	if _, err := store.Transact("", "legacy-1", Object{"request": "legacy"}, func(v *View) (any, error) {
		v.State["sources"] = Array{}
		return Object{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return store
}

// A store without state.reviews cannot get a new report, verdict or
// activation, and the approval reads refuse. The Host writes nothing and
// makes no partial index.
func TestLegacyStoreRefusesTheApprovalOperations(t *testing.T) {
	store := legacyStore(t)
	before := must(store.Read(nil))
	if _, has := before.State["reviews"]; has || before.Cursor == "" {
		t.Fatal("the legacy store must have history and no review index")
	}
	ref := "sha256:" + strings.Repeat("0", 64)
	requests := map[string]Object{
		"proposals": {},
		"proposal":  {"snapshot_ref": ref},
		"qualify":   {"snapshot_ref": ref, "plan_ref": ref},
		"decide":    {"report_ref": ref, "reason": "r", "disposition": "reject"},
		"activate":  {"decision_ref": ref, "targets": Array{}},
	}
	for op, fields := range requests {
		q := Object{"op": op}
		for k, v := range fields {
			q[k] = v
		}
		if !Reads[op] {
			q["expected_revision"], q["request_key"] = before.Cursor, "legacy-"+op
		}
		host := (&world{t: t, store: store}).host("reviewer-a", "reviewer", &CELRunner{Runtime: &fakeEngine{build: engineBuild}})
		host = host.As("reviewer-a", []string{"reviewer", "runner"})
		_, err := host.Handle(q)
		if code := faultCode(t, err); code != "index_unavailable" {
			t.Errorf("%s: %s, want index_unavailable", op, code)
		}
		if f := err.(*Fault); f.Stage != "store" {
			t.Errorf("%s: stage %s, want store", op, f.Stage)
		}
	}
	after := must(store.Read(nil))
	if after.Cursor != before.Cursor {
		t.Fatalf("the root changed from %s to %s", before.Cursor, after.Cursor)
	}
	// An operation that does not use the index works and adds no index.
	w := &world{t: t, store: store, runner: &CELRunner{Runtime: &fakeEngine{build: engineBuild}}}
	w.must("reviewer", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))
	if _, has := must(store.Read(nil)).State["reviews"]; has {
		t.Fatal("a source write made a partial review index")
	}
}

// functionEngine describes the function that the fixed file names.
type functionEngine struct{}

func (functionEngine) Build() (string, error) { return engineBuild, nil }

func (functionEngine) Execute(files map[string][]byte, command string, _ []byte, _ any) (any, error) {
	return Object{"function_id": string(files["fixed"]), "bundle_ref": ContentRef(Envelope(Mimes["manifest"], files["manifest"])),
		"implementation_ref": engineBuild}, nil
}

// The list is in submitted_at order, the oldest first, then in
// snapshot_ref order. The expected order is stated from the clock values.
func TestProposalListOrder(t *testing.T) {
	store := NewMemoryStore()
	now := "2026-11-20T09:00:00Z"
	host, err := NewHostWithRunner(store, functionEngine{}, &CELRunner{Runtime: functionEngine{}}, Object{"scope": "demo",
		"runner_actor": "fixture-runner", "worker": "worker-1", "functions": Array{"a.first", "b.second", "c.third"}})
	if err != nil {
		t.Fatal(err)
	}
	host.Clock = fixedClock{&now}
	keys := 0
	write := func(actor, role, op string, fields Object) Object {
		t.Helper()
		keys++
		q := Object{"op": op, "expected_revision": must(store.Read(nil)).Cursor, "request_key": op + "-" + string(rune('a'+keys))}
		for k, v := range fields {
			q[k] = v
		}
		out, err := host.HandleAs(actor, []string{role}, q)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		return out.(Object)["result"].(Object)
	}
	snapshots := map[string]string{}
	for _, x := range []struct{ function, at string }{
		{"c.third", "2026-11-20T10:00:00Z"}, {"a.first", "2026-11-20T09:00:00Z"}, {"b.second", "2026-11-20T10:00:00Z"}} {
		now = x.at
		source := write("reviewer-a", "reviewer", "source", Object{"function": x.function, "from": "2026-11-01", "until": "2027-01-01",
			"text": "rule", "reason": "first", "supersedes": Array{}})["source_ref"]
		b := bundle("rule")
		b["fixed"] = x.function
		snapshots[x.function] = write("author-a", "author", "propose", Object{"function": x.function, "from": "2026-11-01", "until": "2027-01-01",
			"base_snapshot": "", "source_ref": source, "bundle": b, "reason": "first"})["snapshot_ref"].(string)
	}
	now = "2026-11-21T09:00:00Z"
	out, err := host.HandleAs("reviewer-a", []string{"reviewer"}, Object{"op": "proposals"})
	if err != nil {
		t.Fatal(err)
	}
	list := out.(Object)["proposals"].(Array)
	// a.first is the oldest. b.second and c.third have the same time, so
	// their snapshot_ref values give the order.
	later := []string{snapshots["b.second"], snapshots["c.third"]}
	if later[1] < later[0] {
		later[0], later[1] = later[1], later[0]
	}
	want := []string{snapshots["a.first"], later[0], later[1]}
	if len(list) != 3 {
		t.Fatalf("got %d proposals, want 3", len(list))
	}
	for i, x := range list {
		item := x.(Object)
		if item["snapshot_ref"] != want[i] || item["state"] != "awaiting_qualification" {
			t.Errorf("[%d]: %v %v, want %s", i, item["snapshot_ref"], item["state"], want[i])
		}
	}
	if w := list[0].(Object)["wait_seconds"]; !equal(w, Int{"86400"}) {
		t.Errorf("wait_seconds %v, want 86400", w)
	}
}

// An item without submitted_at comes after each item with one.
func TestProposalOrderPutsUntimedLast(t *testing.T) {
	items := []Object{{"snapshot_ref": "sha256:1"}, {"snapshot_ref": "sha256:2", "submitted_at": "2026-11-20T09:00:00Z"},
		{"snapshot_ref": "sha256:0"}}
	proposalOrder(items)
	got := []any{items[0]["snapshot_ref"], items[1]["snapshot_ref"], items[2]["snapshot_ref"]}
	if !equal(Array(got), Array{"sha256:2", "sha256:0", "sha256:1"}) {
		t.Fatalf("order %v", got)
	}
}

// A host for principals has no actor of its own. A request without a
// principal is forbidden, and a context with only one of actor and roles is
// a configuration error.
func TestHostForPrincipals(t *testing.T) {
	base := Object{"scope": "demo", "runner_actor": "fixture-runner", "worker": "worker-1", "functions": Array{"record.visible"}}
	host, err := NewHost(NewMemoryStore(), &fakeEngine{build: engineBuild}, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Handle(Object{"op": "inspect"}); faultCode(t, err) != "forbidden" {
		t.Fatal("a host for principals ran a request without a principal")
	}
	if _, err := host.HandleAs("reviewer-a", []string{"reviewer"}, Object{"op": "inspect"}); err != nil {
		t.Fatal(err)
	}
	if _, err := host.HandleAs("caller-c", []string{"caller"}, Object{"op": "inspect"}); faultCode(t, err) != "forbidden" {
		t.Fatal("a caller principal read the state")
	}
	if host.Context["actor"] != nil {
		t.Fatal("As changed the shared context")
	}
	if _, err := NewHost(NewMemoryStore(), &fakeEngine{build: engineBuild}, with(base, Object{"actor": "a"})); faultCode(t, err) != "configuration_error" {
		t.Fatal("a context with an actor and no roles was accepted")
	}
}

func TestFileClockAcceptsOnlyTheClockFormat(t *testing.T) {
	for value, valid := range map[string]bool{"2026-11-20T09:00:00Z": true, "2026-11-20T09:00:00Z\n": true,
		"2026-11-20T09:00:00.5Z": false, "2026-11-20 09:00:00Z": false, "2026-02-30T09:00:00Z": false, "": false} {
		path := t.TempDir() + "/clock"
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := FileClock{Path: path}.Now()
		if (err == nil) != valid {
			t.Errorf("%q: valid=%v", value, err == nil)
		}
	}
}
