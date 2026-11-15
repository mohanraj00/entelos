package lifecycle

// Go tests for src/protocol/activation/ACTIVATION.md: the vectors, the
// record that the Host writes, and the publisher.

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func activationVectors(t *testing.T) Object {
	raw, err := os.ReadFile("../protocol/activation/vectors/activation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	return must(Decode(raw)).(Object)
}

// activationResult gives the result of the rules in the vector shape.
func activationResult(kind string, raw []byte) Object {
	_, err := checkActivation(kind, raw)
	if err == nil {
		return Object{"valid": true}
	}
	fault := err.(*Fault)
	out := Object{"valid": false, "code": fault.Code}
	if fault.Code == "invalid_activation" {
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

// Without a signing key, the Host writes no activation record.
func TestNoActivationRecordWithoutASigningKey(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	w.approve("pass")
	if index := w.activations(); index != nil {
		t.Fatalf("activations %s", Encode(index))
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

// A Host with a publisher and no signing key refuses each activate.
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
	if must(w.store.Read(nil)).Cursor != before || len(publisher.records) != 0 {
		t.Fatal("an unsigned publish wrote or published")
	}
}
