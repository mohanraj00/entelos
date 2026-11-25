# Step outcome contract

A workflow step can call an agent. This contract tells how the result of one agent run becomes the inputs of a decision. Issue [#113](https://github.com/mohanraj00/entelos/issues/113) adds it. `step.go` holds the Go types. Each declaration is experimental.

The contract is generic. A step runner is an adapter to the system that runs the agent. The core has no step runner and no dependency on such a system. [examples/agent-step-koodam](../../examples/agent-step-koodam/CONTRACT.md) is one optional adapter.

## Terms

- A byte digest is `sha256:` and the SHA-256 of the exact bytes, as 64 lower-case hex characters. `ByteDigest` computes it. It is not a content reference, because a content reference also hashes a media type.
- A run is one execution of the agent. A new execution is a new run.
- The gate is the activated function that decides on the result of the step.

## Clauses

### S1. Request

`StepRequest` has a step name and a map from each input name to the byte digest of that input. A name has 1 to 128 characters from `a-z`, `0-9`, `.`, `_` and `-`, and it starts with a letter or a digit. `StepRequestRef` gives the content reference of the canonical JSON object `{"inputs": {...}, "step": "..."}`. If the request is not valid, the fault is `invalid_step_request`.

The request has no input bytes. The step runner gets the bytes from the workflow, and it makes sure that the run uses the bytes of each digest.

### S2. Result

A step runner gives a `StepResult`: a `StepOutcome`, the output bytes and the trace bytes. The outcome has these fields:

| Field | Value |
|---|---|
| `Status` | `completed`, `failed` or `budget_stopped`. |
| `RequestRef` | The `StepRequestRef` of the request of the run. |
| `RunID` | The identity of the run. |
| `OutputDigest`, `OutputSize` | The byte digest and the byte count of the output. |
| `TraceDigest` | The byte digest of the trace. |
| `Reason` | Why the outcome is not `completed`. |

`CheckStepResult` applies clauses S3 to S6. A result that fails a clause gives the fault `invalid_step_outcome`.

### S3. Status

The status is one of the three values. `failed` is each run that did not complete, for example a failed process, a deadline or a missing output. `budget_stopped` is a run that a spend limit stopped. No other status exists, and no status means approval.

### S4. Request binding

`RequestRef` must equal the `StepRequestRef` of the request. Thus an outcome cannot move to other inputs.

### S5. Completed outcome

A `completed` outcome has a run identity, an output digest, an output size and a trace digest. It has no reason. The output digest and the size must agree with the output bytes. The trace digest must agree with the trace bytes.

### S6. Outcome that is not completed

A `failed` or `budget_stopped` outcome has a reason. It has no output digest, no output size and no output bytes. It has a run identity if a run started. If it has trace bytes, the trace digest must agree with them.

### S7. No approval from a failure

`StepDecisionInputs` is the only path from a step result to a decision. For a `failed` outcome it gives the fault `step_failed`. For a `budget_stopped` outcome it gives the fault `step_budget_stopped`. It gives no inputs, so the workflow has nothing to send to `invoke`. No receipt answers for the run, and no approval can come from it.

A step runner error is not an outcome. The workflow has no result, and it stops the step.

### S8. Decision inputs

For a `completed` outcome, `StepDecisionInputs` gives these `invoke` inputs. Each value is a string:

| Input | Value |
|---|---|
| `step` | The step name. |
| `step_request_ref` | `RequestRef`. |
| `step_run_id` | `RunID`. |
| `step_output` | The output bytes as UTF-8 text. |
| `step_output_digest` | `OutputDigest`. |
| `step_output_size` | `OutputSize` as a decimal number. |
| `step_trace_digest` | `TraceDigest`. |

If the output is not UTF-8 text, the fault is `step_output_not_text`. The workflow can add its own inputs before the `invoke`.

For JSON output, the workflow can decode `step_output` before `invoke` and add the decoded object as a caller variable declared as a `record` in [`cel-typed/v1`](../cel-lifecycle/typed-profile-spec.md#record). For example, the gate can read `step_result.complete` from a `step_result` record with a bool field named `complete`, and read `step_output_digest` as a string. Keep the original `step_output` and `step_output_digest` in the inputs. Then the receipt binds the text, its digest and the decoded record as three separate inputs. The receipt does not show that the record comes from the text. The Host and the gate do not compare them, because a `cel-typed/v1` rule cannot decode JSON text. The workflow alone makes sure that it decodes the record from `step_output`.

### S9. Decision record and replay

The workflow sends the inputs to `invoke` of the gate. The Host stores the canonical input bytes, and the receipt names them with `inputs_ref`. Thus the receipt binds the request, the run identity, the output and the two digests. The receipt is the decision record of the step.

`replay` of the receipt reads the stored inputs and runs the gate again. It does not call the step runner, so a replay uses the bound output with no new run. A new run is a new `invoke` with a new run identity, and it gives a new receipt.

## Evidence

The tests use a fixture step runner. The tests in `step_test.go` use a Go fake of a gate rule for the receipt and replay checks.

- `TestStepSeam`: one run gives a summary of a sample notice. The inputs equal the stated values, and the digests are from `sha256sum`. The receipt binds the inputs. A replay agrees with the original answer, and the runner has 1 run.
- `TestStepFailureCannotProduceAnApproval`: a `failed` run and a `budget_stopped` run give the two faults and no inputs. The store keeps its root. The same outcome with the label `completed` is `invalid_step_outcome`.
- `TestStepResultChecks`: 12 results that disagree with their bytes or their request are refused, and 3 requests that are not valid.
- `TestStepGateCEL`: the Host qualifies a `cel-typed/v1` gate with the CEL runtime, then invokes it with the step inputs and a decoded JSON record. The gate accepts one completed result and refuses another. A failed result gives `step_failed` and no inputs (#209).

## Limits

- The Host does not know a step. A workflow that sends other inputs to `invoke` gets no check from this contract. The gate rule can require the step inputs with its caller variables.
- The core trusts the step runner for the status and the run identity. A runner that reports `completed` for a failed run is outside this contract. The digests show only that the bytes are the bytes of the outcome.
- A digest does not show that the output is correct. The gate rule and its qualification decide what an acceptable output is.
- The Host does not check that a decoded record agrees with `step_output`. The workflow owns the decode. `TestStepGateCEL` has a case where the record disagrees with the text, and the gate gives the answer of the record.
- The trace is the bytes that the adapter gives. This contract does not define a trace format.
