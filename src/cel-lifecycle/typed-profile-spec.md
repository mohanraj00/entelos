# Typed callable profile `cel-typed/v1`

Status: proposed. This file is a specification for review. No host code implements it. Issue [#126](https://github.com/mohanraj00/entelos/issues/126) tracks the work.

The current profile `cel-string-bool/v1` accepts string and bool inputs and gives a bool result. This file specifies a second profile, `cel-typed/v1`. A rule of this profile has a declared type for each input and for its output. The types are integers, dates, lists and records, and the two current types.

The JSON vectors in [conformance/typed/](conformance/typed/) state the expected results of this profile. No harness runs them yet. The vector [storage/typed-values.json](conformance/storage/typed-values.json) records the canonical bytes of each type. The current storage harness runs that vector.

## Scope

- The host keeps `cel-string-bool/v1`. Its rules, limits, descriptor and diagnostics do not change.
- `cel-typed/v1` is a separate profile. A bundle names one profile in its manifest.
- The engine (`src/cel-handoff`) does the type checks. The Host and the storage port store bytes. They do not interpret a type.
- The executor and its WASM bridge refuse a `cel-typed/v1` bundle with `unsupported_profile` until the executor vectors cover this profile.

## Types

A type declaration is a JSON object with a `type` field. The supported types are:

| Type | Declaration | CEL type | Wire value |
|---|---|---|---|
| `bool` | `{"type": "bool"}` | `bool` | A JSON `true` or `false`. |
| `string` | `{"type": "string"}` | `string` | A JSON string with at most `max_string_utf8_bytes` UTF-8 bytes. |
| `int` | `{"type": "int"}` | `int` | A JSON integer from -9007199254740991 to 9007199254740991. |
| `date` | `{"type": "date"}` | `google.protobuf.Timestamp` | A JSON string `YYYY-MM-DD`. |
| `list` | `{"type": "list", "items": "<scalar>"}` | `list(<scalar>)` | A JSON array with at most `max_list_items` items. |
| `record` | `{"type": "record", "fields": [...]}` | `map(string, dyn)` | A JSON object with exactly the declared fields. |

A scalar type is `bool`, `string`, `int` or `date`.

### `int`

- The range is the safe-integer range of IEEE 754 doubles, -(2^53-1) to 2^53-1. A JavaScript caller and the WASM bridge can hold each value without a loss. The CEL `int` type is 64-bit. Thus each wire value is a valid CEL value.
- The wire value must be a JSON number token without a fraction and without an exponent. The tokens `812.0`, `8.12e2` and `1E3` are not integers, also if their value is integral. The engine does not coerce a value.
- The token `-0` is the integer 0. Its canonical text is `0`.
- A JSON string such as `"812"` is not an integer.

### `date`

- The wire value is a JSON string of 10 characters: a 4-digit year, `-`, a 2-digit month, `-` and a 2-digit day. The year is from 0001 to 9999. The date must exist in the proleptic Gregorian calendar. Thus `2024-02-29` is a date, and `2026-02-30`, `2026-3-14` and `2026-03-14T00:00:00Z` are not dates.
- In CEL, a date is the timestamp at 00:00:00 UTC of that day. A rule can compare dates and add a `duration`.
- A `date` output must be a timestamp at 00:00:00 UTC with a year from 0001 to 9999. Else the result is `output_type`.

### `list`

- `items` is the name of a scalar type. A list of lists and a list of records are not in this profile.
- The number of items is from 0 to `max_list_items`. Each item must have the item type.

### `record`

- `fields` is a list of 1 to 16 field declarations. A field declaration is a type declaration with a `name` and an optional `description`. A field name is a simple identifier, `[A-Za-z_][A-Za-z_0-9]*`, and is unique in the record.
- A field type is a scalar type or a list. A record in a record is not in this profile.
- Each declared field is required. A field that is not declared is refused. `null` is not a value of any type.
- In CEL, a record is a `map(string, dyn)`. The CEL checker therefore does not check the field types in the expression. The engine checks each field at the boundary: before the evaluation for an input, and after the evaluation for an output.

### Types that are not in this profile

`null`, floating-point numbers, `uint`, `bytes`, `timestamp`, `duration`, maps with keys that are not declared, optional fields, nested records and lists of lists or records are not in this profile. A declaration of one of them is `unsupported_profile`.

## Bundle declaration

### Manifest

The manifest has `"profile": "cel-typed/v1"`. The other manifest fields are the same as for `cel-string-bool/v1`. The `limits` object has six fields. All six are required and positive:

| Limit | Ceiling |
|---|---|
| `max_request_bytes` | 4096 |
| `max_string_utf8_bytes` | 256 |
| `max_source_utf8_bytes` | 4096 |
| `parser_recursion_limit` | 64 |
| `evaluation_cost_limit` | 1000 |
| `max_list_items` | 64 |

A missing, zero or above-ceiling limit is `invalid_bundle` at the `profile` stage with the field `/limits`. A `cel-string-bool/v1` manifest with `max_list_items` is also `invalid_bundle`.

### Environment

`env.json` has `name`, an optional `description`, `variables` and `output`.

- Each variable is a type declaration with a `name` and an optional `description`.
- `output` is a type declaration without a `name`. It is required.
- An unknown field in the environment, in a variable or in a field declaration is `unsupported_profile` at the `environment` stage, as for `cel-string-bool/v1`.

### Fixed bindings

A fixed value must be a valid wire value of its declared type. Thus a fixed `date` is a JSON string such as `"2026-01-01"`, and a fixed `int` is a JSON integer. A bad fixed value is `invalid_bindings` with the field of the fixed name, for example `/min_score`.

### Load order

`Load` reports the first error in this order. Steps 1 to 3 are the current steps.

1. The manifest, the component references and the build identity.
2. The profile name: a value other than `cel-string-bool/v1` and `cel-typed/v1` is `unsupported_profile` with the field `/profile`.
3. The limits, then the source bytes.
4. The environment JSON, its `name` and its `variables` list.
5. Each variable in the list order: the name, a duplicate name, then the type declaration.
6. The `output` declaration.
7. The fixed bindings and the caller variables, as for `cel-string-bool/v1`.
8. The compilation, then the output type check.

A type declaration is checked in this order. `<at>` is the JSON pointer of the declaration in `env.json`, for example `/variables/2` or `/output`.

| Problem | Code | Field |
|---|---|---|
| `type` is absent, or is not one of the six type names | `unsupported_profile` | `<at>/type` |
| `items` on a type other than `list` | `invalid_environment` | `<at>/items` |
| `fields` on a type other than `record` | `invalid_environment` | `<at>/fields` |
| A `list` without `items` | `invalid_environment` | `<at>/items` |
| `items` is not a scalar type name | `unsupported_profile` | `<at>/items` |
| A `record` without `fields`, with 0 fields or with more than 16 fields | `invalid_environment` | `<at>/fields` |
| A field with a bad or duplicate name | `invalid_environment` | `<at>/fields/<j>` |
| A field with the type `record` | `unsupported_profile` | `<at>/fields/<j>/type` |
| An `output` that is absent | `invalid_environment` | `/output` |

The checks of a field declaration follow the same table, with `<at>/fields/<j>` as its pointer.

### Output type check

After the compilation, the CEL output type of the expression must be assignable to the CEL type of the declared output. Else the result is `output_type` at the `compile` stage with the field `/source`. For a `record` output, an expression of the type `map(string, X)` passes this check for each `X`. The engine checks the fields after each evaluation.

## Calls

### Request and input checks

The request is the same JSON object as for `cel-string-bool/v1`: `bundle_ref` and `inputs`. The engine reads integers as number tokens, so it can see the token text. The checks have this order:

1. The request byte limit, the strict JSON parse, the `inputs` object and the `bundle_ref`, as now.
2. An input name that is not a caller variable is `invalid_input`. The names are checked in sorted order.
3. Each caller variable in the `caller_variables` order. An absent variable is `needs_input`. A bad value is `invalid_input`.

The field of an input diagnostic is a JSON pointer in `inputs` to the first bad value. For a list, the engine checks the item count, then the items in index order. For a record, the engine checks the unknown fields in sorted order, then the declared fields in declaration order. A missing field of a record is `invalid_input`, not `needs_input`, because the record value is malformed. Examples:

| Input | Code | Field |
|---|---|---|
| `"sources"` is absent | `needs_input` | `/sources` |
| `"score": "812"` | `invalid_input` | `/score` |
| `"score": 812.0` | `invalid_input` | `/score` |
| `"score": 9007199254740992` | `invalid_input` | `/score` |
| `"sources": ["registry", 7]` | `invalid_input` | `/sources/1` |
| `"window": {"start": "2026-01-01"}` with a missing `end` field | `invalid_input` | `/window/end` |

### Output checks

After the evaluation, the engine checks the CEL value against the declared output type. It converts a valid value to its wire value. A value that does not have the declared type is `output_type` at the `output` stage. The field is a JSON pointer in the response, from `/value`. Examples:

| Output | Field |
|---|---|
| A `date` timestamp at 01:00:00 UTC | `/value` |
| An `int` of 9008000000000000 | `/value` |
| A record without the declared field `reason` | `/value/reason` |
| A record with the field `note` that is not declared | `/value/note` |
| A list with more than `max_list_items` items | `/value` |

The engine checks a record output in the same order as a record input: the unknown fields in sorted order, then the declared fields in declaration order.

A string output with more than `max_string_utf8_bytes` UTF-8 bytes is also `output_type`. Errors in the evaluation keep their current codes: `evaluation_error`, `resource_exhausted` and `cancelled`.

### Response

A response with the status `ok` has `value`, the wire value of the output. A response with another status has no `value`. The other response fields do not change.

### Descriptor

`describe` gives `profile` `cel-typed/v1`. `inputs` stays a JSON Schema object with `required` and `additionalProperties` `false`. `output` is the JSON Schema of the output declaration. Each declaration maps to a schema:

| Type | Schema |
|---|---|
| `bool` | `{"type": "boolean"}` |
| `string` | `{"type": "string", "x-max-utf8-bytes": <max_string_utf8_bytes>}` |
| `int` | `{"type": "integer", "minimum": -9007199254740991, "maximum": 9007199254740991}` |
| `date` | `{"type": "string", "format": "date", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}` |
| `list` | `{"type": "array", "items": <item schema>, "maxItems": <max_list_items>}` |
| `record` | `{"type": "object", "properties": {...}, "required": [<field names in declaration order>], "additionalProperties": false}` |

A declared `description` is a `description` key in the schema of that variable or field. JSON Schema cannot state the token rule of `int` or the calendar rule of `date`. The engine checks are the contract, and the schema is a guide for a caller.

## Canonical encoding and records

The canonical bytes of a typed value are the canonical JSON of [storage-port-spec.md](storage-port-spec.md#terms): sorted object keys, no insignificant whitespace, UTF-8 without escapes for non-ASCII characters and one final newline. Each valid wire value has one canonical text:

- An `int` is its decimal text without a leading zero, a `+` or a fraction. `-0` becomes `0`.
- A `date` is its 10-character string.
- A `record` is a JSON object with sorted keys. The declaration order of the fields has no effect on the bytes.
- A `list` keeps its item order.

The Host decodes `inputs` with the record codec and gives the canonical bytes to the engine. The receipt `inputs_ref` is the content reference of these bytes with the media type `application/json`, as `InputDigest` computes it now. The codec keeps the text of a number that is not an integer token. For example, `812.0` and `8.12e2` both become `812.0`, and `1E3` becomes `1000.0`. The engine then refuses that value with `invalid_input`, and the receipt keeps the canonical bytes of the refused input. Thus a type error gives a clear diagnostic and a record.

The response record of a receipt holds the canonical encoding of the response, also of a typed `value`. Two valid requests with equal typed values have equal canonical bytes and an equal `inputs_ref`. A replay sends the retained bytes to the retained build and bundle, so it gives the same response bytes.

## Lifecycle

The lifecycle records do not change. These rules apply to a typed bundle:

- **Build identity.** The engine build that implements `cel-typed/v1` has a new `implementation_ref`. A snapshot that names an earlier build fails closed with `implementation_unavailable`. This also applies to `cel-string-bool/v1` snapshots: each of them needs a new qualification on the new build before it can serve on that build. A passed report of an earlier build is not carried onto the new build.
- **Contract.** The profile, `env.json` and the limits are part of the bundle contract. A later proposal of the same function and scope must keep `profile`, `environment_ref` and `limits` of the base manifest, or the result is `contract_changed`. Thus a type change, an output change or a move from `cel-string-bool/v1` to `cel-typed/v1` needs a new function ID.
- **Qualification.** `Bundle.Check` compares each case as now. For a typed bundle, the `value` of an `ok` case must be a valid wire value of the output type. The case passes only if the canonical bytes of the expected value equal the canonical bytes of the actual value. An expected value that is not a valid wire value fails the case. The other case rules do not change. The report binds the bundle, the build and the plan, as now.
- **Approval and activation.** A decision binds the report, and an activation binds the decision. The snapshot binds the bundle, and the bundle binds the profile and the types. Thus the reviewer approves the types with the revision. The activation rules do not change.
- **Callers.** A caller reads the descriptor of the selected activation, then sends `expected_bundle_ref` with each `invoke`. A caller must read the descriptor again after a change of the activation.
- **Restatement.** A restatement runs the retained input on a later accepted root of the same function. The contract rule keeps the types of that function. Thus the retained input has the same types on the later root.
- **Ingest.** An ingested response holds a typed `value`. The Host checks the identities, as now. It does not run the rule again, and it does not check the type of the value.

## Conformance vectors

Each file in [conformance/typed/](conformance/typed/) has the format `typed-profile-vector/v1`:

| Field | Meaning |
|---|---|
| `id`, `description` | The vector name and its purpose. |
| `bundle` | `function_id`, `profile`, `caller_variables`, `limits`, `environment`, `source` and `fixed`, as in the executor vectors. The bundle has no build identity. A harness binds the bundle to the build under test. |
| `steps` | The steps, in order. |

Each step has a `name`, an `op` and an `expect`:

- `load` loads the bundle. A `replace` object, if present, replaces the same top-level keys of the vector bundle for this step only. `expect` is `{"ok": true}` or `{"diagnostic": {"code", "stage", "field"}}`.
- `describe` loads the vector bundle and checks `expect.inputs` and `expect.output` against the descriptor.
- `run` loads the bundle, with `replace` if present, and sends a request. `inputs_text` is the exact text of the `inputs` object, so that a harness can send a token such as `812.0` or `-0` without a change. The harness writes the request as `{"bundle_ref": "<bound ref>", "inputs": <inputs_text>}`. `expect` has `status`, `value` for an `ok` status and `field` for a diagnostic.
- `check` runs `Bundle.Check` with `cases`. `expect` has the report `status` and `cases`, the `passed` value of each case in order.

A harness compares the diagnostic `code`, `stage` and `field`. It does not compare the message.

The expected values come from this specification and from the CEL semantics of each expression. No candidate implementation produced them. The digests in [storage/typed-values.json](conformance/storage/typed-values.json) are SHA-256 values of the stated canonical text. They were computed with Python `hashlib`, not with the record codec. `TestStoragePortVectors` computes the same references with the record codec on the Git store and the memory store.

## Limits

- This profile is proposed. The vectors in `conformance/typed/` have no harness, and no host code is in this change.
- A record is a `map(string, dyn)` in CEL. An expression such as `window.start + 1` compiles, then fails in the evaluation with `evaluation_error`.
- The profile has no floating-point type. A decimal score needs an integer unit, for example basis points.
- The `int` range is smaller than the CEL `int` range.
- An intermediate value in CEL can be outside the `int` range. Only the inputs and the output are checked.
- A content reference is an identity, not a signature.
