package agentstep

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// fakeCluster gives the Agent states of a test in order, then the last one
// again. The states use the field names of the koodam API v1alpha1.
type fakeCluster struct {
	creates  [][]byte
	states   []string
	reads    int
	log      []byte
	logError error
}

const uid = "6f1c2a9e-3d0b-4c56-9a41-0e7d5b8c2f13"

func (c *fakeCluster) Create(manifest []byte) ([]byte, error) {
	c.creates = append(c.creates, manifest)
	return []byte(`{"metadata":{"name":"step-abc-x7k2p","namespace":"steps","uid":"` + uid + `"}}`), nil
}

func (c *fakeCluster) Agent(namespace, name string) ([]byte, error) {
	if namespace != "steps" || name != "step-abc-x7k2p" {
		return nil, errors.New("not found")
	}
	state := c.states[min(c.reads, len(c.states)-1)]
	c.reads++
	return []byte(`{"apiVersion":"core.koodam.dev/v1alpha1","kind":"Agent","metadata":{"name":"step-abc-x7k2p","namespace":"steps","uid":"` + uid + `","resourceVersion":"9"},"spec":{"budget":"2"},"status":` + state + `}`), nil
}

func (c *fakeCluster) Output(namespace, job string) ([]byte, error) {
	if job != "step-abc-x7k2p-run" {
		return nil, errors.New("not found")
	}
	return c.log, c.logError
}

const (
	running = `{"runID":"` + uid + `","phase":"Running","job":"step-abc-x7k2p-run"}`
	// succeeded is a status without its last brace.
	succeeded = `{"runID":"` + uid + `","phase":"Succeeded","job":"step-abc-x7k2p-run","output":{"path":"/out/summary.txt","sha256":"` + summaryHex + `","size":78}`
	ended     = succeeded + `}`
	revoked   = `,"revokedAt":"2026-11-19T06:00:00Z","usage":{"spend":"0.0002","calls":1}}`
	good      = succeeded + revoked
)

func adapter(c Cluster) *Adapter {
	return &Adapter{Cluster: c, Namespace: "steps", Image: "registry.example.com/summary:v1", Command: []string{"summarize"}, Model: "mock-1",
		Budget: "2", DeadlineSeconds: 120, OutputPath: "/out/summary.txt", Inputs: map[string][]byte{"notice": []byte(notice)},
		Wait: 10 * time.Second, Settle: 3 * time.Second, Poll: time.Second, Sleep: func(time.Duration) {}}
}

