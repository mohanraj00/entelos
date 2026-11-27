# Policy verdict `entelos.policy/1`

This file specifies a second kind of verdict: a policy verdict. Issue [#154](https://github.com/mohanraj00/entelos/issues/154) tracks it. An approved policy decides a proposal with one of three outcomes: accept, refuse or route to a person.

Status: **implemented** in the Go lifecycle Host ([#210](https://github.com/mohanraj00/entelos/issues/210)). The write `policy_decide` writes this record, and the read `policy_replay` replays it. The schemas and the vectors in this directory are the contract. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors without a Host. `TestPolicyVectors` of `lifecycle-core` runs the same vectors on the rule functions that `policy_decide`, `policy_replay` and `activate` use. `TestPolicyVerdictLifecycleOnCEL` runs a typed policy on the CEL engine.

## Terms

| Term | Meaning |
|---|---|
| Item | The proposal that the policy decides: one snapshot, its report and its candidate reference ([CONTENT.md](../content/CONTENT.md#candidate-reference)). |
| Policy | A CEL rule of the profile `cel-typed/v1` with a `string` output. It goes through `propose`, `qualify`, a human `decide` and `activate`, as each rule does. |
| Policy function | A function that the Host context names as a policy. |
| Policy call | One evaluation of the active policy on the inputs of one item. |
| Receipt | The part of the verdict that names the policy call: the policy version, the input digest, the build and the value. |
| Human verdict | A `decide` record of the approval API ([APPROVAL.md](../approval/APPROVAL.md)). A reviewer principal writes it. |
| Policy verdict | A record of this file. The Host writes it. |
| Verdict requirement | `human` or `policy`. It says which verdicts can approve an item of a head. |

## Decision

1. A verdict has a kind. A `decide` record is a human verdict. A record of format `entelos.policy/1` is a policy verdict.
2. The outcome of a policy verdict is `accept`, `refuse` or `route`.
3. The Host builds the policy inputs from the evidence document, the qualification report and the change class. The producer of the item does not give them.
4. The Host fails closed. An error, a missing input, an unknown input field or an invalid value routes the item to a person.
5. A change to a policy function always needs a human verdict. A policy cannot decide its own change.
6. A `suspend` of the policy activation is the stop control. While a suspend covers the date, each item routes to a person.
7. A policy verdict never counts as a human verdict.
8. The sample rate of the audit is a fixed value of the policy bundle. A person audits each accept that the draw selects.
9. The verdict has a receipt with the policy version, the input digest and the build. A replay of the receipt gives the same outcome.

[KINDS.md](../verdict/KINDS.md) (#128) changes decision 1: a `decide` record is a human verdict only when the Host context names its actor as `human`.

## Policy

A policy is a typed rule ([typed-profile-spec.md](../../cel-lifecycle/typed-profile-spec.md)). Its environment declares each input that it reads and an output of type `string`. Its fixed values hold `audit_rate_ppm`. The approved bytes thus fix the inputs, the expression and the sample rate. A change to one of them is a new snapshot of the policy function, so it needs a human verdict (rule 1).

The Host reads these values of the policy activation that selects the date. [`schema/policy.schema.json`](schema/policy.schema.json) gives the fields.

| Field | Type | Rule |
|---|---|---|
| `function` | string | The policy function. It must be in the `policy_functions` of the Host context. |
| `activation_ref` | digest | The `enable` event of the activation. |
| `snapshot_ref` | digest | The policy snapshot that the activation selects. |
| `bundle_ref` | digest | The bundle reference of that snapshot. |
| `engine_build` | digest | The engine build of that snapshot. |
| `suspended` | bool | `true` if a `suspend` event of this activation covers the date. |
| `inputs` | array | 1 to 64 input names that the environment declares. |
| `audit_rate_ppm` | integer | The sample rate in parts per million: 0 to 1000000. |

A digest is `sha256:` and 64 lowercase hex digits. It is an identity of bytes. It is not a signature.

The Host context gives `policy_functions`, a list of function names, and `requirements`, a list of `{"head", "requirement"}` entries. A head is the head of [CONTENT.md](../content/CONTENT.md#candidate-reference): `{"kind": "cel-bundle", "function", "scope"}` or `{"kind": "content", "collection", "partition"}`. A head with no entry has the requirement `human`.

The Host checks the two fields when it starts. Each error is `configuration_error`:

- `policy_functions` is not a list of one name, or the Host does not serve that function. This version calls one policy, so the list has one name.
- `requirements` is not a list, an entry has other fields than `head` and `requirement`, a head has the wrong fields, a requirement is not `human` or `policy`, or two entries have the same head.

The Host reads the policy fields from the policy activation and its snapshot:

- `inputs` is the `caller_variables` list of the manifest.
- `audit_rate_ppm` is the fixed value of that name.
- The manifest has the profile `cel-typed/v1`, and the output of `env.json` is `{"type": "string"}`. If not, or if a bundle file is not a JSON object, rule 5 fails.
- `suspended` is `true` if no enable event of the function is live at the date, and a `suspend` event that covers the date has an enable event in `targets` whose interval covers the date. Then the Host reads the fields of that enable event.

## Policy inputs

The Host builds each input. A producer, a reviewer or a caller cannot supply an input value.

| Input | Type | Value | No value when |
|---|---|---|---|
| `report_passed` | bool | `true` if the report refusal of the item report is an empty string ([APPROVAL.md](../approval/APPROVAL.md#proposal-states)). | The item has no report. |
| `change_class` | string | The `change_class` of the item report. The qualification runner of the application gives it. | The item has no report, or the report has no change class. |
| `evidence_human_reviewed` | bool | The policy input of [EVIDENCE.md](../evidence/EVIDENCE.md#policy-inputs). | The snapshot cites no evidence, or the evidence is not valid. |
| `evidence_model_reviewed` | bool | The same. | The same. |
| `evidence_all_automatic` | bool | The same. | The same. |
| `evidence_corrected` | bool | The same. | The same. |
| `measure_max_bound_bp` | int | The largest `bound_bp` of the measure summary of the item report ([PROMOTION.md](../promotion/PROMOTION.md#measure-summary)), in basis points. A policy cannot read the bound of one route. | The item has no report, or a rule of PROMOTION.md fails for the item. |
| `measure_gold_digest` | string | The `gold_digest` of the same measure summary. | The same. |

The call gets only the inputs that the policy declares. The input object has one member for each declared name.

The `inputs_ref` of the call is the input digest of [CONTRACTS.md](../../../docs/CONTRACTS.md): `sha256:` and the lowercase hex SHA-256 of `application/json`, a NUL byte and the canonical record bytes of the input object. The canonical bytes are UTF-8 JSON with sorted member names, no white space and a final newline. `InputDigest` of `lifecycle-core` computes this value.

## Rules

The Host applies these rules to an item on a date, in this order. The first rule that fails gives the result. Each failure routes the item to a person.

| Step | Rule | Result |
|---|---|---|
| 1 | The item head is not a `cel-bundle` head with a function in `policy_functions`. | `route`, reason `policy_change` |
| 2 | The verdict requirement of the item head is `policy`. | `route`, reason `human_required` |
| 3 | A policy activation selects the date. | `route`, reason `no_policy` |
| 4 | No `suspend` event of that activation covers the date. | `route`, reason `policy_suspended` |
| 5 | The policy agrees with the policy schema, and its function is in `policy_functions`. | `route`, reason `invalid_policy` |
| 6 | Each declared input is a name of the policy inputs table. | `route`, reason `unknown_field` |
| 7 | Each declared input has a value. | `route`, reason `missing_input` |
| 8 | The call gives a value, not an error. | `route`, reason `policy_error` |
| 9 | The value is the string `accept`, `refuse` or `route`. | `route`, reason `invalid_output` |

If all of the rules pass, the outcome is the value of the call, and the reason is `policy_output`.

Steps 1 to 7 come before the call. If one of them fails, the Host does not call the policy, and the verdict has no receipt. Steps 8 and 9 come after the call, so the verdict has a receipt.

Rule 1 comes before rule 2. Thus an entry with the requirement `policy` for a policy head has no effect. Rule 4 comes before rule 5, so a suspend stops each call, also for a policy that is not valid.

A name that rule 6 refuses is not a field that a producer can add. The Host builds no value for it, so the policy cannot read it.

## Verdict record

[`schema/verdict.schema.json`](schema/verdict.schema.json) gives the fields.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.policy/1`. |
| `kind` | string | `policy`. |
| `outcome` | string | `accept`, `refuse` or `route`. |
| `reason` | string | `policy_output`, or the reason of the first rule that failed. |
| `date` | string | The date of the selection, `YYYY-MM-DD`. |
| `item.snapshot_ref` | digest | The item snapshot. |
| `item.report_ref` | digest or null | The item report. `null` if the item has no report. |
| `item.candidate` | object | The candidate reference `{"kind", "ref"}`. |
| `receipt` | object or null | The receipt of the call. `null` if no call occurred. |
| `receipt.policy_function`, `receipt.activation_ref`, `receipt.policy_snapshot_ref`, `receipt.bundle_ref`, `receipt.engine_build` | | The policy fields of the call. Together they are the policy version and the build. |
| `receipt.inputs_ref` | digest | The input digest of the call. |
| `receipt.output` | string or null | The value of the call. `null` if the call gave an error. The schema has no length limit for it. The `max_string_utf8_bytes` limit of the policy bundle bounds it. |
| `receipt.error` | string or null | The error code of the engine. `null` if the call gave a value. |
| `audit` | object or null | The audit draw. Not `null` only if `outcome` is `accept`. |
| `audit.rate_ppm` | integer | The `audit_rate_ppm` of the policy. |
| `audit.nonce` | string | 64 lowercase hex digits. The Host gets 32 bytes from a cryptographic random source for each accept. |
| `audit.selected` | bool | `true` if a person must audit this accept. |

A field that is not in this table is not permitted.

## Audit sample

The draw is the first 4 bytes of the nonce, as an unsigned big-endian integer. The accept is selected if `draw * 1000000 < audit_rate_ppm * 2^32`. Thus a rate of 0 selects no accept, and a rate of 1000000 selects each accept. The comparison uses integers only.

The nonce comes from the Host, not from the item. The author of an item cannot change bytes to get an unselected draw. The verdict records the nonce, so a replay gives the same selection.

An audit is a human verdict on the same snapshot. A `reject` from the auditor makes the snapshot `rejected` (see the next section). If the snapshot is already active, the reviewer must also `suspend` that activation. The audit does not stop the accept before a person reads it.

## Verdict requirement

The Host applies these rules to the verdicts of one snapshot:

1. If the item changes a policy function, the requirement is `human`. Else it is the requirement of the item head.
2. If the requirement is `human`, the Host ignores each policy verdict.
3. If a remaining verdict is a human `reject` or a policy `refuse`, the result is `rejected`.
4. If a remaining verdict is a human `accept` or a policy `accept`, the result is `satisfied`.
5. Else the result is `awaiting_human`.

A policy verdict does not satisfy a human verdict requirement. A policy `route` is not a verdict for or against the item. A policy `refuse` is final for the snapshot, as a human `reject` is. To change the item, propose a new revision.

## Replay

A replay takes a stored policy verdict, the stored input object, the policy snapshot that the store has for `receipt.policy_snapshot_ref`, and the build of the engine of the replay. It applies these checks in this order:

| Step | Check | Result |
|---|---|---|
| 1 | The verdict has a receipt. | `no_receipt` |
| 2 | The input digest of the input object is `receipt.inputs_ref`. | `input_mismatch` |
| 3 | The store has the policy snapshot, and its `bundle_ref` is `receipt.bundle_ref`. | `artifact_unavailable` |
| 4 | The build of the engine is `receipt.engine_build`. | `implementation_unavailable` |

Then the replay calls the policy on the input object. Rules 8 and 9 give the outcome. The result is `same` if the outcome, the value and the error are equal to the stored ones, and the stored nonce and rate give the stored `audit.selected`. Else the result is `differs`, with the replayed outcome. A replay does not change the stored verdict.

A verdict without a receipt has nothing to replay. The rule order and the stored state of the date give its outcome again.

## Host behavior

### `policy_decide`

`policy_decide` is a write of the role `reviewer`. The body is `{"report_ref", "expected_revision", "request_key"}`. It has no input, no outcome and no reason. The Host applies these request checks before the rules. Each failure is a refusal, and the Host writes no record:

1. The report and its snapshot are in the Host scope.
2. The head of the subject has the candidate of the snapshot: else `stale_verdict`.
3. No verdict on the snapshot is a reject: else `proposal_rejected`.
4. The snapshot has no earlier policy accept or refuse: else `duplicate_verdict`. A `route` does not stop a later `policy_decide`.

The date is the UTC date of the Host clock at the write. Then the Host applies the rules of this file. The call goes through the `invoke` receipt path of the Host. Thus the Host stores the canonical input bytes at `inputs_ref` and writes an `invoke` receipt with the same `inputs_ref`, policy snapshot and build. A refusal of that path, for example `implementation_unavailable`, is the error code of the call, and the rule 8 result is `policy_error`. An engine status other than `ok` is also the error code.

The Host writes a record of the store kind `policy_verdict`. The record has the format `entelos.record/1` and the fields `function`, `scope`, `from` and `until` of the item snapshot, `snapshot_ref`, `report_ref`, `verdict` (the verdict record of this file), `requester` (the principal of the request), `decided_at` and `receipt_ref` (the `invoke` receipt, only if a call occurred). The scope fields let the `record` read and `inspect` check the scope.

For an `accept` or a `refuse`, the Host also writes a `decision` record of the kind `policy`. Its `actor` is the policy function, its `disposition` is `accept` or `reject`, its `reason` is `policy_output` and its `policy_verdict_ref` names the policy verdict record. The other fields are the fields of a `decide` record. Thus `activate`, the verdict states of [VERDICT.md](../verdict/VERDICT.md) and the counts of [KINDS.md](../verdict/KINDS.md) read it. A `route` writes no decision record, so the proposal stays `awaiting_verdict`.

The result is `{"policy_verdict_ref", "verdict"}`, and `decision_ref` for an accept or a refuse.

### Verdict requirement in the Host

The requirement rules use these verdicts: a `decide` record is a human verdict, and a decision record with `policy_verdict_ref` is a policy verdict. A `decide` record of an actor of the kind `model` or `policy` stays a human verdict for these rules. The counts of KINDS.md still count it for its own kind only.

If the requirement of a snapshot is `human`, the Host ignores each policy verdict on it: in the verdict states, the `approval` read, the counts, the `proposals` read and `decide`. `activate` applies the requirement rules after the checks of VERDICT.md:

- `rejected` gives `proposal_rejected`.
- `awaiting_human` gives `human_verdict_required`.
- An activated decision of a policy verdict that the Host ignores gives `human_verdict_required`.

After these checks, the verdict policy of KINDS.md applies. A policy accept counts only for the kind `policy`, so a verdict policy with `"human": 1` refuses it with `verdicts_required`.

### `policy_replay`

`policy_replay` is a read of the role `reviewer`. The body is `{"policy_verdict_ref", "cursor"}`. The Host reads the stored input object at `receipt.inputs_ref`, the policy snapshot at `receipt.policy_snapshot_ref` and the build of its engine. Then it applies the replay checks of this file. The call runs the stored bundle files on the engine, and it writes nothing. The result is the replay result of the vectors, with `cursor` and `policy_verdict_ref`.

The `replay` operation of the `invoke` receipt also works. It writes a replay receipt with `matches_original`.

### Other parts

- The qualification report gets the optional field `change_class`. The `ChangeClass` field of the runner verdict gives it. A report without a change class keeps the bytes of an earlier Host.
- The stop control is the current `suspend` operation, with the policy activation in `targets`.
- The Go API and the JSONL Host have `policy_decide` and `policy_replay`. The wire protocol of the sidecar and the HTTP binding does not have them.

## Versions

The major version is the number after the slash in `format`. A reader reads only the major version that it knows. A new field, a new reason, a new outcome or a new policy input needs a new major version, because a reader of version 1 refuses a field or a value that it does not know.

The two `measure_` inputs of [PROMOTION.md](../promotion/PROMOTION.md) ([#152](https://github.com/mohanraj00/entelos/issues/152)) are part of version 1. They came before a Host read version 1.

## Vectors

[`vectors/policy-cases.json`](vectors/policy-cases.json) has a `defaults` object and three lists. The field `synthetic_policy` gives the policy expression of the cases. A person read that expression to state each call value. The checker does not run it.

Each item of `cases` has a `name`, an optional `note`, and an `expect` with the verdict record. It can replace one of the members of `defaults`: `date`, `policy_functions`, `requirements`, `policy`, `item`, `sources` and `nonce`. `policy` is `null` if no activation selects the date. `sources.evidence` is the four evidence inputs or `null`. `sources.report` is `{"passed", "change_class"}` or `null`. A case has `call`, `{"output": <string>}` or `{"error": <code>}`, only if the rules reach the call.

Each item of `replays` has `of`, the name of the case that has the stored verdict, `inputs`, `store_policy` (`{"snapshot_ref", "bundle_ref"}` or `null`), `build`, an optional `call` and `expect`: `{"replay": "same"}`, `{"replay": "differs", "outcome": …}` or `{"replay": "refused", "code": …}`.

Each item of `requirements` has `requirement`, `changes_policy`, `verdicts` and `expect`: `satisfied`, `rejected` or `awaiting_human`. A verdict is `{"kind": "human", "disposition"}` or `{"kind": "policy", "outcome"}`.

The expected `inputs_ref` values were calculated with `printf` and `shasum -a 256` on the envelope bytes. The checker calculates them again with Python `hashlib`.

`check_spec.py` does these checks:

1. It checks each expected verdict against the verdict schema.
2. It applies the rules of this file to each case, and compares the result with `expect`. A case that has a `call` but routes before the call is an error.
3. It applies the replay checks to each replay, and the requirement rules to each requirement case.
4. It checks that each reason of the rules table, each outcome, a selected and an unselected audit draw, each replay result and each requirement result has one or more cases. It checks that a case shows a policy accept on a human head as `awaiting_human`.
5. A self-test changes a copy of the vectors and checks that the checker reports each change.

The checker applies the same rules as the Host. Agreement of the checker and the vectors shows that the two agree with each other. `TestPolicyVectors` runs the cases, the replays and the requirement cases on the Go rule functions `judgePolicy`, `replayPolicy` and `verdictRequirement`. The vectors state each call value, so this test does not run CEL. The Host tests in `policy_test.go` run `policy_decide`, `activate`, `policy_replay` and `replay` through `Handle` with a fake engine. `TestPolicyVerdictLifecycleOnCEL` runs the path `propose`, `qualify`, `policy_decide`, `activate`, `policy_replay` and `replay` with a typed policy on the CEL engine.

## Limits

- The vectors state each call value, so `TestPolicyVectors` does not run CEL. Only `TestPolicyVerdictLifecycleOnCEL` runs a policy on the engine, and it needs `LIFECYCLE_CEL_BINARY`.
- The vectors use invented digests, so they do not go through a store. The Host tests and the CEL test go through a store, and they cover a part of the reasons.
- A policy reads only the largest bound of a measure summary. It cannot read the bound of one route.
- This version calls one policy function for each Host.
- The `proposal` read does not list the policy verdicts, and the `proposals` read does not show a selected accept as an audit task. The `policy_decide` result and the `record` read give the verdict.
- The decision stream of [EVENTS.md](../events/EVENTS.md) does not export a policy verdict.
- The wire protocol does not have `policy_decide` or `policy_replay`.
- The change class is a statement of the qualification runner. Entelos does not check it. The runner policy of the Host decides which runner it trusts.
- The evidence inputs are statements of the producer ([EVIDENCE.md](../evidence/EVIDENCE.md#limits)). A policy that reads them trusts the producer.
- The audit selects a sample. It does not show that the other accepts are correct. An audit happens after the accept, so a selected accept can be active before a person reads it.
- A policy verdict has no signature. Its digests are identities, not signatures.
- This file does not specify a decision record in `docs/decisions`.
