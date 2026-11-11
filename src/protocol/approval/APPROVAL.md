# Approval API 1.1 (proposed)

This file specifies the approval API of issue [#130](https://github.com/mohanraj00/entelos/issues/130). A reviewer client uses it to list the open proposals, read the evidence of one proposal and record a verdict. The reviewer does not need Git. Entelos does not supply a user interface.

Status: **proposed**. No code implements this file. The schemas and vectors in this directory are the contract for the implementation PR. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) checks their structure and their arithmetic. It does not run a Host.

The API extends [wire protocol 1.0](../PROTOCOL.md) to version 1.1. A 1.0 client sees no change.

## Terms

| Term | Meaning |
|---|---|
| Proposal | A `snapshot` record that `propose` wrote. Its reference is the `snapshot_ref`. |
| Evidence | The snapshot, its source record, its bundle text, and each qualification report on it, with the plan of the report. |
| Verdict | A `decision` record that `decide` wrote. Its `disposition` is `accept` or `reject`. |
| Principal | One credential with an actor and a set of roles. The operator writes it. A request cannot select it. |
| Open proposal | A proposal that is the current head of its function and scope, and that has no verdict. |

## What changes

1. The sidecar can authenticate more than one principal. Each request frame runs as the principal of its own credential.
2. The Host has a clock port. It records `submitted_at` on a new snapshot and `decided_at` on a new decision.
3. The Host keeps an index of the reports and the decisions of each snapshot, in `state.reviews`.
4. Two read operations are new: `proposals` and `proposal`.
5. `decide` takes an optional `disposition`. A `reject` verdict is new.
6. `decide` and `activate` refuse an `accept` after a `reject` on the same snapshot.
7. `decide` refuses a second verdict from the same actor on the same snapshot.

Items 6 and 7 make the approval stricter. They do not remove a refusal or a check of 1.0.

## Principals and authentication

The sidecar takes `--credentials-file` or `--token-file`. It refuses to start with both or with neither.

With `--token-file`, the behavior of v0.3.0 stays the same. The context file gives the one actor and its roles.

With `--credentials-file`, the file has this form. The [schema](schema/credentials.schema.json) defines it.

```json
{"format":"entelos.credentials/1","principals":[
  {"token_sha256":"<64 lowercase hex digits>","actor":"reviewer-a","roles":["reviewer"]}
]}
```

These rules apply:

- The file must be a regular file with mode 0600 and the owner of the sidecar process. Else the sidecar does not start.
- The context file must not contain `actor` or `roles`. If it contains one of them, the result is `configuration_error`. Thus each actor comes from one source.
- `token_sha256` is the SHA-256 digest of the 64 ASCII hex digits of the token. The file does not contain a token. A copy of the file does not give a usable credential.
- Each `token_sha256` and each `actor` must be unique. Each role must be a Host role name. A duplicate or an unknown role is `configuration_error`.
- For each frame, the sidecar calculates the digest of the prelude token. It compares that digest with each entry in constant time, and it compares all entries. If no entry agrees, the response is `unauthenticated`, as in 1.0.
- The sidecar reads the file again for each request frame, before it reads the frame length. Thus a removed principal gets `unauthenticated` on its next frame, also on a connection that is already open. If the sidecar cannot read the file, or the file is not valid, the frame gets `unauthenticated` and the Host receives no request.
- The Host context for the frame is the context file plus the `actor` and `roles` of the principal. The client cannot change them.

The digest is a verifier for a secret token. It is not a signature and it gives no authority by itself. The application authenticates the person to its own interface. Entelos authenticates the credential and selects the actor and the roles. A panel that holds the credentials of many reviewers can act as each of them. The operator must issue one credential to each reviewer and give it only to the panel session of that reviewer.

## Clock

The Host gets a clock port. The sidecar and the JSONL host use the UTC system clock, in whole seconds. The format is `YYYY-MM-DDTHH:MM:SSZ`.

- `propose` adds `submitted_at` to the snapshot record.
- `decide` adds `decided_at` to the decision record.
- `proposals` and `proposal` return `as_of`, the clock value of the read.

The clock is not part of the idempotency payload. A retry with the same `request_key` returns the stored result and its stored time. The Host uses the clock for no selection, activation, expiry or refusal. The invocation `date` stays the only date that selects an activation. A recorded time is an operator statement, not a proof of time.

