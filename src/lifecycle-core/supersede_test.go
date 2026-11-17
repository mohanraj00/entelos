package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// supersedeRun runs the steps of one supersession vector on a fresh memory
// store, with the fixture runner and a signing key.
type supersedeRun struct {
	t        *testing.T
	w        *world
	verdicts map[string]string
	source   string
	plan     string
	head     string
	// decisions has the decision of each update and interval.
	decisions map[string]string
	// headBundle is the bundle of the head proposal.
	headBundle string
	events     map[string]string
	bundles    map[string]string
	receipts   map[string]Object
}

func newSupersedeRun(t *testing.T) *supersedeRun {
	verdicts := map[string]string{}
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: verdicts})
	w.signer, _ = newTestSigner(t)
	wide := Object{"function": "record.visible", "from": "2025-01-01", "until": "2028-01-01"}
	source := w.must("reviewer", "reviewer", "source", with(wide, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))["source_ref"].(string)
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"].(string)
	return &supersedeRun{t: t, w: w, verdicts: verdicts, source: source, plan: plan, decisions: map[string]string{},
		events: map[string]string{}, bundles: map[string]string{}, receipts: map[string]Object{}}
}

// decision proposes, qualifies and accepts the update for one interval.
func (r *supersedeRun) decision(id, from, until string) string {
	key := id + " " + from + " " + until
	if ref, ok := r.decisions[key]; ok {
		// A later proposal made the verdict stale (VERDICT.md). A
		// proposal of the same bytes makes it current again.
		if r.headBundle != r.bundles[key] {
			r.head = r.w.must("author", "author", "propose", Object{"function": "record.visible", "from": from, "until": until,
				"base_snapshot": r.head, "source_ref": r.source, "bundle": bundle(strings.ReplaceAll(key, " ", "-")), "reason": "again " + id})["snapshot_ref"].(string)
			r.headBundle = r.bundles[key]
		}
		return ref
	}
	rule := strings.ReplaceAll(key, " ", "-")
	r.verdicts[bundleRefOf(rule)] = "passed"
	proposal := r.w.must("author", "author", "propose", Object{"function": "record.visible", "from": from, "until": until,
		"base_snapshot": r.head, "source_ref": r.source, "bundle": bundle(rule), "reason": "update " + id})
	r.head = proposal["snapshot_ref"].(string)
	r.bundles[key] = proposal["bundle_ref"].(string)
	r.headBundle = r.bundles[key]
	qualified := r.w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": r.head, "plan_ref": r.plan})
	ref := r.w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "accept " + id})["decision_ref"].(string)
	r.decisions[key] = ref
	return ref
}

// refs gives the event references of the update names. An unknown name
// gives a reference that no record has.
func (r *supersedeRun) refs(names any) Array {
	out := Array{}
	for _, name := range elements(names) {
		ref, ok := r.events[name.(string)]
		if !ok {
			ref = "sha256:" + strings.Repeat("0", 64)
		}
		out = append(out, ref)
	}
	return out
}

// name gives the update name of an event reference.
func (r *supersedeRun) name(ref any) string {
	for name, known := range r.events {
		if known == ref {
			return name
		}
	}
	return "unknown event"
}

func outcome(name string, err error) any {
	if err != nil {
		if fault, ok := err.(*Fault); ok {
			return Object{"error": fault.Code}
		}
		return Object{"error": err.Error()}
	}
	return name
}

