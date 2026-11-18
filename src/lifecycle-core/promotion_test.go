package lifecycle

import (
	"os"
	"testing"
)

func promotionVectors(t *testing.T) Object {
	t.Helper()
	raw, err := os.ReadFile("../protocol/promotion/vectors/promotion-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	return must(Decode(raw)).(Object)
}

// promotionInputs is inputs_of() of src/protocol/promotion/check_spec.py:
// the candidate and the report of a case. A nil report is no report.
func promotionInputs(doc, c Object) (Object, Object) {
	candidate, has := c["candidate"].(Object)
	if !has {
		candidate = Object{"kind": "content", "manifest": doc["manifests"].(Object)[c["manifest"].(string)]}
	}
	stated, has := c["report"]
	if has && stated == nil {
		return candidate, nil
	}
	report := Object{"passed": true, "measure": nil}
	if has {
		report["passed"] = stated.(Object)["passed"]
	}
	if name, ok := c["summary"].(string); ok {
		report["measure"] = must(Decode(Encode(doc["summaries"].(Object)[name])))
	}
	return candidate, report
}

func promotionCollections(doc, c Object) Array {
	if list, has := c["promotion_collections"].(Array); has {
		return list
	}
	return doc["defaults"].(Object)["promotion_collections"].(Array)
}

// TestPromotionVectors runs the hand-written checks of PROMOTION.md against
// the Go rules. For the policy cases, the measure inputs are absent exactly
// when the expected verdict routes with missing_input.
func TestPromotionVectors(t *testing.T) {
	doc := promotionVectors(t)
	checks := doc["checks"].(Array)
	if len(checks) == 0 {
		t.Fatal("no checks")
	}
	for _, x := range checks {
		c := x.(Object)
		candidate, report := promotionInputs(doc, c)
		inputs, rule := judgePromotion(promotionCollections(doc, c), candidate, report)
		got := Object{"valid": rule == ""}
		if rule == "" {
			got["inputs"] = inputs
		} else {
			got["rule"] = rule
		}
		if string(Encode(got)) != string(Encode(c["expect"])) {
			t.Errorf("%s: got %s, want %s", c["name"], Encode(got), Encode(c["expect"]))
		}
	}
	for _, x := range doc["cases"].(Array) {
		c := x.(Object)
		candidate, report := promotionInputs(doc, c)
		inputs, rule := judgePromotion(promotionCollections(doc, c), candidate, report)
		missing := lookup(c["expect"], "reason", nil) == "missing_input"
		if missing != (rule != "") {
			t.Errorf("%s: rule %q, but the verdict reason is %v", c["name"], rule, lookup(c["expect"], "reason", nil))
		}
		if refusal, has := c["refusal"].(Object); has {
			largest, _ := number(inputs["measure_max_bound_bp"])
			limit, _ := number(refusal["limit_bp"])
			if largest == nil || largest.Cmp(limit) <= 0 {
				t.Errorf("%s: the largest bound %v is not above the limit %v", c["name"], inputs["measure_max_bound_bp"], refusal["limit_bp"])
			}
		}
	}
}

// measureRunner is the content fixture runner with a measure summary for
// each candidate. A candidate without a summary gives no measure.
type measureRunner struct {
	*contentRunner
	measures map[string]any
}

func (r *measureRunner) Qualify(in QualificationInput) (Verdict, error) {
	verdict, err := r.contentRunner.Qualify(in)
	verdict.Measure = r.measures[in.Candidate.BundleRef]
	return verdict, err
}

// TestPromotionRead runs the promotion read on stored reports. The read
// gives the inputs of a bound summary and the rule of each other report.
// It writes nothing, and only a reviewer can use it.
func TestPromotionRead(t *testing.T) {
	doc := promotionVectors(t)
	manifests, summaries := doc["manifests"].(Object), doc["summaries"].(Object)
	runner := &measureRunner{&contentRunner{status: map[string]string{}, names: map[string]string{}}, map[string]any{}}
	w := newWorld(t, runner)
	subject := "content:extraction-profiles/fee-schedule"
	w.functions = Array{subject}
	w.context = Object{"promotion_collections": Array{"extraction-profiles"}}
	wide := Object{"function": subject, "from": "2025-01-01", "until": "2029-01-01"}
	source := w.must("reviewer", "reviewer", "source", with(wide, Object{"text": "the promotion rules", "reason": "first", "supersedes": Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "measure"}}, "origin": "independent",
		"supersedes": Array{}})["plan_ref"]
	snapshots := map[string]any{}
	for _, name := range []string{"v2.0.0", "v2.1.0"} {
		out := w.must("author", "author", "propose", Object{"candidate": Object{"kind": "content", "manifest": manifests[name]},
			"from": "2026-11-01", "until": "2027-01-01", "source_ref": source, "reason": "promote " + name})
		snapshots[name] = out["snapshot_ref"]
		ref := out["candidate"].(Object)["ref"].(string)
		runner.status[ref], runner.names[ref] = "passed", ref
	}
	qualify := func(name string, summary any) string {
		ref := judgeCandidate(Object{"kind": "content", "manifest": manifests[name]}).Ref
		runner.measures[ref] = summary
		out := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshots[name], "plan_ref": plan})
		report := out["report"].(Object)
		if _, has := report["measure"]; has != (summary != nil) || has && !equal(report["measure"], summary) {
			t.Fatalf("report of %s keeps measure %s", name, Encode(report["measure"]))
		}
		return out["report_ref"].(string)
	}
	summary := func(name string) any { return must(Decode(Encode(summaries[name]))) }
	bound := qualify("v2.0.0", summary("v2.0.0"))
	none := qualify("v2.1.0", nil)
	other := qualify("v2.1.0", summary("v2.0.0"))
	root := must(w.store.Read(nil)).Cursor
	read := func(h *Host, ref string) Object {
		out, err := h.Handle(Object{"op": "promotion", "report_ref": ref})
		if err != nil {
			t.Fatal(err)
		}
		return out.(Object)
	}
	reviewer := w.host("reviewer", "reviewer", runner)
	want := lookup(doc["checks"].(Array)[0], "expect", nil).(Object)["inputs"]
	if got := read(reviewer, bound); got["promotion"] != "valid" || !equal(got["inputs"], want) || got["snapshot_ref"] != snapshots["v2.0.0"] {
		t.Fatalf("bound summary: %s", Encode(got))
	}
	for ref, rule := range map[string]string{none: "no_summary", other: "digest_binding"} {
		if got := read(reviewer, ref); got["promotion"] != "invalid" || got["rule"] != rule || got["inputs"] != nil {
			t.Fatalf("want rule %s: %s", rule, Encode(got))
		}
	}
	w.context = nil
	if got := read(w.host("reviewer", "reviewer", runner), bound); got["rule"] != "not_promotion" {
		t.Fatalf("a Host without promotion_collections: %s", Encode(got))
	}
	if must(w.store.Read(nil)).Cursor != root {
		t.Fatal("the promotion read wrote state")
	}
	if _, err := w.host("reader", "reader", runner).Handle(Object{"op": "promotion", "report_ref": bound}); faultCode(t, err) != "forbidden" {
		t.Fatal("a reader got the promotion read")
	}
}

func TestPromotionContext(t *testing.T) {
	for _, bad := range []any{"extraction-profiles", Array{""}, Array{Int{Text: "1"}}} {
		_, err := NewHostWithRunner(NewMemoryStore(), &fakeEngine{build: engineBuild}, &CELRunner{Runtime: &fakeEngine{build: engineBuild}},
			Object{"scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": Array{"record.visible"}, "promotion_collections": bad})
		if faultCode(t, err) != "configuration_error" {
			t.Fatalf("promotion_collections %s: %v", Encode(bad), err)
		}
	}
}
