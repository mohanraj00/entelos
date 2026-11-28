# `import_finding`

This page describes the v0.8.0 release.

Import a finding record as a candidate test case for a source.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `author` |
| Protocol | Host only: the Go `Handle` call of an embedding application. The wire protocol does not have it |
| Contract | [FINDING.md](../../src/protocol/../lifecycle-core/FINDING.md) |

## Example

The Host request:

```json
{
 "op": "import_finding",
 "source_ref": "<source_ref>",
 "finding": "<finding record text>",
 "expected_revision": "<cursor>",
 "request_key": "<request key>"
}
```

The result has the fields `candidate_ref`.

The Go test `TestImportFindingGivesTheCandidate` in [finding_test.go](../../src/lifecycle-core/finding_test.go) sends this request. A value in angle brackets comes from an earlier step of the test. The test helper adds the current cursor as `expected_revision` and a `request_key`, as each write needs.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `invalid_request`: the finding record is not valid, or `source_ref` is not a source.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
