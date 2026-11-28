# `approval`

This page describes the v1.0.0 release.

Read the verdict state of one candidate: `current`, `stale`, `rejected` or `unknown`.

| | |
|---|---|
| Kind | Read. |
| Role | `reader` |
| Protocol | Wire protocol 1.3 |
| Contract | [VERDICT.md](../../src/protocol/verdict/VERDICT.md) |

## Example

The request:

```json
{
 "protocol": "1.3",
 "request_id": "c-15",
 "operation": "approval",
 "body": {
  "function": "content:loan-rules/region-a",
  "candidate": {
   "kind": "content",
   "ref": "sha256:855bfa697544..."
  }
 }
}
```

The expected response:

```json
{
 "protocol": "1.3",
 "request_id": "c-15",
 "status": "ok",
 "result": {
  "cursor": "<c2>",
  "approved": false,
  "state": "unknown",
  "counts": {
   "human": 0,
   "model": 0,
   "policy": 0
  },
  "required": {
   "human": 0,
   "model": 0,
   "policy": 0
  },
  "verdicts": []
 }
}
```

The step `approval-unknown` of [session-content.json](../../src/protocol/vectors/session-content.json) has this request. `verify.py` runs it through the Go sidecar with `test_protocol.py`. `<name>` is a value from an earlier step of the same file. An object of the expected response can have more fields than the vector names. Long values are cut.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `invalid_request`: a request before 1.3 uses this read.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
