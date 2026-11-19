package lifecycle

import (
	"strconv"
	"testing"
)

// The sample notice and its summary are invented. The digests are from
// sha256sum of the two texts, not from ByteDigest.
const (
	stepNotice        = "Notice 14: The Elm Street branch library closes at 17:00 on 24 December 2026. The return boxes stay open. A loan that is due on that day moves to 28 December 2026.\n"
	stepNoticeDigest  = "sha256:1a9c453fd5043166346e9cf4a9636ff793351306be7b36fe7a06c3ca5b045343"
	stepSummary       = "Elm Street branch closes at 17:00 on 24 December 2026. A loan due that day moves to 28 December 2026.\n"
	stepSummaryDigest = "sha256:e02fec428261c1d330ddafc575a7d3aed568faadce7faeca5150acd8310a8dbf"
)

// fixtureStepRunner gives the result that the test states. It counts its
// runs, and each run has a new identity.
type fixtureStepRunner struct {
	runs           int
	status, reason string
	output         []byte
}

func (r *fixtureStepRunner) RunStep(req StepRequest) (StepResult, error) {
	ref, err := StepRequestRef(req)
	if err != nil {
		return StepResult{}, err
	}
	r.runs++
	trace := []byte(`{"run":` + strconv.Itoa(r.runs) + `,"status":"` + r.status + `"}`)
	res := StepResult{Trace: trace, Outcome: StepOutcome{Status: r.status, RequestRef: ref,
		RunID: "run-" + strconv.Itoa(r.runs), TraceDigest: ByteDigest(trace), Reason: r.reason}}
	if r.status == StepCompleted {
		res.Output = r.output
		res.Outcome.OutputDigest, res.Outcome.OutputSize = ByteDigest(r.output), int64(len(r.output))
	}
	return res, nil
}

// stepGateEngine is a Go fake of a gate rule. The rule accepts a summary of
// 1 to 120 bytes.
type stepGateEngine struct{ fakeEngine }

func (e *stepGateEngine) Execute(files map[string][]byte, command string, inputs []byte, cases any) (any, error) {
	out, err := e.fakeEngine.Execute(files, command, inputs, cases)
	if err != nil || command != "run" {
		return out, err
	}
	decoded, err := Decode(inputs)
	if err != nil {
		return nil, err
	}
	summary, _ := decoded.(Object)["step_output"].(string)
	out.(Object)["result"] = len(summary) > 0 && len(summary) <= 120
	return out, nil
}

var stepRequest = StepRequest{Step: "notice.summary", Inputs: map[string]string{"notice": stepNoticeDigest}}

// activeGate activates the gate rule and gives its bundle reference.
func activeGate(t *testing.T) (*world, string) {
	t.Helper()
	passing := bundleRefOf("gate")
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed"}})
	w.engine = &stepGateEngine{fakeEngine{build: engineBuild}}
	snapshot, plan, _ := w.candidate("gate")
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "gate"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})
	w.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
	return w, passing
}

// The seam test of #113. One run gives the summary. The receipt binds the
// run identity and the digests, and a replay uses the bound output with no
// new run.
func TestStepSeam(t *testing.T) {
	if ByteDigest([]byte(stepNotice)) != stepNoticeDigest {
		t.Fatal("ByteDigest differs from sha256sum for the notice")
	}
	w, gate := activeGate(t)
	runner := &fixtureStepRunner{status: StepCompleted, output: []byte(stepSummary)}
	res, err := runner.RunStep(stepRequest)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := StepDecisionInputs(stepRequest, res)
	if err != nil {
		t.Fatal(err)
	}
	requestRef := InputDigest(Object{"step": "notice.summary", "inputs": Object{"notice": stepNoticeDigest}})
	want := Object{"step": "notice.summary", "step_request_ref": requestRef, "step_run_id": "run-1",
		"step_output": stepSummary, "step_output_digest": stepSummaryDigest, "step_output_size": "102",
		"step_trace_digest": ByteDigest([]byte(`{"run":1,"status":"completed"}`))}
	if !equal(inputs, want) {
		t.Fatalf("inputs %v, want %v", inputs, want)
	}
	answer := w.must("caller", "caller", "invoke", Object{"function": "record.visible", "date": "2026-11-15",
		"expected_bundle_ref": gate, "inputs": inputs})
	receipt := answer["receipt"].(Object)
	if answer["actual"].(Object)["result"] != true || receipt["inputs_ref"] != InputDigest(want) {
		t.Fatalf("answer %v", answer)
	}
	replay := w.must("reader", "reader", "replay", Object{"receipt_ref": answer["receipt_ref"], "reason": "audit"})
	again := replay["receipt"].(Object)
	if again["inputs_ref"] != receipt["inputs_ref"] || again["matches_original"] != true || again["previous_receipt"] != answer["receipt_ref"] {
		t.Fatalf("replay %v", again)
	}
	if runner.runs != 1 {
		t.Fatalf("%d runs, want 1: a replay must not run the step", runner.runs)
	}
}

