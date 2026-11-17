# Supersession of an activated update

This file specifies how a new activation supersedes an earlier one. Issue [#129](https://github.com/mohanraj00/entelos/issues/129) tracks it.

Status: **implemented in the Go Host, experimental**. The vectors in [`vectors/supersession-cases.json`](vectors/supersession-cases.json) have expected values that a person wrote from this text. No Host output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors with its own model of the selection. `TestSupersessionVectors` runs the same vectors on the Go Host.

## Terms

| Term | Meaning |
|---|---|
| Update | One activated revision: an `enable` event with a half-open interval from `from` to `until`. |
| Mask | An entry in `targets` of a later event. It removes an event only at the dates in the interval of the later event. |
| Supersession | An entry in `supersedes` of a later `enable` event. It removes an update at each date from the `from` date of the later event. It has no end. |
| Successor | The update that supersedes an earlier update. |

## Example

Update U1 is active from 2026-01-01. Update U2 corrects it and supersedes it from 2026-03-01.

- A replay of an answer from February uses U1, and gives the same answer as before.
- A new request with a date in February also selects U1. That date is before the supersession.
- A new request with a date on or after 2026-03-01 never selects U1. If U2 is live at the date, the request selects U2. If no update is live, the result is `out_of_scope`.

## Request

`activate` has an optional field `supersedes`. It is an array of event references. An absent field is the same as an empty array. The JSONL Host, Go API and wire protocol 1.2 accept it. Wire protocol 1.0 and 1.1 refuse the field with `invalid_request` at the protocol stage. See [#180](https://github.com/mohanraj00/entelos/issues/180).

The Host accepts the request only if each entry agrees with these rules. Else the result is `invalid_supersession`, and the write stores nothing.

1. The entry is the reference of an event of the same function and scope.
2. The event is an `enable` event. A `suspend` event cannot be superseded.
3. The `from` date of the new activation is before the `until` date of the event. A supersession that starts at or after the end of the update changes nothing, so the Host refuses it.
4. The array has no entry two times.

The new activation can start before the superseded update. Then the superseded update is live at no date.

`activate` gives `ambiguous_activation` if a live update overlaps the new interval and the request names it in neither `targets` nor `supersedes`. A superseded update does not also need an entry in `targets`.

## Record

The activation record is the `event` record. If the request supersedes one or more updates, the record has the field `supersedes` with their event references, in the order of the request. A record that supersedes nothing has no `supersedes` field, so its bytes are the same as before this change.

A Host with a signing key also writes a signed record of each activation record. See [SIGNED.md](../signed/SIGNED.md). The signature thus covers the `supersedes` field.

## Selection

The Host finds the live update of a function, a scope and a date from the events in their stored order. For each event of the function and scope:

1. If the date is on or after the `from` date of the event, remove each update in its `supersedes` field from the live set.
2. If the date is in the interval of the event, remove each event in its `targets` field from the live set. Then, if the event is an `enable` event, add it to the live set.

One live update is the selection. No live update gives `out_of_scope`. More than one gives `ambiguous_activation`.

These results follow from the rules:

- A superseded update is not selected at a date on or after the start of its successor. This is true after the successor ends, and after a `suspend` event masks the successor. To restore an earlier rule, activate it again.
- A superseded update is selected at a date before the start of its successor, as before.
- In a chain U1, U2, U3, where each update supersedes the one before it, each date selects one update at most.

## Answers and replay

- `replay` runs the selection of the old receipt. It does not select again. Thus the replay of an old answer gives the same result as before, also after a supersession of its update.
- `restate` selects again at a later cursor for the date of the old receipt, with the rules of the Selection section.
- `invoke` uses the live update at the date. If no update is live, `invoke` examines the activation that the worker loaded, as [CONTRACTS.md](../../../docs/CONTRACTS.md#expiry-and-revocation) says. If a later activation supersedes the loaded activation at the date, the result is `activation_superseded`. This applies in each stale mode, also `continue_stale`. Thus a superseded update never gives a new answer on or after the start of its successor.

## Limits

- A supersession cannot be removed. It is a record, and records are not deleted.
- The check does not examine if the successor is a correct replacement. The reviewer who activates it states that.
- The vectors run with a fixture runner and a fake engine. They show the selection and the replay binding. They do not run a CEL rule.
