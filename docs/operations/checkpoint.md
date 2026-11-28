# `checkpoint`

This page describes the v0.8.0 release.

Save the progress of a claimed task, or complete it.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `maintainer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [checkpoint.schema.json](../../src/protocol/schema/checkpoint.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "checkpoint": "1",
 "expected_revision": "3d4c0ee0e502...",
 "note": "Correction source requested; no rule change yet.",
 "op": "checkpoint",
 "request_key": "save-correction-work",
 "status": "working",
 "task_id": "sha256:38cf004faf05...",
 "token": "sha256:e74901628bf1..."
}
```

The response:

```json
{
 "data": {
  "cursor": "f3ba82310b2a...",
  "result": {
   "checkpoint": "2",
   "correction_snapshot": "",
   "function": "record.visible",
   "note": "Correction source requested; no rule change yet.",
   "observation_ref": "sha256:38cf004faf05...",
   "owner": "maintainer",
   "scope": "demo",
   "status": "working",
   "task_id": "sha256:38cf004faf05...",
   "token": "sha256:e74901628bf1..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `save-correction-work` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `maintainer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `stale_fence`: the token is from an earlier owner of the task.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
