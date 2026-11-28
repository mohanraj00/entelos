# `qualify`

This page describes the v1.0.0 release.

Run the plan cases against one snapshot and record a report with the status `passed`, `failed` or `unresolved`.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `runner` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [qualify.schema.json](../../src/protocol/schema/qualify.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "expected_revision": "0c627ec80f1d...",
 "op": "qualify",
 "plan_ref": "sha256:1df618bebe01...",
 "request_key": "initial-qualify",
 "snapshot_ref": "sha256:80f4ceae1728..."
}
```

The response:

```json
{
 "data": {
  "cursor": "c64870bfa9c2...",
  "result": {
   "report": "<object with 15 fields>",
   "report_ref": "sha256:2bc0d97fcc6f..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-qualify` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `runner`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `forbidden`: the actor does not have the role `runner`.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
