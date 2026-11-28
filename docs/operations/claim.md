# `claim`

This page describes the v0.8.0 release.

Take an open correction task. The result has a token for later checkpoints.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `maintainer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [claim.schema.json](../../src/protocol/schema/claim.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "checkpoint": "0",
 "expected_revision": "5aea05d66c3c...",
 "op": "claim",
 "request_key": "claim-correction",
 "task_id": "sha256:38cf004faf05..."
}
```

The response:

```json
{
 "data": {
  "cursor": "3d4c0ee0e502...",
  "result": {
   "checkpoint": "1",
   "function": "record.visible",
   "note": "",
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

The step `claim-correction` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `maintainer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `task_claimed`: another maintainer owns the task.
- `forbidden`: the actor is not a maintainer.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
