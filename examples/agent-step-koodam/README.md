# Agent step as a koodam Agent run

This example is an optional adapter. It runs a workflow agent step as one [koodam](https://github.com/mohanraj00/koodam) Agent run on Kubernetes, and it gives the result to a gate decision of the lifecycle Host.

- [STEP.md](../../src/lifecycle-core/STEP.md) is the generic step outcome contract of the core.
- [CONTRACT.md](CONTRACT.md) is the contract of this adapter, with a reference to a core clause for each rule.

The synthetic step asks an agent to summarize an invented library notice. The gate accepts a summary of 1 to 160 bytes. The receipt of the gate binds the run identity, the output and the digests. A replay of the receipt uses the bound output and makes no Agent.

## Files

| File | Contents |
|---|---|
| `adapter.go` | The adapter: the Agent manifest, the wait and the status map. |
| `kubectl.go` | The cluster access through the `kubectl` executable. |
| `standin.go` | The stand-in for the Agent fields of koodam#296. |
| `adapter_test.go` | Tests with a fake cluster. They need no cluster. |
| `workflow_test.go` | The Host with the gate, the receipt check and the replay check. |
| `kind_test.go` | The same checks on a kind cluster. `kind/seam.sh` runs them. |
| `kind/pins.env` | The pins: kind, the node image, the koodam chart and the agent image. |

## Run the tests without a cluster

```sh
go -C examples/agent-step-koodam test -count=1 ./...
```

The kind tests do not run in this command. `verify.py` runs this command.

## Run the seam test on kind

You need `docker`, `kind` v0.33.0, `kubectl`, `helm`, `go` and `openssl`. Give a new work directory outside the checkout:

```sh
sh examples/agent-step-koodam/kind/seam.sh "$TASK_WORK"
```

The script does these steps:

1. It makes the kind cluster `entelos-agent-step` with a kubeconfig in the work directory. It does not change your kubeconfig.
2. It installs the koodam chart of `kind/pins.env` by its digest, with the bundled gateway and the mock model.
3. It runs `TestKindSeam`, `TestKindFailedRun` and `TestKindBudgetStop`. Each test creates one Agent.
4. It writes `seam.log` and `agents.json` to the work directory, and it deletes the cluster.

The agent of the test is a shell script, not a model. It checks the digest of the notice, calls the mock model 1 time for a spend record, and writes the first sentence of the notice as the summary.

## Limits

- The seam test pins the released koodam chart 0.3.0. That release does not have the run identity and the output digest, so the test uses a stand-in for those fields. See [CONTRACT.md](CONTRACT.md#stand-in).
- The gate is a frozen Go function, and the qualification runner is a fixture that runs no rule. The example does not use the CEL runtime.
- The Host uses the memory store. The example keeps no store after the test.
- No CI job runs the kind test. The evidence is a run from a clean checkout on a local kind cluster.
- The adapter is experimental, and so is the step outcome contract.
