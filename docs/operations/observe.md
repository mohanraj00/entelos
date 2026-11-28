# `observe`

This page describes the v0.8.0 release.

Report a problem with one receipt. The Host opens a correction task.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `observer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [observe.schema.json](../../src/protocol/schema/observe.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "expected_revision": "b828d2aa6ac7...",
 "message": "<80 characters>",
 "op": "observe",
 "receipt_ref": "sha256:455078dcf822...",
 "request_key": "report-discrepancy",
 "route": "source"
}
```

The response:

```json
{
 "data": {
  "cursor": "5aea05d66c3c...",
  "result": {
   "observation_ref": "sha256:38cf004faf05...",
   "task_id": "sha256:38cf004faf05..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `report-discrepancy` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `observer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `invalid_request`: a field is missing or has the wrong type.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
