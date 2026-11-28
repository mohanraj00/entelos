# `decision_events`

This page describes the v0.8.0 release.

Read the decision events after a cursor, one page at a time.

| | |
|---|---|
| Kind | Read. |
| Role | `event_reader` |
| Protocol | Wire protocol 1.2 and later |
| Contract | [EVENTS.md](../../src/protocol/events/EVENTS.md) |

## Example

The Host request:

```json
{
 "op": "decision_events",
 "limit": 256
}
```

The result has the fields `events`, `next_cursor`, `stream_start`.

The Go test `TestDecisionEventsHostCaptureAndResume` in [decision_events_test.go](../../src/lifecycle-core/decision_events_test.go) sends this request. A value in angle brackets comes from an earlier step of the test.

On the socket, put the fields without `op` in `body`, and the name in `operation`. [README.md](README.md#two-forms-of-a-request) shows the two forms.

## Errors

Each error has a `status` and a `diagnostic` with a `code`. The tests show these codes for this operation:

- `invalid_request`: the cursor is not valid.

Each write can also give `stale_revision`, `idempotency_conflict` and `forbidden`. [PROTOCOL.md](../../src/protocol/PROTOCOL.md#diagnostics) gives the stages of the codes.
