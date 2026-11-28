# `policy_decide`

This page describes the v0.8.0 release.

Write the policy verdict on one report: accept, refuse or route to a person.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Host only: the Go `Handle` call of an embedding application. The wire protocol does not have it |
| Contract | [POLICY.md](../../src/protocol/policy/POLICY.md) |

## Example

The Host request:

```json
{
 "op": "policy_decide",
 "report_ref": "<report_ref>"
}
```

The result has the fields `policy_verdict_ref`.

The Go test `TestPolicyDecideAcceptActivates` in [policy_test.go](../../src/lifecycle-core/policy_test.go) sends this request. A value in angle brackets comes from an earlier step of the test.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `duplicate_verdict`: the report already has a policy verdict.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
