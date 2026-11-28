# `ack`

This page describes the v0.8.0 release.

Acknowledge one outbox event after the worker loaded it.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `worker` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [ack.schema.json](../../src/protocol/schema/ack.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "event_ref": "sha256:d9da2ed8186f...",
 "expected_revision": "0037cf8308dd...",
 "op": "ack",
 "request_key": "future-ack"
}
```

The response:

```json
{
 "data": {
  "cursor": "2cf5b14e5566...",
  "result": {
   "event_ref": "sha256:d9da2ed8186f...",
   "status": "acknowledged",
   "worker": "local-worker"
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `future-ack` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `worker`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `stale_revision`: `expected_revision` is not the current cursor.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
