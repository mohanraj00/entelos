// Package agentstep is an optional adapter from the step outcome contract of
// the lifecycle core to a koodam Agent run. CONTRACT.md has the clauses.
package agentstep

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// Cluster is the access of the adapter to the Kubernetes API. Kubectl is the
// implementation for a real cluster.
type Cluster interface {
	// Create makes one object from a JSON manifest and gives the JSON of
	// the object that the API server stored.
	Create(manifest []byte) ([]byte, error)
	// Agent gives the JSON of one Agent.
	Agent(namespace, name string) ([]byte, error)
	// Output gives the bytes that the agent container of a run Job wrote
	// to its standard output.
	Output(namespace, job string) ([]byte, error)
}

// Adapter runs each step as one new Agent. It is a lifecycle.StepRunner.
type Adapter struct {
	Cluster   Cluster
	Namespace string
	// Image, Command and Model are the spec fields of the Agent.
	Image   string
	Command []string
	Model   string
	// Budget is the spec.budget of the Agent, as a plain decimal number.
	Budget string
	// DeadlineSeconds is the spec.deadlineSeconds of the Agent.
	DeadlineSeconds int64
	// OutputPath is the spec.output.path of the Agent. The command writes
	// the output to this file and also to its standard output.
	OutputPath string
	// Inputs has the bytes of each input of a request.
	Inputs map[string][]byte
	// Wait is the maximum time for the run, and Settle is the maximum time
	// from the end of the run to the revoke of its key.
	Wait, Settle time.Duration
	// Poll is the time between two reads of the Agent.
	Poll time.Duration
	// Sleep is time.Sleep if it is nil, and Now is time.Now if it is nil.
	Sleep func(time.Duration)
	Now   func() time.Time
}

// The limits of the env transport of clause K3.
const (
	maxInputBytes = 32 << 10
	maxOutput     = 64 << 10
)

var (
	budgetPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	hexDigest     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	outputReason  = regexp.MustCompile(`^[A-Za-z]{1,64}$`)
)

// envName gives the env variable of an input name: "notice.v1" is
// ENTELOS_INPUT_NOTICE_V1.
func envName(input string) string {
	return "ENTELOS_INPUT_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(input))
}

// manifest gives the Agent of one request (clauses K2 to K4).
func (a *Adapter) manifest(req lifecycle.StepRequest, requestRef string) ([]byte, error) {
	if !budgetPattern.MatchString(a.Budget) || !path.IsAbs(a.OutputPath) || path.Clean(a.OutputPath) != a.OutputPath || path.Dir(a.OutputPath) == "/" {
		return nil, errors.New("the adapter needs a decimal budget and an absolute output path in a directory")
	}
	if a.Poll <= 0 || a.Wait <= 0 || a.Settle < 0 {
		return nil, errors.New("the adapter needs a poll time and a wait time that are more than zero")
	}
	env := map[string]string{"ENTELOS_STEP": req.Step, "ENTELOS_STEP_REQUEST_REF": requestRef, "ENTELOS_OUTPUT_PATH": a.OutputPath}
	names := make([]string, 0, len(req.Inputs))
	for name := range req.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		digest := req.Inputs[name]
		raw, ok := a.Inputs[name]
		if !ok || lifecycle.ByteDigest(raw) != digest {
			return nil, fmt.Errorf("the bytes of input %s do not have the digest of the request", name)
		}
		if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
			return nil, fmt.Errorf("input %s is not text", name)
		}
		// Each input has two variables. No variable can have two sources:
		// the inputs "a" and "a_sha256" are refused, and also "a.b" and "a-b".
		key := envName(name)
		for _, variable := range []string{key, key + "_SHA256"} {
			if _, used := env[variable]; used {
				return nil, fmt.Errorf("two inputs give the env variable %s", variable)
			}
		}
		total += len(raw)
		env[key], env[key+"_SHA256"] = string(raw), strings.TrimPrefix(digest, "sha256:")
	}
	if total > maxInputBytes {
		return nil, errors.New("the inputs are too large for env variables")
	}
	spec := map[string]any{
		"image": a.Image, "command": a.Command, "model": a.Model, "budget": a.Budget, "env": env,
		"output": map[string]any{"path": a.OutputPath},
		"podTemplate": map[string]any{"spec": map[string]any{
			"volumes": []any{map[string]any{"name": "step-output", "emptyDir": map[string]any{}}},
			"containers": []any{map[string]any{"name": "agent",
				"volumeMounts": []any{map[string]any{"name": "step-output", "mountPath": path.Dir(a.OutputPath)}}}},
		}},
	}
	if a.DeadlineSeconds > 0 {
		spec["deadlineSeconds"] = a.DeadlineSeconds
	}
	return json.Marshal(map[string]any{
		"apiVersion": "core.koodam.dev/v1alpha1", "kind": "Agent",
		"metadata": map[string]any{"namespace": a.Namespace, "generateName": "step-" + requestRef[len("sha256:"):len("sha256:")+12] + "-"},
		"spec":     spec,
	})
}

