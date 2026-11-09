# Add a consumer example

Start with one invented decision, such as whether a document is eligible for
export. Write the expected answers from the invented requirement before running
the rule. [Tier eligibility](../examples/tier-eligibility/README.md) is a
complete example with a dated change, a refusal and a preserved earlier answer.

## Write the consumer files

Under `examples/<name>/`, write these files:

| File | Contents |
|---|---|
| `fixtures/callable.json` | Function ID, CEL expression, declared string/bool variables, caller variable names, fixed bindings, budgets and hand-written cases. [Author fixture fields](CONTRACTS.md#author-fixtures) and [case rules](CONTRACTS.md#callable-cases). |
| `fixtures/lifecycle.json` | Your function and scope, each stage's dates, source text, fixed values and independent cases, plus the inputs and expected results for your demonstration. `Driver` requires `function` and `scope`; your script defines the remaining fields. |
| `run.py` | Prepare and check a bundle, send the lifecycle requests, compare observed and expected results, and write the trace. Take `--work`, `--cel-binary`, `--author-binary` and `--host` as arguments. |
| `README.md` | Explain the invented requirement, the expected behavior, the command and the evidence files. |

Keep the shared code in `src/`. Run
[`prepare.py`](../src/cel-handoff/tools/prepare.py) with
`--fixture <callable.json> --bundle <work>/bundle --binary <cel-handoff>`.
It writes the four bundle files, `cases.json` and `upstream-tests.json`. Run
`cel-expr eval -env <bundle>/env.json -expr '<CEL expression>' -tests <bundle>/upstream-tests.json`
with the expression from the fixture and the pinned author tool. Then run
`cel-handoff -bundle <bundle> -cases <bundle>/cases.json check` to check all
cases, including refusals. The report must have status `passed`.

## Reuse the lifecycle client

Import `lifecycle_client` from the checkout by path, as
[`tier-eligibility/run.py`](../examples/tier-eligibility/run.py) does:

```python
from pathlib import Path
import sys

source = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(source / "src/lifecycle-core/client"))
from lifecycle_client import Driver, MIMES, content_ref, decode, encode, envelope
```

Construct a subclass of `Driver` with `(work, binary, scenario, host)`,
where `work` is a new path, `binary` is `cel-handoff`, `scenario` is the
decoded lifecycle fixture and `host` is the Go JSONL executable. The base
class creates the local actor contexts and uses a bare store under `work`.
Keep its generic methods:

- `call(actor, op, name=None, expected="ok", **fields)` sends one operation
  and records the request and response. It fills the revision and retry key
  for writes and asserts the outer status. It returns the response's `data`
  on success or the response with its diagnostic on an expected refusal.
- `cursor()` gives the current accepted Git root. `result(data)` extracts
  the inner result of a successful write.
- `check(name, question, passed, actual)` records an assertion and stops the
  run if it fails.

Write consumer-specific `bundle`, `source_plan` and `propose` methods and
your own sequence of lifecycle calls. The tier example stores cases in its
stage fixtures. The record-filter driver builds them in `cases()`.
[`scenario.py --fixture`](../src/lifecycle-core/client/scenario.py) drives
only `record.visible`. Its bundle, cases and demonstration assume
`allowed_state`, `state` and `archived`.

Use the [request field table](CONTRACTS.md#lifecycle-request-records) to send
`source` and `plan` as reviewer, `propose` as author, `qualify` as runner
and `decide` and `activate` as reviewer. Load as worker before invoking as
caller. The first proposal uses `base_snapshot=""`. Later proposals use the
most recent proposal's `snapshot_ref`, even if it failed qualification.
Use the [bundle media types](CONTRACTS.md#content-references-for-a-stage-bundle)
to recompute references when fixed values or rule bytes change. The
`bundle` argument maps `manifest`, `environment`, `source` and `fixed`
to the UTF-8 strings of the four files.

For example, when a stage changes its fixed values, build the fixed bytes
with `fixed = encode(stage_fixed_values)`. Set
`manifest["fixed_ref"] = content_ref(envelope(MIMES["fixed"], fixed))`
before encoding the manifest. Keep the environment and source bytes from
the prepared bundle if that stage uses the same declarations and rule.

Read `event_ref` from the result of `activate`. To suspend the first revision,
pass that reference in `targets` with the interval you want to mask. When
activating a replacement directly, list every earlier live activation that
the new interval overlaps. See [activation targets](CONTRACTS.md#activation-targets).

For a changed rule, write a new proposal and leave the first receipt intact.
Use a reader context to replay the original receipt and restate it at the
later accepted cursor. Check the values and both receipts' selected
snapshots. Include one refusal such as a stale bundle or missing input.
The unchanged [`caller.py`](../src/lifecycle-core/client/caller.py) can call
the new function with `--function`, `--date` and `--inputs`. See the
[caller command](WALKTHROUGH.md#6-call-as-a-caller) for the other arguments.

## Run and record it

Keep the bundle, store, `results.json` and `commands.json` under the external
`--work` directory. Use `driver.trace` and `driver.checks` for the lifecycle
evidence. Write the partial trace and the error on failure and
exit with status 1. Include a top-level `status` of `passed` or `failed` in
`results.json`.

In [`verify.py`](../verify.py), add a `run(...)` call and a `check(...)`
immediately after the `tier-eligibility` check. Pass `sys.executable`, `-B`,
the example's `run.py`, a fresh subdirectory of `w`, and the built
`cel-handoff`, `cel-expr` and `lifecycle-core` binaries in `w / 'bin'`.
Read the new example's `results.json`. Require both exit code 0 and status
`passed`, and include the example's errors if either condition fails.

Regenerate `checksums.json` with
`python3 -B tools/check_project.py --write-checksums`. Run `make check` and
run `make verify TASK_WORK="$TASK_WORK"` with a fresh external work path.
Keep the logs of failed runs and record the commit with the checks.
