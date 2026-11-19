package lifecycle

// The step outcome interface of a workflow agent step. STEP.md has the
// contract. This file has no Host operation: a step result becomes the
// inputs of an ordinary invoke, and the receipt binds them.

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"unicode/utf8"
)

// The statuses of a step outcome (STEP.md, clause S3).
const (
	StepCompleted     = "completed"
	StepFailed        = "failed"
	StepBudgetStopped = "budget_stopped"
)

var (
	stepName   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	stepRunID  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	stepReason = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._:/-]{0,255}$`)
)

// StepRequest names one agent step and gives its inputs by byte digest.
type StepRequest struct {
	// Step is the name of the step.
	Step string
	// Inputs has one byte digest for each input name.
	Inputs map[string]string
}

// StepOutcome is the report of a step runner for one request.
type StepOutcome struct {
	// Status is StepCompleted, StepFailed or StepBudgetStopped.
	Status string
	// RequestRef is the StepRequestRef of the request that the run got.
	RequestRef string
	// RunID is the identity of the run. A new run has a new identity.
	RunID string
	// OutputDigest and OutputSize are the byte digest and the byte count of
	// the output. Only a completed outcome has them.
	OutputDigest string
	OutputSize   int64
	// TraceDigest is the byte digest of the trace of the run.
	TraceDigest string
	// Reason tells why the outcome is not completed.
	Reason string
}

// StepResult is an outcome with the bytes that its digests name.
type StepResult struct {
	Outcome StepOutcome
	Output  []byte
	Trace   []byte
}

// StepRunner runs one agent step. An error means that the runner has no
// outcome: the caller has nothing to record and nothing to decide on.
type StepRunner interface {
	RunStep(StepRequest) (StepResult, error)
}

// ByteDigest is "sha256:" and the SHA-256 of the exact bytes, as 64
// lower-case hex characters. It is not a content reference: ContentRef also
// hashes a media type.
func ByteDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stepFault(code, message string) *Fault { return NewFault(code, message, "step") }

// StepRequestRef checks the request and gives the content reference of its
// canonical JSON. An outcome names its request with this value.
func StepRequestRef(req StepRequest) (string, error) {
	if !stepName.MatchString(req.Step) {
		return "", stepFault("invalid_step_request", "the step name is not valid")
	}
	inputs := Object{}
	for name, digest := range req.Inputs {
		if !stepName.MatchString(name) {
			return "", stepFault("invalid_step_request", "an input name is not valid")
		}
		if !refPattern.MatchString(digest) {
			return "", stepFault("invalid_step_request", "input "+name+" has no valid digest")
		}
		inputs[name] = digest
	}
	return InputDigest(Object{"step": req.Step, "inputs": inputs}), nil
}

// CheckStepResult applies clauses S2 to S6 of STEP.md to a result.
func CheckStepResult(req StepRequest, res StepResult) error {
	requestRef, err := StepRequestRef(req)
	if err != nil {
		return err
	}
	invalid := func(message string) error { return stepFault("invalid_step_outcome", message) }
	o := res.Outcome
	if o.RequestRef != requestRef {
		return invalid("the outcome is for a different request")
	}
	if o.RunID != "" && !stepRunID.MatchString(o.RunID) {
		return invalid("the run identity is not valid")
	}
	if o.TraceDigest == "" && len(res.Trace) != 0 {
		return invalid("the trace has no digest")
	}
	if o.TraceDigest != "" && o.TraceDigest != ByteDigest(res.Trace) {
		return invalid("the trace digest differs from the trace bytes")
	}
	switch o.Status {
	case StepCompleted:
		if o.RunID == "" || o.TraceDigest == "" || o.Reason != "" {
			return invalid("a completed outcome needs a run identity and a trace digest, and no reason")
		}
		if o.OutputDigest != ByteDigest(res.Output) || o.OutputSize != int64(len(res.Output)) {
			return invalid("the output digest or size differs from the output bytes")
		}
	case StepFailed, StepBudgetStopped:
		if o.OutputDigest != "" || o.OutputSize != 0 || len(res.Output) != 0 {
			return invalid("an outcome that is not completed has no output")
		}
		if !stepReason.MatchString(o.Reason) {
			return invalid("an outcome that is not completed needs a reason")
		}
	default:
		return invalid("the status is not known")
	}
	return nil
}

// StepDecisionInputs gives the invoke inputs that bind a completed result.
// It is the only path from a step result to a decision. A failed run or a
// budget stop gives a fault and no inputs, so no receipt can answer for it.
func StepDecisionInputs(req StepRequest, res StepResult) (Object, error) {
	if err := CheckStepResult(req, res); err != nil {
		return nil, err
	}
	o := res.Outcome
	switch o.Status {
	case StepFailed:
		return nil, stepFault("step_failed", "the run failed: "+o.Reason)
	case StepBudgetStopped:
		return nil, stepFault("step_budget_stopped", "the budget stopped the run: "+o.Reason)
	}
	if !utf8.Valid(res.Output) {
		return nil, stepFault("step_output_not_text", "the output is not UTF-8 text")
	}
	return Object{
		"step":               req.Step,
		"step_request_ref":   o.RequestRef,
		"step_run_id":        o.RunID,
		"step_output":        string(res.Output),
		"step_output_digest": o.OutputDigest,
		"step_output_size":   strconv.FormatInt(o.OutputSize, 10),
		"step_trace_digest":  o.TraceDigest,
	}, nil
}
