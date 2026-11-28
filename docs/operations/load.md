# `load`

This page describes the v1.0.0 release.

Prepare the active bundle of a function for a date on the worker. `invoke` answers only after a load.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `worker` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [load.schema.json](../../src/protocol/schema/load.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "date": "2026-10-15",
 "expected_revision": "ef3f35da8fef...",
 "function": "record.visible",
 "op": "load",
 "request_key": "initial-load"
}
```

The response:

```json
{
 "data": {
  "cursor": "6cdf5894ba27...",
  "result": {
   "bundle_ref": "sha256:ac0f00224bd8...",
   "engine_build": "sha256:8ce0f5a21a9c...",
   "event_ref": "sha256:03d902de6302...",
   "observed_cursor": "ef3f35da8fef...",
   "snapshot_ref": "sha256:80f4ceae1728...",
   "worker": "local-worker"
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-load` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `worker`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `out_of_scope`: no active event covers the date.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
