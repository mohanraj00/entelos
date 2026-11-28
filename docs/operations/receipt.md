# `receipt`

This page describes the v1.0.0 release.

Read one receipt: the snapshot, the input, the build, the answer and the stale state.

| | |
|---|---|
| Kind | Read. |
| Role | `reader` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [receipt.schema.json](../../src/protocol/schema/receipt.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "op": "receipt",
 "receipt_ref": "sha256:455078dcf822..."
}
```

The response:

```json
{
 "data": {
  "actual_bundle": "sha256:31616ccb5c3b...",
  "engine_build": "sha256:8ce0f5a21a9c...",
  "executor_id": "local-worker",
  "format": "entelos.record/1",
  "function": "record.visible",
  "host_tools": {
   "git": "git version 2.54.0 (Apple Git-157)",
   "go": "go1.27.2"
  },
  "inputs_ref": "sha256:416391757ca0...",
  "invocation_id": "november-before-correction",
  "lifecycle_build": "sha256:62f5a00aedf0...",
  "log_cursor": 3,
  "mode": "invoke",
  "previous_receipt": "",
  "reason": "",
  "response_ref": "sha256:942e5339d465...",
  "scope": "demo",
  "selection": {
   "bundle_ref": "sha256:31616ccb5c3b...",
   "date": "2026-11-15",
   "decision_ref": "sha256:82d90b6e75ad...",
   "engine_build": "sha256:8ce0f5a21a9c...",
   "event_ref": "sha256:d9da2ed8186f...",
   "function": "record.visible",
   "registry_cursor": "2cf5b14e5566...",
   "report_ref": "sha256:e597037b0c65...",
   "scope": "demo",
   "snapshot_ref": "sha256:1ced099c2df0..."
  },
  "stale": false,
  "stale_mode": "fail_closed",
  "stale_reason": ""
 },
 "status": "ok"
}
```

The step `receipt` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reader`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `artifact_unavailable`: the receipt is not in the store.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
