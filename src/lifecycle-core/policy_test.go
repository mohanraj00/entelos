package lifecycle

// The vectors of src/protocol/policy/vectors/policy-cases.json, run on the
// rule functions that policy_decide, policy_replay and activate use. The
// expected values come from the vector file. A person wrote them from
// POLICY.md. No expected value comes from a run of this code.

import (
	"os"
	"strings"
	"testing"
)

func policyVectors(t *testing.T) Object {
	t.Helper()
	raw, err := os.ReadFile("../protocol/policy/vectors/policy-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	return must(Decode(raw)).(Object)
}

// vectorCall gives the call of a vector case. A case without a call fails
// the test if the rules reach the call.
func vectorCall(t *testing.T, name string, call any) func(Object) (any, any) {
	return func(Object) (any, any) {
		c, ok := call.(Object)
		if !ok {
			t.Errorf("%s: the rules reach the call, and the case has no call", name)
			return nil, "no_call"
		}
		return c["output"], c["error"]
	}
}

func TestPolicyVectors(t *testing.T) {
	vectors := policyVectors(t)
	defaults := vectors["defaults"].(Object)
	stored := map[string]Object{}
	for _, raw := range vectors["cases"].(Array) {
		c := merge(copyObject(defaults), raw.(Object))
		name := text(c["name"])
		it := c["item"].(Object)
		values := Object{}
		sources := c["sources"].(Object)
		if evidence, ok := sources["evidence"].(Object); ok {
			for k, x := range evidence {
				values[k] = x
			}
		}
		if report, ok := sources["report"].(Object); ok {
			values["report_passed"] = report["passed"]
			if class := report["change_class"]; class != nil {
				values["change_class"] = class
			}
		}
		policy, _ := c["policy"].(Object)
		verdict, _ := judgePolicy(policyCase{date: text(c["date"]), functions: c["policy_functions"].(Array),
			requirements: c["requirements"].(Array), policy: policy, typed: true,
			item: Object{"snapshot_ref": it["snapshot_ref"], "report_ref": it["report_ref"], "candidate": it["candidate"]},
			head: it["head"].(Object), values: values, call: vectorCall(t, name, c["call"]),
			nonce: func() string { return text(c["nonce"]) }})
		if !equal(verdict, c["expect"]) {
			t.Errorf("%s: verdict\n%s\nwant\n%s", name, Encode(verdict), Encode(c["expect"]))
		}
		stored[name] = verdict
	}
	for _, raw := range vectors["replays"].(Array) {
		r := raw.(Object)
		name := text(r["name"])
		policy, _ := r["store_policy"].(Object)
		got := replayPolicy(stored[text(r["of"])], r["inputs"].(Object), policy, text(r["build"]), vectorCall(t, name, r["call"]))
		if !equal(got, r["expect"]) {
			t.Errorf("%s: replay %s, want %s", name, Encode(got), Encode(r["expect"]))
		}
	}
	for _, raw := range vectors["requirements"].(Array) {
		r := raw.(Object)
		var verdicts []Object
		for _, x := range r["verdicts"].(Array) {
			verdicts = append(verdicts, x.(Object))
		}
		if got := verdictRequirement(text(r["requirement"]), r["changes_policy"] == true, verdicts); got != r["expect"] {
			t.Errorf("%s: requirement %s, want %s", r["name"], got, r["expect"])
		}
	}
}

// The rules do not call a policy that is not a typed rule with a string
// output (rule 5), and a call value that is not a string routes the item.
func TestPolicyShapeRules(t *testing.T) {
	defaults := policyVectors(t)["defaults"].(Object)
	it := defaults["item"].(Object)
	base := policyCase{date: "2026-11-20", functions: defaults["policy_functions"].(Array), requirements: defaults["requirements"].(Array),
		policy: defaults["policy"].(Object), typed: true, item: Object{"snapshot_ref": it["snapshot_ref"]}, head: it["head"].(Object),
		values: Object{"report_passed": true, "change_class": "typo"}, nonce: newNonce}
	untyped := base
	untyped.typed = false
	untyped.call = func(Object) (any, any) { t.Error("the rules called an untyped policy"); return "accept", nil }
	if verdict, _ := judgePolicy(untyped); verdict["reason"] != "invalid_policy" || verdict["receipt"] != nil {
		t.Errorf("untyped policy: %s", Encode(verdict))
	}
	boolean := base
	boolean.call = func(Object) (any, any) { return true, nil }
	verdict, _ := judgePolicy(boolean)
	if verdict["reason"] != "policy_error" || at(verdict, "receipt", "error") != "output_type" || at(verdict, "receipt", "output") != nil {
		t.Errorf("bool output: %s", Encode(verdict))
	}
	if nonce := newNonce(); !nonceText.MatchString(nonce) {
		t.Errorf("nonce %q", nonce)
	}
}

func TestPolicyContext(t *testing.T) {
	base := Object{"actor": "a", "roles": Array{"reviewer"}, "scope": "demo", "runner_actor": "r", "worker": "w",
		"functions": Array{"record.visible", "approval-policy"}}
	head := Object{"kind": "cel-bundle", "function": "record.visible", "scope": "demo"}
	good := []Object{
		{"policy_functions": Array{"approval-policy"}},
		{"requirements": Array{Object{"head": head, "requirement": "policy"}}},
		{"requirements": Array{Object{"head": Object{"kind": "content", "collection": "c", "partition": "p"}, "requirement": "human"}}},
	}
	bad := []Object{
		{"policy_functions": Array{}},
		{"policy_functions": Array{"approval-policy", "record.visible"}},
		{"policy_functions": Array{"not-served"}},
		{"policy_functions": "approval-policy"},
		{"requirements": Array{Object{"head": head, "requirement": "model"}}},
		{"requirements": Array{Object{"head": head, "requirement": "policy"}, Object{"head": head, "requirement": "human"}}},
		{"requirements": Array{Object{"head": Object{"kind": "cel-bundle", "function": "x"}, "requirement": "policy"}}},
		{"requirements": Array{Object{"head": head, "requirement": "policy", "note": "x"}}},
		{"requirements": Object{}},
	}
	for _, extra := range good {
		if _, err := NewHost(NewMemoryStore(), &fakeEngine{build: engineBuild}, merge(copyObject(base), extra)); err != nil {
			t.Errorf("%s: %v", Encode(extra), err)
		}
	}
	for _, extra := range bad {
		_, err := NewHost(NewMemoryStore(), &fakeEngine{build: engineBuild}, merge(copyObject(base), extra))
		if fault, ok := err.(*Fault); !ok || fault.Code != "configuration_error" {
			t.Errorf("%s: %v, want configuration_error", Encode(extra), err)
		}
	}
}

// policyEngine is a Go fake of the CEL runtime. It describes each bundle
// with the function_id of its manifest. A run of a cel-typed/v1 bundle gives
// the stated output or the stated error status. It is not a CEL engine; the
// CEL scenario TestPolicyVerdictLifecycleOnCEL runs a real policy.
type policyEngine struct {
	build, output, status string
	runs                  int
}

func (e *policyEngine) Build() (string, error) { return e.build, nil }

func (e *policyEngine) Execute(files map[string][]byte, command string, inputs []byte, cases any) (any, error) {
	manifest := must(Decode(files["manifest"])).(Object)
	bundle := ContentRef(Envelope(Mimes["manifest"], files["manifest"]))
	function := lookup(manifest, "function_id", "record.visible")
	switch command {
	case "describe":
		return Object{"function_id": function, "bundle_ref": bundle, "implementation_ref": e.build}, nil
	case "run":
		out := Object{"status": "ok", "bundle_ref": bundle, "implementation_ref": e.build}
		if manifest["profile"] != policyProfile {
			out["result"] = true
			return out, nil
		}
		e.runs++
		if e.status != "" {
			out["status"] = e.status
		} else {
			out["value"] = e.output
		}
		return out, nil
	}
	return nil, NewFault("unexpected_command", "the policy engine does not run "+command, "runtime")
}

const policyManifest = `{"caller_variables":["report_passed"],"environment_ref":"env","function_id":"approval-policy","limits":{},"profile":"cel-typed/v1"}`

var policyBundle = Object{"manifest": policyManifest, "environment": `{"output":{"type":"string"}}`,
	"source": `report_passed ? "accept" : "refuse"`, "fixed": `{"audit_rate_ppm":1000000}`}

var recordHead = Object{"kind": "cel-bundle", "function": "record.visible", "scope": "demo"}

// newPolicyWorld gives a world with an active policy approval-policy and a
// qualified item of record.visible. It gives the item report reference.
func newPolicyWorld(t *testing.T, engine *policyEngine, requirement string) (*world, string) {
	t.Helper()
	now := "2026-11-20T09:00:00Z"
	passing, policyRef := bundleRefOf("pass"), ContentRef(Envelope(Mimes["manifest"], []byte(policyManifest)))
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed", policyRef: "passed"}})
	w.now, w.engine = &now, engine
	w.functions = Array{"record.visible", "approval-policy"}
	w.context = Object{"policy_functions": Array{"approval-policy"}}
	if requirement != "" {
		w.context["requirements"] = Array{Object{"head": recordHead, "requirement": requirement}}
	}
	policyWindow := with(window, Object{"function": "approval-policy"})
	source := w.must("reviewer", "reviewer", "source", with(policyWindow, Object{"text": "approval policy", "reason": "policy", "supersedes": Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"]
	proposal := w.must("author", "author", "propose", with(policyWindow, Object{"base_snapshot": "", "source_ref": source, "bundle": policyBundle, "reason": "policy"}))
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})["report_ref"]
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "a person approves the policy"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})
	snapshot, itemPlan, _ := w.candidate("pass")
	itemReport := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": itemPlan})["report_ref"].(string)
	return w, itemReport
}

