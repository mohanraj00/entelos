# Content proposal `entelos.content/1`

This file specifies a second kind of candidate: a content proposal. Issues [#153](https://github.com/mohanraj00/entelos/issues/153) (the spec) and [#175](https://github.com/mohanraj00/entelos/issues/175) (the Host) track it. A content proposal goes from proposal to activation with no CEL file.

Status: **implemented in the Host, unstable**. The Host reads this format through the Host request field `candidate` of `propose`. The wire protocol does not have that field yet. The schemas and the vectors in this directory are the contract. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors, without a Host. The Go tests of `src/lifecycle-core` run the same vectors against the Host. [Decision 0003](../../../docs/decisions/0003-content-kind.md) records the kind model.

## Terms

| Term | Meaning |
|---|---|
| Candidate | The object that a `propose` request gives for review. It has a `kind`. |
| Kind | `cel-bundle` or `content`. The kind selects the rules that apply to the candidate. |
| CEL bundle | The four files of the current behavior: `manifest`, `environment`, `source` and `fixed`. |
| Content manifest | A JSON text that lists content items by digest. It is the candidate of kind `content`. |
| Content item | One document of the application, for example a list of rules. Entelos keeps only its digest, its size and its declared media type. |
| Candidate reference | `{"kind", "ref"}`. It identifies one candidate. A report, a verdict and a later policy verdict name a candidate with it. |
| Head | The key of a proposal list. The current head of a key is the last proposal for that key. |

## Decision

1. A candidate has a kind. The kind `cel-bundle` keeps the current behavior. The kind `content` is a content manifest.
2. Entelos stores the manifest text. It does not read, fetch or store the content items.
3. A runner of the application qualifies a content candidate through the existing qualification runner port. The runner reads the items. Entelos does not.
4. `load` and `invoke` stay for `cel-bundle` only.
5. The head of a content proposal is a collection and a partition. The head of a CEL bundle proposal stays a function and a scope.
6. The candidate reference is generic. A record that names a candidate uses `{"kind", "ref"}` for each kind.

## Candidate

A candidate is one JSON object. [`schema/candidate.schema.json`](schema/candidate.schema.json) gives its fields.

| Kind | Fields | Rule |
|---|---|---|
| `cel-bundle` | `kind`, `bundle` | `bundle` has the four string fields `manifest`, `environment`, `source` and `fixed`, as the 1.0 `propose` body has. |
| `content` | `kind`, `manifest` | `manifest` is the content manifest, as a string. |

A field that is not in this table is not permitted.

For `cel-bundle`, the candidate rules of this file stop after the candidate schema. The bundle checks and the `bundle_ref` of the current Host apply without change. The candidate reference is `{"kind":"cel-bundle","ref":<bundle_ref>}`.

## Content manifest

The content manifest is a JSON text. It is one object. [`schema/manifest.schema.json`](schema/manifest.schema.json) gives its fields.

| Field | Type | Rule |
|---|---|---|
| `format` | string | `entelos.content/1`. |
| `collection` | string | The name of the collection. 1 to 128 characters from `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`. |
| `partition` | string | The name of the partition in the collection. The same characters. |
| `base` | string | An empty string for the first proposal of a head. Else the candidate `ref` of the content proposal that this proposal changes. |
| `items` | array | 1 to 1024 items, in ascending order of `item_id`. |
| `items[].item_id` | string | A name, unique in the manifest. The same characters as `collection`. |
| `items[].digest` | string | `sha256:` and 64 lowercase hex digits. |
| `items[].size` | integer | The size of the item in bytes. 0 or more. |
| `items[].media_type` | string | Optional. A media type such as `text/markdown`, in lowercase. |
| `items[].applicability` | object | Optional. The applicability values of the item. 1 to 16 entries. Each name has the same characters as `collection`. Each value is an array of 1 to 8 levels. Each level is a string of 1 to 256 characters. |
| `metadata` | object | Optional. 0 to 64 entries. Each name has the same characters as `collection`. Each value is a string of 0 to 1024 characters. |
| `evidence` | object | Optional. A reference to an evidence document. |
| `evidence.format` | string | `entelos.evidence/1`. |
| `evidence.digest` | string | `sha256:` and 64 lowercase hex digits. |

A field that is not in this table is not permitted. The names in `metadata` and in `applicability` are data, not fields.

The `evidence` reference names an evidence document of format `entelos.evidence/1` ([#148](https://github.com/mohanraj00/entelos/issues/148)) by its digest. This file does not specify how the Host gets or checks that document.

A digest is an identity of bytes. It is not a signature. Entelos does not read the items, so it cannot check an item digest or an item size. The qualification runner of the application must check them.

## Applicability

An item can declare applicability values ([#106](https://github.com/mohanraj00/entelos/issues/106)). A store uses them to select the items that apply to a read. The application declares the names and their meaning. For example, `{"region": ["eu", "de"]}` is the field `region` with two levels, from the widest to the narrowest.

Version 1 has one value type: an array of level strings. Entelos records the values as exact data. It does not interpret them, and it does not compare one level with another level. A store applies them as exact filters.

The values are on the items. The effective interval stays on the activation, and it applies to all of the items of the manifest. The activation confirms the values of the items. It does not give a second set.

For example, a manifest has two items with different regions:

```json
{"format":"entelos.content/1","collection":"loan-rules","partition":"region-a","base":"",
 "items":[
  {"item_id":"rules-de","digest":"sha256:dd…","size":200,"applicability":{"region":["eu","de"]}},
  {"item_id":"rules-fr","digest":"sha256:ff…","size":180,"applicability":{"region":["eu","fr"]}}]}
```

The `activate` request of its decision confirms both sets:

```json
{"op":"activate","decision_ref":"…","targets":[],
 "applicability":{"rules-de":{"region":["eu","de"]},"rules-fr":{"region":["eu","fr"]}}}
```

The case `two-regions-confirmed` of the lifecycle vectors runs this example with the exact text.

## Candidate reference

The `ref` of a content candidate is `sha256:` and the lowercase hex SHA-256 digest of the exact UTF-8 bytes of the manifest text. The Host does not canonicalize the text. Thus a change of one byte, also of white space, gives a new reference.

These rules apply to the reference:

- A report binds the candidate reference of its snapshot. A report qualifies a snapshot only if the `kind` and the `ref` of the two are equal.
- A changed manifest has a new reference. Thus it needs a new qualification. A passed report never moves to changed bytes.
- A verdict and a later policy verdict ([#154](https://github.com/mohanraj00/entelos/issues/154)) name the candidate with the same `{"kind", "ref"}`.

The head of a candidate is:

| Kind | Head |
|---|---|
| `cel-bundle` | `{"kind":"cel-bundle","function","scope"}`. The `function` comes from the `propose` body. The `scope` comes from the Host context. |
| `content` | `{"kind":"content","collection","partition"}`, from the manifest. |

## Rules

A reader applies these rules to a candidate in this order. The first rule that fails gives the result.

| Step | Rule | Result |
|---|---|---|
| 1 | The candidate is an object, and `kind` is a string. | `invalid_candidate`, rule `kind` |
| 2 | `kind` is `cel-bundle` or `content`. | `unsupported_candidate_kind` |
| 3 | The candidate agrees with the candidate schema of its kind. | `invalid_candidate`, rule `candidate_schema` |
| 4 | The manifest text has 262144 UTF-8 bytes or fewer. | `invalid_content`, rule `too_large` |
| 5 | The manifest text is one JSON value, and that value is an object. No object in the text has the same member name two times. | `invalid_content`, rule `json` |
| 6 | `format` is a string of the shape `entelos.content/<number>`. The number has one or more digits and no leading zero. | `invalid_content`, rule `format` |
| 7 | The number after the slash is `1`. | `unsupported_content_format` |
| 8 | Each member name in the manifest is a field of the manifest table. | `invalid_content`, rule `unknown_field` |
| 9 | The manifest agrees with the manifest schema. | `invalid_content`, rule `schema` |
| 10 | The manifest has 1024 items or fewer. | `invalid_content`, rule `too_many_items` |
| 11 | Each `item_id` is unique. | `invalid_content`, rule `duplicate_item` |
| 12 | The items are in strictly ascending order of `item_id`, by Unicode code point. | `invalid_content`, rule `item_order` |
| 13 | `metadata` has 64 entries or fewer. | `invalid_content`, rule `too_many_metadata` |
| 14 | Each name in `metadata` has 1 to 128 characters from `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`. | `invalid_content`, rule `metadata_name` |
| 15 | Each `applicability` of an item has 1 to 16 entries. | `invalid_content`, rule `applicability_count` |
| 16 | Each name in an `applicability` has 1 to 128 characters from `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`. | `invalid_content`, rule `applicability_name` |

Steps 4 to 16 apply only to the kind `content`. A `cel-bundle` candidate that passes step 3 is valid for this file.

The refusal of rule `unknown_field` gives the path of the first unknown member name, in the order of the text. The path has the form `items[0].colour` or `colour`. Thus the author can see which field the Host refused. The reader looks for unknown names only in an object that the manifest table defines. Thus it does not look in `metadata` or in `applicability`. A value of the wrong type is a rule `schema` refusal.

Rule 12 makes one order for each list of items. Rule 11 comes first, so a duplicate gets its own reason.

A candidate that passes all of the rules is valid. The result gives the kind. For `content`, the result also gives the candidate `ref` and the head.

## Host behavior

### Subject

The Host keeps one record model for each kind. A record of the lifecycle has the fields `function` and `scope`. For a content head, the value of `function` is the subject `content:<collection>/<partition>`, for example `content:loan-rules/region-a`. Thus the existing rules for scope, interval, source, plan, supersession, signature and approval apply to a content head without change.

- The embedding host installs a subject in the `functions` list of the Host context, as it installs a CEL function. A subject that is not installed gives `out_of_scope`.
- A `source`, a `plan` and a `suspend` request name the subject in the `function` field.
- A CEL bundle never has a function name that starts with `content:`. `propose` refuses it with `invalid_request`.

### Requests

- The `propose` body has an optional field `candidate`. A body with `bundle` and no `candidate` is a `cel-bundle` candidate, as in 1.0. A body with both is `invalid_request`. A `cel-bundle` candidate gives the same snapshot as the `bundle` field.
- For a `content` candidate, the body has no `function` and no `base_snapshot`. Else `propose` refuses with `invalid_request`. The body has `from`, `until`, `source_ref`, `reason` and an optional `stale_mode`, as for a CEL bundle.
- The Host applies the rules of this file before it writes the snapshot. A refusal stops the write, and the root does not change. The message of a rule refusal is `rule <rule>`. For rule `unknown_field`, it is `rule unknown_field: unknown field <path>`.
- The `base` of the manifest must be the candidate `ref` of the current head, or an empty string if the head has no proposal. Else `propose` refuses with `stale_base`.
- The source must cover the subject and the interval of the proposal. Else `propose` refuses with `source_not_applicable`.
- The `propose` result has `snapshot_ref`, `candidate` and `head`.

### Records

A content snapshot has these fields in addition to `function`, `scope`, `from`, `until`, `source_ref`, `reason`, `producer`, `stale_mode` and `submitted_at`:

| Field | Value |
|---|---|
| `candidate` | `{"kind":"content","ref":<ref>}`. |
| `collection`, `partition` | From the manifest. |
| `base` | The `base` of the manifest. |
| `manifest_ref` | The store reference of the exact manifest text, with the media type `application/vnd.entelos.content-manifest+json`. |
| `bundle_ref` | The candidate `ref`. Thus the qualification bindings of a CEL bundle apply: a passed verdict must name this `ref`. |
| `evidence_reference` | The `evidence` object of the manifest, if the manifest has one. |
| `applicability` | An object with the `applicability` of each item that has one, by `item_id`. A snapshot whose items have no values has no field. |

A content snapshot has no `files`, no `engine_build` and no `base_snapshot`. A CEL snapshot has no `candidate` field and keeps the bytes of an earlier Host.

### Qualification and activation

- `qualify` gives the exact manifest text to the qualification runner, as the file `content`. The `Candidate` of the runner has the candidate `ref` in `BundleRef` and an empty `EngineBuild`.
- A report of a content snapshot has the field `candidate`. Admission refuses a report whose `candidate` differs from the `candidate` of its snapshot, with `stale_qualification`. A passed verdict must name the candidate `ref` in `bundle_ref`, else the status is `failed`.
- The CEL runner refuses a content candidate with the diagnostic `unsupported_candidate_kind`, so the report is `unresolved`. An application installs its own runner for content.
- `decide` and `activate` work as for a CEL bundle. The activation interval and the dated selection do not change. The event record has the bytes of a CEL event record, with the subject in `function`.
- `activate` takes the Host field `applicability`, an object. It confirms the values of the snapshot. It must be equal to the `applicability` of the snapshot: the same items, the same names, and the same levels in the same order. If it is not equal, `activate` refuses with `applicability_mismatch` and writes no record. A snapshot with values thus needs the field. For a snapshot with no values, the field is absent or `{}`. This rule also applies to a CEL snapshot, which has no values.
- The event record of an activation with values has the field `applicability`, with the confirmed values. Thus a signed activation record names them. An event record with no values has the bytes of an earlier Host.
- `resolve` of a content subject gives the `applicability` of the event in the selection, if the event has it. A replay at an earlier cursor gives the values that the activation at that root confirmed.
- `resolve` of a content subject gives the selection with `candidate`, `collection` and `partition`. It has `bundle_ref` (the candidate `ref`) and no `engine_build`.
- A replay of a content activation is `resolve` with the `cursor` of an earlier root. It gives the candidate reference that the date selected at that root. It does not run the content.
- `load`, `invoke`, `describe` and `ingest` of a content snapshot refuse with `unsupported_candidate_kind`.
- `proposals` lists a content proposal with its subject in `function` and its candidate `ref` in `bundle_ref`. `proposal` gives the snapshot, with `candidate` and `evidence_reference`, and the manifest text as the file `content` in `bundle`.

| Code | Use |
|---|---|
| `stale_base` | The `base` of the manifest is not the `ref` of the current head. |
| `applicability_mismatch` | The `applicability` of an `activate` request is not equal to the values of the snapshot items. |
| `unsupported_candidate_kind` | Also from `load`, `invoke`, `describe` and `ingest` of a content snapshot, and from the CEL runner. |
| `stale_qualification` | Also for a report whose candidate differs from the candidate of its snapshot. |

### Host decoder

The Host decodes the manifest with the strict record decoder. It gives rule `json` for a duplicate member name, a number that is not finite as a 64-bit float, a lone surrogate escape, an integer of more than 4300 digits and 1000 or more levels of nesting. The vectors do not test these edges, and the Python checker can give another rule for them.

## Versions

The major version is the number after the slash in `format`. A reader reads only the major version that it knows. An unknown major version gives `unsupported_content_format`. After version 1 is stable, a new field or a new enum value needs a new major version, because a reader of version 1 refuses a field that it does not know.

Version 1 is unstable. Before it is stable, an optional field can come into version 1. The field `items[].applicability` came this way. A manifest without it keeps its bytes and its `ref`. An earlier reader refuses a manifest with the field, with rule `unknown_field`. Thus no reader accepts values that it does not record. A new kind is not a new manifest version. A reader that does not know a kind gives `unsupported_candidate_kind`.

## Vectors

[`vectors/content-cases.json`](vectors/content-cases.json) has two lists.

Each item of `cases` has:

- `name`: a unique name;
- `candidate`: the candidate, or a value that is not a candidate;
- in `candidate`, `{"$pad": {"text": T, "count": N}}` in place of the `manifest` string is T, with N spaces before its last character;
- in `candidate`, `{"$items": {"text": T, "item": I, "count": N}}` in place of the `manifest` string is T, with `$ITEMS` replaced by N copies of I, with a comma between each two copies. In copy number k, from 0, `{i}` in I is k as 4 decimal digits;
- `expect`: one of these:
  - `{"valid": true, "kind": "cel-bundle"}`;
  - `{"valid": true, "kind": "content", "ref": "sha256:…", "head": {…}}`;
  - `{"valid": false, "code": "invalid_candidate" or "invalid_content", "rule": "<rule>"}`, and `"field": "<path>"` for rule `unknown_field`;
  - `{"valid": false, "code": "unsupported_candidate_kind" or "unsupported_content_format"}`.

The expected `ref` values were calculated with `shasum -a 256` on the bytes of each manifest text. The checker calculates them again with Python `hashlib`.

Each item of `bindings` has a `report` and a `snapshot`, each a candidate reference, and `expect`: `bound` or `stale_qualification`.

`check_spec.py` does these checks:

1. It applies the rules of this file to each case, and compares the result with `expect`.
2. It checks that each rule and each code in the rules table has one or more cases.
3. It applies the binding rule to each binding, and compares the result with `expect`.
4. A self-test changes a copy of the vectors and checks that the checker reports each change.

The checker applies the same rules that the Host applies. Agreement of the checker and the vectors shows that the two agree with each other. The Go tests `TestContentCandidateVectors` and `TestHostProposeAppliesTheContentVectors` run the cases against the Host rules and the Host `propose`.

[`vectors/lifecycle-cases.json`](vectors/lifecycle-cases.json) has the lifecycle vectors. Each case starts with an empty store, a source and a plan for the subject `content:loan-rules/region-a`. `manifests` gives each manifest text and its `ref`, calculated with `shasum -a 256`. A manifest with item values also has `applicability`: the values that the snapshot, the event and the selection record. Each step has one operation and an optional `expect`:

- `propose`: the manifest name, with `from` and `until`. The result is the name, or `{"error": <code>}`.
- `qualify`: the manifest name and the `verdict` of the fixture runner. `runner_names` is the manifest that the verdict names, if it is not the same. The result is the report status.
- `decide`: an accept verdict on the last report. `activate`: an activation of the last decision, with the optional `applicability` of the request. The result is `accept` or the name, or `{"error": <code>}`.
- `state`: the `state` of the proposal in the approval API.
- `mark`: keep the current root as a named cursor. `resolve`: the date, and an optional `cursor` name. The result is the manifest name of the selected candidate.
- `load`, `invoke`, `describe`: the date. The result is the error.

The Go test `TestContentLifecycleVectors` runs them on the Host with a fixture runner. The fixture runner checks that it got the exact manifest bytes. The test also checks the `applicability` of each snapshot, each event and each selection against the manifest entry. `check_spec.py` checks each `ref`, each manifest against the rules, the `applicability` of each manifest entry against its items, the result of each `activate` against the confirmation rule and each step name.

## Limits

- The wire protocol has no `candidate` field and no `activate` field `applicability` yet. Only a Host request (the Go API or the JSONL Host) can give a content candidate or confirm its values.
- `proposals` gives no separate `candidate` or `evidence_reference` field in its list item. A reader gets them from `proposal`.
- The Host does not get or check the evidence document that `evidence` names.
- Entelos cannot check an item digest, an item size or a media type. The qualification runner of the application is responsible for them.
- The manifest has no signature and no author authentication. The caller of `propose` is responsible for the source of the manifest. A signed activation record of the Host names the subject, not the manifest text.
- The manifest does not record when an item was written or by whom.
- The content subject uses the `function` field. A reader of a record must read the `content:` prefix to know the kind of the subject.
