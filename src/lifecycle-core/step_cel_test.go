package lifecycle

import (
	"os"
	"testing"
)

func TestStepGateCEL(t *testing.T) {
	binary := os.Getenv("LIFECYCLE_CEL_BINARY")
	if binary == "" {
		if os.Getenv("LIFECYCLE_REQUIRE_SCENARIO") != "" {
			t.Fatal("LIFECYCLE_CEL_BINARY is not set")
		}
		t.Skip("LIFECYCLE_CEL_BINARY is not set")
	}
	runtime, err := NewCELRuntime(binary, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	build, err := runtime.Build()
	if err != nil {
		t.Fatal(err)
	}

	const function, date = "notice.gate", "2026-11-15"
	const completeOutput, incompleteOutput = `{"complete":true}`, `{"complete":false}`
	// SHA-256 of the two JSON texts, computed with shasum.
	const completeDigest = "sha256:ccbc6726842d83aea07933c211d212d83713fd67442f8d13bd743ad93fb89bc3"
	const incompleteDigest = "sha256:484d6acf1a08b7b0f196ad73f7f2bddfb98d5ab86e9318d63c6bb717dc8b85a4"
	callerVariables := Array{"step", "step_request_ref", "step_run_id", "step_output", "step_output_digest", "step_output_size", "step_trace_digest"}
	var variables Array
	for _, name := range callerVariables {
		variables = append(variables, Object{"name": name, "type": "string"})
	}
	callerVariables = append(callerVariables, "step_result")
	variables = append(variables,
		Object{"name": "step_result", "type": "record", "fields": Array{Object{"name": "complete", "type": "bool"}}},
		Object{"name": "known_digests", "type": "list", "items": "string"},
	)
	bundle := typedBundle(t, Object{
		"function_id": function, "profile": "cel-typed/v1", "caller_variables": callerVariables,
		"limits": Object{"max_request_bytes": 4096, "max_string_utf8_bytes": 256, "max_source_utf8_bytes": 4096,
			"parser_recursion_limit": 64, "evaluation_cost_limit": 1000, "max_list_items": 4},
		"environment": Object{"name": "notice-gate", "variables": variables, "output": Object{"type": "bool"}},
		"source":      `step_output_digest in known_digests && step_result.complete`,
		"fixed":       Object{"known_digests": Array{completeDigest, incompleteDigest}},
	}, build)

	// The qualification plan states inputs and answers independently of the step projection.
	complete := Object{"step": "notice.summary", "step_request_ref": "test-request", "step_run_id": "test-run",
		"step_output": completeOutput, "step_output_size": "17",
		"step_output_digest": completeDigest,
		"step_trace_digest":  stepNoticeDigest, "step_result": Object{"complete": true}}
	incomplete := with(complete, Object{"step_output": incompleteOutput, "step_output_size": "18",
		"step_output_digest": incompleteDigest,
		"step_result":        Object{"complete": false}})
	cases := Array{
		Object{"case": "complete", "inputs": complete, "status": "ok", "value": true},
		Object{"case": "incomplete", "inputs": incomplete, "status": "ok", "value": false},
		Object{"case": "unknown digest", "inputs": with(complete, Object{"step_output_digest": stepNoticeDigest}), "status": "ok", "value": false},
	}

	w := newWorld(t, &CELRunner{Runtime: runtime})
	w.engine, w.functions = runtime, Array{function}
	target := Object{"function": function, "from": "2026-11-01", "until": "2027-01-01"}
	source := w.must("reviewer", "reviewer", "source", with(target, Object{
		"text": "Require a known output digest and a complete summary.", "reason": "notice gate", "supersedes": Array{},
	}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": cases,
		"origin": "hand-written gate cases", "supersedes": Array{}})["plan_ref"]
	proposal := w.must("author", "author", "propose", with(target, Object{
		"base_snapshot": "", "source_ref": source, "bundle": bundle, "reason": "notice gate",
	}))
	qualification := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})
	if at(qualification, "report", "status") != "passed" {
		t.Fatalf("gate qualification: %s", Encode(qualification))
	}
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualification["report_ref"], "reason": "approve the gate"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})
	w.must("worker", "worker", "load", Object{"function": function, "date": date})

	runner := &fixtureStepRunner{status: StepCompleted}
	for _, tc := range []struct {
		name, output string
		want         bool
	}{
		{"accepted", completeOutput, true},
		{"refused", incompleteOutput, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner.output = []byte(tc.output)
			res, err := runner.RunStep(stepRequest)
			if err != nil {
				t.Fatal(err)
			}
			inputs, err := StepDecisionInputs(stepRequest, res)
			if err != nil {
				t.Fatal(err)
			}
			// The workflow adds the decoded record and keeps the original text and digest.
			output, ok := inputs["step_output"].(string)
			if !ok {
				t.Fatalf("step_output: got %T, want string", inputs["step_output"])
			}
			inputs["step_result"], err = Decode([]byte(output))
			if err != nil {
				t.Fatal(err)
			}
			answer, err := w.do("caller", "caller", w.runner, "invoke", Object{
				"function": function, "date": date, "expected_bundle_ref": proposal["bundle_ref"], "inputs": inputs,
			})
			if err != nil {
				t.Fatal(err)
			}
			if at(answer, "actual", "status") != "ok" || at(answer, "actual", "value") != tc.want {
				t.Fatalf("gate answer: %s, want ok with value %t", Encode(answer), tc.want)
			}
			if at(answer, "receipt", "inputs_ref") != InputDigest(inputs) {
				t.Fatalf("receipt did not bind the step inputs: %s", Encode(answer))
			}
		})
	}

	t.Run("failed", func(t *testing.T) {
		runner.status, runner.reason = StepFailed, "exit 1"
		res, err := runner.RunStep(stepRequest)
		if err != nil {
			t.Fatal(err)
		}
		inputs, err := StepDecisionInputs(stepRequest, res)
		if inputs != nil || faultCode(t, err) != "step_failed" {
			t.Fatalf("failed step: inputs %v, error %v", inputs, err)
		}
	})
}
