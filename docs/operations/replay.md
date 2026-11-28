# `replay`

This page describes the v0.8.0 release.

Run an earlier receipt again on its own snapshot and build. The answer must match the original answer.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reader` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [replay.schema.json](../../src/protocol/schema/replay.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "expected_revision": "11cf60b117ec...",
 "op": "replay",
 "reason": "Reproduce the earlier answer without resolving latest.",
 "receipt_ref": "sha256:455078dcf822...",
 "request_key": "pinned-replay"
}
```

The response:

```json
{
 "data": {
  "cursor": "1f22739b4a80...",
  "result": {
   "actual": {
    "bundle_ref": "sha256:31616ccb5c3b...",
    "cost": 5,
    "implementation_ref": "sha256:8ce0f5a21a9c...",
    "status": "ok",
    "value": true
   },
   "receipt": "<object with 20 fields>",
   "receipt_ref": "sha256:123efedd1f2e..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `pinned-replay` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reader`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `input_mismatch`: the request changes the inputs of the receipt.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
