# `suspend`

This page describes the v0.8.0 release.

Stop the active events of a function for a date interval, with no replacement. A read in that interval then has no answer.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [suspend.schema.json](../../src/protocol/schema/suspend.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "expected_revision": "6cfc041be25c...",
 "from": "2026-11-01",
 "function": "record.visible",
 "op": "suspend",
 "reason": "stop serving incorrect source data",
 "request_key": "suspend-wrong-future",
 "targets": [
  "sha256:d9da2ed8186f..."
 ],
 "until": "2026-12-01"
}
```

The response:

```json
{
 "data": {
  "cursor": "f509353c4ef7...",
  "result": {
   "event_ref": "sha256:59e22cdc4bab..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `suspend-wrong-future` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `stale_revision`: `expected_revision` is not the current cursor.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
