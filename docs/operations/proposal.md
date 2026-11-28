# `proposal`

This page describes the v0.8.0 release.

Read one proposal with its reports, verdicts and evidence.

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
 "request_id": "v-8",
 "operation": "proposal",
 "body": {
  "snapshot_ref": "<snap-1>"
 }
}
```

The expected response:

```json
{
 "protocol": "1.1",
 "request_id": "v-8",
 "status": "ok",
 "result": {
  "cursor": "<string>",
  "as_of": "2026-11-22T09:00:00Z",
  "proposal": {
   "snapshot_ref": "<snap-1>",
   "function": "record.visible",
   "scope": "demo",
   "from": "2026-10-01",
   "until": "2026-12-01",
   "producer": "author-a",
   "reason": "initial proposal",
   "bundle_ref": "<bundle-1>",
   "stale_mode": "fail_closed",
   "state": "awaiting_verdict",
   "refusal": "",
   "report_refs": [
    "<report-1>"
   ],
   "decision_refs": [],
   "submitted_at": "2026-11-20T09:00:00Z",
   "wait_seconds": 172800
  },
  "snapshot": {
   "format": "entelos.record/1",
   "producer": "author-a",
   "submitted_at": "2026-11-20T09:00:00Z",
   "bundle_ref": "<bundle-1>"
  },
  "source": {
   "format": "entelos.record/1",
   "text": "<75 characters>"
  },
  "bundle": {
   "manifest": "<string>",
   "environment": "<string>",
   "source": "<string>",
   "fixed": "<string>"
  },
  "reports": [
   "<object with 5 fields>"
  ],
  "decisions": []
 }
}
```

The step `read-evidence` of [approval-verdicts.json](../../src/protocol/approval/vectors/approval-verdicts.json) has this request. The Go test `TestApprovalVectors` runs it through the sidecar socket. `<name>` is a value from an earlier step of the same file. An object of the expected response can have more fields than the vector names. Long values are cut.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `artifact_unavailable`: the snapshot is not in the store.
- `unsupported_subject`: a 1.1 or 1.2 request reads a content proposal.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
