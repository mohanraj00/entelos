# `invoke`

This page describes the v1.0.0 release.

Answer one call with the loaded bundle and record a receipt. The caller sends the bundle reference from `describe`.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `caller` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [invoke.schema.json](../../src/protocol/schema/invoke.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "date": "2026-10-15",
 "expected_bundle_ref": "sha256:ac0f00224bd8...",
 "expected_revision": "6cdf5894ba27...",
 "function": "record.visible",
 "inputs": {
  "archived": false,
  "state": "ready"
 },
 "op": "invoke",
 "request_key": "october-call"
}
```

The response:

```json
{
 "data": {
  "cursor": "0e04e210ebae...",
  "result": {
   "actual": {
    "bundle_ref": "sha256:ac0f00224bd8...",
    "cost": 5,
    "implementation_ref": "sha256:8ce0f5a21a9c...",
    "status": "ok",
    "value": true
   },
   "receipt": "<object with 19 fields>",
   "receipt_ref": "sha256:ab156ed98c78..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `october-call` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `caller`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `revision_not_loaded`: the worker did not load the active bundle.
- `artifact_mismatch`: `expected_bundle_ref` is not the active bundle.
- `activation_revoked`: a suspension stopped the event, and the bundle fails closed.
- `activation_expired`: the date is after the interval, and the bundle fails closed.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
