# `inspect`

This page describes the v1.0.0 release.

Read the full lifecycle state at the current cursor.

| | |
|---|---|
| Kind | Read. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [inspect.schema.json](../../src/protocol/schema/inspect.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "op": "inspect"
}
```

The response:

```json
{
 "data": {
  "cursor": "ad85ad1b8a33...",
  "state": {
   "events": [
    "sha256:03d902de6302...",
    "sha256:d9da2ed8186f...",
    "sha256:59e22cdc4bab...",
    "<1 more>"
   ],
   "executors": {
    "local-worker": [
     "sha256:ab156ed98c78...",
     "sha256:0211b79e237a...",
     "sha256:455078dcf822...",
     "<4 more>"
    ]
   },
   "heads": {
    "[\"record.visible\",\"demo\"]": "sha256:cf0d1f271127..."
   },
   "outbox": "<object with 4 fields>",
   "plans": [
    "sha256:1df618bebe01...",
    "sha256:f147a500d3d9...",
    "sha256:f7043b7ca6b6..."
   ],
   "receipts": [
    "sha256:ab156ed98c78...",
    "sha256:0211b79e237a...",
    "sha256:455078dcf822...",
    "<4 more>"
   ],
   "requests": "<object with 42 fields>",
   "reviews": "<object with 5 fields>",
   "schema": "local-lifecycle/v1",
   "sources": [
    "sha256:0d07c37f0dcb...",
    "sha256:2a8b26695f20...",
    "sha256:160eb9ee1b09..."
   ],
   "tasks": "<object with 1 fields>",
   "workers": "<object with 1 fields>"
  }
 },
 "status": "ok"
}
```

The step `inspect` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `scope_mismatch`: a record at the cursor has another scope.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
