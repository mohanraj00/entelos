# Finding import contract

An agent test can find a defect. This contract tells how one finding record of that test becomes a candidate qualification case, and how a reviewer approves the candidate. Issue [#112](https://github.com/mohanraj00/entelos/issues/112) adds it. `finding.go` holds the Go code. Each declaration is experimental.

The input is a finding record of [nookku](https://github.com/mohanraj00/nookku), record version 1.0. Section 10 of the nookku `SPEC.md` defines the record. The core has no dependency on nookku. It reads the record text with the rules of this file.

## Terms

- A finding record is one confirmed defect of an agent, as one JSON object. It has the messages of the test, the expected value, the observed value, the adjudication and a seal.
- A candidate is the stored result of an import. It is not a case.
- A case is an entry of the `cases` list of a plan. The [runner contract](RUNNER.md) gives that list to the qualification runner.
- A byte digest is `sha256:` and the SHA-256 of the exact bytes, as 64 lower-case hex characters.

## Clauses

### F1. Input

The Host-only write `import_finding` has the role `author` and two fields:

| Field | Value |
|---|---|
| `source_ref` | The source that the candidate belongs to. The candidate gets the function, the scope and the interval of this source. |
| `finding` | The exact text of one finding record, as a string. The limit is 256 KiB of UTF-8 bytes. |

The request names no runner, no snapshot and no plan. The wire protocol does not have this operation.

### F2. Reader rules

The Host reads the text as strict JSON: one object, no duplicate key, no lone surrogate, and no `NaN` or `Infinity`. Then it applies these rules in this order. The first rule that fails refuses the import.

| Rule | The import is refused if |
|---|---|
| `too_large` | The text is larger than the limit. |
| `not_json` | The text is not one strict JSON object. |
| `not_finding` | The field `record` is not `nookku.finding`. |
| `version_form` | The field `version` is not a string of the form `<major>.<minor>`, two whole numbers with no leading zero. |
| `version_major` | The major version is not 1. |
| `missing_field` | A field of record version 1.0 is not in the record. |
| `missing_key` | An object of the record does not have a key of record version 1.0. A key with the value `null` is a key. |
| `field_value` | A field has a type or a value that record version 1.0 does not permit. This includes an unknown value of a closed set, and a mismatch with the author type `model`. |
| `turn_not_in_script` | `turn.number` is not the number of a message of `script`, or `turn.input` is not that message. |
| `seal_digest` | `seal.sha256` is not the SHA-256 of the UTF-8 bytes of `seal.text`. |
| `seal_text` | `seal.text` is not a JSON object with the `test` of the record and an object `files`. |
| `seal_coverage` | A name of `seal.files` does not have the same SHA-256 in the `files` of the seal text. |
| `seal_sources` | `seal.files` does not name each sealed file of the record kind, or it does not name the script copy with `script.sha256`. |

The version rules are the reader rules of the record format:

- The reader ignores a field that it does not know, in the record and in each object of the record. The candidate does not get such a field.
- The reader accepts a newer minor version, for example `1.7`.
- The reader refuses an unknown major version. It does not guess the meaning of the fields.

The rule `version_major` gives the fault `unsupported_finding_version`. Each other rule gives the fault `invalid_finding`. A refused import writes nothing.

The seal rules show that the record agrees with its own seal text. They do not show that the sealed test is unchanged, and they do not show who wrote the record.

### F3. Candidate

A valid record gives one stored record of the kind `case_candidate`:

| Field | Value |
|---|---|
| `function`, `scope`, `from`, `until`, `source_ref` | The values of the source. |
| `importer` | The actor of the import. |
| `input_digest` | The byte digest of the exact `finding` text. |
| `origin` | The fields that identify the finding: `record`, `version`, `id`, `kind`, `subject`, `seal_digest`, `adjudication` (`outcome`, `reason`, `author`, `author_type`), `gate` and `producer_version`. |
| `case` | The case that a runner gets after approval. |

The `case` object has the format `entelos.finding-case/1`:

| Field | Value |
|---|---|
| `format` | `entelos.finding-case/1`. |
| `name` | The `id` of the finding. |
| `messages` | The messages of `script` from message 1 to message `turn.number`, in order. If `turn` is `null`, each message. |
| `expected` | `value`, `match`, `tolerance` and `field` of the finding, or `null` for an issue row. The match rules are the rules of the record format. |
| `issue` | `class`, `evidence` and `text` of an issue row, or `null` for a mismatch. |

The candidate does not copy `observed`, `turn.reply`, the adjudication time or the seal text. The input digest binds each byte of the record. The Host does not keep the record text, so the embedding application keeps it.

The import writes the candidate and its entry in the state index `case_candidates`. It writes no qualification report, no verdict and no event. It gives no passed flag, and it activates nothing. The same importer, source and text give the same candidate reference.

### F4. Approval

The Host-only write `decide_case` has the role `reviewer` and three fields: `candidate_ref`, `disposition` and `reason`. The disposition is `accept` or `reject`, and the request must give it.

The Host applies these checks in this order:

1. The disposition is `accept` or `reject`, and the reason is not empty. Else the fault is `invalid_request`.
2. The reviewer is not the importer of the candidate. Else the fault is `separation_required`.
3. The candidate has no decision. Else the fault is `duplicate_case_decision`.
4. For `accept`, the candidate has an expected value. Else the fault is `case_without_expectation`.

A valid request gives one stored record of the kind `case_decision`. It has the function, the scope and the interval of the candidate, `candidate_ref`, `input_digest`, `actor`, `reason`, `disposition` and `decided_at`. If the Host context gives a kind for the actor, the record has `kind`.

A candidate has one decision at most. A reject is final for that candidate. A candidate of an issue row has no expected value, so a reviewer cannot accept it. The reviewer writes the check by hand as a case of a plan.

A case decision is not a verdict on a snapshot. It changes no report, no head and no event.

### F5. Plan

A candidate is not a case before an accept decision. Only a plan makes a case of it.

The `plan` request takes the Host-only field `imports`: a list of candidate references. For each reference, the Host applies these checks:

1. The reference is in the list one time. Else the fault is `invalid_plan`.
2. The candidate has the `source_ref` of the plan. Else the fault is `case_source_mismatch`.
3. The candidate has an accept decision. Else the fault is `case_not_approved`.

The `cases` list of the plan record has the cases of the request, and then the `case` object of each import in the request order. The record field `imports` names each `candidate_ref` with its `decision_ref`. A plan needs one case or more, from `cases` or from `imports`. The request always has the `cases` field, and the list can be empty if `imports` is not. A plan with no import has no `imports` field, so its bytes do not change.

`qualify` gives the runner only the cases of the plan record. Thus a candidate with no accept decision has no effect on a qualification. A refused plan writes nothing.

### F6. Runner

The Host does not interpret a case. The installed runner reads the format `entelos.finding-case/1` and applies the expected value to the candidate. A runner that does not know the format must give a fault, not a passed status. The admission rules of the [runner contract](RUNNER.md#admission) apply with no change.

## Example

The example is invented. An agent answers questions about the due dates of a sample library catalog. The catalog is the fixed data of a snapshot.

1. The old catalog gives 2026-12-16 for one title. The answer key of the test gives 2026-12-09. A person adjudicates the mismatch as `agent_wrong`, and the test tool exports one finding record.
2. An author imports the record. The candidate names the subject of the finding, which is the digest of the old catalog.
3. A reviewer accepts the candidate, and installs a plan that supersedes the first plan and imports the candidate.
4. The qualification of the old snapshot with the new plan fails. The qualification of a snapshot with the corrected catalog passes.

## Evidence

`finding_test.go` has these tests. They use the in-memory store and a fixture runner that answers each case from the fixed data of the candidate. A person wrote the records and the expected values in that file.

- `TestFindingReaderRules` applies each rule of clause F2 to a changed record. It includes an unknown field, a newer minor version and two unknown major versions.
- `TestImportFindingGivesTheCandidate` compares the stored candidate with hand-written values.
- `TestImportFindingRefusals`, `TestCaseDecisionRules` and `TestPlanTakesOnlyAnAcceptedCandidate` cover each fault of clauses F2, F4 and F5.
- `TestFindingCaseFailsOnTheOldSnapshotAndPassesOnTheCorrectedSnapshot` runs the example. Before the approval, a second `qualify` gives the same report reference, and a plan with the candidate is refused with no new state.

The seam test `tests/test_nookku_seam.py` gives records of nookku to `TestFindingSeam`, which runs the reader, the import and the example on them. `make verify` runs it as the check `nookku-seam`.

The seam test runs nookku 0.6.1. `tests/nookku-seam/requirements.txt` pins the wheel of PyPI by its SHA-256, and `verify.py` installs it in a virtual environment. The released tool runs the example with a local agent that has fixed replies, and no model runs. It exports three records: one mismatch, one issue row of a person and one mismatch of a gate run. The test also makes nine changed copies of the mismatch record. The copies have an unknown field, a newer minor version, no `subject`, no `script.sha256`, an unknown major version, two seal mismatches, a wrong turn input and a model author. A person wrote the expected rule of each copy, and the reader of the release must agree on each refusal.

## Limits

- No shipped runner reads the case format. The CEL runner does not. The fixture runner of the tests reads only the `exact` match on one field.
- The reader is stricter than the reader of nookku in one case: it refuses a duplicate JSON key.
- A case decision has no signed record. The signed records of [SIGNED.md](../protocol/signed/SIGNED.md) cover only verdicts and events.
- The wire protocol and the sidecar have no `import_finding`, no `decide_case` and no `imports` field.
- A candidate belongs to one source. A plan of another source needs a new import.
- The subject of a finding is text that the caller of the test tool gave. The Host records it and does not compare it with a snapshot.
- The seam test has no model row. `finding_test.go` covers a model row with a hand-written record.
