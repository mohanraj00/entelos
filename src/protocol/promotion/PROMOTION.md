# Profile promotion (proposed)

This file specifies a typed decision to promote a version of an extraction profile. Issue [#152](https://github.com/mohanraj00/entelos/issues/152) tracks it. A profile tells a document-extraction tool, for example groundgate, how to read one family of documents. A change to a profile is a knowledge change, so it gets the same approval, receipt and audit as a content update.

Status: **proposed**. The Go lifecycle Host stores the measure summary of the runner and applies the rules of this file in the promotion read. No Host writes a policy verdict yet. The schema and the vectors in this directory are the contract for the implementation. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors. It uses the checkers of [CONTENT.md](../content/CONTENT.md) and [POLICY.md](../policy/POLICY.md) for their rules. It does not run a Host or a CEL engine.

## Terms

| Term | Meaning |
|---|---|
| Profile | The JSON text that configures the extraction tool for one family of documents. Entelos does not read it. |
| Measure report | The JSON text that the measure tool of the application wrote for one profile version. It has the bound of each route, the gold set digest and the audit labels. Entelos does not read it. |
| Route | One way that the tool admits the value of a field, for example `checked` or `cleared`. |
| Bound | The upper bound of the admitted error rate on one route, from the measure report. |
| Basis point | 1/10000. A bound of 0.02 is 200 basis points. |
| Measure summary | The object of format `entelos.measure/1` that the qualification runner gives in the report. |
| Promotion | A content proposal of this file: a manifest that names one profile version and its measure report. |
| Limit | The largest bound that the policy accepts. It is a value of the policy, not of the runner. |

## Decision

1. A promotion is a content proposal of [CONTENT.md](../content/CONTENT.md). The rules for the head, the interval, the approval, the activation and the activation record apply without change.
2. The Host context field `promotion_collections` lists the collections whose content proposals are promotions. The partition is the profile id. Each version of a profile is a new proposal of the same head.
3. The manifest has two items: `profile` and `measure`. Their digests name the profile and the measure report.
4. The qualification runner of the application reads the two items. It checks their digests, and it gives the measure summary in the report. Entelos does not read the items.
5. A bound is an integer in basis points, so that the typed profile `cel-typed/v1` needs no float.
6. The Host builds two policy inputs from the measure summary. A policy verdict of [POLICY.md](../policy/POLICY.md) can accept the promotion when each bound is in the limit. Else the policy refuses it, or routes it to a person.
7. A limit is in the policy. A change to a limit is a change to the policy function, so it always needs a human verdict (POLICY.md, rule 1).

## Candidate

A promotion is a content candidate. Its manifest agrees with [CONTENT.md](../content/CONTENT.md#content-manifest), and it has these extra rules:

| Field | Rule |
|---|---|
| `collection` | A name in the `promotion_collections` of the Host context, for example `extraction-profiles`. |
| `partition` | The profile id, for example `fee-schedule`. |
| `items` | Two items: `measure`, then `profile`. Each one has the `media_type` `application/json`. |
| `metadata.profile_id` | The profile id. It is equal to `partition`. |
| `metadata.profile_version` | The profile version, as `MAJOR.MINOR.PATCH`, for example `2.0.0`. |

The manifest can have other metadata entries. They have no effect in this file.

## Measure summary

The qualification report gets an optional field `measure`. It is the measure summary. [`schema/summary.schema.json`](schema/summary.schema.json) gives its fields.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.measure/1`. |
| `profile_digest` | digest | The digest of the profile bytes. It is equal to the digest of the item `profile`. |
| `measure_digest` | digest | The digest of the measure report bytes. It is equal to the digest of the item `measure`. |
| `gold_digest` | digest | The digest of the gold set of the measure, from the measure report. |
| `labels_digest` | digest | The digest of the audit labels of the measure, from the measure report. |
| `routes` | array | 1 to 2000 routes. |
| `routes[].field` | string | The field name, 1 to 256 characters. |
| `routes[].route` | string | The route name, 1 to 64 characters. |
| `routes[].bound_bp` | integer | The bound in basis points: 0 to 10000. |

A field that is not in this table is not permitted. Thus the summary has no limit: the runner cannot set the limit.

A digest is `sha256:` and 64 lowercase hex digits. It is an identity of bytes. It is not a signature. The runner of the application must calculate the two item digests from the bytes that it read.

### Bound conversion

The measure report gives a bound as a decimal string from 0 to 1, for example `0.0605`. The runner converts it to basis points:

1. The string is `0` or `1`, then optionally `.` and one or more digits. Else the bound is not valid.
2. The value is not more than 1. Else the bound is not valid.
3. `bound_bp` is the smallest integer that is not less than the value times 10000.

Thus the conversion rounds up. A rounded bound is never less than the measured bound. For example, `0.02` gives 200, `0.0605` gives 605 and `0.050001` gives 501. The runner refuses the report if a bound is not valid.

## Rules

The Host applies these rules to an item, in this order. The first rule that fails gives the result. If a rule fails, the two measure inputs have no value. Thus a policy that reads them routes the item to a person with the reason `missing_input` of POLICY.md.

| Step | Rule | Result |
|---|---|---|
| 1 | The candidate has the kind `content`. | `not_promotion` |
| 2 | The candidate passes the rules of CONTENT.md. | `content` |
| 3 | The `collection` is in `promotion_collections`. | `not_promotion` |
| 4 | The manifest has the items `measure` and `profile` and no other item. Each one has the `media_type` `application/json`. | `items` |
| 5 | `metadata.profile_id` is equal to `partition`, and `metadata.profile_version` has the form `MAJOR.MINOR.PATCH`. | `metadata` |
| 6 | The item has a report, and the report has a measure summary. | `no_summary` |
| 7 | The summary agrees with the summary schema. | `summary_schema` |
| 8 | `profile_digest` and `measure_digest` are equal to the digests of the items `profile` and `measure`. | `digest_binding` |
| 9 | No two routes have the same `field` and `route`. | `duplicate_route` |

Rule 8 binds the summary to the manifest. The Host has the manifest text, because it is the candidate. Thus a summary of one version cannot qualify another version.

## Policy inputs

If all of the rules pass, the Host builds these values. [POLICY.md](../policy/POLICY.md#policy-inputs) lists them with its other inputs.

| Input | Type | Value |
|---|---|---|
| `measure_max_bound_bp` | int | The largest `bound_bp` of the routes. |
| `measure_gold_digest` | string | The `gold_digest` of the summary. |

A policy compares the largest bound with its limit, so one limit applies to each route. A policy can also name the gold set digest that it trusts. A measure on another gold set then routes to a person.

## Example

The vectors use this synthetic policy. It reads `report_passed`, `measure_gold_digest` and `measure_max_bound_bp`:

```
!report_passed ? "refuse" : (measure_gold_digest != "sha256:99…99" ? "route" : (measure_max_bound_bp <= 500 ? "accept" : "refuse"))
```

The limit is 500 basis points, or 0.05.

- **Accepted.** Version `2.0.0` of the profile `fee-schedule` has three routes, each with a bound of 0.02 (200). The largest bound is 200, so the policy accepts. The case `promotion-accepted` has the exact manifest and the verdict.
- **Refused.** Version `2.1.0` has the bound 0.062 (620) on the route `fee`/`cleared`. The largest bound is 620, so the policy refuses. The case `promotion-refused` has the verdict.

The reason of a refusal is in the stored records. The receipt of the verdict has the input digest of `{"measure_gold_digest", "measure_max_bound_bp": 620, "report_passed": true}`. The report has the summary, and its routes show which route is above the limit. The vectors state that route, and the checker compares it with the summary.

## Activation record

The activation record of [ACTIVATION.md](../activation/ACTIVATION.md) does not change. Its `candidate.ref` is the SHA-256 of the manifest text, and the manifest names the profile digest. A consumer applies these steps:

1. Get the manifest text by `candidate.ref`.
2. Calculate the SHA-256 of the exact text, and compare it with `candidate.ref`. Else refuse the record with `candidate_mismatch`.
3. Read the digest of the item `profile`, and `metadata.profile_version`.

The consumer then loads only the profile bytes with that digest.

## Host behavior

The Go lifecycle Host applies this section (#152). The wire protocol does not have the promotion read yet.

- The Host context gets the optional field `promotion_collections`, a list of nonempty collection names. A value of another type gives `configuration_error` when the Host starts.
- The runner gives the measure summary in `Verdict.Measure` ([RUNNER.md](../../lifecycle-core/RUNNER.md#verdict)). If it is not nil, `qualify` stores it as the report field `measure`. `qualify` does not check it. A report without a summary keeps the bytes of an earlier Host.
- The promotion read applies the rules of this file to the stored report and its snapshot.
- The verdict requirement of a promotion head is `policy` only if the Host context gives that requirement for the head. Else a person decides each promotion.

### The promotion read

Role: `reviewer`, as `proposal`. Body: `{"report_ref": string, "cursor"?: string}`. The report and its snapshot must be in the host scope.

Result: `{"cursor", "report_ref", "snapshot_ref", "promotion", "rule"?, "inputs"?}`.

| `promotion` | Meaning | Other fields |
|---|---|---|
| `valid` | The candidate and the report pass all of the rules. | `inputs`: the two measure inputs. |
| `invalid` | A rule fails. | `rule`: the result of the first rule that fails. |

The candidate is the kind and the exact manifest text of the snapshot. A Host without `promotion_collections` gives `not_promotion` for each report. The read writes nothing. The rules apply also to a failed report: the policy reads `report_passed` separately.

No Host builds the policy inputs of POLICY.md yet, because no Host writes a policy verdict. The promotion read gives the two inputs that a later policy evaluation will use.

## Versions

The major version is the number after the slash in `format`. A reader reads only the major version that it knows. A new field of the measure summary needs a new major version.

POLICY.md says that a new policy input needs a new major version of the policy verdict. No Host reads `entelos.policy/1` yet, so this file adds its two inputs to version 1.

## Vectors

[`vectors/promotion-cases.json`](vectors/promotion-cases.json) has a `defaults` object, a `manifests` map of exact manifest texts, a `summaries` map and four lists.

- `bounds`: a decimal string and the expected `bound_bp`, or `null` if the bound is not valid.
- `checks`: a manifest name (or a `candidate`), a summary name (or `null`), an optional `report` and the expected result: `{"valid": true, "inputs"}` or `{"valid": false, "rule"}`.
- `cases`: a manifest, a summary, an optional `report`, an optional `item`, an optional `call` and the expected policy verdict. A refused case can have `refusal`: the limit and the routes above it.
- `activations`: the `candidate` of a record, a manifest name and the expected result of the consumer steps.

The expected manifest refs were calculated with `printf` and `shasum -a 256` on the exact manifest text. The expected `inputs_ref` values were calculated with `printf` and `shasum -a 256` on the envelope bytes. The checker calculates them again with Python `hashlib`.

`check_spec.py` does these checks:

1. It applies the bound conversion to each item of `bounds`.
2. It applies the rules of this file to each check. It uses the CONTENT.md checker for rule 2.
3. It gives the measure inputs to the POLICY.md checker and compares the verdict of each case with `expect`. It checks each expected verdict against the verdict schema of POLICY.md.
4. It checks that the routes of a refusal are the routes above the limit, and that the synthetic policy has that limit.
5. It applies the consumer steps to each activation.
6. It checks that each rule of the rules table has a check, and that the cases have an accept, a refuse, a route by the policy and a route before the call.
7. A self-test changes a copy of the vectors and checks that the checker reports each change.

The checker and the Host apply the same rules. Agreement of the checker and the vectors shows that the two agree with each other. `TestPromotionVectors` in `src/lifecycle-core` runs the checks against the Go rules, and it checks that the measure inputs are absent exactly for each case that routes with `missing_input`. `TestPromotionRead` runs the promotion read on reports that a fixture runner stored. A fixture runner is not the runner of an application.

## Limits

- No Host writes a policy verdict yet. A valid promotion read does not show that entelos accepts a promotion.
- The Host does not check the summary at `qualify`. A runner can store a summary that a rule refuses. The promotion read then gives the rule.
- Entelos does not read the profile or the measure report. The summary is a statement of the qualification runner of the application. The runner policy of the Host decides which runner it trusts.
- One limit applies to all routes. A limit for each route needs a new input.
- The rules do not compare the profile version with the version of the base proposal. The `base` of CONTENT.md orders the proposals of a head.
- The checker does not run CEL. The call values are statements of a person who read the synthetic policy.
- The item digests in the vectors are synthetic. No profile or measure report has these bytes.