// The negative test of #113. A failed run and a budget stop give a fault and
// no inputs. The store keeps its root, so no receipt answers for the run.
func TestStepFailureCannotProduceAnApproval(t *testing.T) {
	w, _ := activeGate(t)
	before, err := w.store.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	for status, code := range map[string]string{StepFailed: "step_failed", StepBudgetStopped: "step_budget_stopped"} {
		runner := &fixtureStepRunner{status: status, reason: "exit 1"}
		res, err := runner.RunStep(stepRequest)
		if err != nil {
			t.Fatal(err)
		}
		inputs, err := StepDecisionInputs(stepRequest, res)
		if inputs != nil || faultCode(t, err) != code {
			t.Fatalf("%s: inputs %v, error %v", status, inputs, err)
		}
		// The same outcome with the completed label has no output digest.
		res.Outcome.Status, res.Outcome.Reason = StepCompleted, ""
		if _, err := StepDecisionInputs(stepRequest, res); faultCode(t, err) != "invalid_step_outcome" {
			t.Fatalf("%s with a completed label: %v", status, err)
		}
	}
	after, err := w.store.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Cursor != before.Cursor {
		t.Fatal("a failed step changed the store")
	}
}

// CheckStepResult refuses a result that does not agree with its own bytes or
// with its request.
func TestStepResultChecks(t *testing.T) {
	good := func() StepResult {
		res, err := (&fixtureStepRunner{status: StepCompleted, output: []byte(stepSummary)}).RunStep(stepRequest)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if err := CheckStepResult(stepRequest, good()); err != nil {
		t.Fatal(err)
	}
	failed := func() StepResult {
		res, _ := (&fixtureStepRunner{status: StepFailed, reason: "exit 1"}).RunStep(stepRequest)
		return res
	}
	cases := map[string]func() (StepRequest, StepResult){
		"changed output bytes": func() (StepRequest, StepResult) {
			r := good()
			r.Output = []byte("other text\n")
			return stepRequest, r
		},
		"changed output size": func() (StepRequest, StepResult) { r := good(); r.Outcome.OutputSize++; return stepRequest, r },
		"changed trace bytes": func() (StepRequest, StepResult) { r := good(); r.Trace = []byte("{}"); return stepRequest, r },
		"trace without a digest": func() (StepRequest, StepResult) {
			r := failed()
			r.Outcome.TraceDigest = ""
			return stepRequest, r
		},
		"no run identity":       func() (StepRequest, StepResult) { r := good(); r.Outcome.RunID = ""; return stepRequest, r },
		"bad run identity":      func() (StepRequest, StepResult) { r := good(); r.Outcome.RunID = "run 1\n"; return stepRequest, r },
		"completed with reason": func() (StepRequest, StepResult) { r := good(); r.Outcome.Reason = "x"; return stepRequest, r },
		"unknown status":        func() (StepRequest, StepResult) { r := good(); r.Outcome.Status = "approved"; return stepRequest, r },
		"failed with an output": func() (StepRequest, StepResult) {
			r := failed()
			r.Output = []byte(stepSummary)
			return stepRequest, r
		},
		"failed with an output digest": func() (StepRequest, StepResult) {
			r := failed()
			r.Outcome.OutputDigest = stepSummaryDigest
			return stepRequest, r
		},
		"failed without a reason": func() (StepRequest, StepResult) { r := failed(); r.Outcome.Reason = ""; return stepRequest, r },
		"other request": func() (StepRequest, StepResult) {
			return StepRequest{Step: "notice.summary", Inputs: map[string]string{"notice": stepSummaryDigest}}, good()
		},
	}
	for name, make := range cases {
		req, res := make()
		if err := CheckStepResult(req, res); err == nil || faultCode(t, err) != "invalid_step_outcome" {
			t.Errorf("%s: got %v, want invalid_step_outcome", name, err)
		}
	}
	for name, req := range map[string]StepRequest{
		"no step name":   {Inputs: map[string]string{"notice": stepNoticeDigest}},
		"bad input name": {Step: "notice.summary", Inputs: map[string]string{"Notice 1": stepNoticeDigest}},
		"bad digest":     {Step: "notice.summary", Inputs: map[string]string{"notice": "sha256:abc"}},
	} {
		if _, err := StepRequestRef(req); err == nil || faultCode(t, err) != "invalid_step_request" {
			t.Errorf("%s: got %v, want invalid_step_request", name, err)
		}
	}
	binary := &fixtureStepRunner{status: StepCompleted, output: []byte{0xff, 0xfe}}
	res, _ := binary.RunStep(stepRequest)
	if _, err := StepDecisionInputs(stepRequest, res); faultCode(t, err) != "step_output_not_text" {
		t.Errorf("binary output: %v", err)
	}
}