// agent is the part of an Agent that the adapter reads. The field names are
// the names of the koodam API v1alpha1.
type agent struct {
	Metadata struct {
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"metadata"`
	Status struct {
		RunID  string `json:"runID"`
		Phase  string `json:"phase"`
		Job    string `json:"job"`
		Output *struct {
			SHA256 string `json:"sha256"`
			Size   *int64 `json:"size"`
			Reason string `json:"reason"`
		} `json:"output"`
		RevokedAt string `json:"revokedAt"`
		Usage     *struct {
			Spend string `json:"spend"`
		} `json:"usage"`
	} `json:"status"`
}

func (g *agent) ended() bool { return g.Status.Phase == "Succeeded" || g.Status.Phase == "Failed" }

// trace gives the trace bytes of clause K8: the identity, the spec and the
// status of the Agent, as JSON with sorted keys.
func trace(raw []byte) ([]byte, error) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	metadata, _ := object["metadata"].(map[string]any)
	return json.Marshal(map[string]any{"apiVersion": object["apiVersion"], "kind": object["kind"],
		"metadata": map[string]any{"namespace": metadata["namespace"], "name": metadata["name"], "uid": metadata["uid"]},
		"spec":     object["spec"], "status": object["status"]})
}

// RunStep creates one Agent, waits for the end of the run and maps the
// status to a step result. An error means that the adapter has no outcome.
func (a *Adapter) RunStep(req lifecycle.StepRequest) (lifecycle.StepResult, error) {
	none := lifecycle.StepResult{}
	requestRef, err := lifecycle.StepRequestRef(req)
	if err != nil {
		return none, err
	}
	manifest, err := a.manifest(req, requestRef)
	if err != nil {
		return none, err
	}
	created, err := a.Cluster.Create(manifest)
	if err != nil {
		return none, fmt.Errorf("create the Agent: %w", err)
	}
	var g agent
	if err := json.Unmarshal(created, &g); err != nil || g.Metadata.Name == "" {
		return none, errors.New("the API server gave no Agent name")
	}
	name, sleep, now := g.Metadata.Name, a.Sleep, a.Now
	if sleep == nil {
		sleep = time.Sleep
	}
	if now == nil {
		now = time.Now
	}
	// Clause K5: wait for the end of the run, then for the revoke. The
	// two limits are elapsed time, so a slow read also uses them.
	var raw []byte
	deadline, settling := now().Add(a.Wait), false
	for {
		if raw, err = a.Cluster.Agent(a.Namespace, name); err != nil {
			return none, fmt.Errorf("read the Agent %s: %w", name, err)
		}
		g = agent{}
		if err := json.Unmarshal(raw, &g); err != nil {
			return none, fmt.Errorf("decode the Agent %s: %w", name, err)
		}
		if g.ended() && !settling {
			deadline, settling = now().Add(a.Settle), true
		}
		if (g.ended() && g.Status.RevokedAt != "") || !now().Before(deadline) {
			break
		}
		sleep(a.Poll)
	}
	traceBytes, err := trace(raw)
	if err != nil {
		return none, err
	}
	res := lifecycle.StepResult{Trace: traceBytes, Outcome: lifecycle.StepOutcome{RequestRef: requestRef,
		RunID: g.Status.RunID, TraceDigest: lifecycle.ByteDigest(traceBytes)}}
	stop := func(status, reason string) (lifecycle.StepResult, error) {
		res.Outcome.Status, res.Outcome.Reason = status, reason
		return res, nil
	}
	// Clauses K6 and K7, in this order.
	switch {
	case !g.ended():
		return stop(lifecycle.StepFailed, "the run did not end in the wait time")
	case g.Status.RevokedAt == "" || g.Status.Usage == nil:
		return stop(lifecycle.StepFailed, "the run has no spend record")
	}
	spend, budget := new(big.Rat), new(big.Rat)
	budget.SetString(a.Budget)
	text := g.Status.Usage.Spend
	if text == "" {
		text = "0"
	}
	if !budgetPattern.MatchString(text) {
		return stop(lifecycle.StepFailed, "the spend record is not a decimal number")
	}
	spend.SetString(text)
	switch out := g.Status.Output; {
	case spend.Cmp(budget) >= 0:
		return stop(lifecycle.StepBudgetStopped, "spend "+text+" of budget "+a.Budget)
	case g.Status.Phase != "Succeeded":
		return stop(lifecycle.StepFailed, "phase "+g.Status.Phase)
	case g.Status.RunID == "":
		return stop(lifecycle.StepFailed, "the status has no run identity")
	case out == nil || out.Reason != "" || out.Size == nil || !hexDigest.MatchString(out.SHA256):
		reason := "none"
		if out != nil && outputReason.MatchString(out.Reason) {
			reason = out.Reason
		} else if out != nil && out.Reason != "" {
			reason = "unknown"
		}
		return stop(lifecycle.StepFailed, "the status has no output digest: "+reason)
	case *out.Size > maxOutput:
		return stop(lifecycle.StepFailed, "the output is too large")
	}
	// Clause K9: the status digest decides which bytes are the output.
	output, err := a.Cluster.Output(a.Namespace, g.Status.Job)
	if err != nil {
		return stop(lifecycle.StepFailed, "the output is not readable")
	}
	digest := "sha256:" + g.Status.Output.SHA256
	if lifecycle.ByteDigest(output) != digest || int64(len(output)) != *g.Status.Output.Size {
		return stop(lifecycle.StepFailed, "the output bytes differ from the status digest")
	}
	res.Output, res.Outcome.Status = output, lifecycle.StepCompleted
	res.Outcome.OutputDigest, res.Outcome.OutputSize = digest, *g.Status.Output.Size
	return res, nil
}
