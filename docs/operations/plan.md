# `plan`

This page describes the v0.8.0 release.

Record the independent test cases for one source. Qualification runs a candidate against these cases.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [plan.schema.json](../../src/protocol/schema/plan.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "cases": [
  {
   "case": "visible-current",
   "inputs": {
    "archived": false,
    "state": "ready"
   },
   "status": "ok",
   "value": true
  },
  {
   "case": "different-state",
   "inputs": {
    "archived": false,
    "state": "closed"
   },
   "status": "ok",
   "value": false
  },
  {
   "case": "archived-current",
   "inputs": {
    "archived": true,
    "state": "ready"
   },
   "status": "ok",
   "value": false
  }
 ],
 "expected_revision": "8d13c7cda9f6...",
 "op": "plan",
 "origin": "Independent host fixture from the invented source table",
 "request_key": "initial-plan",
 "source_ref": "sha256:0d07c37f0dcb...",
 "supersedes": []
}
```

The response:

```json
{
 "data": {
  "cursor": "91e858e2a9e5...",
  "result": {
   "plan_ref": "sha256:1df618bebe01..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-plan` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `artifact_unavailable`: the `source_ref` is not in the store.
- `stale_revision`: `expected_revision` is not the current cursor.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
