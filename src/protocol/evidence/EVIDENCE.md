# Evidence format `entelos.evidence/1` (proposed)

This file specifies the admission evidence that an external producer gives for a proposal. It is the first part of issue [#104](https://github.com/mohanraj00/entelos/issues/104), and issue [#148](https://github.com/mohanraj00/entelos/issues/148) tracks it.

Status: **proposed**. No Host reads this format. The schema and the vectors in this directory are the contract for a later implementation in #104. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) checks the vectors against the schema and the rules of this file. It does not run a Host.

## Terms

| Term | Meaning |
|---|---|
| Producer | A system outside entelos that admits facts from source documents. It keeps its receipts in its own store. |
| Fact | One admitted statement. A proposal cites it. |
| Receipt | The record of the producer for one admitted fact. Entelos keeps only its digest. |
| Source | The document that contains the fact. Entelos keeps only its digest. |
| Span | The location of the fact in the source: a half-open range of byte offsets. |
| Review | The result of a verification of one fact by a reviewer, before the producer admitted it. |

## Document

An evidence document is one JSON object. [`schema/evidence.schema.json`](schema/evidence.schema.json) gives its fields.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.evidence/1`. |
| `producer.id` | string | The name of the producer. 1 to 128 characters from `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`. |
| `producer.version` | string | The version of the producer. The same characters. |
| `facts` | array | 1 to 256 facts. |
| `facts[].fact_id` | string | A name, unique in the document. The same characters. |
| `facts[].receipt_digest` | string | `sha256:` and 64 lowercase hex digits. |
| `facts[].source_digest` | string | `sha256:` and 64 lowercase hex digits. |
| `facts[].span.start` | integer | 0 or more. |
| `facts[].span.end` | integer | More than `start`. |
| `facts[].admission` | string | `automatic` or `reviewed`. |
| `facts[].review` | object | Required if `admission` is `reviewed`. Not permitted if `admission` is `automatic`. |
| `facts[].review.outcome` | string | `confirmed` or `rejected`. |
| `facts[].review.reviewer` | string | The name of the reviewer. The same characters as a name. |
| `facts[].review.reviewer_kind` | string | `human` or `model`. |

A field that is not in this table is not permitted.

A digest is an identity of the bytes that the producer hashed. It is not a signature. Entelos does not read the receipt or the source, so it cannot check that a digest is correct. The producer is responsible for its digests.

## Rules

A reader applies these rules in this order. The first rule that fails gives the result.

| Step | Rule | Result |
|---|---|---|
| 1 | The document is an object, and `format` is a string of the shape `entelos.evidence/<number>`. The number has one or more digits and no leading zero. | `invalid_evidence`, rule `format` |
| 2 | The number after the slash is `1`. | `unsupported_evidence_format` |
| 3 | The document agrees with the schema. | `invalid_evidence`, rule `schema` |
| 4 | The document has 256 facts or fewer. | `invalid_evidence`, rule `too_many_facts` |
| 5 | Each `fact_id` is unique. | `invalid_evidence`, rule `duplicate_fact` |
| 6 | For each fact, `span.end` is more than `span.start`. | `invalid_evidence`, rule `span_order` |
| 7 | For each fact with `admission` `reviewed`, `review` is present. | `invalid_evidence`, rule `review_required` |
| 8 | For each fact with `admission` `automatic`, `review` is not present. | `invalid_evidence`, rule `review_forbidden` |
| 9 | No fact has `review.outcome` `rejected`. | `invalid_evidence`, rule `rejected_fact` |

Rule 9 refuses a citation of a fact that its reviewer rejected. The producer must not admit such a fact, so a proposal cannot cite it.

A document that passes all of the rules is valid.

## Policy inputs

A valid document gives these inputs to an approval policy. Each input is a bool, so a rule of the current callable profile can read it.

| Input | Value |
|---|---|
| `evidence_human_reviewed` | `true` if one or more facts have `review.reviewer_kind` `human`. |
| `evidence_model_reviewed` | `true` if one or more facts have `review.reviewer_kind` `model`. |
| `evidence_all_automatic` | `true` if each fact has `admission` `automatic`. |

For example, a policy can refuse an automatic approval if `evidence_human_reviewed` is `true`. Then a person must decide the proposal with `decide`.

## Citation in a proposal

This section is a proposal for #104. No code implements it.

- The `propose` request gets an optional field `evidence`, with one evidence document.
- The Host applies the rules of this file before it writes the snapshot. A refusal stops the write, and the root does not change.
- The snapshot records the document. Entelos stores the digests, the spans, the reviews and the policy inputs. It does not copy the receipts or the sources.
- The approval policy reads the policy inputs.

## Versions

The major version is the number after the slash in `format`. A reader reads only the major version that it knows. An unknown major version gives `unsupported_evidence_format`. A new field or a new enum value needs a new major version, because a reader of version 1 refuses a field that it does not know.

## Vectors

[`vectors/evidence-cases.json`](vectors/evidence-cases.json) has a list of cases. Each case has:

- `name`: a unique name;
- `evidence`: the document, or a value that is not a document;
- in `evidence`, `{"$repeat": {"count": N, "fact": {…}}}` in place of `facts` is a list of N copies of the fact. The runner replaces `{i}` in `fact_id` with the index, from 0;
- `expect`: one of these:
  - `{"valid": true, "inputs": {…}}`, with the three policy inputs;
  - `{"valid": false, "code": "invalid_evidence", "rule": "<rule>"}`;
  - `{"valid": false, "code": "unsupported_evidence_format"}`.

`check_spec.py` does these checks:

1. It applies the rules of this file to each case, and compares the result with `expect`.
2. It checks that each rule and each code in the tables has one or more cases.
3. A self-test changes a copy of the cases and checks that the checker reports each change.

The checker applies the same rules that a Host will apply. Agreement of the checker and the vectors shows that the two agree with each other. It does not show that a later Host agrees with them. The implementation in #104 must run the vectors against the Host.

## Limits

- No Host reads this format yet. A passing check does not show that entelos accepts evidence.
- Entelos cannot check a digest, a span or a review, because it does not read the receipts or the sources. The policy inputs are statements of the producer.
- The format has no signature and no producer authentication. The caller of `propose` is responsible for the source of the document.
- The span uses byte offsets. The format does not say which encoding the source uses.
- The format does not record when a fact was admitted or reviewed.
