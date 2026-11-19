# Decision events `entelos.decision-event/1`

This file specifies a read-only export of decisions for an outside producer. Issue [#158](https://github.com/mohanraj00/entelos/issues/158) tracks it. One recorded verdict gives one event for each fact in its proposal. Issue [#197](https://github.com/mohanraj00/entelos/issues/197) adds the rule for a verdict with no per-fact marks: see [Unmarked verdicts](#unmarked-verdicts).

Status: **implemented** in the Go Host. Protocol 1.2 exposes the read on the
socket and HTTP bindings. The JSONL Host calls the same implementation.
The schema and hand-written vectors state the contract. Their expected values
come from this text. [`check_spec.py`](check_spec.py) checks them without a Host.
`TestDecisionEventVectors` and `TestDecisionExportVectors` run them on the Go
checker and the real Host read. They do not change the vector expectations.
`TestDecisionEventsReadPastUnmarkedVerdict` records a verdict with no marks
through protocol 1.1 and reads through its gap on the socket and HTTP bindings.
`TestJSONLDecisionEventsReadPastUnmarkedVerdict` does the same on the JSONL Host.

## Terms

| Term | Meaning |
|---|---|
| Decision | An immutable entelos verdict on one proposal, identified by `decision_ref`. It is distinct from the producer decision that identifies a fact. |
| Fact | One entry in the cited evidence document's `facts` array ([EVIDENCE.md](../evidence/EVIDENCE.md)). |
| Stream | The append-only sequence for one store, one authorized scope and one fixed selection of proposals. Its identity also binds the event major version. |
| Stream start | The fixed store revision at which the Host starts to require recorded actor kinds and per-fact blind marks. Earlier decisions are outside the stream. |
| Position | One place in the stream for one cited fact of one decision in the stream. A cursor at position N is after N positions. Position 0 precedes the first position. |
| Blind | A person gave the verdict without seeing the proposed value of this fact beforehand. |
| Unmarked verdict | A decision in the stream that cites facts and has no recorded per-fact marks. Its record has no `fact_reviews` field. |
| Gap | The positions of the facts of one unmarked verdict. A gap position has no event. |

## Decision

1. Export each recorded decision at or after the stream start separately. Do not merge reviewers, replace an earlier verdict or export only the latest verdict. A later correction appends a new decision's events and preserves the earlier events.
2. For a decided proposal, export exactly one event per cited fact, including facts that the reviewer left unchanged. A proposal without cited facts gives no events. A pending proposal or a policy `route` gives no events. An unmarked verdict gives no events, and its positions are a gap.
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

The Host enables required actor kinds and per-fact blind marks in one committed store revision. It records that revision once as the stream start. The exporter reads this saved revision. It does not search for the first valid decision. A decision recorded before this revision is outside the stream, even if it has provenance. Such decisions consume no event positions and cannot stop a read. A decision recorded at or after this revision is subject to the read checks. An unmarked verdict is the one exception: it gives a gap and no events, as the section [Unmarked verdicts](#unmarked-verdicts) says. Missing provenance never moves the stream start. A new store records the start in its first revision.

Each page gives this revision as `stream_start`, an opaque string of 1 to 256 characters. The value states that the stream excludes earlier decisions. It is fixed for the stream namespace. It is not an event cursor or an authority. The Host must preserve the start and its order relative to decisions when it rebuilds the export.

Decisions in the stream are in durable store append order, not timestamp order or digest order. Concurrent writes must have one durable order before export. The events of a decision are contiguous and follow the original evidence `facts` array order. The export does not sort by fact digest or group by receipt. Each stream position starts at 1 and increases by one per cited fact of each decision in the stream. Each position has one event, or it is a gap position of an unmarked verdict. Non-decision writes and decisions without facts consume no position.

A version 1 cursor is `ev1:<stream>:<position>`. `stream` is 64 lowercase hex digits, an immutable namespace assigned by the exporter to this stream. It is not a content hash or an authority. `position` is a decimal integer from 0 to 9007199254740991, with no leading zero except 0 itself. A consumer stores and returns the whole cursor without editing it. The event at position N has cursor `ev1:<stream>:N`. A cursor at a gap position is valid, and the Host can give it as `next_cursor`.

A cursor binds the store, stream start, authorized scope, fixed selection and event version. A cursor of another stream, a malformed cursor, an unknown position or a position after the current end is `invalid_cursor`. An implementation must never silently restart at 0. The stream must retain its prefix while it accepts its cursors. If retention, a rebuild or a selection change cannot preserve that prefix and namespace, retire the stream and refuse its old cursors. Authorization still applies on every read; a cursor grants no access.

## Unmarked verdicts

A verdict can cite facts and have no per-fact marks. A client on protocol 1.0 or 1.1 cannot send `fact_reviews`, and a client on protocol 1.2 can omit it. If the actor is human or has no recorded kind, the decision record then has no `fact_reviews` field. This is an unmarked verdict.

These rules are normative:

1. An unmarked verdict in the stream keeps its positions: one position for each cited fact, in the stream order. These positions are a gap. No event has a gap position.
2. A gap has one meaning only: the stream has a verdict with cited facts and no blind marks, and the export omits one fact for each gap position. A gap never means a lost event. Each position that is not a gap position has exactly one event, and a reader gets it exactly one time when it resumes from each `next_cursor`.
3. Only the record decides. A decision is unmarked if it cites one fact or more and its record has no `fact_reviews` field. A record that has the field is never unmarked. If its marks are not complete or not valid, or if it has marks and no actor kind, the read gives `unexportable_decision`.
4. The export never supplies a mark or a kind. It does not give `human` for a missing kind or `false` for a missing mark. A gap does not tell the disposition, the fact digests or the actor of the verdict.
5. An unmarked verdict cannot stop a read. It also cannot make a page before the stream end empty. The page rule of [Read](#read) is exact: the limit counts events, and a gap position does not count.
6. The verdict stays in the store, and it keeps its place in the saved decision list. The approval, the verdict counts and the activation rules do not change. The reviewer reads `record` and `proposal` show the verdict.
7. The event format, the read schemas, the cursor format and the stream namespace do not change. The position of each event is the position that v0.5.0 gave it.

A reader finds a gap from the cursors. For a page, the count of omitted facts is the position of `next_cursor`, minus the position of the request cursor, minus the count of the events of the page. The sum of these counts is correct when each request uses the `next_cursor` of the page before it. A gap at the stream end shows in the cursor of the last page. A reader can also compare the positions of two events that follow one another. A reader that needs each human verdict can stop or raise an alarm when this count is not 0.

The other methods were not selected, for these reasons:

- A refusal of the verdict at `decide` gives a new refusal to each client on protocol 1.0 and 1.1. It also does not help a store that already has an unmarked verdict.
- An event with an "unknown" blind state needs a new value or a new field. A reader of version 1 refuses that event, so it needs a new major version. The cursor binds the major version, so each reader must start a new stream.
- A permanent refusal, which v0.5.0 gave, lets one verdict from an old client stop each reader of the stream.
- An unmarked verdict with no positions gives a reader no signal. A reviewer can then keep a verdict out of the export, and the reader cannot detect it or count it. This method also gives the later events other positions than v0.5.0 gave.
- A limit that counts gap positions gives an empty page before the stream end, with a later cursor. v0.5.0 states that an empty page keeps the request cursor and that a consumer can save the cursor of each event. Such a consumer then stops at a gap that is as large as its limit, and it loses the later events with no error.

### Upgrade from v0.5.0

The Host of v0.5.0 gave the same positions, and it refused each page that had a gap position. Thus a reader that stopped at an unmarked verdict has a saved cursor before the gap.

After the upgrade, the reader sends its saved cursor again. The page starts with the first event after the gap. If the stream ends in the gap, the page has no events, and `next_cursor` is the cursor at the gap end. The stream is the same, and each event has the position that v0.5.0 counted for it. The reader gets no duplicate, and it loses no event. A cursor from a later Host names the same position on a v0.5.0 Host.

### Effect on a consumer

- A consumer that checks for contiguous event positions must accept a jump. The jump is the gap, and its size is the count of the omitted facts.
- An empty page still means that the stream has no later event at this time, as on v0.5.0. Its `next_cursor` can be after the request cursor, if the stream ends in a gap. A consumer can save that cursor or keep its old cursor: the next read gives the same events for each of the two.
- A consumer that saves the cursor of each event, and not `next_cursor`, loses no event. It finds a gap at the stream end only from `next_cursor`.
- A consumer gets no event for the facts of an unmarked verdict. A consumer that needs an event for each human verdict must make sure that each client sends `fact_reviews` through protocol 1.2.
- The events have the same fields and the same rules as before.

## Read

The operation is `decision_events`. It needs the `event_reader` role. That role permits no other operation, and the `reviewer` role does not permit this read. Protocol
1.2 exposes it through the socket and HTTP bindings. Versions 1.0 and 1.1
refuse it. The JSONL request uses `op: "decision_events"`. Each read checks the
role and selects only the Host scope and installed functions. A request
cannot change that selection.

Request: `{"cursor"?: string, "limit": integer}`. `limit` is 1 to 256. No field selects an actor, scope or filter. Omit `cursor` to start before position 1 of the authorized stream.

Success: `{"events": [event, ...], "next_cursor": string, "stream_start": string}`. Every page has `stream_start`, including the first page and an empty page. An empty stream returns its position 0 cursor. There is no separate end flag: a later read may see newly appended events.

The page rule is exact. The input position is P, and the stream has N positions.

1. `limit` counts events. A gap position has no event, and it does not count.
2. `events` has the events of the first `limit` positions after P that are not gap positions, in position order. If the stream has fewer such positions after P, `events` has the event of each one.
3. If the page has `limit` events, the page is full. `next_cursor` is the cursor of its last event.
4. If the page has fewer events than `limit`, the page reached the stream end. `next_cursor` is the cursor at N. Thus `next_cursor` is after the last event only if the stream ends in a gap.
5. An empty page means that the stream has no event after P. Its `next_cursor` is the cursor at N. It equals the request cursor if P equals N. It is after the request cursor only if the positions P+1 through N are all gap positions.
6. A request cursor before a gap, in a gap or after a gap is valid. The page starts at the next position that is not a gap position.

For a stream with no unmarked verdict, this rule gives the page that v0.5.0 gave: the events of exactly the positions P+1 through min(P+limit, N), and `next_cursor` is the last returned event cursor, or the cursor at P for an empty page. The rule differs only for a stream with an unmarked verdict, where v0.5.0 refused each page with a gap position.

No page before the stream end is empty, and a gap never uses the room of a page. Thus a reader that stops at an empty page, or that saves only the cursor of each event, gets each later event on its next read.

The read pins one committed prefix for its entire page. Appends after that prefix are visible on a later read. A retry with the same cursor sees the same events up to that prefix, and may see further events if there was room in the page. No previously exported position changes. A page may stop in the middle of one proposal. Resuming from its `next_cursor` returns the next event, with no duplicate and no lost event. Reading at the end returns an empty page until an append.

Apply these checks in order before returning any event or cursor:

| Step | Rule | Result |
|---|---|---|
| 1 | The request agrees with the request schema. | `invalid_request` |
| 2 | The supplied cursor, if any, is valid for this stream and prefix. | `invalid_cursor` |
| 3 | Each decision at or after the stream start that contributes an event to this page has complete recorded fact identities, an actor kind, one valid blind mark for each fact, and valid corrections under the event rules. An unmarked verdict contributes a gap and no event, so this step does not apply to it. | `unexportable_decision` |

Step 3 applies to the complete decisions contributing events to this page, even if a page boundary cuts a decision. On refusal return no partial page and no next cursor. Decisions before the stream start do not contribute events or positions. Unmarked verdicts contribute gap positions and no events. For a decision with marks, an incomplete provenance at or after the stream start is a refusal, not a default of `human` or `false`. Never infer blind from the absence of an original value in this export. This contract does not change the format of existing verdict records or invent provenance for them.

## Host storage and setup

The first successful Host write adds `state.decision_stream`. The same commit
holds its random namespace seed, `after` (the prior revision), and a
`decisions` list. Each new verdict appends its reference to that list in the
same transaction. The store compare-and-swap gives these appends one order.
If the start commit records a verdict, the list holds that verdict. Else the
list is empty. Earlier verdicts are not in the stream. In an empty store,
`after` is the empty string.

The Host omits `decision_stream` from the state copy that `inspect` returns.
It keeps the stored state unchanged. Use `decision_events` to read the stream.
Each read checks authorization.

A revision cannot hold its own content hash. The saved `after` boundary names
the parent of the start revision. The read finds that parent's next revision
in the pinned history. It returns this revision as `stream_start`. If `after`
is the empty string, the start is the first revision of the history. It does
not inspect verdict validity to find the start. Later writes keep the boundary
and seed. The namespace binds that seed, the start, event version, scope and
sorted installed functions.

The embedding application can call `Host.StartDecisionEvents` before a read.
This setup commits the boundary if it is absent. It changes no verdict.
A second call writes nothing. Use it to start an empty store or to set the
boundary of an older store before its next Host write. A store with no saved
boundary has no stream. A read of it gives `invalid_cursor`. Reads never
initialize a stream. A new store saves the boundary in its first Host revision.

`decide` reuses the recorded `kind` from `actor_kinds` ([KINDS.md](../verdict/KINDS.md)).
A human request can supply `fact_reviews`, one object per cited fact in evidence
order. Each object has `blind` and optional `corrected_value`. The decision
record retains this array. Only a human accept can have a correction. A model
or policy verdict records false marks for each fact. These marks come from
the deciding actor's kind. They do not come from incoming evidence reviews.

Old requests can omit `fact_reviews`, and an actor can still have no kind.
This preserves the old approval contract. Such a verdict with cited facts is an
unmarked verdict. The Host appends it to the `decisions` list as each other
verdict, and the read gives a gap for its facts. The read does not supply
missing human marks or kinds. A fact-free verdict consumes no position.

## Consumer

An invented producer, `sample-ledger`, keeps facts and receipts in its own store. It reads a page, checks each event version, and joins the fact and receipt digests to its records. It keeps policy and model verdicts separate from human verdicts and can select human blind corrections for its own review process. Entelos makes no learning or training choice for it.

After applying a page, the producer commits its results and `next_cursor` together. It can instead commit after each event using that event's cursor. A retry after a lost response can repeat a page; the producer must commit its checkpoint atomically with its effects or deduplicate by the whole event cursor. The export's exclusive resume rule does not promise exactly-once effects in the consumer. On a refusal it keeps its old checkpoint and reports the error.

For example, a synthetic person approves a proposal of three facts and corrects only the second to `green`. The events are `approve`, `correct` with `corrected_value: "green"`, and `approve`, each with `actor_kind: "human"`. If the first page ends after the second fact, its next read begins with the third. A policy approval produces three `approve` events with `actor_kind: "policy"` and `blind: false`.

## Versions

A reader of version 1 refuses an unknown major version with `unsupported_event_format` before interpreting other fields. It must not downgrade it, accept its known fields or checkpoint past it. New fields, dispositions or actor kinds require a new major version. Version 0 is also unsupported. A malformed or missing format is `invalid_event`, rule `format`.

## Vectors

[`vectors/event-cases.json`](vectors/event-cases.json) contains named `cases` with `event` and `expect`. Expectations are `{"valid": true}`, `{"valid": false, "code": "unsupported_event_format"}` or `{"valid": false, "code": "invalid_event", "rule": ...}`. In an event, `{"$text": {"char": C, "count": N}}` is a string of N copies of C; this keeps the length boundary cases compact.

[`vectors/export-cases.json`](vectors/export-cases.json) contains `streams` and named `cases`. Each stream has its namespace, saved `stream_start` revision and a list of synthetic `decisions` in append order. These are normalized test inputs, not a new stored record format. Each decision has a boolean `before_stream_start` that states its position relative to the saved revision. The true values form a prefix. The exporter must use store order to determine this relation.

A decision also has `decision_ref`, `actor_kind`, `disposition` (`approve` or `reject`) and `facts` in evidence order. Each fact has `fact_digest`, `evidence_receipt_digest`, `blind` and optional `corrected_value`. Presence of that value on an approval means a correction; presence on a rejection is invalid. A decision with one fact or more, where no fact has `blind` or `corrected_value`, is an unmarked verdict. It stands for a record with no `fact_reviews` field, and it can also have no `actor_kind`. Its facts are gap positions. A kind or mark can also be missing in an earlier decision or a refusal case. If some facts of a decision have a mark and others do not, or if a decision has marks and no kind, the read gives a refusal at or after the start. An invalid kind or mark also gives a refusal there. A decision reference occurs only once in a stream.

Each export case names a `stream`, gives a `request` and states the exact `expect`: `{"ok": true, "events": [...], "next_cursor": ..., "stream_start": ...}` or `{"ok": false, "code": ...}`. Optional `after` names an earlier successful case in the same stream or a retained prefix of it; the request cursor must equal that case's expected `next_cursor`. The streams include a retained prefix followed by an append, a second verdict on the same fact, proposals without facts, decisions before the stream start and unmarked verdicts before, between and after marked decisions. The cases have a gap inside a page, a full page before a gap, a page that ends in a gap at the stream end, a page that holds only a gap, and a resume from a cursor before, inside and after a gap.

`check_spec.py` checks the event rules, exact projection and read rules, coverage of every refusal rule, all dispositions and actor kinds, both marks, middle and end cursors, retries and appends. It also checks the stream start marker, the exclusion of earlier decisions and the gap of each unmarked verdict. Its self-test mutates copies of the inputs and expectations. It checks for wrong digests, content, provenance, ordering, duplicates, gaps, cursors and stream starts. It also checks that an unmarked verdict cannot get events, cannot lose its positions, cannot hold the cursor and cannot give a refusal. It uses the Python standard library and the existing local schema validator. It does not write files or derive expected values from its projection.

## Limits

- Decisions before the saved start stay outside the stream. The Host does not invent their provenance. Old requests can still record a verdict without marks. Such a verdict with facts is an unmarked verdict, and the export gives a gap for it. A gap tells the count of the omitted facts. It does not tell how many verdicts they are from, and no Host setting refuses an unmarked verdict at `decide`.
- The read scans the saved decision list and retained history at one revision. The page limit bounds the event count, not the scan cost or response bytes. The transport can give `response_too_large`. The Host has no stream compaction or retirement tool. Keep the prefix and the start history while accepting cursors.
- A policy actor can call `decide`. The Host does not yet write the automatic policy verdict of POLICY.md. A policy route has no decision event.
- The application states actor kinds and blind marks. The Host cannot prove what a person saw outside the review interface.
- Digests can identify sensitive facts. A corrected value is content. The application must apply its access and retention policy to both.
- The vectors use synthetic records. Go tests check both stores, resume, append, closed event fields, the read past an unmarked verdict and the shared transport path. No test starts a v0.5.0 Host, so the upgrade rule comes from the v0.5.0 read rules and not from a run of that Host. They do not prove consumer effects, power-failure recovery or a retention policy.
