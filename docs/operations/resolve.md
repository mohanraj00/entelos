# `resolve`

This page describes the v0.8.0 release.

Select the active snapshot of a function for a date, at the current cursor or at an earlier `cursor`.

| | |
|---|---|
| Kind | Read. |
| Role | `reader` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [resolve.schema.json](../../src/protocol/schema/resolve.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "date": "2026-10-15",
 "function": "record.visible",
 "op": "resolve"
}
```

The response:

```json
{
 "data": {
  "bundle_ref": "sha256:ac0f00224bd8...",
  "date": "2026-10-15",
  "decision_ref": "sha256:c469fa50dd62...",
  "engine_build": "sha256:8ce0f5a21a9c...",
  "event_ref": "sha256:03d902de6302...",
  "function": "record.visible",
  "registry_cursor": "eddc8c97acf1...",
  "report_ref": "sha256:2bc0d97fcc6f...",
  "scope": "demo",
  "snapshot_ref": "sha256:80f4ceae1728..."
 },
 "status": "ok"
}
```

The step `resolve` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reader`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `out_of_scope`: the function is not in the Host context.
- `unsupported_subject`: a 1.0 to 1.2 request names a content subject.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