// An accept of the active policy activates the item, counts as the kind
// policy, and replays to the same outcome.
func TestPolicyDecideAcceptActivates(t *testing.T) {
	engine := &policyEngine{build: engineBuild, output: "accept"}
	w, report := newPolicyWorld(t, engine, "policy")
	out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
	verdict := out["verdict"].(Object)
	if verdict["outcome"] != "accept" || verdict["reason"] != "policy_output" || verdict["date"] != "2026-11-20" ||
		at(verdict, "receipt", "inputs_ref") != InputDigest(Object{"report_passed": true}) || at(verdict, "audit", "selected") != true {
		t.Fatalf("verdict %s", Encode(verdict))
	}
	host := w.host("reviewer", "reviewer", w.runner)
	approval := must(w.host("reader", "reader", w.runner).Handle(Object{"op": "approval", "function": "record.visible", "candidate": Object{"kind": "cel-bundle", "ref": bundleRefOf("pass")}})).(Object)
	if approval["approved"] != true || at(approval, "counts", "policy") != 1 || at(approval, "counts", "human") != 0 {
		t.Fatalf("approval %s", Encode(approval))
	}
	event := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": out["decision_ref"], "targets": Array{}})
	if event["event_ref"] == nil {
		t.Fatalf("activate %v", event)
	}
	replay := func() Object {
		return must(host.Handle(Object{"op": "policy_replay", "policy_verdict_ref": out["policy_verdict_ref"]})).(Object)
	}
	if got := replay(); got["replay"] != "same" {
		t.Fatalf("policy_replay %s", Encode(got))
	}
	engine.output = "route"
	if got := replay(); got["replay"] != "differs" || got["outcome"] != "route" {
		t.Fatalf("policy_replay after a change %s", Encode(got))
	}
	engine.output = "accept"
	record := must(host.Handle(Object{"op": "record", "kind": policyVerdictKind, "ref": out["policy_verdict_ref"]})).(Object)
	replayed := w.must("reader", "reader", "replay", Object{"receipt_ref": record["receipt_ref"], "reason": "policy audit"})
	if at(replayed, "receipt", "matches_original") != true {
		t.Fatalf("replay %s", Encode(replayed))
	}
	// A second policy accept or refuse of the same snapshot is a duplicate.
	if _, err := w.do("reviewer", "reviewer", w.runner, "policy_decide", Object{"report_ref": report}); faultCode(t, err) != "duplicate_verdict" {
		t.Fatalf("second policy_decide: %v", err)
	}
}

