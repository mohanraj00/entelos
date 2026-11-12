# Content proposal `entelos.content/1` (proposed)

This file specifies a second kind of candidate: a content proposal. Issue [#153](https://github.com/mohanraj00/entelos/issues/153) tracks it. A content proposal goes from proposal to activation with no CEL file.

Status: **proposed**. No Host reads this format. The schemas and the vectors in this directory are the contract for the implementation in v0.5. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) applies the rules of this file to the vectors. It does not run a Host.

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
| `metadata` | object | Optional. 0 to 64 entries. Each name has the same characters as `collection`. Each value is a string of 0 to 1024 characters. |
| `evidence` | object | Optional. A reference to an evidence document. |
| `evidence.format` | string | `entelos.evidence/1`. |
| `evidence.digest` | string | `sha256:` and 64 lowercase hex digits. |

A field that is not in this table is not permitted. The names in `metadata` are data, not fields.

The `evidence` reference names an evidence document of format `entelos.evidence/1` ([#148](https://github.com/mohanraj00/entelos/issues/148)) by its digest. This file does not specify how the Host gets or checks that document.

A digest is an identity of bytes. It is not a signature. Entelos does not read the items, so it cannot check an item digest or an item size. The qualification runner of the application must check them.

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

Steps 4 to 14 apply only to the kind `content`. A `cel-bundle` candidate that passes step 3 is valid for this file.

The refusal of rule `unknown_field` gives the path of the first unknown member name, in the order of the text. The path has the form `items[0].colour` or `colour`. Thus the author can see which field the Host refused. The reader looks for unknown names only in an object that the manifest table defines. A value of the wrong type is a rule `schema` refusal.

Rule 12 makes one order for each list of items. Rule 11 comes first, so a duplicate gets its own reason.

A candidate that passes all of the rules is valid. The result gives the kind. For `content`, the result also gives the candidate `ref` and the head.

## Host behavior (proposal for v0.5)

This section is a proposal. No code implements it. The vectors in this directory do not test it.

- The `propose` body gets an optional field `candidate`. A body with `bundle` and no `candidate` is a `cel-bundle` candidate, as in 1.0. A body with both is `invalid_request`.
- For a `content` candidate, the body has no `function`. The Host applies the rules of this file before it writes the snapshot. A refusal stops the write, and the root does not change.
- The snapshot records the candidate reference, the head and the manifest text. It does not record a `bundle_ref`.
- The `base` of the manifest must be the candidate `ref` of the current head, or an empty string if the head has no proposal. Else `propose` refuses with `stale_base`.
- `qualify` gives the manifest text to the runner of the application. The report records the candidate reference.
- `decide` and `activate` work as for a CEL bundle. The activation interval and the dated selection do not change.
- `load` and `invoke` on a `content` snapshot refuse with `unsupported_candidate_kind`.
- `proposals` and `proposal` of the approval API list a content proposal with its head, its candidate reference and its evidence reference.
- `replay` of a content activation gives the candidate reference that the date selected. It does not run the content.

| Code | Use |
|---|---|
| `stale_base` | The `base` of the manifest is not the `ref` of the current head. |
| `unsupported_candidate_kind` | Also from `load` and `invoke` on a `content` snapshot. |

## Versions

The major version is the number after the slash in `format`. A reader reads only the major version that it knows. An unknown major version gives `unsupported_content_format`. A new field or a new enum value needs a new major version, because a reader of version 1 refuses a field that it does not know. A new kind is not a new manifest version. A reader that does not know a kind gives `unsupported_candidate_kind`.

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

The checker applies the same rules that a Host will apply. Agreement of the checker and the vectors shows that the two agree with each other. It does not show that a later Host agrees with them. The implementation in v0.5 must run the vectors against the Host.

## Limits

- No Host reads this format yet. A passing check does not show that entelos accepts a content proposal.
- The vectors do not go from `propose` to `activate` and to `replay` on a Host. Those lifecycle vectors, with a fixture runner, come with the v0.5 code.
- The codes of the Host behavior section have no vector.
- Entelos cannot check an item digest, an item size or a media type. The qualification runner of the application is responsible for them.
- The manifest has no signature and no author authentication. The caller of `propose` is responsible for the source of the manifest.
- The manifest does not record when an item was written or by whom.
- This file does not specify a decision record in `docs/decisions`. The owner decides the kind model when the owner accepts this spec.
