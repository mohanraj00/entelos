package agentstep

import (
	"os"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// The synthetic agent checks the digest of its input and calls the mock
// model of the gateway 1 time, so that the run has a spend record. Then it
// writes the first sentence of the notice as the summary. The mock answer is
// not the summary: no model writes the output of this test.
const (
	callModel = `curl -sf -o /dev/null -H "Authorization: Bearer $OPENAI_API_KEY" -H 'Content-Type: application/json' ` +
		`"$OPENAI_BASE_URL/chat/completions" -d '{"model":"mock-1","messages":[{"role":"user","content":"Summarize the notice."}]}'`
	summarize = `set -e
test "$(printf '%s' "$ENTELOS_INPUT_NOTICE" | sha256sum | cut -d' ' -f1)" = "$ENTELOS_INPUT_NOTICE_SHA256"
` + callModel + `
printf '%s' "$ENTELOS_INPUT_NOTICE" | sed -n '1s/\. .*/./p' | tee "$ENTELOS_OUTPUT_PATH"
`
)

// kindAdapter gives an adapter for the cluster of ENTELOS_KIND_KUBECONFIG.
// kind/seam.sh sets the variables. Without them, the test does not run.
func kindAdapter(t *testing.T, script, budget string) *Adapter {
	t.Helper()
	kubeconfig, image, namespace := os.Getenv("ENTELOS_KIND_KUBECONFIG"), os.Getenv("ENTELOS_AGENT_IMAGE"), os.Getenv("ENTELOS_KIND_NAMESPACE")
	if kubeconfig == "" || image == "" || namespace == "" {
		t.Skip("kind/seam.sh runs this test on a kind cluster")
	}
	var cluster Cluster = Kubectl{Kubeconfig: kubeconfig}
	if os.Getenv("ENTELOS_KOODAM_STANDIN_296") == "1" {
		cluster = &StandIn296{Cluster: cluster}
	}
	return &Adapter{Cluster: cluster, Namespace: namespace, Image: image, Command: []string{"sh", "-c", script}, Model: "mock-1",
		Budget: budget, DeadlineSeconds: 120, OutputPath: "/out/summary.txt", Inputs: map[string][]byte{"notice": []byte(notice)},
		Wait: 5 * time.Minute, Settle: 2 * time.Minute, Poll: 2 * time.Second}
}

// The seam test of #113 on a cluster: one Agent run gives the summary, the
// receipt binds the run, and the replay makes no Agent.
func TestKindSeam(t *testing.T) {
	a := kindAdapter(t, summarize, "2")
	w := newWorkflow(t)
	res := w.seam(a)
	t.Logf("run identity %s, output %s, %d bytes, trace %s", res.Outcome.RunID, res.Outcome.OutputDigest, res.Outcome.OutputSize, res.Outcome.TraceDigest)
	t.Logf("trace: %s", res.Trace)
}

// The negative test of #113 on a cluster: a failed Agent run gives the
// outcome "failed", and the gate gets no inputs.
func TestKindFailedRun(t *testing.T) {
	a := kindAdapter(t, "echo 'The agent stops with an error.'; exit 1", "2")
	res, err := a.RunStep(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("run identity %s: %s, %s", res.Outcome.RunID, res.Outcome.Status, res.Outcome.Reason)
	newWorkflow(t).refuse(res, lifecycle.StepFailed, "step_failed")
}

// A run that spends its budget gives the outcome "budget_stopped", also if
// its process ends with success. One call of the mock model costs more than
// this budget.
func TestKindBudgetStop(t *testing.T) {
	a := kindAdapter(t, summarize, "0.00001")
	res, err := a.RunStep(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("run identity %s: %s, %s", res.Outcome.RunID, res.Outcome.Status, res.Outcome.Reason)
	newWorkflow(t).refuse(res, lifecycle.StepBudgetStopped, "step_budget_stopped")
}
