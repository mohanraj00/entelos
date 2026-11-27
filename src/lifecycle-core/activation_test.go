package lifecycle

// Go tests for src/protocol/activation/ACTIVATION.md: the vectors, the
// activation record and the suspension record that the Host writes, and
// the publisher.

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func activationVectors(t *testing.T) Object { return publishedVectors(t, "activation-cases.json") }

func publishedVectors(t *testing.T, name string) Object {
	raw, err := os.ReadFile("../protocol/activation/vectors/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return must(Decode(raw)).(Object)
}

// activationResult gives the result of the rules in the vector shape.
func activationResult(kind string, raw []byte) Object {
	_, err := checkActivation(kind, raw)
	return ruleResult(err, "invalid_activation")
}

// ruleResult gives the result of one rule table in the vector shape.
// invalid is the code that has rules.
func ruleResult(err error, invalid string) Object {
	if err == nil {
		return Object{"valid": true}
	}
	fault := err.(*Fault)
	out := Object{"valid": false, "code": fault.Code}
	if fault.Code == invalid {
		rule, rest, _ := strings.Cut(fault.Message, ": ")
		out["rule"] = rule
		if rule == "unknown_field" {
			out["field"] = rest
		}
	}
	return out
}

// The Go mapping and the Go rules give the values that a person wrote from
// ACTIVATION.md.
func TestActivationVectors(t *testing.T) {
	doc := activationVectors(t)
	for _, x := range doc["mappings"].(Array) {
		c := x.(Object)
		records := c["records"].(Object)
		snapshot := records["snapshot"].(Object)
		var base Object
		if truthy(lookup(snapshot, "base_snapshot", nil)) {
			base = records["base_snapshot"].(Object)
		}
		got := activationRecord(records["event"].(Object), c["event_ref"].(string), snapshot, base, records["report"].(Object),
			records["decision"].(Object), c["request_key"].(string))
		if equal(c["expect"], Object{"none": true}) {
			if got != nil {
				t.Errorf("%s: got %s, want no record", c["name"], Encode(got))
			}
			continue
		}
		if !equal(got, c["expect"]) {
			t.Errorf("%s: got %s", c["name"], Encode(got))
		}
		if result := activationResult("activation", Encode(got)); !equal(result, Object{"valid": true}) {
			t.Errorf("%s: the mapped record is not valid: %s", c["name"], Encode(result))
		}
	}
	for _, x := range doc["records"].(Array) {
		c := x.(Object)
		if got := activationResult(c["record_kind"].(string), []byte(c["record"].(string))); !equal(got, c["expect"]) {
			t.Errorf("%s: got %s, want %s", c["name"], Encode(got), Encode(c["expect"]))
		}
	}
}

// The Go mapping and the Go rules of the suspension record give the values
// that a person wrote from ACTIVATION.md.
func TestSuspensionVectors(t *testing.T) {
	doc := publishedVectors(t, "suspension-cases.json")
	result := func(kind string, raw []byte) Object {
		_, err := checkSuspension(kind, raw)
		return ruleResult(err, "invalid_suspension")
	}
	for _, x := range doc["mappings"].(Array) {
		c := x.(Object)
		got := suspensionRecord(c["records"].(Object)["event"].(Object), c["event_ref"].(string), c["request_key"].(string))
		if equal(c["expect"], Object{"none": true}) {
			if got != nil {
				t.Errorf("%s: got %s, want no record", c["name"], Encode(got))
			}
			continue
		}
		if !equal(got, c["expect"]) {
			t.Errorf("%s: got %s", c["name"], Encode(got))
		}
		if r := result("suspension", Encode(got)); !equal(r, Object{"valid": true}) {
			t.Errorf("%s: the mapped record is not valid: %s", c["name"], Encode(r))
		}
	}
	for _, x := range doc["records"].(Array) {
		c := x.(Object)
		if got := result(c["record_kind"].(string), []byte(c["record"].(string))); !equal(got, c["expect"]) {
			t.Errorf("%s: got %s, want %s", c["name"], Encode(got), Encode(c["expect"]))
		}
	}
}

// testPublisher keeps each record that it gets and refuses with refuse.
type testPublisher struct {
	refuse          error
	records, signed [][]byte
}

func (p *testPublisher) Publish(record, signed []byte) error {
	p.records, p.signed = append(p.records, record), append(p.signed, signed)
	return p.refuse
}

// decided gives a decision of a passed CEL candidate.
func (w *world) decided(rule string) (snapshot, decision string) {
	snapshot, plan, _ := w.candidate(rule)
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	return snapshot, w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "fixture verdict"})["decision_ref"].(string)
}