// Each route writes a verdict and no decision, and activate has nothing to
// activate.
func TestPolicyDecideRoutes(t *testing.T) {
	cases := []struct {
		name, requirement, output, status, reason string
		suspend                                   bool
	}{
		{"human-head", "", "accept", "", "human_required", false},
		{"route-output", "policy", "route", "", "policy_output", false},
		{"invalid-output", "policy", "approve", "", "invalid_output", false},
		{"engine-error", "policy", "", "evaluation_error", "policy_error", false},
		{"suspended", "policy", "accept", "", "policy_suspended", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			engine := &policyEngine{build: engineBuild, output: c.output, status: c.status}
			w, report := newPolicyWorld(t, engine, c.requirement)
			if c.suspend {
				events := must(w.host("reviewer", "reviewer", w.runner).Handle(Object{"op": "inspect"})).(Object)["state"].(Object)["events"].(Array)
				w.must("reviewer", "reviewer", "suspend", with(window, Object{"function": "approval-policy", "targets": Array{events[0]}, "reason": "stop"}))
			}
			runs := engine.runs
			out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
			verdict := out["verdict"].(Object)
			if verdict["outcome"] != "route" || verdict["reason"] != c.reason || out["decision_ref"] != nil || verdict["audit"] != nil {
				t.Fatalf("verdict %s", Encode(out))
			}
			if called := engine.runs > runs; called != (verdict["receipt"] != nil) {
				t.Fatalf("call %v, receipt %v", called, verdict["receipt"])
			}
		})
	}
}

