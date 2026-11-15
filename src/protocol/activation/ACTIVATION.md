# Activation record

This file specifies a generic activation record. Issue [#105](https://github.com/mohanraj00/entelos/issues/105) tracks it. An activation record is one self-contained statement of an activation: the candidate, its base, the receipts, the actors, the partition, the interval and the applicability values. A consumer, for example a store with gated collections, verifies it offline and makes no call to entelos.

Status: **proposed**. No Host writes this record yet. The schema and the vectors in this directory are the contract. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors, without a Host.

## Terms

| Term | Meaning |
|---|---|
| Event | The stored `enable` event of an activation. [SIGNED.md](../signed/SIGNED.md) calls it an activation record of the kind `event`. |
| Activation record | A stored record of the kind `activation`. The Host writes it from the event and the records that the event names. |
| Signed activation record | A signed record ([SIGNED.md](../signed/SIGNED.md)) of an activation record. |
| Consumer | A system that receives a signed activation record and a key set, and checks them with no call to entelos. |
| Publisher | An optional adapter of the embedding host. It gives a signed activation record to a consumer. |

## Decision

1. The activation record is a new record kind, `activation`. It is a lifecycle record of the format `entelos.record/1`, as each other record of the Host.
2. The Host writes one activation record for each `enable` event, in the same transaction, when the Host has a signing key. It signs the activation record as it signs the event. Without a signing key, the Host writes no activation record, and its behavior does not change.
3. The record copies its values from the event, the snapshot, the qualification report and the decision. It adds no value that the store does not have. Thus the record is a function of the stored records and the request key.
4. The record has no time field. A retry of the same `activate` request makes the same bytes, the same reference and the same signature.
5. The publisher is optional. Entelos core does not need a consumer or a store. The Host calls the publisher before it commits the activation. If the publisher refuses, the activation does not become active, and the Host stores no record.
6. The event and its signed record do not change. The activation record is an additional record.

The reason for decision 5: a consumer can refuse an update, for example because its head is not the `base`. Then the update must not become active in entelos either. A commit can still fail after the consumer accepts. Decision 4 makes the retry safe, because the consumer gets the same bytes again.

## Activation record

An activation record is one JSON object. [`schema/activation.schema.json`](schema/activation.schema.json) gives its fields. A field that is not in this table is not permitted.

| Field | Type | Value |
|---|---|---|
| `format` | string | `entelos.record/1`. |
| `scope` | string | The `scope` of the event. |
| `function` | string | The `function` of the event. For a content candidate, it is the subject `content:<collection>/<partition>`. |
| `event_ref` | string | The reference of the event. |
| `request_key` | string | The `request_key` of the `activate` request, or an empty string if the request has none. It is for audit and idempotence. |
| `candidate` | object | `{"kind", "ref"}`. For a content snapshot, the `candidate` of the snapshot. For a CEL snapshot, `{"kind":"cel-bundle","ref":<bundle_ref>}`. |
| `base` | string | The candidate `ref` that this candidate changes, or an empty string. For a content snapshot, the `base` of the snapshot. For a CEL snapshot, the `bundle_ref` of its `base_snapshot`, or an empty string if it has none. |
| `collection` | string | Only for a content candidate: the `collection` of the snapshot. |
| `partition` | string | Only for a content candidate: the `partition` of the snapshot. |
| `from` | string | The `from` date of the event. The interval includes this date. |
| `until` | string | The `until` date of the event. The interval does not include this date. |
| `source_ref` | string | The `source_ref` of the snapshot. |
| `snapshot_ref` | string | The `snapshot_ref` of the event. |
| `report_ref` | string | The `report_ref` of the event: the qualification receipt. |
| `decision_ref` | string | The `decision_ref` of the event: the approval receipt. |
| `producer` | string | The `producer` of the snapshot. |
| `runner` | string | The `runner` of the report. |
| `approver` | string | The `actor` of the decision. |
| `activator` | string | The `actor` of the event. |
| `targets` | array | The `targets` of the event: the references of the updates that the event masks. |
| `supersedes` | array | The `supersedes` of the event, or an empty array. |
| `applicability` | object | The `applicability` of the event ([#106](https://github.com/mohanraj00/entelos/issues/106)), or an empty object. It has the values of each item, by `item_id`. |

A reference is `sha256:` and 64 lowercase hex digits. A date has the form `YYYY-MM-DD`. The record does not copy the text of the manifest, of the source or of the evidence. The consumer gets them by reference.

## Mapping

This table gives the origin of each field. The Host reads each record in the same transaction as the event.

| Origin | Fields |
|---|---|
| The event | `scope`, `function`, `from`, `until`, `snapshot_ref`, `report_ref`, `decision_ref`, `targets`, `supersedes`, `applicability`; `actor` as `activator`; its reference as `event_ref` |
| The snapshot | `candidate` (or `bundle_ref` for a CEL snapshot), `base` (or the `base_snapshot` for a CEL snapshot), `collection`, `partition`, `source_ref`, `producer` |
| The base snapshot of a CEL snapshot | its `bundle_ref` as `base` |
| The report | `runner` |
| The decision | `actor` as `approver` |
| The request | `request_key` |

The Host writes an activation record only for an `enable` event. A `suspend` event has no activation record.

## Rules

A consumer applies these rules to the record bytes, after the checks of [SIGNED.md](../signed/SIGNED.md), in this order. The first rule that fails gives the result.

| Step | Rule | Result |
|---|---|---|
| 1 | The `record_kind` of the signed record is `activation`. | `unexpected_record_kind` |
| 2 | The record bytes are one JSON object. No object in the text has the same member name two times. | `invalid_activation`, rule `json` |
| 3 | `format` is `entelos.record/1`. | `unsupported_record_format` |
| 4 | Each member name in the record is a field of the record table. | `invalid_activation`, rule `unknown_field` |
| 5 | The record agrees with the schema. | `invalid_activation`, rule `schema` |
| 6 | The record has `collection` and `partition` if, and only if, the `kind` of `candidate` is `content`. | `invalid_activation`, rule `candidate_fields` |
| 7 | For a content candidate, `function` is `content:` and `collection`, `/` and `partition`. | `invalid_activation`, rule `subject` |
| 8 | Each date is a real calendar date, and `from` is before `until`. | `invalid_activation`, rule `interval` |

The refusal of rule `unknown_field` gives the path of the first unknown member name, in the order of the text, for example `candidate.digest`. The names in `applicability` are data, not fields.

A record that passes all of the rules is valid. The result gives the record. A valid record shows what the signer activated. These checks stay with the consumer:

- The `scope`, the `function`, or the `collection` and the `partition`, are the head that the consumer keeps.
- The `base` is the candidate that the consumer has as the current head. Else the consumer refuses the update.
- The `candidate` names the bytes that the consumer receives. For content, the consumer calculates the SHA-256 of the manifest text and compares it with the `ref`.
- The interval and the `applicability` values are the filters that the consumer applies. Entelos does not interpret them.
- The key set is the one that the consumer trusts for this signer.

## Publisher

The publisher is an adapter of the embedding host. The Go interface comes with the Host change. It has one operation: give the bytes of the activation record and of its signed record to a consumer, and get an accept or a refusal with a reason.

- The Host calls the publisher in the `activate` write, after it stages and signs the records, and before it commits them.
- On an accept, the Host commits the write.
- On a refusal, the Host commits nothing. The result is `publish_refused` with the stage `publish`, and the message has the reason of the consumer.
- On a failure of the commit after an accept, the consumer has a record that the store does not have. A retry of the same request makes the same bytes, and the consumer gets them again. The consumer must accept a record that it already has.
- A Host with a publisher and no signing key refuses each `activate` with `publish_unsigned`, because a consumer cannot verify the record.

| Code | Use |
|---|---|
| `publish_refused` | The publisher refused the activation record. The message has the reason. |
| `publish_unsigned` | The Host has a publisher and no signing key. |

## Vectors

[`vectors/activation-cases.json`](vectors/activation-cases.json) has two lists.

Each item of `mappings` has:

- `name`: a unique name;
- `records`: the `event`, the `snapshot`, the `report`, the `decision`, and for a CEL snapshot with a base the `base_snapshot`, as objects;
- `event_ref` and `request_key`;
- `expect`: the activation record, or `{"none": true}` for an event that has no activation record.

Each item of `records` has:

- `name`: a unique name;
- `record_kind` and `record`: the record kind of the signed record and the record text, as a string;
- `expect`: `{"valid": true}`, or `{"valid": false, "code": <code>}` with `"rule"`, and `"field"` for rule `unknown_field`.

`check_spec.py` does these checks:

1. It applies the mapping of this file to each item of `mappings`, and compares the result with `expect`.
2. It checks each expected record of `mappings` against the rules.
3. It applies the rules of this file to each item of `records`, and compares the result with `expect`.
4. It checks that each rule and each code of the rules table has one or more cases.
5. A self-test changes a copy of the vectors and checks that the checker reports each change.

The checker applies the same rules that the Host will apply. Agreement of the checker and the vectors shows that the two agree with each other.

## Limits

- No Host writes the record yet. The Host change, the publisher interface, the record kind `activation` in [SIGNED.md](../signed/SIGNED.md) and a Go reader come in a later change of #105.
- The record has no trusted time. The signature shows who signed, not when.
- A `suspend` event has no activation record. A consumer cannot learn of a suspension from this record.
- The Host stores no record of a refused publish. The caller gets the code and the reason.
- An activation before the signing key has no activation record.
