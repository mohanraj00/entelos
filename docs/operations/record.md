# `record`

This page describes the v0.8.0 release.

Read one stored record by its reference and kind.

| | |
|---|---|
| Kind | Read. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [record.schema.json](../../src/protocol/schema/record.schema.json) |

## Example

The request:

```json
{
 "protocol": "1.0",
 "request_id": "l-6",
 "operation": "record",
 "body": {
  "ref": "<s1>",
  "kind": "source"
 }
}
```

The expected response:

```json
{
 "protocol": "1.0",
 "request_id": "l-6",
 "status": "ok",
 "result": {
  "attester": "reviewer-a",
  "format": "entelos.record/1",
  "from": "2026-11-01",
  "function": "record.visible",
  "reason": "initial rule",
  "scope": "tenant-example",
  "supersedes": [],
  "text": "Archived records are not visible.",
  "until": "2026-12-01"
 }
}
```

The step `read-source` of [session-lifecycle.json](../../src/protocol/vectors/session-lifecycle.json) has this request. `verify.py` runs it through the Go sidecar with `test_protocol.py`. `<name>` is a value from an earlier step of the same file. Long values are cut.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `artifact_unavailable`: the reference is not in the store.
- `scope_mismatch`: the record has another scope.
- `invalid_request`: the reference is not a `sha256:` reference.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
