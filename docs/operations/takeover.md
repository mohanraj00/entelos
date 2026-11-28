# `takeover`

This page describes the v0.8.0 release.

Give a claimed task to a new owner. The token of the old owner stops working.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [takeover.schema.json](../../src/protocol/schema/takeover.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "checkpoint": "2",
 "expected_revision": "f3ba82310b2a...",
 "op": "takeover",
 "owner": "other-maintainer",
 "reason": "Manually resume the saved correction task.",
 "request_key": "manual-takeover",
 "task_id": "sha256:38cf004faf05..."
}
```

The response:

```json
{
 "data": {
  "cursor": "75b892b0c78e...",
  "result": {
   "checkpoint": "3",
   "correction_snapshot": "",
   "function": "record.visible",
   "note": "Manually resume the saved correction task.",
   "observation_ref": "sha256:38cf004faf05...",
   "owner": "other-maintainer",
   "scope": "demo",
   "status": "working",
   "task_id": "sha256:38cf004faf05...",
   "token": "sha256:2d2dc1dcc836..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `manual-takeover` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `stale_revision`: `expected_revision` is not the current cursor.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