// A completed run gives the bound receipt, and the replay makes no Agent.
func TestCompletedRun(t *testing.T) {
	c := &fakeCluster{states: []string{`{}`, running, ended, good}, log: []byte(summary)}
	w := newWorkflow(t)
	res := w.seam(adapter(c))
	if res.Outcome.RunID != uid || len(c.creates) != 1 {
		t.Fatalf("run %q, %d Agents", res.Outcome.RunID, len(c.creates))
	}
	var manifest struct {
		APIVersion, Kind string
		Metadata         struct{ Namespace, GenerateName string }
		Spec             struct {
			Image, Model, Budget string
			Command              []string
			DeadlineSeconds      int64
			Env                  map[string]string
			Output               struct{ Path string }
			PodTemplate          struct {
				Spec struct {
					Volumes    []struct{ Name string }
					Containers []struct {
						Name         string
						VolumeMounts []struct{ Name, MountPath string }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(c.creates[0], &manifest); err != nil {
		t.Fatal(err)
	}
	s := manifest.Spec
	if manifest.APIVersion != "core.koodam.dev/v1alpha1" || manifest.Kind != "Agent" || manifest.Metadata.Namespace != "steps" ||
		!strings.HasPrefix(manifest.Metadata.GenerateName, "step-") || s.Image != "registry.example.com/summary:v1" || s.Model != "mock-1" ||
		s.Budget != "2" || s.DeadlineSeconds != 120 || s.Output.Path != "/out/summary.txt" || len(s.Command) != 1 {
		t.Fatalf("manifest %s", c.creates[0])
	}
	if s.Env["ENTELOS_INPUT_NOTICE"] != notice || "sha256:"+s.Env["ENTELOS_INPUT_NOTICE_SHA256"] != noticeDigest ||
		s.Env["ENTELOS_STEP_REQUEST_REF"] != res.Outcome.RequestRef || s.Env["ENTELOS_OUTPUT_PATH"] != "/out/summary.txt" {
		t.Fatalf("env %v", s.Env)
	}
	pod := s.PodTemplate.Spec
	if len(pod.Volumes) != 1 || len(pod.Containers) != 1 || pod.Containers[0].Name != "agent" || pod.Containers[0].VolumeMounts[0].MountPath != "/out" {
		t.Fatalf("pod template %+v", pod)
	}
	// The trace is the Agent that the adapter read, without the fields of
	// the API server that change with each write.
	var traced map[string]any
	if err := json.Unmarshal(res.Trace, &traced); err != nil {
		t.Fatal(err)
	}
	if traced["status"].(map[string]any)["usage"].(map[string]any)["spend"] != "0.0002" || traced["metadata"].(map[string]any)["uid"] != uid ||
		traced["metadata"].(map[string]any)["resourceVersion"] != nil {
		t.Fatalf("trace %s", res.Trace)
	}
}

// Each status that is not a completed run with a spend below the budget
// gives a defined outcome, and the gate gets no inputs.
func TestRunThatDidNotComplete(t *testing.T) {
	head := `{"runID":"` + uid + `","job":"step-abc-x7k2p-run",`
	output := `"output":{"path":"/out/summary.txt","sha256":"` + summaryHex + `","size":78}`
	cases := []struct {
		name, state, status, code string
		log                       string
		logError                  error
	}{
		{"failed run", head + `"phase":"Failed","output":{"path":"/out/summary.txt","reason":"NotFound"}` + revoked, lifecycle.StepFailed, "step_failed", "", nil},
		{"failed run with an output file", head + `"phase":"Failed",` + output + revoked, lifecycle.StepFailed, "step_failed", summary, nil},
		{"budget stop of a failed run", head + `"phase":"Failed","revokedAt":"2026-11-19T06:00:00Z","usage":{"spend":"2.0001"}}`, lifecycle.StepBudgetStopped, "step_budget_stopped", "", nil},
		{"budget stop of a run that ended with success", head + `"phase":"Succeeded",` + output + `,"revokedAt":"2026-11-19T06:00:00Z","usage":{"spend":"2"}}`, lifecycle.StepBudgetStopped, "step_budget_stopped", summary, nil},
		{"run that does not end", running, lifecycle.StepFailed, "step_failed", "", nil},
		{"no revoke", ended, lifecycle.StepFailed, "step_failed", summary, nil},
		{"no usage", succeeded + `,"revokedAt":"2026-11-19T06:00:00Z"}`, lifecycle.StepFailed, "step_failed", summary, nil},
		{"spend that is not a number", succeeded + `,"revokedAt":"2026-11-19T06:00:00Z","usage":{"spend":"1e-3"}}`, lifecycle.StepFailed, "step_failed", summary, nil},
		{"no run identity", `{"phase":"Succeeded","job":"step-abc-x7k2p-run",` + output + revoked, lifecycle.StepFailed, "step_failed", summary, nil},
		{"no output digest", head + `"phase":"Succeeded"` + revoked, lifecycle.StepFailed, "step_failed", summary, nil},
		{"output reason", head + `"phase":"Succeeded","output":{"path":"/out/summary.txt","reason":"NotRegularFile"}` + revoked, lifecycle.StepFailed, "step_failed", summary, nil},
		{"output reason that is not a word", head + `"phase":"Succeeded","output":{"reason":"line 1\nline 2"}` + revoked, lifecycle.StepFailed, "step_failed", summary, nil},
		{"other output bytes", good, lifecycle.StepFailed, "step_failed", "Another text of the same run.\n", nil},
		{"output that is not readable", good, lifecycle.StepFailed, "step_failed", "", errors.New("no pod")},
	}
	w := newWorkflow(t)
	for _, tc := range cases {
		c := &fakeCluster{states: []string{tc.state}, log: []byte(tc.log), logError: tc.logError}
		res, err := adapter(c).RunStep(request)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if err := lifecycle.CheckStepResult(request, res); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		t.Logf("%s: %s, %s", tc.name, res.Outcome.Status, res.Outcome.Reason)
		w.refuse(res, tc.status, tc.code)
	}
}

// The adapter makes no Agent if it does not have the bytes of the request.
func TestInputsMustHaveTheRequestDigests(t *testing.T) {
	for name, change := range map[string]func(*Adapter){
		"other bytes": func(a *Adapter) { a.Inputs = map[string][]byte{"notice": []byte("another notice\n")} },
		"no bytes":    func(a *Adapter) { a.Inputs = nil },
		"bad budget":  func(a *Adapter) { a.Budget = "500m" },
		"bad path":    func(a *Adapter) { a.OutputPath = "/out/../etc/summary.txt" },
	} {
		c := &fakeCluster{states: []string{good}, log: []byte(summary)}
		a := adapter(c)
		change(a)
		if _, err := a.RunStep(request); err == nil || len(c.creates) != 0 {
			t.Errorf("%s: error %v, %d Agents", name, err, len(c.creates))
		}
	}
}

// v030Cluster has the Agent of koodam v0.3.0: the CRD refuses spec.output,
// and the status has no runID and no output.
type v030Cluster struct{ fakeCluster }

func (c *v030Cluster) Create(manifest []byte) ([]byte, error) {
	if strings.Contains(string(manifest), `"output"`) {
		return nil, errors.New(`strict decoding error: unknown field "spec.output"`)
	}
	return c.fakeCluster.Create(manifest)
}

// The stand-in gives the fields of koodam#296 on a v0.3.0 status.
func TestStandIn296(t *testing.T) {
	old := `{"phase":"Succeeded","job":"step-abc-x7k2p-run"` + revoked
	c := &v030Cluster{fakeCluster{states: []string{`{"phase":"Running","job":"step-abc-x7k2p-run"}`, old}, log: []byte(summary)}}
	if _, err := adapter(c).RunStep(request); err == nil {
		t.Fatal("the v0.3.0 CRD accepted spec.output")
	}
	w := newWorkflow(t)
	res := w.seam(adapter(&StandIn296{Cluster: c}))
	if res.Outcome.RunID != uid || len(c.creates) != 1 {
		t.Fatalf("run %q, %d Agents", res.Outcome.RunID, len(c.creates))
	}
	failed := &v030Cluster{fakeCluster{states: []string{`{"phase":"Failed","job":"step-abc-x7k2p-run"` + revoked}}}
	bad, err := adapter(&StandIn296{Cluster: failed}).RunStep(request)
	if err != nil {
		t.Fatal(err)
	}
	w.refuse(bad, lifecycle.StepFailed, "step_failed")
}
