# Adapter contract: a step as a koodam Agent run

This adapter runs a workflow agent step as one [koodam](https://github.com/mohanraj00/koodam) Agent run. It implements the `StepRunner` of the [step outcome contract](../../src/lifecycle-core/STEP.md). The clauses S1 to S9 are the clauses of that contract. Issue [#113](https://github.com/mohanraj00/entelos/issues/113) adds this adapter.

The adapter is optional. It is a Go module of its own, and the lifecycle core does not import it. It has no Kubernetes library: it runs the `kubectl` executable.

The koodam fields are the fields of the Agent API `core.koodam.dev/v1alpha1`. The fields `spec.output`, `status.runID` and `status.output` are from [koodam#296](https://github.com/mohanraj00/koodam/issues/296) and the koodam document `docs/agent-output.md`, sections "Run identity" and "Output digest". No koodam release has them yet. See [Stand-in](#stand-in).

## Clauses

### K1. One step, one Agent

Each call of `RunStep` creates one new Agent with `metadata.generateName`. The adapter never uses an Agent again. This agrees with the koodam rule that one Agent is one run. Thus a new run has a new run identity (S2).

### K2. Request

The adapter computes `StepRequestRef` (S1) before it creates the Agent. The name of the Agent starts with `step-` and the first 12 hex characters of the request reference.

### K3. Inputs

The adapter has the bytes of each input. If the bytes of an input do not have the digest of the request, `RunStep` gives an error and creates no Agent (S1).

Each input goes to the agent container in `spec.env`:

| Variable | Value |
|---|---|
| `ENTELOS_STEP` | The step name. |
| `ENTELOS_STEP_REQUEST_REF` | The request reference. |
| `ENTELOS_OUTPUT_PATH` | The path of the output file. |
| `ENTELOS_INPUT_<NAME>` | The text of the input. |
| `ENTELOS_INPUT_<NAME>_SHA256` | The SHA-256 of the input, as 64 hex characters. |

`<NAME>` is the input name in upper case, with `_` for `.` and `-`. If two inputs give the same variable, `RunStep` gives an error and creates no Agent. For example, the inputs `notice` and `notice_sha256` both give `ENTELOS_INPUT_NOTICE_SHA256`. Thus no input can replace the bytes or the digest of another input. An input must be UTF-8 text with no NUL byte. The sum of the inputs is 32 KiB at most.

### K4. Limits of the run

The adapter sets `spec.model`, `spec.budget` and `spec.deadlineSeconds` from its configuration. The budget is a plain decimal number, for example `2` or `0.5`. The adapter sets `spec.output.path`, and it adds an `emptyDir` volume for the directory of that path with `spec.podTemplate`.

### K5. End of the run

The adapter reads the Agent until `status.phase` is `Succeeded` or `Failed` and `status.revokedAt` has a value. Koodam writes `status.usage` before `status.revokedAt`, so the spend record is final at that time. The adapter stops after the wait time for the run, or after the settle time from the end of the run to the revoke. The two limits are elapsed time, so the time of each read counts. The poll time and the wait time must be more than zero, or `RunStep` gives an error and creates no Agent.

### K6. Status map

The adapter applies these rules in order. The first rule that agrees gives the status (S3).

| Order | Agent status | Step status | Reason |
|---|---|---|---|
| 1 | The phase is not `Succeeded` or `Failed` after the wait time. | `failed` | The run did not end. |
| 2 | No `status.revokedAt` or no `status.usage`. | `failed` | The run has no spend record. |
| 3 | `status.usage.spend` is not a plain decimal number. | `failed` | The spend record is not readable. |
| 4 | The spend is equal to the budget or larger. | `budget_stopped` | Clause K7. |
| 5 | The phase is `Failed`. | `failed` | The phase. |
| 6 | No `status.runID`. | `failed` | No run identity. |
| 7 | No `status.output`, or it has a `reason`, or it has no `sha256` and `size`. | `failed` | The koodam reason: `NotFound`, `NotRegularFile`, `ReadError` or `NoResult`. |
| 8 | The size is larger than 64 KiB. | `failed` | The output is too large. |
| 9 | The output bytes are not readable, or they do not agree with `sha256` and `size`. | `failed` | Clause K9. |
| 10 | None of the rules above. | `completed` | |

An empty `status.usage.spend` is the spend 0.

### K7. Budget stop

Koodam gives no status for a budget stop. The gateway refuses each model call after the budget, and the process of the agent decides what it does then. Thus the adapter reads the spend record. If the spend reached the budget, the status is `budget_stopped`, also if the phase is `Succeeded`. The adapter cannot know if a refused call changed the output, so it does not accept that output.

### K8. Run identity and trace

The run identity (S2) is `status.runID`. Koodam defines it as the UID of the Agent.

The trace (S2) is a JSON object with sorted keys. It has the `apiVersion`, the `kind`, the namespace, the name and the UID of the Agent, and its `spec` and its `status` as the adapter read them last. The status has the phase, the Job name, the times, the key alias, the key hash and the usage with the spend. The key hash is not a credential, as the koodam API says. The trace does not have the model calls. Koodam sends those spans to a collector and gives no digest for them.

### K9. Output

The command of the Agent writes the output to the file of `spec.output.path` and also to its standard output. The adapter reads the standard output of the container `agent` of the Job `status.job`. `status.output.sha256` and `status.output.size` decide: if the bytes do not agree with them, the status is `failed` (S5, S6).

A workflow with a durable volume can read the file there. The check against the status digest stays the same.

### K10. Result and errors

A `completed` result has the run identity, the output, the two digests and the trace (S5). Each other result has no output (S6), and `StepDecisionInputs` refuses it (S7). `RunStep` gives an error, and no result, if `kubectl` fails before the adapter can read an ended Agent or if the configuration is not valid.

## Stand-in

Koodam v0.3.0 is the latest koodam release that is not a prerelease. Its CRD refuses `spec.output`, and its status has no `runID` and no `output`. Until a koodam release has the fields of koodam#296, the seam test uses `StandIn296` (`standin.go`) between the adapter and `kubectl`:

- It removes `spec.output` from the manifest.
- It adds `status.runID` with the UID of the Agent, as koodam#296 defines it.
- For a `Succeeded` run, it adds `status.output` with the SHA-256 and the size of the standard output of the agent container. For a `Failed` run, it adds the reason `NoResult`.

With the stand-in, clause K9 compares the standard output with a digest of the same bytes. Thus the seam test does not show that koodam computes the digest. The released koodam controller does all other work of the run: the Job, the gateway key, the budget, the phase, the revoke and the usage.

[`kind/pins.env`](kind/pins.env) sets `KOODAM_STANDIN_296=1`. The repin to the first koodam release with the fields sets it to 0 and removes `standin.go`.

## Limits

- The adapter does not delete an Agent. It does not stop a run after its wait time.
- The env transport of K3 is for small text. A larger input needs a volume through `spec.podTemplate`, which this adapter does not make.
- The adapter trusts the cluster of its kubeconfig. A person who can write the Agent status can change the outcome.
- The output is 64 KiB at most, because it becomes an `invoke` input.
