# Quickstart: the record-filter loop

This demo runs the complete record-filter loop with one command. It needs Go, Python 3 and Git (see [SUPPORT.md](../../docs/SUPPORT.md)). The first run downloads the pinned Go modules.

```sh
python3 -B examples/quickstart/demo.py --work "$TASK_WORK"
```

Use a new directory outside this checkout for `--work`. The demo does not write to the checkout. It writes the build, the Go caches and the Git lifecycle store to the work directory.

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

The expected values come from the `expected` field of the fixture, not from the demo output. If an answer does not agree with the fixture, the demo prints `Result: failed` and exits with status 1. The full trace is in `loop/scenario-results.json`.

## Limits

The demo shows one invented example on a local Git store. It does not show authentication, a production database or performance. The `quickstart` check in `verify.py` runs this demo and compares its build identity and answers with the verified build and the fixture.