// A refusal of the receipt path is a policy_error verdict. Its inputs stay
// in the store, so a replay on the original build gives the same outcome.
func TestPolicyErrorKeepsItsInputs(t *testing.T) {
	engine := &policyEngine{build: engineBuild, output: "accept"}
	w, report := newPolicyWorld(t, engine, "policy")
	engine.build = "sha256:" + strings.Repeat("f", 64)
	out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
	verdict := out["verdict"].(Object)
	if verdict["reason"] != "policy_error" || at(verdict, "receipt", "error") != "implementation_unavailable" || out["decision_ref"] != nil {
		t.Fatalf("verdict %s", Encode(out))
	}
	engine.build = engineBuild
	replay := must(w.host("reviewer", "reviewer", w.runner).Handle(Object{"op": "policy_replay", "policy_verdict_ref": out["policy_verdict_ref"]})).(Object)
	if replay["replay"] != "differs" || replay["outcome"] != "accept" {
		t.Fatalf("policy_replay %s", Encode(replay))
	}
}

// A refuse rejects the snapshot. A policy accept on a head that needs a
// human verdict does not activate, and a verdict policy that needs a human
// is not met by a policy verdict (KINDS.md rule 3).
func TestPolicyVerdictCountsOnlyAsPolicy(t *testing.T) {
	t.Run("refuse", func(t *testing.T) {
		w, report := newPolicyWorld(t, &policyEngine{build: engineBuild, output: "refuse"}, "policy")
		w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
		_, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": report, "reason": "a person accepts"})
		if faultCode(t, err) != "proposal_rejected" {
			t.Fatalf("decide after refuse: %v", err)
		}
	})
	t.Run("human-head", func(t *testing.T) {
		w, report := newPolicyWorld(t, &policyEngine{build: engineBuild, output: "accept"}, "policy")
		out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
		delete(w.context, "requirements")
		if _, err := w.do("reviewer", "reviewer", w.runner, "activate", Object{"decision_ref": out["decision_ref"], "targets": Array{}}); faultCode(t, err) != "human_verdict_required" {
			t.Fatalf("activate: %v", err)
		}
		approval := must(w.host("reader", "reader", w.runner).Handle(Object{"op": "approval", "function": "record.visible",
			"candidate": Object{"kind": "cel-bundle", "ref": bundleRefOf("pass")}})).(Object)
		if approval["approved"] != false || at(approval, "counts", "policy") != 0 {
			t.Fatalf("approval %s", Encode(approval))
		}
	})
	t.Run("verdict-policy", func(t *testing.T) {
		w, report := newPolicyWorld(t, &policyEngine{build: engineBuild, output: "accept"}, "policy")
		out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
		w.context["verdict_policy"] = Object{"human": Int{"1"}}
		if _, err := w.do("reviewer", "reviewer", w.runner, "activate", Object{"decision_ref": out["decision_ref"], "targets": Array{}}); faultCode(t, err) != "verdicts_required" {
			t.Fatalf("activate: %v", err)
		}
	})
}

