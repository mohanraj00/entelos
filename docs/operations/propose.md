# `propose`

This page describes the v1.0.0 release.

Propose a candidate for a function and a date interval: a CEL bundle, or a content manifest in 1.3. The Host records a snapshot.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `author` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [propose.schema.json](../../src/protocol/schema/propose.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "base_snapshot": "",
 "bundle": {
  "environment": "<423 characters>",
  "fixed": "{\"allowed_state\":\"ready\"}\n",
  "manifest": "<614 characters>",
  "source": "state == allowed_state && !archived\n"
 },
 "expected_revision": "91e858e2a9e5...",
 "from": "2026-10-01",
 "function": "record.visible",
 "op": "propose",
 "reason": "initial proposal",
 "request_key": "initial-proposal",
 "source_ref": "sha256:0d07c37f0dcb...",
 "until": "2026-12-01"
}
```

The response:

```json
{
 "data": {
  "cursor": "0c627ec80f1d...",
  "result": {
   "bundle_ref": "sha256:ac0f00224bd8...",
   "snapshot_ref": "sha256:80f4ceae1728..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-proposal` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `author`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `stale_proposal`: the `base_snapshot` is not the current head. The test step `stale-competing-proposal` shows it.
- `invalid_request`: a field is missing or has the wrong type.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
