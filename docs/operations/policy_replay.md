# `policy_replay`

This page describes the v0.8.0 release.

Run a stored policy verdict again and compare the result.

| | |
|---|---|
| Kind | Read. |
| Role | `reviewer` |
| Protocol | Host only: the Go `Handle` call of an embedding application. The wire protocol does not have it |
| Contract | [POLICY.md](../../src/protocol/policy/POLICY.md) |

## Example

The Host request:

```json
{
 "op": "policy_replay",
 "policy_verdict_ref": "<policy_verdict_ref>"
}
```

The contract gives the result.

The Go test `TestPolicyVerdictLifecycleOnCEL` in [policy_test.go](../../src/lifecycle-core/policy_test.go) sends this request. A value in angle brackets comes from an earlier step of the test.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `artifact_unavailable`: the policy verdict is not in the store.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