For conformance runs, the sidecar takes `--clock-file`. Before each request, it reads one time value from that file. The vector runner writes the file. The release documentation must state that this flag is for tests.

## Review index

The Host adds `reviews` to the store state. It is an object. Each key is a `snapshot_ref`. Each value is `{"reports":[…],"decisions":[…]}`, in write order.

- `qualify` appends its `report_ref` to the entry of its snapshot.
- `decide` appends its `decision_ref` to the entry of the snapshot of its report.
- A new store starts with `"reviews":{}`. Thus the `inspect-empty` vector of 1.0 changes. The implementation PR must change that expected value and say so.

A store from v0.3.0 has no `reviews`. Its decisions are not in an index, so the Host cannot tell which proposals are open. The checks `duplicate_verdict` and `proposal_rejected` also need the complete verdict history of a snapshot. Thus the Host fails closed on such a store:

- `proposals`, `proposal`, `qualify`, `decide` and `activate` refuse with `index_unavailable`. The Host writes no record and does not make a partial index.
- The other operations do not use the index. For example, `resolve`, `invoke`, `replay` and `restate` work as in 1.0.

Thus a v0.3.0 store cannot get a new report, verdict or activation. There is no migration in this version. A later migration must build the index from all the stored decisions before the Host accepts it.

## Proposal states

The Host calculates one state for each proposal, in this order. The first rule that applies gives the state.

| State | Rule |
|---|---|
| `rejected` | A verdict on the snapshot has `disposition` `reject`. |
| `accepted` | A verdict on the snapshot has `disposition` `accept`. |
| `superseded` | The snapshot is not the current head of its function and scope. |
| `awaiting_qualification` | The snapshot has no report. |
| `awaiting_verdict` | The report refusal of the last report is an empty string. |
| `not_approvable` | The report refusal of the last report is not empty. |

The *report refusal* of a report is the code of the first `accept` check that fails among checks 1, 2 and 6 of `decide` (below), or an empty string. These checks do not use the reason or the actor of a request. Thus `invalid_request`, `separation_required` and `duplicate_verdict` never show here.

The proposal item has `refusal`. For `not_approvable`, it is the report refusal of the last report. For each other state, it is an empty string.

## Operations

All three operations need the `reviewer` role. A principal without it gets `forbidden`. A request with protocol `1.0` for `proposals` or `proposal` gets `invalid_request`, because 1.0 has no such operation.

### `proposals` (read)

Body: `{"cursor"?: string}`. The [schema](schema/proposals.schema.json) defines the body and the result.

Result: `{"cursor", "as_of", "proposals": [item, …]}`. The list contains each open proposal of the host scope and of the host functions. It omits a proposal of another scope or function. It is not a refusal. The order is by `submitted_at`, the oldest first, then by `snapshot_ref`. A snapshot without `submitted_at` comes last.

A proposal item has these fields:

| Field | Value |
|---|---|
| `snapshot_ref`, `function`, `scope`, `from`, `until`, `producer`, `reason`, `bundle_ref`, `stale_mode` | Copied from the snapshot. |
| `submitted_at` | From the snapshot. Omitted if the snapshot has none. |
| `wait_seconds` | `as_of` minus `submitted_at`, in seconds. Omitted if `submitted_at` is omitted. The Host does not clamp a negative value, so a clock that goes back is visible. |
| `state`, `refusal` | From the state table. |
| `report_refs`, `decision_refs` | From `state.reviews`, in write order. |

### `proposal` (read)

Body: `{"snapshot_ref": string, "cursor"?: string}`. The [schema](schema/proposal.schema.json) defines the body and the result.

The Host reads the snapshot as `receipt` reads a receipt: an unknown reference is `artifact_unavailable`, a record of another kind is `invalid_record`, and a snapshot of another scope or function is `out_of_scope`.

Result:

| Field | Value |
|---|---|
| `cursor`, `as_of` | As in `proposals`. |
| `proposal` | The proposal item. It can have any state. |
| `snapshot`, `source` | The snapshot record and its source record. |
| `bundle` | An object from each bundle component name to its stored text. |
| `reports` | One entry for each report, in write order: `{"report_ref", "report", "plan_ref", "plan", "refusal"}`. `refusal` is the report refusal of this report. |
| `decisions` | One entry for each verdict, in write order: `{"decision_ref", "decision"}`. |

