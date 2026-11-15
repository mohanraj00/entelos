# Decision events `entelos.decision-event/1` (proposed)

This file specifies a read-only export of decisions for an outside producer. Issue [#158](https://github.com/mohanraj00/entelos/issues/158) tracks it. One recorded verdict gives one event for each fact in its proposal.

Status: **proposed**. No Host exports these events. This part adds the spec, schema and hand-written vectors only. The expected values are stated from this text, not captured from code output. [`check_spec.py`](check_spec.py) applies these rules without a Host. A later job adds the Host export and runs the vectors against it.

## Terms

| Term | Meaning |
|---|---|
| Decision | An immutable entelos verdict on one proposal, identified by `decision_ref`. It is distinct from the producer decision that identifies a fact. |
| Fact | One entry in the cited evidence document's `facts` array ([EVIDENCE.md](../evidence/EVIDENCE.md)). |
| Stream | The append-only sequence for one store, one authorized scope and one fixed selection of proposals. Its identity also binds the event major version. |
| Stream start | The fixed store revision at which the Host starts to require recorded actor kinds and per-fact blind marks. Earlier decisions are outside the stream. |
| Position | The number of events before or at a cursor. Position 0 precedes the first event. |
| Blind | A person gave the verdict without seeing the proposed value of this fact beforehand. |

## Decision

1. Export each recorded decision at or after the stream start separately. Do not merge reviewers, replace an earlier verdict or export only the latest verdict. A later correction appends a new decision's events and preserves the earlier events.
2. For a decided proposal, export exactly one event per cited fact, including facts that the reviewer left unchanged. A proposal without cited facts gives no events. A pending proposal or a policy `route` gives no events.
3. Map an approval API `accept` to `approve` and `reject` to `reject` ([APPROVAL.md](../approval/APPROVAL.md)). Map a policy `accept` to `approve` and `refuse` to `reject` ([POLICY.md](../policy/POLICY.md)). For a human approval with fact corrections, replace `approve` with `correct` only for the corrected facts. A rejection has no corrections.
4. The actor kind is the kind of the recorded deciding actor, not the producer or a reviewer cited in the incoming evidence. The closed set is `human`, `model`, `policy`. A policy verdict always exports `policy` and never counts as `human`.
5. Record the blind mark per fact. Only a human verdict can have `blind: true`. A human who saw the proposed value has `blind: false`. Model and policy verdicts have `blind: false`; the human blind condition does not apply to them.
6. Export only the digests, disposition, actor kind, blind mark, cursor and any corrected value. Do not copy the proposal, original value, evidence document, producer receipt, source text, reviewer identity, reason or metadata.
7. Export never writes a verdict, advances an activation, acknowledges processing or changes any stored decision. A cursor is a read position, not an approval or an authentication credential.

## Event

An event is one JSON object. [`schema/event.schema.json`](schema/event.schema.json) defines its structure and the read request and page structures in `$defs`. The rules below also apply; the schema alone does not enforce the relations between fields.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.decision-event/1`. The number after the slash is the major version. |
| `cursor` | string | The cursor immediately after this event. |
| `decision_ref` | digest | The entelos decision record. All events of one decision have the same reference. |
| `disposition` | string | `approve`, `reject` or `correct`. |
| `fact_digest` | digest | The original `facts[].decision_digest` in the cited evidence. It identifies the producer's fact before this entelos verdict. |
| `evidence_receipt_digest` | digest | The original `facts[].receipt_digest`. It identifies the producer receipt, not the evidence document or a later correction receipt. |
| `actor_kind` | string | `human`, `model` or `policy`, from recorded provenance. |
| `blind` | bool | The mark of rule 5, from recorded provenance. Required even when false. |
| `corrected_value` | string or null | Required exactly when `disposition` is `correct`. The new value given by the person, at most 65536 characters, as in evidence `data.value`. An empty string and null are valid corrections. Omitted for `approve` and `reject`. |

A digest is `sha256:` and 64 lowercase hex digits. Fact and receipt digests use the producer's `digest_scheme`; the export copies them without recomputing them. A digest identifies bytes. It is not a signature or proof of their source. A correction keeps the original fact and receipt digests so the producer can find the original fact.

A field outside this table is refused, including a field that holds extra content on a valid correction. `corrected_value` is the sole content-bearing field. It holds only the corrected fact value, never an envelope for source material or metadata.

## Event rules

A reader applies these rules in order. The first failing rule refuses the event. It does not skip it or advance its checkpoint past it.

| Step | Rule | Result |
|---|---|---|
| 1 | The event is an object and `format` matches `entelos.decision-event/<number>`, with no leading zero except the number 0 itself. | `invalid_event`, rule `format` |
| 2 | The major version is 1. | `unsupported_event_format` |
| 3 | The event agrees with the schema, including the closed fields and values, and its cursor position is from 1 to 9007199254740991. | `invalid_event`, rule `schema` |
| 4 | `corrected_value` is present exactly when `disposition` is `correct`. | `invalid_event`, rule `correction` |
| 5 | A correction is human. A true blind mark is human. | `invalid_event`, rule `provenance` |

## Order and cursors

The Host enables required actor kinds and per-fact blind marks in one committed store revision. It records that revision once as the stream start. The exporter reads this saved revision. It does not search for the first valid decision. A decision recorded before this revision is outside the stream, even if it has provenance. Such decisions consume no event positions and cannot stop a read. A decision recorded at or after this revision is subject to the read checks. Missing provenance never moves the stream start. A new store records the start in its first revision.

Each page gives this revision as `stream_start`, an opaque string of 1 to 256 characters. The value states that the stream excludes earlier decisions. It is fixed for the stream namespace. It is not an event cursor or an authority. The Host must preserve the start and its order relative to decisions when it rebuilds the export.

Decisions in the stream are in durable store append order, not timestamp order or digest order. Concurrent writes must have one durable order before export. The events of a decision are contiguous and follow the original evidence `facts` array order. The export does not sort by fact digest or group by receipt. Each stream position starts at 1 and increases by one per event. Non-decision writes and decisions without facts consume no event position.

A version 1 cursor is `ev1:<stream>:<position>`. `stream` is 64 lowercase hex digits, an immutable namespace assigned by the exporter to this stream. It is not a content hash or an authority. `position` is a decimal integer from 0 to 9007199254740991, with no leading zero except 0 itself. A consumer stores and returns the whole cursor without editing it. The event at position N has cursor `ev1:<stream>:N`.

A cursor binds the store, stream start, authorized scope, fixed selection and event version. A cursor of another stream, a malformed cursor, an unknown position or a position after the current end is `invalid_cursor`. An implementation must never silently restart at 0. The stream must retain its prefix while it accepts its cursors. If retention, a rebuild or a selection change cannot preserve that prefix and namespace, retire the stream and refuse its old cursors. Authorization still applies on every read; a cursor grants no access.

## Read

This is a logical read contract, not a new operation in wire protocol 1.0 or 1.1. A later Host job supplies the transport and access checks.

Request: `{"cursor"?: string, "limit": integer}`. `limit` is 1 to 256. No field selects an actor, scope or filter. Omit `cursor` to start before position 1 of the authorized stream.

Success: `{"events": [event, ...], "next_cursor": string, "stream_start": string}`. Every page has `stream_start`, including the first page and an empty page. For input position P and a stream of N events, return exactly positions P+1 through min(P+limit, N). `next_cursor` is the last returned event cursor, or the cursor at P for an empty page. An empty stream returns its position 0 cursor. There is no separate end flag: a later read may see newly appended events.

The read pins one committed prefix for its entire page. Appends after that prefix are visible on a later read. A retry with the same cursor sees the same events up to that prefix, and may see further events if there was room in the page. No previously exported position changes. A page may stop in the middle of one proposal. Resuming from its `next_cursor` returns the next fact, with no duplicate and no gap. Reading at the end returns an empty page until an append.

Apply these checks in order before returning any event or cursor:

| Step | Rule | Result |
|---|---|---|
| 1 | The request agrees with the request schema. | `invalid_request` |
| 2 | The supplied cursor, if any, is valid for this stream and prefix. | `invalid_cursor` |
| 3 | Each selected decision at or after the stream start has complete recorded fact identities, actor kind and per-fact blind marks, and valid corrections under the event rules. | `unexportable_decision` |

Step 3 applies to the complete decisions contributing events to this page, even if a page boundary cuts a decision. On refusal return no partial page and no next cursor. Decisions before the stream start do not contribute events. Missing provenance at or after the stream start is a refusal, not a default of `human` or `false`. Never infer blind from the absence of an original value in this export. This contract does not change the format of existing verdict records or invent provenance for them.

## Consumer

An invented producer, `sample-ledger`, keeps facts and receipts in its own store. It reads a page, checks each event version, and joins the fact and receipt digests to its records. It keeps policy and model verdicts separate from human verdicts and can select human blind corrections for its own review process. Entelos makes no learning or training choice for it.

After applying a page, the producer commits its results and `next_cursor` together. It can instead commit after each event using that event's cursor. A retry after a lost response can repeat a page; the producer must commit its checkpoint atomically with its effects or deduplicate by the whole event cursor. The export's exclusive resume rule does not promise exactly-once effects in the consumer. On a refusal it keeps its old checkpoint and reports the error.

For example, a synthetic person approves a proposal of three facts and corrects only the second to `green`. The events are `approve`, `correct` with `corrected_value: "green"`, and `approve`, each with `actor_kind: "human"`. If the first page ends after the second fact, its next read begins with the third. A policy approval produces three `approve` events with `actor_kind: "policy"` and `blind: false`.

## Versions

A reader of version 1 refuses an unknown major version with `unsupported_event_format` before interpreting other fields. It must not downgrade it, accept its known fields or checkpoint past it. New fields, dispositions or actor kinds require a new major version. Version 0 is also unsupported. A malformed or missing format is `invalid_event`, rule `format`.

## Vectors

[`vectors/event-cases.json`](vectors/event-cases.json) contains named `cases` with `event` and `expect`. Expectations are `{"valid": true}`, `{"valid": false, "code": "unsupported_event_format"}` or `{"valid": false, "code": "invalid_event", "rule": ...}`. In an event, `{"$text": {"char": C, "count": N}}` is a string of N copies of C; this keeps the length boundary cases compact.

[`vectors/export-cases.json`](vectors/export-cases.json) contains `streams` and named `cases`. Each stream has its namespace, saved `stream_start` revision and a list of synthetic `decisions` in append order. These are normalized test inputs, not a new stored record format. Each decision has a boolean `before_stream_start` that states its position relative to the saved revision. The true values form a prefix. The exporter must use store order to determine this relation.

A decision also has `decision_ref`, `actor_kind`, `disposition` (`approve` or `reject`) and `facts` in evidence order. Each fact has `fact_digest`, `evidence_receipt_digest`, `blind` and optional `corrected_value`. Presence of that value on an approval means a correction; presence on a rejection is invalid. A kind or mark can be missing in an earlier decision or a refusal case. An invalid kind or mark gives a refusal at or after the start. A decision reference occurs only once in a stream.

Each export case names a `stream`, gives a `request` and states the exact `expect`: `{"ok": true, "events": [...], "next_cursor": ..., "stream_start": ...}` or `{"ok": false, "code": ...}`. Optional `after` names an earlier successful case in the same stream or a retained prefix of it; the request cursor must equal that case's expected `next_cursor`. The streams include a retained prefix followed by an append, a second verdict on the same fact, proposals without facts and decisions before the stream start.

`check_spec.py` checks the event rules, exact projection and read rules, coverage of every refusal rule, all dispositions and actor kinds, both marks, middle and end cursors, retries and appends. It also checks the stream start marker and the exclusion of earlier decisions. Its self-test mutates copies of the inputs and expectations. It checks for wrong digests, content, provenance, ordering, duplicates, gaps, cursors and stream starts. It uses the Python standard library and the existing local schema validator. It does not write files or derive expected values from its projection.

## Limits

- No Host or transport implements the export. The synthetic decisions do not test authentication, authorization, durable ordering, retention, concurrent writes or provenance capture. Those checks belong to the Host job.
- Existing approval records do not record what the reviewer saw. Decisions before the stream start stay outside the export. The later Host job must record the start and require new provenance at or after it. This spec adds neither. Model verdict capture and fact corrections also need later Host work.
- A blind mark is a recorded statement of the review interface. The export cannot prove what a person saw outside that interface.
- Digests can identify sensitive facts, and a corrected value is content. The application's access and retention policy must cover the export. This spec exports no source material beyond that corrected value.
- Agreement of the checker and hand-written vectors shows their agreement only. A later Host must run these vectors on its actual export without replacing the expected values with its own output.