func (w *world) stored(ref any, kind string) []byte {
	root, err := w.store.Read(nil)
	if err != nil {
		w.t.Fatal(err)
	}
	raw, err := root.GetRaw(ref, Media(kind))
	if err != nil {
		w.t.Fatal(err)
	}
	return raw
}

// A signing Host writes one activation record for each enable event. The
// record names the stored records, and a consumer verifies it offline. A
// suspend event has no activation record.
func TestHostWritesTheActivationRecord(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed", bundleRefOf("next"): "passed"}})
	signer, keys := newTestSigner(t)
	w.signer = signer
	first, approval := w.decided("pass")
	event := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": approval, "targets": Array{}})["event_ref"].(string)
	// A second candidate with a base, which masks the first event.
	source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "second", "supersedes": Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent",
		"supersedes": Array{}})["plan_ref"]
	next := w.must("author", "author", "propose", with(window, Object{"base_snapshot": first, "source_ref": source, "bundle": bundle("next"),
		"reason": "second"}))["snapshot_ref"]
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": next, "plan_ref": plan})["report_ref"]
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "fixture verdict"})["decision_ref"]
	masked := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{event}})["event_ref"].(string)
	suspension := w.must("reviewer", "reviewer", "suspend", with(window, Object{"targets": Array{masked}, "reason": "stop"}))["event_ref"].(string)
	index := w.activations()
	if len(index) != 2 || index[suspension] != nil {
		t.Fatalf("activations %s", Encode(index))
	}
	for ref, want := range map[string]Object{
		event:  {"base": "", "candidate": Object{"kind": "cel-bundle", "ref": bundleRefOf("pass")}, "targets": Array{}},
		masked: {"base": bundleRefOf("pass"), "candidate": Object{"kind": "cel-bundle", "ref": bundleRefOf("next")}, "targets": Array{event}},
	} {
		signed, err := w.read(Object{"op": "record", "kind": "signed", "ref": w.signatures()[index[ref].(string)]})
		if err != nil {
			t.Fatal(err)
		}
		result, err := VerifyActivation(Encode(signed), keys)
		if err != nil {
			t.Fatal(err)
		}
		record := result["record"].(Object)
		for k, v := range merge(want, Object{"event_ref": ref, "scope": "demo", "function": "record.visible", "from": "2026-11-01",
			"until": "2027-01-01", "producer": "author", "runner": "fixture-runner", "approver": "reviewer", "activator": "reviewer",
			"supersedes": Array{}, "applicability": Object{}}) {
			if !equal(record[k], v) {
				t.Errorf("%s: %s is %s, want %s", ref, k, Encode(record[k]), Encode(v))
			}
		}
		if key, _ := record["request_key"].(string); !strings.HasPrefix(key, "activate-") {
			t.Errorf("request_key %v", record["request_key"])
		}
	}
	// A signed decision is not an activation record.
	signed, err := w.read(Object{"op": "record", "kind": "signed", "ref": w.signatures()[approval]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyActivation(Encode(signed), keys); faultCode(t, err) != "unexpected_record_kind" {
		t.Fatalf("decision: %v", err)
	}
}

// Without a signing key, the Host writes no activation record and no
// suspension record.
func TestNoActivationRecordWithoutASigningKey(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	_, event := w.approve("pass")
	w.must("reviewer", "reviewer", "suspend", with(window, Object{"targets": Array{event}, "reason": "stop"}))
	if index := w.activations(); index != nil {
		t.Fatalf("activations %s", Encode(index))
	}
	if index := w.suspensions(); index != nil {
		t.Fatalf("suspensions %s", Encode(index))
	}
}

// A signing Host writes one suspension record for each suspend event. A
// consumer verifies it offline. A suspension record is not an activation
// record, and an activation record is not a suspension record.
func TestHostWritesTheSuspensionRecord(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	signer, keys := newTestSigner(t)
	w.signer = signer
	_, event := w.approve("pass")
	suspended := Object{"function": "record.visible", "from": "2026-12-01", "until": "2027-01-01", "targets": Array{event}, "reason": "stop"}
	suspension := w.must("reviewer", "reviewer", "suspend", suspended)["event_ref"].(string)
	index := w.suspensions()
	if len(index) != 1 || w.activations()[suspension] != nil || index[event] != nil {
		t.Fatalf("suspensions %s", Encode(index))
	}
	signedOf := func(ref any) []byte {
		signed, err := w.read(Object{"op": "record", "kind": "signed", "ref": w.signatures()[text(ref)]})
		if err != nil {
			t.Fatal(err)
		}
		return Encode(signed)
	}
	result, err := VerifySuspension(signedOf(index[suspension]), keys)
	if err != nil {
		t.Fatal(err)
	}
	record := result["record"].(Object)
	key, _ := record["request_key"].(string)
	delete(record, "request_key")
	want := Object{"format": RecordFormat, "scope": "demo", "function": "record.visible", "event_ref": suspension, "from": "2026-12-01",
		"until": "2027-01-01", "targets": Array{event}, "actor": "reviewer"}
	if !equal(record, want) || !strings.HasPrefix(key, "suspend-") || result["record_kind"] != "suspension" {
		t.Fatalf("record %s, request_key %q", Encode(record), key)
	}
	// The stored event keeps its reason. The record does not copy it.
	if stored := must(Decode(w.stored(suspension, "event"))).(Object); stored["reason"] != "stop" {
		t.Fatalf("event %s", Encode(stored))
	}
	if _, err := VerifyActivation(signedOf(index[suspension]), keys); faultCode(t, err) != "unexpected_record_kind" {
		t.Fatalf("a suspension record as an activation record: %v", err)
	}
	if _, err := VerifySuspension(signedOf(w.activations()[event]), keys); faultCode(t, err) != "unexpected_record_kind" {
		t.Fatalf("an activation record as a suspension record: %v", err)
	}
	// The signed suspend event is not a suspension record.
	if _, err := VerifySuspension(signedOf(suspension), keys); faultCode(t, err) != "unexpected_record_kind" {
		t.Fatalf("an event as a suspension record: %v", err)
	}
	// A changed record kind fails the signature check. The reference is the
	// reference of the same bytes with the other kind.
	raw := w.stored(index[suspension], "suspension")
	forged := must(Decode(signedOf(index[suspension]))).(Object)
	forged["record_kind"], forged["record_ref"] = "activation", ContentRef(append([]byte(Media("activation")+"\x00"), raw...))
	if _, err := VerifyActivation(Encode(forged), keys); faultCode(t, err) != "signature_invalid" {
		t.Fatalf("a changed record kind: %v", err)
	}
}

// The publisher gets the stored bytes of the suspension record before the
// commit. A refusal stores nothing and suspends nothing, and a retry of
// the same request gives the same bytes.
func TestPublisherRefusalStoresNoSuspension(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	signer, keys := newTestSigner(t)
	publisher := &testPublisher{}
	w.signer, w.publisher = signer, publisher
	_, event := w.approve("pass")
	publisher.records, publisher.signed, publisher.refuse = nil, nil, errors.New("the target is not known")
	before := must(w.store.Read(nil)).Cursor
	q := with(window, Object{"op": "suspend", "expected_revision": before, "request_key": "suspend-once", "targets": Array{event}, "reason": "stop"})
	host := w.host("reviewer", "reviewer", w.runner)
	_, err := host.Handle(copyObject(q))
	if f, ok := err.(*Fault); !ok || f.Code != "publish_refused" || f.Stage != "publish" ||
		!strings.Contains(f.Message, "the suspension record: the target is not known") {
		t.Fatalf("refusal: %v", err)
	}
	if must(w.store.Read(nil)).Cursor != before || len(publisher.records) != 1 || w.suspensions() != nil {
		t.Fatal("a refused publish changed the store")
	}
	// The update is still active.
	resolve := func() (any, error) {
		return w.host("reader", "reader", w.runner).Handle(Object{"op": "resolve", "function": "record.visible", "date": "2026-12-01"})
	}
	if selected, err := resolve(); err != nil || selected.(Object)["event_ref"] != event {
		t.Fatalf("resolve after a refused suspension: %v %v", selected, err)
	}
	publisher.refuse = nil
	out, err := host.Handle(copyObject(q))
	if err != nil {
		t.Fatal(err)
	}
	suspension := out.(Object)["result"].(Object)["event_ref"]
	if len(publisher.records) != 2 || !bytes.Equal(publisher.records[0], publisher.records[1]) || !bytes.Equal(publisher.signed[0], publisher.signed[1]) {
		t.Fatal("the retry gave other bytes")
	}
	ref := w.suspensions()[text(suspension)]
	if !bytes.Equal(publisher.records[1], w.stored(ref, "suspension")) ||
		!bytes.Equal(publisher.signed[1], w.stored(w.signatures()[text(ref)], signedKind)) {
		t.Fatal("the publisher got other bytes than the store keeps")
	}
	result, err := VerifySuspension(publisher.signed[1], keys)
	if err != nil || item(result["record"], "event_ref") != suspension || item(result["record"], "request_key") != "suspend-once" {
		t.Fatalf("verify: %v %s", err, Encode(result))
	}
	if _, err := VerifyActivation(publisher.signed[1], keys); faultCode(t, err) != "unexpected_record_kind" {
		t.Fatalf("the published suspension as an activation: %v", err)
	}
	if _, err := resolve(); faultCode(t, err) != "out_of_scope" {
		t.Fatalf("resolve after the suspension: %v", err)
	}
	// A stored request is a reuse. The publisher gets nothing.
	if out, err := host.Handle(copyObject(q)); err != nil || out.(Object)["reused"] != true || len(publisher.records) != 2 {
		t.Fatalf("reuse: %v %v", out, err)
	}
}

// The publisher gets the stored bytes before the commit. A refusal stores
// nothing, and a retry of the same request gives the same bytes.
func TestPublisherRefusalStoresNothing(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	signer, keys := newTestSigner(t)
	publisher := &testPublisher{refuse: errors.New("the head is not the base")}
	w.signer, w.publisher = signer, publisher
	_, decision := w.decided("pass")
	before := must(w.store.Read(nil)).Cursor
	q := Object{"op": "activate", "expected_revision": before, "request_key": "activate-once", "decision_ref": decision, "targets": Array{}}
	host := w.host("reviewer", "reviewer", w.runner)
	_, err := host.Handle(copyObject(q))
	if f, ok := err.(*Fault); !ok || f.Code != "publish_refused" || f.Stage != "publish" || !strings.Contains(f.Message, "the head is not the base") {
		t.Fatalf("refusal: %v", err)
	}
	if must(w.store.Read(nil)).Cursor != before || len(publisher.records) != 1 {
		t.Fatal("a refused publish changed the store")
	}
	publisher.refuse = nil
	out, err := host.Handle(copyObject(q))
	if err != nil {
		t.Fatal(err)
	}
	event := out.(Object)["result"].(Object)["event_ref"]
	if len(publisher.records) != 2 || !bytes.Equal(publisher.records[0], publisher.records[1]) || !bytes.Equal(publisher.signed[0], publisher.signed[1]) {
		t.Fatal("the retry gave other bytes")
	}
	ref := w.activations()[text(event)]
	if !bytes.Equal(publisher.records[1], w.stored(ref, "activation")) ||
		!bytes.Equal(publisher.signed[1], w.stored(w.signatures()[text(ref)], signedKind)) {
		t.Fatal("the publisher got other bytes than the store keeps")
	}
	result, err := VerifyActivation(publisher.signed[1], keys)
	if err != nil || item(result["record"], "event_ref") != event || item(result["record"], "request_key") != "activate-once" {
		t.Fatalf("verify: %v %s", err, Encode(result))
	}
	// A stored request is a reuse. The publisher gets nothing.
	if out, err := host.Handle(copyObject(q)); err != nil || out.(Object)["reused"] != true || len(publisher.records) != 2 {
		t.Fatalf("reuse: %v %v", out, err)
	}
}

// A Host with a publisher and no signing key refuses each activate and
// each suspend.
func TestPublisherNeedsASigningKey(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	publisher := &testPublisher{}
	w.publisher = publisher
	_, decision := w.decided("pass")
	before := must(w.store.Read(nil)).Cursor
	_, err := w.do("reviewer", "reviewer", w.runner, "activate", Object{"decision_ref": decision, "targets": Array{}})
	if f, ok := err.(*Fault); !ok || f.Code != "publish_unsigned" || f.Stage != "publish" {
		t.Fatalf("got %v", err)
	}
	// The refusal comes before the Host examines the targets.
	_, err = w.do("reviewer", "reviewer", w.runner, "suspend", with(window, Object{"targets": Array{decision}, "reason": "stop"}))
	if f, ok := err.(*Fault); !ok || f.Code != "publish_unsigned" || f.Stage != "publish" {
		t.Fatalf("suspend: got %v", err)
	}
	if must(w.store.Read(nil)).Cursor != before || len(publisher.records) != 0 {
		t.Fatal("an unsigned publish wrote or published")
	}
}

// raceActivations sends two activate requests of one decision, with two
// request keys, at the same root. second runs inside the first write, after
// its apply and before its compare-and-swap.
func raceActivations(t *testing.T, w *world, publisher *testPublisher, hook func(second func())) {
	_, first := w.decided("pass")
	other := first
	root := must(w.store.Read(nil)).Cursor
	request := func(key, decision string) Object {
		return Object{"op": "activate", "expected_revision": root, "request_key": key, "decision_ref": decision, "targets": Array{}}
	}
	var won any
	hook(func() {
		out, err := w.host("reviewer", "reviewer", w.runner).Handle(request("activate-second", other))
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		won = out.(Object)["result"].(Object)["event_ref"]
	})
	_, err := w.host("reviewer", "reviewer", w.runner).Handle(request("activate-first", first))
	if faultCode(t, err) != "stale_revision" {
		t.Fatalf("first: %v", err)
	}
	if won == nil || len(publisher.records) != 1 {
		t.Fatalf("published %d records, want only the record of the write that won", len(publisher.records))
	}
	if ref := w.activations()[text(won)]; !bytes.Equal(publisher.records[0], w.stored(ref, "activation")) {
		t.Fatal("the published record is not the stored record")
	}
}

func raceWorld(t *testing.T, store Store) (*world, *testPublisher) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed", bundleRefOf("next"): "passed"}})
	w.store = store
	w.signer, _ = newTestSigner(t)
	publisher := &testPublisher{}
	w.publisher = publisher
	return w, publisher
}