func (r *supersedeRun) step(step Object) any {
	w := r.w
	at := Object{"function": "record.visible", "date": step["date"]}
	switch {
	case step["activate"] != nil:
		id, from, until := step["activate"].(string), step["from"].(string), step["until"].(string)
		q := Object{"decision_ref": r.decision(id, from, until), "targets": r.refs(lookup(step, "targets", Array{}))}
		if _, has := step["supersedes"]; has {
			q["supersedes"] = r.refs(step["supersedes"])
		}
		out, err := w.do("reviewer", "reviewer", w.runner, "activate", q)
		if err == nil {
			r.events[id] = out["event_ref"].(string)
			r.bundles[id] = r.bundles[id+" "+from+" "+until]
		}
		return outcome(id, err)
	case step["suspend"] != nil:
		id := step["suspend"].(string)
		out, err := w.do("reviewer", "reviewer", w.runner, "suspend", Object{"function": "record.visible", "from": step["from"],
			"until": step["until"], "targets": r.refs(step["targets"]), "reason": "stop"})
		if err == nil {
			r.events[id] = out["event_ref"].(string)
		}
		return outcome(id, err)
	case step["resolve"] != nil:
		out, err := w.host("reader", "reader", w.runner).Handle(Object{"op": "resolve", "function": "record.visible", "date": step["resolve"]})
		if err != nil {
			return outcome("", err)
		}
		return r.name(out.(Object)["event_ref"])
	case step["answer"] != nil:
		// A new request: the worker loads the selection, then the caller invokes.
		loaded, err := w.do("worker", "worker", w.runner, "load", at)
		if err != nil {
			return outcome("", err)
		}
		answer, err := w.do("caller", "caller", w.runner, "invoke", with(at, Object{"expected_bundle_ref": loaded["bundle_ref"], "inputs": Object{"state": "ready"}}))
		if err != nil {
			return outcome("", err)
		}
		receipt := answer["receipt"].(Object)
		receipt["ref"] = answer["receipt_ref"]
		r.receipts[step["answer"].(string)] = receipt
		name := r.name(item(receipt["selection"], "event_ref"))
		if receipt["actual_bundle"] != r.bundles[name] {
			r.t.Errorf("answer %s: bundle %v is not the bundle of %s", step["answer"], receipt["actual_bundle"], name)
		}
		return name
	case step["replay"] != nil:
		old := r.receipts[step["replay"].(string)]
		out, err := w.do("reader", "reader", w.runner, "replay", Object{"receipt_ref": old["ref"], "reason": "check an old answer"})
		if err != nil {
			return outcome("", err)
		}
		again := out["receipt"].(Object)
		// The replay gives the bytes of the old answer, from the old update.
		if again["matches_original"] != true || again["response_ref"] != old["response_ref"] || again["actual_bundle"] != old["actual_bundle"] ||
			!equal(again["selection"], old["selection"]) {
			r.t.Errorf("replay %s: %s", step["replay"], Encode(again))
		}
		return r.name(item(again["selection"], "event_ref"))
	case step["record"] != nil:
		stored, err := w.read(Object{"op": "record", "kind": "event", "ref": r.events[step["record"].(string)]})
		if err != nil {
			return outcome("", err)
		}
		// The record names each update that it supersedes, by reference.
		if !equal(lookup(stored, "supersedes", Array{}), r.refs(step["supersedes"])) {
			r.t.Errorf("record %s: supersedes %s", step["record"], Encode(lookup(stored, "supersedes", Array{})))
		}
		// A record with no supersession has no supersedes field.
		if _, has := stored["supersedes"]; has != (len(elements(step["supersedes"])) > 0) {
			r.t.Errorf("record %s: supersedes field present is %v", step["record"], has)
		}
		return nil
	}
	r.t.Fatalf("unknown step %s", Encode(step))
	return nil
}

// The Host gives the stated result of each step of the supersession
// vectors. A person wrote the expected values from SUPERSESSION.md. Each
// activation record of a run is also a signed record.
func TestSupersessionVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "supersession", "vectors", "supersession-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range doc.(Object)["cases"].(Array) {
		c := c.(Object)
		t.Run(c["name"].(string), func(t *testing.T) {
			r := newSupersedeRun(t)
			for i, x := range c["steps"].(Array) {
				step := x.(Object)
				got := r.step(step)
				want, has := step["expect"]
				if !has && (step["activate"] != nil || step["suspend"] != nil) {
					want = got
					if _, failed := got.(Object); failed {
						t.Fatalf("step %d: %s", i, Encode(got))
					}
				}
				if !equal(got, want) {
					t.Errorf("step %d %s: got %s", i, Encode(step), Encode(got))
				}
			}
			index := r.w.signatures()
			for name, ref := range r.events {
				if _, signed := index[ref]; !signed {
					t.Errorf("the event of %s has no signed record", name)
				}
			}
		})
	}
}

// A worker that loaded a superseded update gives no new answer after the
// successor ends, also in the continue_stale mode. After a mask in the same
// position ends, the old update is live again, as before.
func TestSupersededUpdateGivesNoStaleAnswer(t *testing.T) {
	for _, mode := range []string{"supersedes", "targets"} {
		verdicts := map[string]string{bundleRefOf("old"): "passed", bundleRefOf("new"): "passed"}
		w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: verdicts})
		wide := Object{"function": "record.visible", "from": "2026-01-01", "until": "2027-01-01"}
		source := w.must("reviewer", "reviewer", "source", with(wide, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))["source_ref"]
		plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"]
		activate := func(rule, from, until, base string, extra Object) (string, string) {
			proposal := w.must("author", "author", "propose", Object{"function": "record.visible", "from": from, "until": until,
				"base_snapshot": base, "source_ref": source, "bundle": bundle(rule), "reason": rule, "stale_mode": "continue_stale"})
			qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})
			decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "accept"})["decision_ref"]
			event := w.must("reviewer", "reviewer", "activate", merge(Object{"decision_ref": decision, "targets": Array{}}, extra))["event_ref"]
			return proposal["snapshot_ref"].(string), event.(string)
		}
		head, first := activate("old", "2026-01-01", "2027-01-01", "", Object{})
		w.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-02-01"})
		activate("new", "2026-03-01", "2026-05-01", head, Object{mode: Array{first}})
		// On 2026-06-01 the successor is over, and the worker still has the old update.
		_, err := w.do("caller", "caller", w.runner, "invoke", Object{"function": "record.visible", "date": "2026-06-01",
			"expected_bundle_ref": bundleRefOf("old"), "inputs": Object{"state": "ready"}})
		if mode == "supersedes" {
			if faultCode(t, err) != "activation_superseded" {
				t.Fatalf("superseded update: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("masked update: %v", err)
		}
	}
}
