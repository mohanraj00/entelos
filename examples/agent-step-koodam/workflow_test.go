package agentstep

import (
	"strconv"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// The sample notice and its summary are invented. The digests are from
// sha256sum of the two texts, not from the code under test.
const (
	notice        = "Notice 14: The Elm Street branch library closes at 17:00 on 24 December 2026. The return boxes stay open. A loan that is due on that day moves to 28 December 2026.\n"
	noticeDigest  = "sha256:1a9c453fd5043166346e9cf4a9636ff793351306be7b36fe7a06c3ca5b045343"
	summary       = "Notice 14: The Elm Street branch library closes at 17:00 on 24 December 2026.\n"
	summaryHex    = "a26e5b2c30d8882bdfdaf490e55edbcfcd4d59f4edb5ca76a0cac3c1ea5b64e1"
	summaryDigest = "sha256:" + summaryHex
	summarySize   = 78
	gateFunction  = "notice.summary.gate"
)

var request = lifecycle.StepRequest{Step: "notice.summary", Inputs: map[string]string{"notice": noticeDigest}}

// gateRuntime is a frozen Go function in place of a CEL rule. It accepts a
// summary of 1 to 160 bytes. It is not the CEL runtime.
type gateRuntime struct{}

var gateBuild = lifecycle.ContentRef(lifecycle.Envelope(lifecycle.Media("fixture-engine"), []byte("notice.summary.gate/1")))

func (gateRuntime) Build() (string, error) { return gateBuild, nil }

func (gateRuntime) Execute(files map[string][]byte, command string, inputs []byte, cases any) (any, error) {
	out := lifecycle.Object{"bundle_ref": lifecycle.ContentRef(lifecycle.Envelope(lifecycle.Mimes["manifest"], files["manifest"])), "implementation_ref": gateBuild}
	switch command {
	case "describe":
		out["function_id"] = gateFunction
	case "run":
		decoded, err := lifecycle.Decode(inputs)
		if err != nil {
			return nil, err
		}
		text, _ := decoded.(lifecycle.Object)["step_output"].(string)
		out["status"], out["result"] = "ok", len(text) > 0 && len(text) <= 160
	default:
		return nil, lifecycle.NewFault("unexpected_command", "the gate does not run "+command, "runtime")
	}
	return out, nil
}

// tableRunner is a fixture qualification runner. It gives the verdict
// "passed" for each bundle and runs no rule.
type tableRunner struct{}

var tableIdentity = lifecycle.ContentRef(lifecycle.Envelope(lifecycle.Media("fixture-runner"), []byte("table")))

func (tableRunner) Identity() (string, error)           { return tableIdentity, nil }
func (tableRunner) Expected(lifecycle.Candidate) string { return tableIdentity }
func (tableRunner) Qualify(in lifecycle.QualificationInput) (lifecycle.Verdict, error) {
	return lifecycle.Verdict{Outcome: lifecycle.Object{"status": "passed", "bundle_ref": in.Candidate.BundleRef, "implementation_ref": tableIdentity}}, nil
}

// workflow is one memory store with the gate activated.
type workflow struct {
	t     *testing.T
	store *lifecycle.MemoryStore
	host  *lifecycle.Host
	keys  int
	gate  any
}

func (w *workflow) cursor() string {
	w.t.Helper()
	root, err := w.store.Read(nil)
	if err != nil {
		w.t.Fatal(err)
	}
	return root.Cursor
}

func (w *workflow) do(actor, role, op string, fields lifecycle.Object) (lifecycle.Object, error) {
	w.t.Helper()
	w.keys++
	q := lifecycle.Object{"op": op, "expected_revision": w.cursor(), "request_key": op + "-" + strconv.Itoa(w.keys)}
	for k, v := range fields {
		q[k] = v
	}
	out, err := w.host.As(actor, []string{role}).Handle(q)
	if err != nil {
		return nil, err
	}
	return out.(lifecycle.Object)["result"].(lifecycle.Object), nil
}

func (w *workflow) must(actor, role, op string, fields lifecycle.Object) lifecycle.Object {
	w.t.Helper()
	out, err := w.do(actor, role, op, fields)
	if err != nil {
		w.t.Fatalf("%s: %v", op, err)
	}
	return out
}

func newWorkflow(t *testing.T) *workflow {
	t.Helper()
	store := lifecycle.NewMemoryStore()
	host, err := lifecycle.NewHostWithRunner(store, gateRuntime{}, tableRunner{}, lifecycle.Object{"scope": "sample-library",
		"runner_actor": "runner", "worker": "worker-1", "functions": lifecycle.Array{gateFunction}})
	if err != nil {
		t.Fatal(err)
	}
	w := &workflow{t: t, store: store, host: host}
	window := lifecycle.Object{"function": gateFunction, "from": "2026-11-01", "until": "2027-01-01"}
	with := func(extra lifecycle.Object) lifecycle.Object {
		for k, v := range window {
			extra[k] = v
		}
		return extra
	}
	source := w.must("reviewer", "reviewer", "source", with(lifecycle.Object{"text": "A summary has 1 to 160 bytes.", "reason": "first", "supersedes": lifecycle.Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", lifecycle.Object{"source_ref": source, "cases": lifecycle.Array{lifecycle.Object{"name": "one"}}, "origin": "independent", "supersedes": lifecycle.Array{}})["plan_ref"]
	// The source text is a note. gateRuntime holds the rule.
	rule := "A summary has 1 to 160 bytes. A frozen Go function applies this rule."
	proposal := w.must("author", "author", "propose", with(lifecycle.Object{"base_snapshot": "", "source_ref": source, "reason": "first",
		"bundle": lifecycle.Object{"manifest": `{"caller_variables":[],"environment_ref":"env","limits":{},"profile":"p","rule":"gate"}`, "environment": "{}", "source": rule, "fixed": "{}"}}))
	report := w.must("runner", "runner", "qualify", lifecycle.Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})["report_ref"]
	decision := w.must("reviewer", "reviewer", "decide", lifecycle.Object{"report_ref": report, "reason": "gate"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", lifecycle.Object{"decision_ref": decision, "targets": lifecycle.Array{}})
	w.must("worker", "worker", "load", lifecycle.Object{"function": gateFunction, "date": "2026-11-19"})
	w.gate = proposal["bundle_ref"]
	return w
}

// decide sends a step result to the gate and gives the invoke result.
func (w *workflow) decide(res lifecycle.StepResult) (lifecycle.Object, error) {
	w.t.Helper()
	inputs, err := lifecycle.StepDecisionInputs(request, res)
	if err != nil {
		return nil, err
	}
	return w.do("caller", "caller", "invoke", lifecycle.Object{"function": gateFunction, "date": "2026-11-19", "expected_bundle_ref": w.gate, "inputs": inputs})
}

// seam runs one step and checks the receipt and the replay of clause S9.
// It gives the result of the step.
func (w *workflow) seam(runner lifecycle.StepRunner) lifecycle.StepResult {
	w.t.Helper()
	res, err := runner.RunStep(request)
	if err != nil {
		w.t.Fatal(err)
	}
	o := res.Outcome
	if o.Status != lifecycle.StepCompleted || o.OutputDigest != summaryDigest || o.OutputSize != summarySize || string(res.Output) != summary || o.RunID == "" {
		w.t.Fatalf("outcome %+v, output %q", o, res.Output)
	}
	answer, err := w.decide(res)
	if err != nil {
		w.t.Fatal(err)
	}
	receipt := answer["receipt"].(lifecycle.Object)
	want := lifecycle.InputDigest(lifecycle.Object{"step": "notice.summary", "step_request_ref": o.RequestRef, "step_run_id": o.RunID,
		"step_output": summary, "step_output_digest": summaryDigest, "step_output_size": "78", "step_trace_digest": o.TraceDigest})
	if answer["actual"].(lifecycle.Object)["result"] != true || receipt["inputs_ref"] != want {
		w.t.Fatalf("answer %v, want inputs_ref %s", answer, want)
	}
	replay := w.must("reader", "reader", "replay", lifecycle.Object{"receipt_ref": answer["receipt_ref"], "reason": "audit"})["receipt"].(lifecycle.Object)
	if replay["inputs_ref"] != want || replay["matches_original"] != true {
		w.t.Fatalf("replay %v", replay)
	}
	return res
}

// refuse checks that a result cannot reach the gate and that the store
// keeps its root.
func (w *workflow) refuse(res lifecycle.StepResult, status, code string) {
	w.t.Helper()
	before := w.cursor()
	if res.Outcome.Status != status || res.Outcome.OutputDigest != "" || len(res.Output) != 0 {
		w.t.Fatalf("outcome %+v, want %s with no output", res.Outcome, status)
	}
	answer, err := w.decide(res)
	fault, ok := err.(*lifecycle.Fault)
	if answer != nil || !ok || fault.Code != code {
		w.t.Fatalf("answer %v, error %v, want %s", answer, err, code)
	}
	if w.cursor() != before {
		w.t.Fatal("a run that did not complete changed the store")
	}
}
