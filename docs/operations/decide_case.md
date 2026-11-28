# `decide_case`

This page describes the v0.8.0 release.

Accept or reject a candidate test case. `plan` takes only an accepted case.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Host only: the Go `Handle` call of an embedding application. The wire protocol does not have it |
| Contract | [FINDING.md](../../src/protocol/../lifecycle-core/FINDING.md) |

## Example

The Host request:

```json
{
 "op": "decide_case",
 "candidate_ref": "<candidate_ref>",
 "disposition": "accept",
 "reason": "read"
}
```

The contract gives the result.

The Go test `TestFindingCaseFailsOnTheOldSnapshotAndPassesOnTheCorrectedSnapshot` in [finding_test.go](../../src/lifecycle-core/finding_test.go) sends this request. A value in angle brackets comes from an earlier step of the test.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `forbidden`: the actor is not a reviewer.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
