# `activate`

This page describes the v0.8.0 release.

Make an approved decision effective for its date interval. `targets` names the earlier events that the activation replaces.

| | |
|---|---|
| Kind | Write. It takes `expected_revision` and `request_key`. |
| Role | `reviewer` |
| Protocol | Wire protocol 1.0 and later |
| Contract | [activate.schema.json](../../src/protocol/schema/activate.schema.json) |

## Example

The Host request, as the JSONL host `lifecycle-core` reads it:

```json
{
 "decision_ref": "sha256:c469fa50dd62...",
 "expected_revision": "45779a393cba...",
 "op": "activate",
 "request_key": "initial-activate",
 "targets": []
}
```

The response:

```json
{
 "data": {
  "cursor": "ef3f35da8fef...",
  "result": {
   "event_ref": "sha256:03d902de6302..."
  },
  "reused": false
 },
 "status": "ok"
}
```

The step `initial-activate` of [scenario.py](../../src/lifecycle-core/client/scenario.py) sends this request as the actor `reviewer`. `verify.py` and the README demo run it. Long values are cut, and `...` ends a cut reference.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `forbidden`: the actor is not a reviewer.
- `ambiguous_activation`: the activation leaves two live events for one date, because `targets` does not name the earlier event.
- `idempotency_conflict`: the `request_key` was used for a different request.
- `proposal_rejected`: a reviewer rejected the proposal.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
