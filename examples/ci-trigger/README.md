# CI trigger: a second consumer, and an ordinary function beside the rule

This example is the second neutral consumer of `src/`. It answers one invented question: should CI run for a pull-request event? The rule is the CEL function `ci.pr_should_run`. The same decision also runs as an ordinary typed Python function behind the same lifecycle port, so the example compares the two. It does not change a file in `src/`.

The example imports the shared Python client in `src/lifecycle-core/client` and reads the record bundle that `verify.py` prepares with `src/cel-handoff/tools/prepare.py`. It copies neither.

## The rule

The function is `ci.pr_should_run`. The scope is `ci-demo`, the worker is `ci-worker`, and the repository is `example/widgets`.

| Name | Type | Supplied by |
|---|---|---|
| `action` | string | the caller |
| `base_branch` | string | the caller |
| `draft` | bool | the caller |
| `target_branch` | string | the host (fixed value) |

The CEL profile is `cel-string-bool/v1`. The ordinary runtime uses `ordinary-ci/v1` under the same input contract. A caller cannot send `target_branch`; the `fixed-input-override` case gives `invalid_input`.

The rule is `!draft && base_branch == target_branch && action in [...]`. The three dated stages make the amendment and the correction:

| Stage | Interval | `target_branch` | Allowed actions |
|---|---|---|---|
| initial | 2026-10-01 to 2026-12-01 | `main` | `opened`, `synchronize` |
| amendment | 2026-11-01 to 2026-12-01 | `release` | `opened`, `synchronize`, `reopened` |
| correction | 2026-11-01 to 2026-12-01 | `release` | `opened`, `synchronize`, `reopened`, `ready_for_review` |

An observation reports a pull request that left draft, `ready_for_review`, on the release branch, which the amendment did not allow. The correction adds that action.

## Run it

`verify.py` runs this example as its `consumer-reuse` step, with the binaries of the same run. To run it alone, use the binaries from a complete `verify.py` run:

```sh
python3 -B examples/ci-trigger/run.py --work "$(mktemp -d)/ci" \
  --cel-binary "$TASK_WORK/bin/cel-handoff" --author-binary "$TASK_WORK/bin/cel-expr" \
  --record-bundle "$TASK_WORK/record-bundle" --host "$TASK_WORK/bin/lifecycle-core"
```

The `--work` directory must not exist and must be outside this checkout. The example writes its stores, the audit sidecar, `results.json` and `commands.json` below it, and it does not overwrite an earlier run.

## Arguments

| Argument | What it is |
|---|---|
| `--work` | New directory outside the checkout for the stores, temporary files and results. |
| `--cel-binary` | The pinned `cel-handoff` executable from a `verify.py` run. |
| `--author-binary` | The pinned `cel-expr` executable from the same run. |
| `--record-bundle` | The prepared record bundle from the same run. |
| `--host` | The Go `lifecycle-core` executable from the same run. |

## Files

| File | What it is |
|---|---|
| `run.py` | Entry point: runs the two consumer loops, the direct lifecycle probes, the author-tool checks and `fsck`, then writes the result. |
| `demo.py` | The `CIDriver` subclass of the shared `Driver`, and `demonstrate()`, the lifecycle steps. |
| `adapter.py` | The application-only projection of a `pull_request` webhook envelope into typed inputs, and the explicit non-atomic audit sidecar. |
| `ordinary.py` | The frozen ordinary typed functions, and the `ordinary-ci/v1` runtime with the same command line as the CEL executable. |
| `support.py` | Locates the shared client in `src/lifecycle-core/client` and loads the fixture. |
| `fixtures/workflow.json` | The invented policy: environment, caller variables, three dated stages with cases, the original inputs and the expected replay and restatement values. |
| `fixtures/ready-review-envelope.json` | The raw webhook envelope (`headers` and `body`) that the projection and the audit use. |

## What the example does

1. It builds the original record-filter store from `src/lifecycle-core/client/scenario.py` on the fixture `src/cel-lifecycle/fixtures/lifecycle-scenario.json`, then copies that store into the CI store, so one history has two consumers.
2. It projects the raw envelope into typed inputs and verifies the audit sidecar against the receipt.
3. For each stage it records the source and the plan, proposes a snapshot from a bundle, has the snapshot qualified and decides it. The wrong formula and the stale proposals are expected refusals.
4. It activates the stages, sends calls through the host, replays the earlier receipt and restates it at the correction cursor.
5. It probes the one-slot worker model, the projection failures and an audit byte failure.
6. It runs the pinned `cel-expr eval` on each stage's cases, and the unchanged `caller.py`.
7. It runs the CI driver loop a second time against the ordinary runtime, then runs `git fsck` on both stores.

## The result

The last line of stdout is a JSON object:

```json
{"status": "passed", "checks": 14, "errors": []}
```

The exit code is 0 when every check passed and 1 otherwise. `<work>/results.json` holds the full result:

| Field | What it holds |
|---|---|
| `status` | `passed` or `failed`. |
| `checks` | One row for each check: `name`, `question`, `passed` and `actual`. |
| `errors` | The exception that stopped the run, if any. |
| `pins` | The receipt pins of the original record-filter loop, the CEL loop and the ordinary loop. |
| `traces`, `direct_operations` | The driver traces and the direct host commands. |
| `measurements`, `limitations` | The recorded operation counts and what the example does not cover. |

`<work>/commands.json` holds each recorded command with its exit code, stdout and stderr. A failed check keeps the partial traces in `results.json`, so the failure stays visible.

## Expected values

The fixture states the original inputs `{"action": "ready_for_review", "base_branch": "release", "draft": false}` and two expectations: the replay of the earlier receipt gives `false`, and the restatement at the correction cursor gives `true`. The cases in each stage also state their expected `true`/`false` values and refusals by hand. They do not come from a run.

## If it fails

The example exits with status 1 and writes `results.json` with `"status": "failed"` and the `errors` list. Keep the work directory of a failed run; the next run needs a new one.

## Limits

- The policy and its choices are invented for the Tekton workflow source. They are not actual project policy.
- No pipeline runs. The example compares a decision with the lifecycle, not a Tekton listener or a CI job.
- Ingress verification is simulated, and the audit sidecar is not atomic with the Git receipt.
- The ordinary baseline shares the lifecycle port. The example makes no claim that one runtime is better.
- The profile has strings and booleans only. Numbers and structured wire types are unsupported.
- Model trials are not run and latency is not benchmarked.
