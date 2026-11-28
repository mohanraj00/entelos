# `promotion`

This page describes the v1.0.0 release.

Read the promotion state of one qualification report.

| | |
|---|---|
| Kind | Read. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.3 |
| Contract | [PROMOTION.md](../../src/protocol/promotion/PROMOTION.md) |

## Example

The Host request:

```json
{
 "op": "promotion",
 "report_ref": "<report_ref>"
}
```

The result has the fields `cursor`, `report_ref`, `snapshot_ref`, `promotion`, `inputs`, `rule`.

The Go test `TestPromotionRead` in [promotion_test.go](../../src/lifecycle-core/promotion_test.go) sends this request. A value in angle brackets comes from an earlier step of the test.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `artifact_unavailable`: the report is not in the store.
- `invalid_request`: a request before 1.3 uses this read.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
