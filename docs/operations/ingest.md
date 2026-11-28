# `ingest`

This page describes the v0.8.0 release.

Add the receipt log of an edge executor to the store. A second ingest of the same entries adds nothing.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `worker` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [ingest.schema.json](../../src/protocol/schema/ingest.schema.json) |

## Example

The request:

```json
{
 "op": "ingest",
 "request_key": "k-1",
 "entries": [
  "e1",
  "e2"
 ]
}
```

The expected response:

```json
{
 "result": {
  "executor_id": "executor-a",
  "log_cursor": 2,
  "added": 2,
  "receipts": [
   "sha256:b526347670e5...",
   "sha256:e227c0db156f..."
  ]
 },
 "reused": false
}
```

The step `first-ingest` of [repeat-ingest.json](../../src/cel-lifecycle/conformance/ingest/repeat-ingest.json) has this request. The Go test `TestIngestVectors` runs it. `<name>` is a value from an earlier step of the same file. Long values are cut.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `upload_not_permitted`: the entries include raw inputs, and the Host context does not permit an upload.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
