# Quickstart: the record-filter loop

This demo runs the complete record-filter loop with one command. It needs Go, Python 3, Git and network access (see [SUPPORT.md](../../docs/SUPPORT.md)). Each run downloads the pinned Go modules into a new module cache in the work directory. Thus, a run without network access fails at `build-host`.

```sh
python3 -B examples/quickstart/demo.py --work "$TASK_WORK"
```

Use a new path outside this checkout for `--work`. The directory must not exist. The demo makes it, and it does not overwrite an earlier run. The demo does not write to the checkout. It writes the build, the Go caches and the Git lifecycle store to the work directory.

## What the demo does

1. It builds the `cel-handoff` host with the same pinned flags and isolated settings as `verify.py`.
2. It prepares the record bundle with `src/cel-handoff/tools/prepare.py`.
3. It runs the unchanged lifecycle scenario `src/cel-lifecycle/scenario.py` on the fixture `src/cel-lifecycle/fixtures/lifecycle-scenario.json`.
4. It prints a short summary and writes `summary.json` to the work directory.

## What the summary shows

- The build identity of the host.
- The answer: on 2026-11-15, the record `{"state":"reviewed","archived":false}` is visible. The answer has a receipt.
- The correction: the approved visible state changes from `reviewed` to `approved`. A reviewer activates it as a new event.
- The replay: the earlier receipt, on its pinned snapshot, gives the same answer again.
- The restatement: the same input under the correction gives a new, separate answer.

The expected values come from the `expected` field of the fixture, not from the demo output. The full trace is in `<work>/loop/scenario-results.json`.

## If the demo fails

The demo prints `Result: failed` and exits with status 1. If a step before the loop fails, the demo also writes the step output to stderr. It always writes `<work>/summary.json` with the reason:

- `failed_step`, `stdout` and `stderr`: a step before the loop failed, for example `build-host`.
- `loop_error` and `loop_output`: the loop failed. `loop_output` keeps the exit code, stdout and stderr of `scenario.py`.
- `errors`: the results file is missing or not valid JSON, or a step or field is missing from the trace.
- `failed_checks`: a value or snapshot does not agree with the fixture. The demo prints each name as `Failed comparison`.

Keep the work directory of a failed run. The next run needs a new work directory.

## Limits

The demo shows one invented example on a local Git store. It does not show authentication, a production database or performance. The `quickstart-failures` check in `verify.py` runs `test_demo.py`, which gives invented failed results to the summary path. The `quickstart` check runs this demo and compares its build identity and answers with the verified build and the fixture.