// A change of a policy function needs a human verdict (rule 1).
func TestPolicyDecideRefusesItsOwnChange(t *testing.T) {
	w, _ := newPolicyWorld(t, &policyEngine{build: engineBuild, output: "accept"}, "policy")
	host := w.host("reviewer", "reviewer", w.runner)
	// A new snapshot of the active policy function.
	source := w.must("reviewer", "reviewer", "source", with(window, Object{"function": "approval-policy", "text": "policy v2", "reason": "v2", "supersedes": Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"]
	heads := must(host.Handle(Object{"op": "inspect"})).(Object)["state"].(Object)["heads"].(Object)
	base := heads[headKey("approval-policy", "demo")]
	proposal := w.must("author", "author", "propose", with(window, Object{"function": "approval-policy", "base_snapshot": base, "source_ref": source,
		"bundle": with(policyBundle, Object{"source": `"accept"`}), "reason": "v2"}))
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})["report_ref"]
	out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
	if at(out, "verdict", "reason") != "policy_change" || out["decision_ref"] != nil {
		t.Fatalf("verdict %s", Encode(out))
	}
}

// evidenceCase gives the evidence document of a case of
// src/protocol/evidence/vectors/evidence-cases.json.
func evidenceCase(t *testing.T, name string) Object {
	t.Helper()
	raw, err := os.ReadFile("../protocol/evidence/vectors/evidence-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range must(Decode(raw)).(Object)["cases"].(Array) {
		if c := x.(Object); c["name"] == name {
			return c["evidence"].(Object)
		}
	}
	t.Fatalf("evidence-cases.json has no case %s", name)
	return nil
}

// The lifecycle of POLICY.md on the CEL engine: a person approves a typed
// policy, and the policy then accepts a typed rule with a passed report and
// a human review. The steps are propose, qualify, policy_decide, activate,
// policy_replay and replay. The test needs LIFECYCLE_CEL_BINARY.
func TestPolicyVerdictLifecycleOnCEL(t *testing.T) {
	binary := os.Getenv("LIFECYCLE_CEL_BINARY")
	if binary == "" {
		if os.Getenv("LIFECYCLE_REQUIRE_SCENARIO") != "" {
			t.Fatal("LIFECYCLE_CEL_BINARY is not set")
		}
		t.Skip("LIFECYCLE_CEL_BINARY is not set")
	}
	rt, err := NewCELRuntime(binary, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	build := must(rt.Build())
	vector := readVector(t, "typed/admission-record.json")["bundle"].(Object)
	now := "2026-06-01T09:00:00Z"
	w := newWorld(t, &CELRunner{Runtime: rt})
	w.now, w.engine = &now, rt
	w.functions = Array{vector["function_id"], "approval-policy"}
	w.context = Object{"policy_functions": Array{"approval-policy"}, "requirements": Array{Object{"head": Object{"kind": "cel-bundle",
		"function": vector["function_id"], "scope": "demo"}, "requirement": "policy"}}}
	interval := Object{"from": "2026-01-01", "until": "2027-01-01"}

	// A person approves the policy. Its expected values are stated here
	// from the policy text, not from a run.
	policy := Object{"function_id": "approval-policy", "profile": "cel-typed/v1", "caller_variables": Array{"evidence_human_reviewed", "report_passed"},
		"limits": vector["limits"], "fixed": Object{"audit_rate_ppm": Int{"1000000"}},
		"environment": Object{"name": "approval-policy", "variables": Array{
			Object{"name": "report_passed", "type": "bool"}, Object{"name": "evidence_human_reviewed", "type": "bool"},
			Object{"name": "audit_rate_ppm", "type": "int"}}, "output": Object{"type": "string"}},
		"source": `report_passed && evidence_human_reviewed ? "accept" : (report_passed ? "route" : "refuse")`}
	policyCases := Array{
		Object{"case": "accept", "inputs": Object{"report_passed": true, "evidence_human_reviewed": true}, "status": "ok", "value": "accept"},
		Object{"case": "route", "inputs": Object{"report_passed": true, "evidence_human_reviewed": false}, "status": "ok", "value": "route"},
		Object{"case": "refuse", "inputs": Object{"report_passed": false, "evidence_human_reviewed": true}, "status": "ok", "value": "refuse"},
	}
	qualified := func(function any, bundle Object, cases Array, evidence Object) (string, string) {
		window := with(interval, Object{"function": function})
		source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "invented rule", "reason": "policy test", "supersedes": Array{}}))["source_ref"]
		plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": cases, "origin": "stated in the test", "supersedes": Array{}})["plan_ref"]
		body := with(window, Object{"base_snapshot": "", "source_ref": source, "bundle": bundle, "reason": "policy test"})
		if evidence != nil {
			body["evidence"] = evidence
		}
		snapshot := w.must("author", "author", "propose", body)["snapshot_ref"].(string)
		report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
		if at(report, "report", "status") != "passed" {
			t.Fatalf("%v: report %s", function, Encode(report))
		}
		return snapshot, report["report_ref"].(string)
	}
	_, policyReport := qualified("approval-policy", typedBundle(t, policy, build), policyCases, nil)
	// The policy cannot decide its own change (rule 1).
	if out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": policyReport}); at(out, "verdict", "reason") != "policy_change" {
		t.Fatalf("policy change: %s", Encode(out))
	}
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": policyReport, "reason": "a person approves the policy"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})

	// The policy decides the typed rule.
	snapshot, report := qualified(vector["function_id"], typedBundle(t, vector, build), typedPlanCases(t), evidenceCase(t, "valid-human-confirmed"))
	out := w.must("reviewer", "reviewer", "policy_decide", Object{"report_ref": report})
	verdict := out["verdict"].(Object)
	inputs := Object{"evidence_human_reviewed": true, "report_passed": true}
	if verdict["outcome"] != "accept" || verdict["reason"] != "policy_output" || at(verdict, "item", "snapshot_ref") != snapshot ||
		at(verdict, "receipt", "inputs_ref") != InputDigest(inputs) || at(verdict, "receipt", "engine_build") != build || at(verdict, "audit", "selected") != true {
		t.Fatalf("verdict %s", Encode(verdict))
	}
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": out["decision_ref"], "targets": Array{}})
	selected := must(w.host("reader", "reader", w.runner).Handle(Object{"op": "resolve", "function": vector["function_id"], "date": "2026-06-01"})).(Object)
	if selected["snapshot_ref"] != snapshot {
		t.Fatalf("resolve %s", Encode(selected))
	}
	replay := must(w.host("reviewer", "reviewer", w.runner).Handle(Object{"op": "policy_replay", "policy_verdict_ref": out["policy_verdict_ref"]})).(Object)
	if replay["replay"] != "same" {
		t.Fatalf("policy_replay %s", Encode(replay))
	}
	record := must(w.host("reviewer", "reviewer", w.runner).Handle(Object{"op": "record", "kind": policyVerdictKind, "ref": out["policy_verdict_ref"]})).(Object)
	replayed := w.must("reader", "reader", "replay", Object{"receipt_ref": record["receipt_ref"], "reason": "policy audit"})
	if at(replayed, "receipt", "matches_original") != true || at(replayed, "actual", "value") != "accept" {
		t.Fatalf("replay %s", Encode(replayed))
	}
}
