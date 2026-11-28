# `proposals`

This page describes the v1.0.0 release.

List the open proposals, one page at a time.

| | |
|---|---|
| Kind | Read. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.1 and later |
| Contract | [APPROVAL.md](../../src/protocol/approval/APPROVAL.md) |

## Example

The request:

```json
{
 "protocol": "1.1",
 "request_id": "v-3",
 "operation": "proposals",
 "body": {}
}
```

The expected response:

```json
{
 "protocol": "1.1",
 "request_id": "v-3",
 "status": "ok",
 "result": {
  "cursor": "<string>",
  "as_of": "2026-11-20T09:00:00Z",
  "proposals": []
 }
}
```

The step `list-before-propose` of [approval-verdicts.json](../../src/protocol/approval/vectors/approval-verdicts.json) has this request. The Go test `TestApprovalVectors` runs it through the sidecar socket. `<name>` is a value from an earlier step of the same file. An object of the expected response can have more fields than the vector names. Long values are cut.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `forbidden`: the actor is not a reviewer.
- `unauthenticated`: the token is not known.
- `invalid_request`: the request has a field that a read does not take, for example `request_key`.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
