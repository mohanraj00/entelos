# `describe`

This page describes the v0.8.0 release.

Give the caller contract of a function for a date: the input schema, the exact bundle reference and the store cursor.

| | |
|---|---|
| Kind | Read. |
| Role | `caller` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [describe.schema.json](../../src/protocol/schema/describe.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "date": "2026-10-15",
 "function": "record.visible",
 "op": "describe"
}
```

The response:

```json
{
 "data": {
  "callable": {
   "bundle_ref": "sha256:ac0f00224bd8...",
   "function_id": "record.visible",
   "implementation_ref": "sha256:8ce0f5a21a9c...",
   "inputs": "<object with 4 fields>",
   "limits": {
    "evaluation_cost_limit": 1000,
    "max_request_bytes": 4096,
    "max_source_utf8_bytes": 4096,
    "max_string_utf8_bytes": 256,
    "parser_recursion_limit": 64
   },
   "output": {
    "type": "boolean"
   },
   "profile": "cel-string-bool/v1"
  },
  "cursor": "ef3f35da8fef...",
  "selection": {
   "bundle_ref": "sha256:ac0f00224bd8...",
   "date": "2026-10-15",
   "decision_ref": "sha256:c469fa50dd62...",
   "engine_build": "sha256:8ce0f5a21a9c...",
   "event_ref": "sha256:03d902de6302...",
   "function": "record.visible",
   "registry_cursor": "ef3f35da8fef...",
   "report_ref": "sha256:2bc0d97fcc6f...",
   "scope": "demo",
   "snapshot_ref": "sha256:80f4ceae1728..."
  }
 },
 "status": "ok"
}
```

The step `describe` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `caller`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `out_of_scope`: no active event covers the date, for example after a suspension.
- `invalid_interval`: the date is not valid.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
