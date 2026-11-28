# Operations

This page describes the v1.0.0 release.

Each page gives one operation: its purpose, a tested example, the result and the errors.

## Two forms of a request

The JSONL host `lifecycle-core` reads one object with `op` and the fields:

```json
{"op":"describe","function":"record.visible","date":"2026-11-15"}
```

The sidecar socket reads an envelope. `operation` is the name, and `body` has the same fields:

```json
{"protocol":"1.0","request_id":"r-1","operation":"describe","body":{"function":"record.visible","date":"2026-11-15"}}
```

[PROTOCOL.md](../../src/protocol/PROTOCOL.md) gives the frame, the token and the versions. A Go application calls `Host.Handle` with the first form.

## Author and approve

| Operation | Purpose |
|---|---|
| [`source`](source.md) | Record an approved source text for one function and one date interval. |
| [`plan`](plan.md) | Record the independent test cases for one source. |
| [`propose`](propose.md) | Propose a candidate for a function and a date interval: a CEL bundle, or a content manifest in 1.3. |
| [`qualify`](qualify.md) | Run the plan cases against one snapshot and record a report with the status `passed`, `failed` or `unresolved`. |
| [`decide`](decide.md) | Record the verdict of a reviewer on one qualification report. |
| [`activate`](activate.md) | Make an approved decision effective for its date interval. |
| [`suspend`](suspend.md) | Stop the active events of a function for a date interval, with no replacement. |

## Call

| Operation | Purpose |
|---|---|
| [`describe`](describe.md) | Give the caller contract of a function for a date: the input schema, the exact bundle reference and the store cursor. |
| [`load`](load.md) | Prepare the active bundle of a function for a date on the worker. |
| [`ack`](ack.md) | Acknowledge one outbox event after the worker loaded it. |
| [`invoke`](invoke.md) | Answer one call with the loaded bundle and record a receipt. |
| [`resolve`](resolve.md) | Select the active snapshot of a function for a date, at the current cursor or at an earlier `cursor`. |
| [`receipt`](receipt.md) | Read one receipt: the snapshot, the input, the build, the answer and the stale state. |

## Correct

| Operation | Purpose |
|---|---|
| [`replay`](replay.md) | Run an earlier receipt again on its own snapshot and build. |
| [`restate`](restate.md) | Give a new, separate answer for the input and date of an earlier receipt, at a later cursor. |
| [`observe`](observe.md) | Report a problem with one receipt. |
| [`claim`](claim.md) | Take an open correction task. |
| [`checkpoint`](checkpoint.md) | Save the progress of a claimed task, or complete it. |
| [`takeover`](takeover.md) | Give a claimed task to a new owner. |

## Review reads

| Operation | Purpose |
|---|---|
| [`proposals`](proposals.md) | List the open proposals, one page at a time. |
| [`proposal`](proposal.md) | Read one proposal with its reports, verdicts and evidence. |
| [`approval`](approval.md) | Read the verdict state of one candidate: `current`, `stale`, `rejected` or `unknown`. |
| [`evidence`](evidence.md) | Read the admission evidence of one proposal and the result of its rules. |
| [`promotion`](promotion.md) | Read the promotion state of one qualification report. |
| [`decision_events`](decision_events.md) | Read the decision events after a cursor, one page at a time. |
| [`inspect`](inspect.md) | Read the full lifecycle state at the current cursor. |
| [`record`](record.md) | Read one stored record by its reference and kind. |

## Edge executor

| Operation | Purpose |
|---|---|
| [`ingest`](ingest.md) | Add the receipt log of an edge executor to the store. |

## Host only

| Operation | Purpose |
|---|---|
| [`policy_decide`](policy_decide.md) | Write the policy verdict on one report: accept, refuse or route to a person. |
| [`policy_replay`](policy_replay.md) | Run a stored policy verdict again and compare the result. |
| [`import_finding`](import_finding.md) | Import a finding record as a candidate test case for a source. |
| [`decide_case`](decide_case.md) | Accept or reject a candidate test case. |
