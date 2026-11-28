# `decide`

This page describes the v0.8.0 release.

Record the verdict of a reviewer on one qualification report. Version 1.1 adds the `reject` disposition.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [decide.schema.json](../../src/protocol/schema/decide.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "expected_revision": "c64870bfa9c2...",
 "op": "decide",
 "reason": "manual fixture approval",
 "report_ref": "sha256:2bc0d97fcc6f...",
 "request_key": "initial-decide"
}
```

The response:

```json
{
 "data": {
  "cursor": "45779a393cba...",
  "result": {
   "decision": "<object with 12 fields>",
   "decision_ref": "sha256:c469fa50dd62..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-decide` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `forbidden`: the actor is not a reviewer.
- `qualification_failed`: the report has the status `failed`.
- `untrusted_runner`: the report is not from the runner that the Host context names in `runner_actor`.
- `stale_qualification`: the source or the plan of the report is no longer valid, or the report names another candidate.
- `separation_required`: the reviewer is also the producer of the snapshot or the runner of the report.
- `duplicate_verdict`: the reviewer already gave a verdict on this report.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