// A write that loses the compare-and-swap does not publish.
func TestPublishOnlyTheWriteThatWins(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		store := NewMemoryStore()
		w, publisher := raceWorld(t, store)
		raceActivations(t, w, publisher, func(second func()) {
			store.BeforePublish = func() error {
				store.BeforePublish = nil
				second()
				return nil
			}
		})
	})
	t.Run("git", func(t *testing.T) {
		store, err := OpenGitStore(t.TempDir() + "/store")
		if err != nil {
			t.Fatal(err)
		}
		w, publisher := raceWorld(t, store)
		raceActivations(t, w, publisher, func(second func()) {
			store.Hook = func(s *GitStore, args []string, data []byte) (GitResult, error) {
				if args[0] == "commit-tree" {
					s.Hook = nil
					second()
				}
				return s.RunGit(args, data)
			}
		})
	})
}

// lockPublisher tries another write while the Host publishes.
type lockPublisher struct {
	write func() error
	err   error
	calls int
}

func (p *lockPublisher) Publish(record, signed []byte) error {
	p.calls++
	p.err = p.write()
	return nil
}

// The Git store publishes while it holds the lock of the root ref. A
// refused publish aborts the ref transaction and leaves no lock.
func TestGitStorePublishesUnderTheRootLock(t *testing.T) {
	store, err := OpenGitStore(t.TempDir() + "/store")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := raceWorld(t, store)
	_, decision := w.decided("pass")
	request := Object{"decision_ref": decision, "targets": Array{}}
	refusing := &testPublisher{refuse: errors.New("no")}
	w.publisher = refusing
	before := must(w.store.Read(nil)).Cursor
	if _, err := w.do("reviewer", "reviewer", w.runner, "activate", request); faultCode(t, err) != "publish_refused" {
		t.Fatalf("refusal: %v", err)
	}
	if must(w.store.Read(nil)).Cursor != before || len(refusing.records) != 1 {
		t.Fatal("a refused publish changed the root")
	}
	if _, err := os.Stat(store.Path + "/" + RootRef + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file: %v", err)
	}
	publisher := &lockPublisher{}
	w.publisher = publisher
	publisher.write = func() error {
		_, err := w.do("reviewer", "reviewer", w.runner, "source", with(window, Object{"text": "other rule", "reason": "other", "supersedes": Array{}}))
		return err
	}
	event := w.must("reviewer", "reviewer", "activate", request)["event_ref"]
	if publisher.calls != 1 || faultCode(t, publisher.err) != "store_error" {
		t.Fatalf("the other write during the publish: %v", publisher.err)
	}
	if root := must(w.store.Read(nil)); root.Cursor == before || w.activations()[text(event)] == nil {
		t.Fatal("the activation is not the new root")
	}
	// The root ref is free again.
	w.must("reviewer", "reviewer", "source", with(window, Object{"text": "third rule", "reason": "third", "supersedes": Array{}}))
}

// otherStore is a store of another package. It does not run commit steps.
type otherStore struct{ Store }

// A Host does not publish through a store that does not run commit steps.
func TestPublisherNeedsAStepStore(t *testing.T) {
	w, publisher := raceWorld(t, otherStore{NewMemoryStore()})
	_, decision := w.decided("pass")
	_, err := w.do("reviewer", "reviewer", w.runner, "activate", Object{"decision_ref": decision, "targets": Array{}})
	if f, ok := err.(*Fault); !ok || f.Code != "publish_unsupported" || f.Stage != "publish" || len(publisher.records) != 0 {
		t.Fatalf("got %v", err)
	}
	_, err = w.do("reviewer", "reviewer", w.runner, "suspend", with(window, Object{"targets": Array{decision}, "reason": "stop"}))
	if f, ok := err.(*Fault); !ok || f.Code != "publish_unsupported" || f.Stage != "publish" || len(publisher.records) != 0 {
		t.Fatalf("suspend: got %v", err)
	}
}
