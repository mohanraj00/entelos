# `source`

This page describes the v0.8.0 release.

Record an approved source text for one function and one date interval. Later plans and proposals cite it by `source_ref`.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [source.schema.json](../../src/protocol/schema/source.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "expected_revision": "",
 "from": "2026-10-01",
 "function": "record.visible",
 "op": "source",
 "reason": "initial requirement",
 "request_key": "initial-source",
 "supersedes": [],
 "text": "<84 characters>",
 "until": "2026-12-01"
}
```

The response:

```json
{
 "data": {
  "cursor": "8d13c7cda9f6...",
  "result": {
   "source_ref": "sha256:0d07c37f0dcb..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-source` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `invalid_interval`: `until` is not after `from`.
- `out_of_scope`: the function is not in the Host context.
- `stale_revision`: `expected_revision` is not the current cursor.
- `idempotency_conflict`: the `request_key` was used for a different request.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