The records are the stored records. The schema requires `format` `entelos.record/1` on each record and does not define the other fields. Thus a change of the case or value types (issue [#126](https://github.com/mohanraj00/entelos/issues/126)) does not change this schema.

### `decide` (write)

Body in 1.1: `{"report_ref", "reason", "disposition"?, "expected_revision", "request_key"}`. `disposition` is `accept` (the default) or `reject`. The [schema](schema/decide.schema.json) defines it. In 1.0, a body with `disposition` stays `invalid_request`.

The Host does the checks in this order. The first check that fails gives the refusal.

Before each other check, the store must have `state.reviews`: `index_unavailable`.

For `accept`:

1. The report and its snapshot are in scope: `artifact_unavailable`, `invalid_record` or `out_of_scope`.
2. The report checks of 1.0: `qualification_failed`, `untrusted_runner`, `implementation_unavailable`, `stale_qualification`.
3. `reason` is not empty: `invalid_request`.
4. The actor differs from the producer and the runner: `separation_required`.
5. The actor has no verdict on this snapshot: `duplicate_verdict`.
6. No verdict on this snapshot is a `reject`: `proposal_rejected`.

For `reject`:

1. The same scope check as `accept`.
2. `reason` is not empty: `invalid_request`.
3. The same separation check: `separation_required`.
4. The same duplicate check: `duplicate_verdict`.

A `reject` does not need a passed report. A reviewer can reject a failed or stale proposal. A `reject` is final for the snapshot. To change the rule, propose a new revision.

The decision record has the fields of 1.0 and one new field:

```json
{"format":"entelos.record/1","function":"…","scope":"…","from":"…","until":"…",
 "snapshot_ref":"…","report_ref":"…","actor":"reviewer-a","reason":"…",
 "disposition":"accept","runner_policy":"fixture-runner","decided_at":"2026-11-22T09:00:00Z"}
```

The result is `{"cursor", "result": {"decision_ref", "decision"}, "reused"}`. `decision` is the stored record. For a 1.0 request, the adapter returns only `decision_ref`, so the 1.0 result does not change.

The record has no field that names the transport. The JSONL host and the sidecar write the same record for the same request, actor and clock value. The record bytes are equal, so the `decision_ref` values are equal.

Two verdicts with `accept` from different actors are permitted. Both stay in the history. This keeps space for the verdict counts of issue [#128](https://github.com/mohanraj00/entelos/issues/128).

### `activate` (write)

`activate` keeps its 1.0 body and checks. Before those checks, it refuses with `index_unavailable` if the store has no `state.reviews`. After those checks, it refuses with `proposal_rejected` if a verdict on the snapshot is a `reject`.

## Refusal codes

The codes in **bold** are new in 1.1. The other codes have the meaning of 1.0.

| Code | Stage | Operations | Condition |
|---|---|---|---|
| `unauthenticated` | `authentication` | all | No principal agrees with the prelude token. |
| `forbidden` | `authority` | all | The principal does not have the role of the operation. |
| `invalid_request` | `protocol` or Host | all | The body does not agree with its schema, the 1.0 request names a 1.1 operation or field, or `reason` is empty. |
| `unsupported_version` | `protocol` | all | The major version is not 1, or the minor version is more than 1. |
| `artifact_unavailable` | `store` | `proposal`, `decide` | The reference is not in the store. |
| `invalid_record` | `store` | `proposal`, `decide` | The reference is a record of another kind. |
| `out_of_scope` | Host | `proposal`, `decide` | The record has another scope or a function that is not installed. |
| **`index_unavailable`** | `store` | `proposals`, `proposal`, `qualify`, `decide`, `activate` | The store has no `state.reviews`. |
| `qualification_failed` | `admission` | `decide` accept | The report did not pass, or its bindings differ. |
| `untrusted_runner` | `admission` | `decide` accept | The report is not from the installed runner. |
| `implementation_unavailable` | `admission` | `decide` accept | The qualified build is not installed. |
| `stale_qualification` | `admission` | `decide` accept | The source or the plan has been superseded on the interval. |
| `separation_required` | `admission` | `decide` | The actor is the producer or the runner. |
| **`duplicate_verdict`** | `admission` | `decide` | The actor already has a verdict on this snapshot. |
| **`proposal_rejected`** | `admission` | `decide` accept, `activate` | A verdict on this snapshot is a `reject`. |
| `stale_revision` | `store` | `decide` | `expected_revision` is not the current root. |
| `idempotency_conflict` | `store` | `decide` | The `request_key` was used with another payload. |
| `response_too_large` | `protocol` | `proposals`, `proposal` | The result is larger than the frame bound. |

## Protocol version

A 1.1 receiver accepts the minor versions 0 and 1. A response has the version of its request when the receiver supports that version. A response to a request with a missing, invalid or unsupported version has `"protocol":"1.1"`. Thus the 1.0 vectors keep their expected values, except the version of the `unsupported_version` responses and the `inspect-empty` state. The implementation PR must change those values and name each one.

## Vectors

`vectors/approval-*.json` use the session format of [PROTOCOL.md](../PROTOCOL.md#vectors), with these additions:

- `principals` at the top level maps a principal name to `{"actor", "roles"}`. The runner makes a random token for each principal and writes the credentials file. The `context` has no `actor` or `roles`.
- `clock` at the top level is the start time. A step can have `clock`, a later time. The runner writes the clock file before the step. A step without `clock` uses the time of the previous step.
- Each step has `principal`, the name of one principal. A step with `"principal":"$unknown"` sends a token that is not in the file.
- `{"$record_bundle":{"allowed_state":[…]}}` in a request is the record-filter bundle that [`prepare.py`](../../cel-handoff/tools/prepare.py) made, with that fixed value. The runner makes it as the [sidecar client](../../../examples/sidecar-client/run.py) does.
- `{"$subset":{…}}` in an expected value matches an object that has each of these fields with an equal value. Other fields can be present.
- `{"$type":"object"}` matches any object.
- `{"$wait":{"since":"snap-1","seconds":172800}}` in an expected value matches the integer `seconds`. `since` names the `$capture` of a `propose` step. The checker uses it to compare the hand-written value with the step clocks.
- `protocol_version` at the top level is `1.1`. It names the receiver version that the file needs. The 1.0 runner reads only `../vectors/session-*.json`, so it does not read these files.
- `parity` at the top level names the steps that the runner also sends to the JSONL host, with the same principal and clock. The `$capture` values of those steps must be equal on both transports.

`check_spec.py` checks these rules without a Host:

1. Each request body of an approval operation agrees with its schema in this directory. Each other operation has a schema in [`../schema/`](../schema/).
2. Each expected success result of an approval operation agrees with its result schema.
3. Each expected failure code is in the refusal table of this file, and each new code is in a vector or in the coverage limits below.
4. Each `$wait` value equals the clock of its step minus the clock of the `propose` step that it names.
5. A self-test changes a copy of the vectors and checks that the checker reports each change.

## Coverage limits

- `index_unavailable` needs a store from v0.3.0. The implementation PR adds a Go test with such a store for each of the five operations. The test also checks that the root does not change. No vector covers it.
- The vectors do not remove a principal while a connection is open. The implementation PR adds a Go test: a frame after the removal on the same connection gets `unauthenticated`.
- `untrusted_runner`, `implementation_unavailable`, `stale_qualification`, `stale_revision`, `idempotency_conflict` and `response_too_large` keep their 1.0 tests. The approval vectors do not repeat them.
- The vectors do not test the file mode and owner checks of the credentials file. The implementation PR adds Go tests for them.
- The vectors do not prove constant-time comparison. A code review must check it.
- The vectors install one function, so a list never has two open proposals. The implementation PR adds a Go test for the order of the list.

## Limits

- The list has no pagination. A store with many open proposals can get `response_too_large`. A later version can add a page cursor.
- `proposals` shows only the head snapshot of each function and scope. An older snapshot without a verdict is `superseded`. `decide` can still accept it, as in 1.0. `proposal` shows its state.
- The existing `record` read does not check the scope of the record. This API does not use it. That gap is outside this spec.
- Issue [#127](https://github.com/mohanraj00/entelos/issues/127) (a verdict stale after an edit) and issue [#128](https://github.com/mohanraj00/entelos/issues/128) (human and model verdicts) are in v0.5. This version adds no verdict kind and no digest rule. A later minor version can add fields to the decision record.
- A verdict records who decided and when. It does not record what the reviewer looked at.
