# `ingest`

This page describes the v1.0.0 release.

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
  {
   "cursor": 1,
   "snapshot_ref": "sha256:f15995b04405...",
   "date": "2026-11-15",
   "inputs_ref": "sha256:d28ea6c24237...",
   "response": {
    "status": "ok",
    "value": true,
    "bundle_ref": "sha256:bbbbbbbbbbbb...",
    "implementation_ref": "sha256:eeeeeeeeeeee..."
   }
  },
  {
   "cursor": 2,
   "snapshot_ref": "sha256:f15995b04405...",
   "date": "2026-11-15",
   "inputs_ref": "sha256:769c9068dee9...",
   "response": {
    "status": "ok",
    "value": false,
    "bundle_ref": "sha256:bbbbbbbbbbbb...",
    "implementation_ref": "sha256:eeeeeeeeeeee..."
   }
  }
 ],
 "expected_revision": "<cursor>"
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

The step `first-ingest` of [repeat-ingest.json](../../src/cel-lifecycle/conformance/ingest/repeat-ingest.json) has this request. The Go test `TestIngestVectors` runs it. The vector names the entries `e1` and `e2`. The test puts the entries of the vector in their place, as shown here, and adds the current cursor as `expected_revision`. `<name>` is a value from an earlier step of the same file. An object of the expected response can have more fields than the vector names. Long values are cut.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `upload_not_permitted`: the entries include raw inputs, and the Host context does not permit an upload.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
