# Evidence format `entelos.evidence/1` (proposed)

This file specifies the admission evidence that an external producer gives for a proposal. It is the first part of issue [#104](https://github.com/mohanraj00/entelos/issues/104), and issue [#148](https://github.com/mohanraj00/entelos/issues/148) tracks it.

Status: **implemented in the evidence read**. Approval API 1.2 retains and returns the producer document. The Go lifecycle Host applies these admission rules in the `evidence` read ([below](#the-evidence-read)). `propose`, an accept `decide` and `activate` of a content proposal also apply them ([CONTENT.md](../content/CONTENT.md#evidence)). For a CEL bundle proposal, `propose` does not apply them yet. The schema and the vectors in this directory are the contract. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) checks the vectors against the schema and the rules of this file. It does not run a Host.

## Terms

| Term | Meaning |
|---|---|
| Producer | A system outside entelos that admits facts from source documents. It keeps its receipts in its own store. |
| Run | One run of the producer on one packet. The run gives one receipt with one or more decisions. |
| Decision | The result of the producer for one candidate fact in a run. |
| Fact | One decision that a proposal cites. |
| Receipt | The record of the producer for one run. Entelos keeps only its digest. |
| Source | The document, reference, page or statement that the fact comes from. Entelos keeps only digests and names. |
| Text | The NFC text of the source that the producer read. Spans are offsets into this text. |
| Span | A location in the text: a half-open range of UTF-8 byte offsets, with an optional page and box. |
| Review | The result of one or more reviewers for one fact, after the producer decided it. |
| Correction | A review that gives a new value. The producer writes a new receipt for it. |

## Document

An evidence document is one JSON object. [`schema/evidence.schema.json`](schema/evidence.schema.json) gives its fields. A field that is not in these tables is not permitted.

Each table uses these value shapes:

- A **name** has 1 to 128 characters from `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`.
- A **text** has 1 to 256 characters, with one or more characters that are not white space, and no control characters (U+0000 to U+001F and U+007F). Spaces and `:` are permitted. A **long text** has the same rule, with 4096 characters maximum. So `data.field`, `data.key`, `data.unit`, `fact_id`, `ref` and each reviewer name have 256 characters maximum, and a control character in them gives `schema`. `data.value` is not a text.
- A **word** matches `^[a-z][a-z0-9_]{0,63}$`. A **code** matches `^[A-Z][A-Z0-9_]{0,63}$`.
- A **digest** is `sha256:` and 64 lowercase hex digits.
- An **attached object** is `{"digest": <digest>, "value": <object>}`. Entelos retains its value. Admission checks its number domain and reads the producer's unit marker when required.
- "or null" means that the field is present, and its value can be `null`. "Optional" means that the field can be absent.

### Numbers

Each JSON number, including a number inside an attached object, uses the [wire protocol number domain](../PROTOCOL.md#request). Write an integer from −9007199254740991 to 9007199254740991 with no decimal point or exponent. A producer can use a string in an attached object to retain an exact decimal.

If a document has a box or one or more judgments, set `producer.producer_data.value.number_units` to `"ppm-milli"` and supply the digest of that attached object. For a probability, multiply by 1000000. For a box, multiply each coordinate in page points by 1000. One point is 1/72 inch. Round to the nearest integer, with ties to even, if conversion needs rounding. The box must still have positive width and height after conversion.

For example, write `970000` for a probability of 0.97 and `700500` for a coordinate of 700.5 points.

### Document and producer

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.evidence/1`. |
| `producer.id` | name | The name of the producer. |
| `producer.version` | name | The version of the producer. |
| `producer.digest_scheme` | string | The name of the producer rule that makes each digest in the document. 1 to 128 characters from the name characters and `:`, `/` and `@`. |
| `producer.producer_data` | attached object | Optional. Other data of the producer for the document. |
| `facts` | array | 1 to 256 facts. |

### Fact identity and source

| Field | Type | Rule |
|---|---|---|
| `facts[].fact_id` | text | A name for the fact, unique in the document. |
| `facts[].receipt_digest` | digest | The receipt of the run. Two facts from one run have the same receipt digest. |
| `facts[].decision_digest` | digest | The decision in the receipt. This digest identifies one fact. |
| `facts[].packet_digest` | digest or null | Optional. The input packet of the run. |
| `facts[].profile_digest` | digest or null | Optional. The producer profile of the run. |
| `facts[].completeness` | attached object | Optional. The completeness report of the run. |
| `facts[].source_kind` | word | The kind of the source, for example `document`, `reference`, `external` or `knowledge`. |
| `facts[].source_digest` | digest or null | The original source bytes, for example a PDF file. `null` if the producer has no original bytes. |
| `facts[].text_digest` | digest or null | The NFC text that the spans are in. `null` if the fact has no text, for example a fact from a policy source. |
| `facts[].url` | long text or null | Optional. The URL of the source. |
| `facts[].ref` | text or null | Optional. The name of a reference in the packet. |
| `facts[].spans` | array | 0 to 64 spans in the text. An empty list means that the fact has no span. |
| `facts[].spans[].start` | integer | 0 or more. |
| `facts[].spans[].end` | integer | More than `start`. |
| `facts[].spans[].page` | integer | Optional. The page number, from 1. |
| `facts[].spans[].box` | object | Optional. `x0`, `y0`, `x1` and `y1`, each an integer in thousandths of a point. A box needs a page. |
| `facts[].pins` | array | Optional. 0 to 16 source pins. |
| `facts[].pins[].url` | long text | The URL of the pinned source. |
| `facts[].pins[].digest` | digest or null | The bytes that the producer got. |
| `facts[].pins[].retrieved` | text or null | The date when the producer got the source, as the producer wrote it. |
| `facts[].pins[].edition` | text or null | The edition or the version of the source. |

A document can hold facts from two or more runs. Each run can have a different profile and a different completeness report. The facts of one run give the same `profile_digest` and the same `completeness`. The rules do not check this.

Span offsets are UTF-8 byte offsets into the NFC text that `text_digest` identifies. They are zero-based and half-open. Two or more spans can give one record or one passage in parts.

### Fact outcome and data

| Field | Type | Rule |
|---|---|---|
| `facts[].outcome` | string | The outcome of the producer: `admitted`, `needs_verification` or `rejected`. |
| `facts[].codes` | array | 0 to 64 codes of the producer, in the order of the producer. For example `ADMITTED_BY_POLICY`, `MODEL_CLEARED` or `EVIDENCE_REANCHORED`. |
| `facts[].data.field` | text | The field of the fact. |
| `facts[].data.key` | text or null | Optional. The key of the field. |
| `facts[].data.value` | string or null | The value of the fact as a string, 65536 characters maximum. Control characters are permitted, so a value can be a passage with line breaks. If a review corrected the value, this is the corrected value. |
| `facts[].data.unit` | text or null | Optional. The unit of the value. |
| `facts[].data.parts` | array | Optional. 0 to 64 parts: `role` (word), `start` and `end` (integer or null), `passed` (bool). |
| `facts[].data.schema_digest` | digest | Optional. The schema of the producer. |
| `facts[].data.policy_digest` | digest | Optional. The policy of the producer. |
| `facts[].data.packs` | array | Optional. 0 to 64 texts: the packs that the decision used. |
| `facts[].data.judgments` | array | Optional. 0 to 64 judgments with `question` (word), `answer` (optional, text or null) and `p` (integer probability in parts per million, from 0 to 1000000). |
| `facts[].producer_data` | attached object | Optional. Other data of the producer for the fact. |

### Review

| Field | Type | Rule |
|---|---|---|
| `facts[].review` | object | Optional. A review is permitted on a fact with each outcome. |
| `facts[].review.status` | string | `admitted`, `confirmed`, `chosen`, `corrected`, `rejected`, `not_in_document`, `open` or `disputed`. |
| `facts[].review.answers` | array | 1 to 16 answers. Each reviewer can give an answer. |
| `facts[].review.answers[].reviewer` | text | The name of the reviewer, for example `person:Ana Diaz`. |
| `facts[].review.answers[].reviewer_kind` | string | `human` or `model`. |
| `facts[].review.answers[].verdict` | word | The verdict of the reviewer, in the words of the producer, for example `confirm` or `correct`. |
| `facts[].review.answers[].time` | string | The UTC time of the answer, `YYYY-MM-DDThh:mm:ss` with optional fractions and `Z`. |
| `facts[].review.answers[].ms` | integer | The duration of the review in milliseconds, 0 or more. |
| `facts[].review.answers[].entry_digest` | digest | Optional. The answer record of the producer. |
| `facts[].review.answers[].producer_data` | attached object | Optional. Other data of the producer for the answer. |
| `facts[].review.task_digest` | digest or null | Optional. The review task. |
| `facts[].review.resolution_digest` | digest | Optional. The resolution record of the producer. |
| `facts[].review.correction_receipt_digest` | digest | The receipt of the correction. Required if `status` is `corrected`. Not permitted for another status. |

The review status `admitted` is an audit: one or more reviewers saw a fact that the producer admitted, and the fact stays admitted. The statuses `confirmed`, `chosen` and `corrected` give the value of a reviewer. The other statuses give no value.

A corrected fact has two receipts. The original receipt gives `receipt_digest`, `decision_digest`, `packet_digest`, `profile_digest`, `completeness`, `outcome` and `codes`. These fields show the decision of the producer before the review. The correction receipt, that `correction_receipt_digest` identifies, gives `spans` and `data`. These fields show the value that the reviewer gave. `source_kind`, `source_digest`, `text_digest`, `url`, `ref` and `pins` are the same in the two receipts. The rules do not check this.

### Digests

Each digest is a value that the producer made with the rule that `producer.digest_scheme` names. A digest can be a hash of raw bytes. It can also be a domain-separated hash of canonical JSON, for example SHA-256 of a prefix, a kind, a NUL byte and the JCS form of an object. Entelos does not compute or check a digest. It compares digests only as strings.

A digest is an identity of the data that the producer hashed. It is not a signature. Entelos does not read the receipt or the source, so it cannot check that a digest is correct. The producer is responsible for its digests.

## Rules

A reader applies these rules in this order. Each rule examines all of the facts before the next rule starts. The first rule that fails gives the result.

| Step | Rule | Result |
|---|---|---|
| 1 | The document is an object, and `format` is a string of the shape `entelos.evidence/<number>`. The number has one or more digits and no leading zero. | `invalid_evidence`, rule `format` |
| 2 | The number after the slash is `1`. | `unsupported_evidence_format` |
| 3 | The document agrees with the schema and the [number rules](#numbers). | `invalid_evidence`, rule `schema` |
| 4 | The document has 256 facts or fewer. | `invalid_evidence`, rule `too_many_facts` |
| 5 | Each `fact_id` is unique. | `invalid_evidence`, rule `duplicate_fact` |
| 6 | For each span, `end` is more than `start`. For each part, `start` and `end` are both null, or `end` is more than `start`. | `invalid_evidence`, rule `span_order` |
| 7 | Each fact with one or more spans has a `text_digest` that is not null. | `invalid_evidence`, rule `span_without_text` |
| 8 | Each span with a box has a page, and the box has `x0` less than `x1` and `y0` less than `y1`. | `invalid_evidence`, rule `locator` |
| 9 | Each review with `status` `corrected` has `correction_receipt_digest`. | `invalid_evidence`, rule `correction_receipt_required` |
| 10 | No review with another status has `correction_receipt_digest`. | `invalid_evidence`, rule `correction_receipt_forbidden` |
| 11 | Each review with `status` `admitted` is on a fact with `outcome` `admitted`. | `invalid_evidence`, rule `review_status_outcome` |
| 12 | Each fact with `outcome` `needs_verification` has a review. | `invalid_evidence`, rule `unreviewed_fact` |
| 13 | No fact has `outcome` `rejected` and no review. No review has `status` `rejected` or `not_in_document`. | `invalid_evidence`, rule `rejected_fact` |
| 14 | No review has `status` `open` or `disputed`. | `invalid_evidence`, rule `open_review` |

Rules 12 to 14 refuse a citation of a fact that has no value. A proposal can cite a fact only if the producer admitted it, or if a review gave it a value. A review can give a value to a fact that the producer rejected, as a correction does.

A document that passes all of the rules is valid.

## Policy inputs

A valid document gives these inputs to an approval policy. Each input is a bool, so a rule of the current callable profile can read it.

| Input | Value |
|---|---|
| `evidence_human_reviewed` | `true` if one or more answers have `reviewer_kind` `human`. |
| `evidence_model_reviewed` | `true` if one or more answers have `reviewer_kind` `model`. |
| `evidence_all_automatic` | `true` if no fact has a review. Then each fact has `outcome` `admitted`. |
| `evidence_corrected` | `true` if one or more reviews have `status` `corrected`. |

For example, a policy can refuse an automatic approval if `evidence_corrected` is `true`. Then a person must decide the proposal with `decide`.

The policy inputs do not read `outcome` codes, `data` or an attached object. A later profile can give more inputs.

## Map from a groundgate receipt

This table shows how a mapping can give one fact from groundgate 0.14.0 (spec v0.12, commit `ca88ff7`). It uses the receipt (SPEC section 5), the `facts@1` export (section 5.1), the digests (section 6) and the review records `resolution@1` and `answer@2`. No code in this repository implements the map. The vector `valid-full-data` uses each field, with invented values.

The seam test [`tests/test_groundgate_seam.py`](../../../tests/test_groundgate_seam.py) implements a part of this map for groundgate 0.16.0, in the test only. It reads the `facts@2` export (section 5.2) and the store records of its packet. It maps the statuses `admitted`, `confirmed` and `chosen`. For a locator it takes `citation.locator.page` and multiplies the four values in `citation.locator.box` by 10 to convert hundredths of a PDF point to thousandths. It adds the unit marker to the producer data. It does not map `corrected`, `packs`, `judgments`, `pins` or the producer's attached objects. It states that the reviewer of the test is `human`.

| entelos field | groundgate value |
|---|---|
| `producer.id`, `producer.version` | `groundgate`, the package version. |
| `producer.digest_scheme` | `groundgate/0.12`: the digest rule of section 6. |
| `fact_id` | The candidate `id` if it is unique in the document. Else a name that the mapping makes, for example from the candidate digest and the decision position. |
| `receipt_digest` | `receipt_sha256`. |
| `decision_digest` | `candidate_sha256` of the decision. |
| `packet_digest` | `packet_sha256` of the export. |
| `profile_digest`, `completeness` | The profile digest and the completeness report of the run of the fact. |
| `source_kind`, `url`, `ref` | `citation.source`, `citation.url`, `citation.ref`. |
| `source_digest`, `text_digest` | `citation.source_sha256`, `citation.text_sha256`. Both are null for `external` and `knowledge`. |
| `spans` | The start and end of `citation.span`, or `[]` if the span is null. Map a page and box using the [number rules](#numbers). |
| `outcome`, `codes` | The decision `outcome` and `codes`. |
| `data.field`, `data.key`, `data.value`, `data.unit` | The decision `field`, `key`, `value` and `unit`. |
| `data.parts` | The decision `parts`. |
| `data.schema_digest`, `data.policy_digest` | `schema_sha256`, `policy_sha256`. |
| `data.packs` | The decision `packs`. |
| `data.judgments` | The receipt `judgments` for the candidate, without `candidate_sha256`. Convert `p` to parts per million and add the unit marker. |
| `pins` | For an external item: `url` and `retrieved`, with `digest` and `edition` null. |
| `producer_data` | Other decision members, for example `proposer`, `missing` and `evidence_units`. |
| `review.status` | The slot `status` of `resolution@1`. |
| `review.answers` | Each `answer@2` record of the slot: `reviewer`, `verdict`, `time` and `ms`. `reviewer_kind` is a statement of the mapping, because `answer@2` has no kind. The other answer members, for example `decision`, `candidate`, `codes_kept`, `reason_seen`, `keys` and `field_keys`, go in `answers[].producer_data`. |
| `review.task_digest`, `review.correction_receipt_digest` | The slot `task`, and the slot `receipt` if the status is `corrected`. If the status is `corrected`, `spans` and `data` come from the decision in that receipt. |

## Citation in a proposal

Approval API [1.2](../approval/APPROVAL.md#evidence-attachment-in-12) (#161) accepts one optional document in the `evidence` field of `propose`. The snapshot retains the document. Entelos keeps the producer's digests and claims in that document. The producer retains the receipts and sources.

For a content proposal, the Host applies the admission rules before it writes a snapshot. A refusal leaves the store root as it was. The evidence read applies the rules to a retained document and derives the policy inputs.

### The evidence read

The Go lifecycle Host has the read `evidence` (#104). Protocol 1.3 has it on the wire (#213).

Role: `reviewer`, as `proposal`. Body: `{"snapshot_ref": string, "cursor"?: string}`. The snapshot must be in the host scope, as for `proposal`.

Result: `{"cursor", "snapshot_ref", "admission", "code"?, "rule"?, "inputs"?}`.

| `admission` | Meaning | Other fields |
|---|---|---|
| `absent` | The snapshot has no evidence document. | None. |
| `valid` | The document passes all of the rules. | `inputs`: the four policy inputs. |
| `invalid` | A rule refuses the document. | `code`, and `rule` for `invalid_evidence`. |

The read applies the rules to the document that the snapshot retains, at the read cursor. It writes nothing. The snapshot bytes and the 1.2 `propose` and `proposal` results do not change, so the stable approval vectors keep their bytes. For a CEL bundle proposal, a document that a rule refuses stays in the snapshot. `propose` 1.2 does not refuse it, because that would change which 1.2 requests are valid ([STABILITY.md](../../../docs/STABILITY.md)). A later wire version can refuse it at `propose`.

## Versions

The major version is the number after the slash in `format`. A reader reads only the major version that it knows. An unknown major version gives `unsupported_evidence_format`. A reader of version 1 refuses a field that it does not know, so a new entelos field or a new enum value needs a new major version.

The experimental format keeps the name `entelos.evidence/1` after [#208](https://github.com/mohanraj00/entelos/issues/208). Admission of an earlier document with a decimal number, an out-of-range integer or a box or judgment without the unit marker gives `invalid_evidence`, rule `schema`. A document with only integers in range and no box or judgments needs no conversion. Earlier snapshots keep their bytes. Approval API 1.2 still attaches documents using its [shape and protocol checks](../approval/APPROVAL.md#evidence-attachment-in-12).

The attached objects are the extension point. A producer can put a new field in `producer.producer_data`, `facts[].completeness`, `facts[].producer_data` or `facts[].review.answers[].producer_data` without a new major version. Words, codes, `source_kind` and `verdict` are not enums, so a new producer value also needs no new major version.

## Vectors

[`vectors/evidence-cases.json`](vectors/evidence-cases.json) has a list of cases. Each case has:

- `name`: a unique name;
- `evidence`: the document, or a value that is not a document;
- in `evidence`, `{"$repeat": {"count": N, "fact": {…}}}` in place of a list is a list of N copies of the fact. The runner replaces `{i}` in `fact_id` with the index, from 0. `{"$repeat": {"count": N, "item": {…}}}` is a list of N copies of the item. `{"$text": {"count": N, "char": "x"}}` is a string of N copies of the character;
- `expect`: one of these:
  - `{"valid": true, "inputs": {…}}`, with the four policy inputs;
  - `{"valid": false, "code": "invalid_evidence", "rule": "<rule>"}`;
  - `{"valid": false, "code": "unsupported_evidence_format"}`.

`check_spec.py` does these checks:

1. It applies the rules of this file to each case, and compares the result with `expect`.
2. It checks that each rule and each code in the tables has one or more cases.
3. A self-test changes a copy of the cases and checks that the checker reports each change.

The checker applies the same rules as the Host. Agreement of the checker and the vectors shows that the two agree with each other. `TestEvidenceAdmissionVectors` in `src/lifecycle-core` runs the same vectors against the Go Host rules. `TestEvidenceRead` checks the evidence read on retained documents.

## Limits

- The Host retains and returns an attachment through approval API 1.2. For a CEL bundle proposal, only the `evidence` read applies the admission rules. `propose` refuses a refused document only for a content proposal. Only the `evidence` read gives the policy inputs. Protocol 1.3 has the read on the wire, and versions 1.0 to 1.2 do not. A passing format check does not show Host admission of evidence.
- No approval policy of the Host reads the policy inputs yet. The policy verdict of [POLICY.md](../policy/POLICY.md) (proposed) is the place for that.
- Entelos cannot check a digest, a span, an outcome, a code or a review, because it does not read the receipts or the sources. The policy inputs are statements of the producer.
- Entelos does not check that an attached object agrees with its digest.
- The format has no signature and no producer authentication. The caller of `propose` is responsible for the source of the document.
- The rules do not check that a span is in its text, that a span starts on a character boundary, or that a page exists.
- The rules do not check that a review time is a real calendar time or that the answers agree with the status.
- The map from groundgate is a table. Only the seam test implements a part of it, for one invented document and groundgate 0.16.0. The repository has no other mapping code and no stored groundgate receipt. A check outside this repository mapped one groundgate receipt with 48 decisions to the schema of commit c4997ef, with 0 schema errors.
- A value longer than 65536 characters does not fit in `data.value`.
- Wire protocol 1.0 refuses a number that is not an integer ([PROTOCOL.md](../PROTOCOL.md)). `data.judgments[].p` and the box coordinates can be fractions. Thus a document with a fractional value cannot go in a `propose` request of wire protocol 1.0. Until the wire protocol accepts these numbers, the #104 implementation must send them in one stated encoding: a decimal string, or an integer with a stated scale, for example `p` in millionths. That choice changes this schema.
