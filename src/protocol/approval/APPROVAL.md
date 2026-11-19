# Approval API 1.2

This file specifies the approval API of issue [#130](https://github.com/mohanraj00/entelos/issues/130). A reviewer client uses it to list the open proposals, read the evidence of one proposal and record a verdict. The reviewer does not need Git. Entelos does not supply a user interface.

Status: **implemented**. The Go Host and the sidecar in [`src/lifecycle-core`](../../lifecycle-core) implement this file. The schemas and vectors in this directory are the contract. The vectors have expected values that a person wrote from this text. No candidate output made them. [`check_spec.py`](check_spec.py) checks their structure and their arithmetic. It does not run a Host. The Go test `TestApprovalVectors` in [`approval_vectors_test.go`](../../lifecycle-core/protocol/approval_vectors_test.go) runs each step on three transports: the sidecar socket, the [HTTP binding](#http-binding) with signed claims, and the HTTP binding with local static tokens. It compares each response with the vector, and it compares the record references of the three runs.

The API extends [wire protocol 1.0](../PROTOCOL.md) to version 1.2. A 1.0 or 1.1 request keeps its earlier result shape.

## Terms

| Term | Meaning |
|---|---|
| Proposal | A `snapshot` record that `propose` wrote. Its reference is the `snapshot_ref`. |
| Evidence | The producer document in the [evidence format](../evidence/EVIDENCE.md), when attached, plus the snapshot, source, bundle and qualification reports with their plans. |
| Verdict | A `decision` record that `decide` wrote. Its `disposition` is `accept` or `reject`. |
| Principal | One credential with an actor and a set of roles. The operator writes it. A request cannot select it. |
| Open proposal | A proposal that is the current head of its function and scope, and that has no verdict. |

## What changes

1. The sidecar can authenticate more than one principal. Each request frame runs as the principal of its own credential.
2. The Host has a clock port. It records `submitted_at` on a new snapshot and `decided_at` on a new decision.
3. The Host keeps an index of the reports and the decisions of each snapshot, in `state.reviews`.
4. Two read operations are new: `proposals` and `proposal`.
5. `decide` takes an optional `disposition`. A `reject` verdict is new.
6. `decide` and `activate` refuse an `accept` after a `reject` on the same snapshot.
7. `decide` refuses a second verdict from the same actor on the same snapshot.

Items 6 and 7 make the approval stricter. They do not remove a refusal or a check of 1.0.

## Contract notes for 1.2 (#161)

Version 1.2 adds `limit` to `proposals`, a page token in its request `cursor`, and `next_cursor` in a paged result. It also adds an optional `evidence` document to `propose` and exposes that document in `proposal`. Version 1.2 also adds the optional `activate.supersedes` array of event references with the same shape as `targets` (#180). [SUPERSESSION.md](../supersession/SUPERSESSION.md) defines the Host rules. The [1.2 schemas](schema/1.2/) define six operations. Versions 1.0 and 1.1 refuse `activate.supersedes` with `invalid_request` at the protocol stage. The [1.1 schemas](schema/) stay in use for 1.1 clients. The JSONL Host, socket and HTTP all call the same Host implementation.

An old `proposals` request without `limit` keeps its exact result fields, including the existing store-revision `cursor`. The dashboard continues to send protocol 1.1 and needs no client change. The envelope now accepts response version `1.2`; the unsupported-minor cases move to `1.3`. The earlier 1.0 and 1.1 successful expectations stay unchanged.

Version 1.2 also adds `decide.fact_reviews` and the `decision_events` read
for #158. Versions 1.0 and 1.1 refuse the field and the operation at the
protocol stage. Their schemas do not change.

## Principals and authentication

The sidecar takes `--credentials-file` or `--token-file`. It refuses to start with both or with neither.

With `--token-file`, the behavior of v0.3.0 stays the same. The context file gives the one actor and its roles.

With `--credentials-file`, the file has this form. The [schema](schema/credentials.schema.json) defines it.

```json
{"format":"entelos.credentials/1","principals":[
  {"token_sha256":"<64 lowercase hex digits>","actor":"reviewer-a","roles":["reviewer"]}
]}
```

These rules apply:

- The file must be a regular file with mode 0600 and the owner of the sidecar process. Else the sidecar does not start.
- The context file must not contain `actor` or `roles`. If it contains one of them, the result is `configuration_error`. Thus each actor comes from one source.
- `token_sha256` is the SHA-256 digest of the 64 ASCII hex digits of the token. The file does not contain a token. A copy of the file does not give a usable credential.
- Each `token_sha256` and each `actor` must be unique. Each role must be a Host role name. A duplicate or an unknown role is `configuration_error`.
- For each frame, the sidecar calculates the digest of the prelude token. It compares that digest with each entry in constant time, and it compares all entries. If no entry agrees, the response is `unauthenticated`, as in 1.0.
- The sidecar reads the file again for each request frame, before it reads the frame length. Thus a removed principal gets `unauthenticated` on its next frame, also on a connection that is already open. If the sidecar cannot read the file, or the file is not valid, the frame gets `unauthenticated` and the Host receives no request.
- The Host context for the frame is the context file plus the `actor` and `roles` of the principal. The client cannot change them.

The digest is a verifier for a secret token. It is not a signature and it gives no authority by itself. The application authenticates the person to its own interface. Entelos authenticates the credential and selects the actor and the roles. A panel that holds the credentials of many reviewers can act as each of them. The operator must issue one credential to each reviewer and give it only to the panel session of that reviewer.

## HTTP binding

Issue [#156](https://github.com/mohanraj00/entelos/issues/156) adds HTTP as a second transport of the sidecar. The socket stays. An HTTP request carries the same request object as a socket frame, and it gets the same response object. The Host, its role checks and its records do not change. Thus a verdict from HTTP is the same record as a verdict from the socket.

### Request and response

- The only method is `POST`. The only path is `/v1/request`, without a query.
- The body is one request object, as in a frame payload. `Content-Type` must be `application/json`. The only permitted parameter is `charset=utf-8`.
- The body has the frame bound of 1048576 bytes. A larger body gets HTTP 413 and `invalid_frame`. The sidecar reads at most one byte more than the bound.
- The response body is the canonical JSON of the response object. The headers are `Content-Type: application/json`, `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.
- The response object is the record. The HTTP status is only a hint for clients and proxies:

| Response status | HTTP status |
|---|---|
| `ok` | 200 |
| `unauthenticated` | 401, with `WWW-Authenticate: Bearer` |
| `forbidden` | 403 |
| `invalid_request`, `invalid_frame`, `unsupported_version` | 400; 413 for a body over the bound; 415 for another media type |
| `internal_error`, `response_too_large` | 500 |
| Each other refusal | 409 |

A wrong path gets 404, and a wrong method gets 405, with `invalid_request` at stage `protocol`. On a loopback address, a `Host` header that does not name a loopback host gets 421 and `invalid_request`. This check refuses a request that DNS rebinding sends to the loopback listener.

The sidecar checks the credential before it reads the body, as the socket checks the prelude before it reads the frame length. An `unauthenticated` response has version `"1.0"` and an empty `request_id`, as on the socket.

### Credentials

Each request has exactly one `Authorization: Bearer <credential>` header. The client never states its own identity: the body cannot select an actor, roles or a scope. The sidecar takes one of two credential modes.

**Signed claims** (`--issuer-file`). The credential is a compact JWS signed with Ed25519 (`alg` `EdDSA`). The issuer file has this form:

```json
{"format":"entelos.issuer/1","issuer":"https://issuer.example","audience":"entelos-approval",
 "max_lifetime_seconds":3600,
 "keys":[{"kty":"OKP","crv":"Ed25519","kid":"k1","x":"<32 bytes in base64url>"}],
 "principals":[{"subject":"reviewer-a","roles":["reviewer"]}]}
```

The sidecar accepts a claim only if all of these are true:

1. The claim has at most 8192 bytes and three base64url segments without padding.
2. The header has only `alg`, `kid` and `typ`. `alg` is `EdDSA`. `typ`, if present, is `JWT`. `kid` names a key of the file. The sidecar accepts no other algorithm, and it ignores no header field.
3. The Ed25519 signature verifies with that key. The sidecar reads the payload only after this check.
4. The header and the payload are strict JSON objects, with no duplicate key.
5. `iss` equals `issuer`. `aud` is `audience`, or a list that contains it.
6. `exp` and `iat` are integers. The time is the UTC system clock, in whole seconds. The time is less than `exp`. `iat` is not negative, and it is at most 60 seconds after the time. `exp` is more than `iat`, and `exp` minus `iat` is at most `max_lifetime_seconds`. If `nbf` is present, it is an integer at most 60 seconds after the time.
7. `tenant` equals the `scope` of the Host context.
8. `sub` is the `subject` of one principal of the file.

The actor is the `sub` value. The roles come from the issuer file, not from the claim. Thus Entelos decides the access. Separation of duties uses this actor, as for a socket principal. The issuer file has the same file checks as the credentials file: a regular file, mode 0600 and the owner of the sidecar. `max_lifetime_seconds` is from 1 to 86400. Each `kid` and each `subject` must be unique, and each role must be a Host role name that is in the list one time. The sidecar reads the file again for each request. Thus a removed key or principal gets `unauthenticated` on its next request. To rotate a key, add the new key to the file. Then let the issuer sign with the new key. Then remove the old key.

The sidecar also reads `--credentials-file` for the socket. If an actor is in both files with different roles, the sidecar does not start. The sidecar does this check only at start. The operator must keep the two files in agreement after a change.

`--clock-file` changes only the Host clock of the records. It does not change the clock of the claim checks.

**Local static tokens** (`--http-local-tokens`). The credential is a token of `--credentials-file`, as on the socket. The sidecar refuses this mode on an address that is not loopback. This mode is for a local run only.

Each refused credential gets `unauthenticated` at stage `authentication`. The message does not give the reason. The sidecar writes the reason to its standard error, and it never writes a credential there.

### Address and TLS

`--http-listen` takes an IP literal and a port, for example `127.0.0.1:8740`. The address must be loopback. `--http-allow-remote` permits another address. The sidecar has no TLS. An address that is not loopback must be behind a front proxy that terminates TLS. The sidecar sends no CORS headers, so a browser page on another origin cannot read a response.

### Static files

`--http-static <directory>` adds public static files to the binding (issue [#157](https://github.com/mohanraj00/entelos/issues/157)). A browser client, for example the [approval dashboard](../../../examples/approval-dashboard/README.md), can then load its page from the origin of the API. The API does not change: a `POST` and each request to `/v1/request` go to the API as above.

- A `GET` or `HEAD` request to another path reads one file of the directory. The path `/` is `index.html`. A file needs no credential, and a file gives no access to the API.
- The permitted extensions are `.html`, `.js`, `.mjs`, `.css`, `.json` and `.svg`. A path with a query, an empty segment, a `..` segment, a segment that starts with `.` or a backslash gets 404. A directory, a file over 4 MiB and a file of another type get 404. The sidecar opens the directory with `os.Root`, so a symbolic link cannot point out of it.
- On a loopback address, a `Host` header that does not name a loopback host gets 421, as for the API.
- A `HEAD` request reads only the file metadata. A `GET` request streams the file. At most 16 static requests read a file at the same time. A request over this bound gets 503.
- A static 404, 421 or 503 has a plain text body. It is not a response object.
- Each file has the headers `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` and a `Content-Security-Policy` that permits scripts, styles, images and connections only from the same origin, with `frame-ancestors 'self'`.

Put only public files in the directory. The sidecar does not check what a file contains.

The server bounds are: 16384 bytes of request headers, 10 seconds for the headers, 30 seconds for the complete request, 5 minutes for the response and 60 seconds for an idle connection.

This binding is not a public internet boundary. The sidecar has no rate limit, no replay record of claims and no revocation list other than the issuer file. A claim stays valid until `exp` if its principal stays in the file. An operator that puts the binding on a network must supply these controls in the front proxy.

## Clock

The Host gets a clock port. The sidecar and the JSONL host use the UTC system clock, in whole seconds. The format is `YYYY-MM-DDTHH:MM:SSZ`.

- `propose` adds `submitted_at` to the snapshot record.
- `decide` adds `decided_at` to the decision record.
- `proposals` and `proposal` return `as_of`, the clock value of the read.

The clock is not part of the idempotency payload. A retry with the same `request_key` returns the stored result and its stored time. The Host uses the clock for no selection, activation, expiry or refusal. The invocation `date` stays the only date that selects an activation. A recorded time is an operator statement, not a proof of time.

For conformance runs, the sidecar takes `--clock-file`. Before each request, it reads one time value from that file. The vector runner writes the file. The release documentation must state that this flag is for tests.

## Review index

The Host adds `reviews` to the store state. It is an object. Each key is a `snapshot_ref`. Each value is `{"reports":[…],"decisions":[…]}`, in write order.

- `qualify` appends its `report_ref` to the entry of its snapshot.
- `decide` appends its `decision_ref` to the entry of the snapshot of its report.
- A new store starts with `"reviews":{}`. Thus the `inspect-empty` vector of 1.0 changes. The implementation PR must change that expected value and say so.

A store from v0.3.0 has no `reviews`. Its decisions are not in an index, so the Host cannot tell which proposals are open. The checks `duplicate_verdict` and `proposal_rejected` also need the complete verdict history of a snapshot. Thus the Host fails closed on such a store:

- `proposals`, `proposal`, `qualify`, `decide` and `activate` refuse with `index_unavailable`. The Host writes no record and does not make a partial index.
- The other operations do not use the index. For example, `resolve`, `invoke`, `replay` and `restate` work as in 1.0.

Thus a v0.3.0 store cannot get a new report, verdict or activation until the migration below builds its index.

### Migration from v0.3.0

The command `entelos-migrate` ([#159](https://github.com/mohanraj00/entelos/issues/159)) builds the review index from the accepted history of the store. The Go function is `lifecycle.MigrateReviewIndex`.

```sh
entelos-migrate --repo <store.git> --dry-run
entelos-migrate --repo <store.git>
```

The command reads each accepted commit, the oldest first. Each commit adds one request key. If the result of that request is a `qualify` result (`report_ref` and `report`), the command appends the report to the entry of its snapshot. If the result is a `decide` result (`decision_ref`, with or without `decision`), the command appends the decision to the entry of the snapshot of its report. This is the index that a 1.1 Host writes for the same requests.

- The command adds one commit. That commit adds `state.reviews` and the request entry `migrate-review-index`. It adds no record and changes no record or other state field.
- `--dry-run` gives the index and the two writes. It writes nothing.
- If the store has an index that agrees with the history, the action is `current` and the command writes nothing. Thus a second run writes nothing.
- An empty store gives the action `empty`. The Host makes the index on the first write.
- The command writes one JSON line: `{"status":"ok","data":{"action":…}}` or a fault record.

The command refuses, and writes nothing, in these cases:

| Code | Cause |
|---|---|
| `index_inconsistent` | The store has `state.reviews`, and it differs from the index that the history gives. Or the request key `migrate-review-index` is in the history, but `state.reviews` is missing. |
| `index_incomplete` | A report or a decision is in the store, but no accepted `qualify` or `decide` request names it. |
| `unexpected_history` | An accepted commit adds more or less than one request key, or removes a request key. |
| `unsupported_record_format` | A named record has a format other than `entelos.record/1`. |
| `invalid_record` | A named record is malformed, or a decision names a snapshot that differs from the snapshot of its report. |
| `invalid_store` | The path is not a store, or the state schema is not `local-lifecycle/v1`. The command does not make a missing repository. |
| `stale_revision` | The root moved during the migration. Run the command again. |

A report from v0.3.0 names the v0.3.0 lifecycle build. Thus `decide` with `accept` refuses it with `implementation_unavailable`, and the list shows its proposal as `not_approvable`. A new `qualify` gives a report that a reviewer can accept. A `reject` does not check the build.

The command reads each commit, with several Git processes for each commit. A long history takes a long time. It does not lock the store: stop each Host on the store before the run. The Go tests use a store that the v0.3.0 Host made ([testdata/migration](../../lifecycle-core/testdata/migration/README.md)).

## Proposal states

The Host calculates one state for each proposal, in this order. The first rule that applies gives the state.

| State | Rule |
|---|---|
| `rejected` | A verdict on the snapshot has `disposition` `reject`. |
| `accepted` | A verdict on the snapshot has `disposition` `accept`. |
| `superseded` | The snapshot is not the current head of its function and scope. |
| `awaiting_qualification` | The snapshot has no report. |
| `awaiting_verdict` | The report refusal of the last report is an empty string. |
| `not_approvable` | The report refusal of the last report is not empty. |

The *report refusal* of a report is the code of the first `accept` check that fails among checks 1, 2 and 6 of `decide` (below), or an empty string. These checks do not use the reason or the actor of a request. Thus `invalid_request`, `separation_required` and `duplicate_verdict` never show here.

The proposal item has `refusal`. For `not_approvable`, it is the report refusal of the last report. For each other state, it is an empty string.

## Operations

All operations in this section need the `reviewer` role, except `decision_events`, which needs the `event_reader` role. A principal without the needed role gets `forbidden`. A request with protocol `1.0` for `proposals` or `proposal` gets `invalid_request`, because 1.0 has no such operation. A request with protocol `1.0` or `1.1` for `decision_events` gets `invalid_request` at the protocol stage, because only 1.2 has that operation.

### `proposals` (read)

Body through 1.1: `{"cursor"?: string}`. The [schema](schema/proposals.schema.json) defines that body and result. In 1.2, the [schema](schema/1.2/proposals.schema.json) also permits `limit`, an integer from 1 to 1000.

Result: `{"cursor", "as_of", "proposals": [item, …]}`. The list contains each open proposal of the host scope and of the host functions. It omits a proposal of another scope or function. It is not a refusal. The order is by `submitted_at`, the oldest first, then by `snapshot_ref`. A snapshot without `submitted_at` comes last.

With `limit`, omit `cursor` or send `""` to start at the current accepted revision. The result adds `next_cursor`. Send that value as `cursor`, together with `limit`, to read the next page. An empty `next_cursor` ends the list. Each page contains at most `limit` items. The limit can change between pages. An empty store gives an empty list and an empty next cursor.

A continuation pins the first page's store revision and `as_of`. Each page therefore sees the same open proposals, states, report references and wait times, in the order above. A proposal opened, closed or superseded after that revision does not alter the traversal. Start again with an empty cursor to see changes. The result `cursor` remains the pinned store revision, usable by `proposal` to read the same evidence view.

The page token is opaque to clients. Its implementation is `approval-page/1.` followed by unpadded base64url JSON with `root`, `offset`, `as_of` and `context`. `context` identifies the Host scope and sorted installed functions. A malformed token, an unavailable root, an invalid offset or a different context gives `invalid_cursor` at stage `store`. A page token without `limit` gives `invalid_request`; without `limit`, other cursors retain the old store-read behavior. Tokens are query positions, not signatures or credentials. Every read still checks the reviewer role and filters to the Host scope and functions.

A proposal item has these fields:

| Field | Value |
|---|---|
| `snapshot_ref`, `function`, `scope`, `from`, `until`, `producer`, `reason`, `bundle_ref`, `stale_mode` | Copied from the snapshot. |
| `submitted_at` | From the snapshot. Omitted if the snapshot has none. |
| `wait_seconds` | `as_of` minus `submitted_at`, in seconds. Omitted if `submitted_at` is omitted. The Host does not clamp a negative value, so a clock that goes back is visible. |
| `state`, `refusal` | From the state table. |
| `report_refs`, `decision_refs` | From `state.reviews`, in write order. |

### `proposal` (read)

Body: `{"snapshot_ref": string, "cursor"?: string}`. The [schema](schema/proposal.schema.json) defines the body and the result.

The Host reads the snapshot as `receipt` reads a receipt: an unknown reference is `artifact_unavailable`, a record of another kind is `invalid_record`, and a snapshot of another scope or function is `out_of_scope`.

Result:

| Field | Value |
|---|---|
| `cursor`, `as_of` | As in `proposals`. |
| `proposal` | The proposal item. It can have any state. |
| `snapshot`, `source` | The snapshot record and its source record. |
| `bundle` | An object from each bundle component name to its stored text. |
| `reports` | One entry for each report, in write order: `{"report_ref", "report", "plan_ref", "plan", "refusal"}`. `refusal` is the report refusal of this report. |
| `decisions` | One entry for each verdict, in write order: `{"decision_ref", "decision"}`. |

In 1.2, the result also has `evidence` when the snapshot has an evidence document. The Host checks only the structure of that document. It does not apply the admission rules of [EVIDENCE.md](../evidence/EVIDENCE.md). The document is the statement of the producer. A client must not show its `outcome` as verified by the Host.

The records are the stored records. The schema requires `format` `entelos.record/1` on each record and does not define the other fields. Thus a change of the case or value types (issue [#126](https://github.com/mohanraj00/entelos/issues/126)) does not change this schema.

### Evidence attachment in 1.2

`propose` accepts optional `evidence`, an object with `format: "entelos.evidence/1"`. The producer supplies the document described by [EVIDENCE.md](../evidence/EVIDENCE.md). The Host retains the object in the snapshot. This binds it to the snapshot content reference and idempotency payload.

The Host checks the required members and JSON types in the [evidence schema](../evidence/schema/evidence.schema.json). This check includes nested members. It applies to each path, including the JSONL Host and direct `Handle` calls. The Host refuses unknown members outside attached `value` objects. These checks do not test patterns or value ranges.

The document can have at most 32 nested object or array levels. The document object is at level 1. The encoded document can have at most 262144 bytes (256 KiB). This count uses compact sorted JSON, raw UTF-8 and one final newline, as the core codec writes it. Both limits include attached `value` objects.

An incorrect structure or format, or a document above either limit, gives `invalid_request`. This check occurs with the other request field checks, before a store transaction. The Host writes nothing for that request. A replacement snapshot must explicitly attach its own evidence. It does not inherit a document.

The 1.2 `proposal` result adds `evidence` with the retained document, when present. When absent, the field is omitted. Reads at an earlier store cursor retain that document. A response for a version below 1.2 omits both the top-level `evidence` field and `snapshot.evidence`. The stored snapshot does not change. A proposal made without evidence keeps its earlier record and response fields.

This issue adds attachment and retrieval only. It does not implement the proposed evidence admission rules, validate the producer's factual claims, derive policy inputs or retrieve external objects. The document is the producer's statement. It remains subject to the protocol integer-only value domain and frame bound.

### `decide` (write)

Body in 1.1: `{"report_ref", "reason", "disposition"?, "expected_revision", "request_key"}`. `disposition` is `accept` (the default) or `reject`. The [schema](schema/decide.schema.json) defines it. In 1.0, a body with `disposition` stays `invalid_request`.

The Host does the checks in this order. The first check that fails gives the refusal.

Before each other check, the store must have `state.reviews`: `index_unavailable`.

For `accept`:

1. The report and its snapshot are in scope: `artifact_unavailable`, `invalid_record` or `out_of_scope`.
2. The report checks of 1.0: `qualification_failed`, `untrusted_runner`, `implementation_unavailable`, `stale_qualification`.
3. `reason` is not empty: `invalid_request`.
4. The actor differs from the producer and the runner: `separation_required`.
5. The actor has no verdict on this snapshot: `duplicate_verdict`.
6. No verdict on this snapshot is a `reject`: `proposal_rejected`.

For `reject`:

1. The same scope check as `accept`.
2. `reason` is not empty: `invalid_request`.
3. The same separation check: `separation_required`.
4. The same duplicate check: `duplicate_verdict`.

A `reject` does not need a passed report. A reviewer can reject a failed or stale proposal. A `reject` is final for the snapshot. To change the rule, propose a new revision.

The decision record has the fields of 1.0 and one new field:

```json
{"format":"entelos.record/1","function":"…","scope":"…","from":"…","until":"…",
 "snapshot_ref":"…","report_ref":"…","actor":"reviewer-a","reason":"…",
 "disposition":"accept","runner_policy":"fixture-runner","decided_at":"2026-11-22T09:00:00Z"}
```

The result is `{"cursor", "result": {"decision_ref", "decision"}, "reused"}`. `decision` is the stored record. For a 1.0 request, the adapter returns only `decision_ref`, so the 1.0 result does not change.

The record has no field that names the transport. The JSONL host and the sidecar write the same record for the same request, actor and clock value. The record bytes are equal, so the `decision_ref` values are equal.

Two verdicts with `accept` from different actors are permitted. Both stay in the history. This keeps space for the verdict counts of issue [#128](https://github.com/mohanraj00/entelos/issues/128).

### Fact reviews in 1.2

The [1.2 decide schema](schema/1.2/decide.schema.json) permits optional
`fact_reviews`. It is an array of at most 256 objects, in the order of the
proposal's cited evidence facts. Each object has required `blind` (bool) and
optional `corrected_value` (string or null, at most 65536 characters). No other
field is permitted. Empty strings and null are valid corrections.

After the existing verdict checks, the Host checks the array. The actor must
have recorded kind `human`. The array must have one object for each cited
fact. A `reject` cannot have corrections. An error gives `invalid_request`
and commits nothing. A valid array is retained as `decision.fact_reviews`.
The snapshot and the evidence document do not change. Model and policy actors
cannot supply this array. The Host records their per-fact marks as false.

The field stays optional. A human verdict without it has no recorded blind
marks. The event read refuses such a verdict when it contributes facts after
the stream start. The Host never infers a human mark from absent content.

### `decision_events` (read, 1.2)

Body: `{"cursor"?: string, "limit": integer}`. The limit is from 1 to 256.
The [schema](schema/1.2/decision_events.schema.json) defines the request and
page. Result: `{"events", "next_cursor", "stream_start"}`.
[EVENTS.md](../events/EVENTS.md) defines the event fields, saved start,
exclusive cursors and checks. The Host applies the request, cursor and source
checks in that order. The refusals are `invalid_request`, `invalid_cursor`
and `unexportable_decision`. The read returns no partial page on refusal.
A verdict that cites facts and has no `fact_reviews` gives no events and stops
no read: see [Unmarked verdicts](../events/EVENTS.md#unmarked-verdicts).
The Host checks the `event_reader` role and scope on every read, including resume. That role permits no other operation. A principal with only the `reviewer` role gets `forbidden`.

### `activate` (write)

`activate` keeps its 1.0 body and checks. Before those checks, it refuses with `index_unavailable` if the store has no `state.reviews`. After those checks, it refuses with `proposal_rejected` if a verdict on the snapshot is a `reject`.

### Scope of `record` and `inspect` (read)

Issue [#160](https://github.com/mohanraj00/entelos/issues/160) adds a scope check to the two reads of 1.0. The rule applies to a 1.0 request and to a 1.1 request, on each binding: the JSONL Host, the socket sidecar and HTTP. Each binding gives the request to the same Host, and the Host does the check.

The scope of a request is the `scope` of the Host context. A request body cannot name a scope: a body with a `scope` field is `invalid_request`. Thus a request at the Host always has a scope. On HTTP, a claim without a `tenant`, or with another `tenant`, is `unauthenticated` before the Host reads the body.

`record` reads the record as before. An unknown reference is `artifact_unavailable`, and a record of another kind is `invalid_record`. Then, if the `scope` field of the record is not equal to the Host scope, the result is `scope_mismatch`. A record without a `scope` field also gets `scope_mismatch`, because the Host cannot show that the record is in its scope. The diagnostic message is fixed text. The refusal has no record content, no reference and no other field of the record.

`inspect` gives a copy of the stored state without `decision_stream`. The Host keeps the stored state unchanged. Use `decision_events` to read the stream. The state names the records of each scope in the store. Thus the Host gives the state only if each record object at that cursor has the Host scope. A record object is an object with a `application/vnd.cel-lifecycle.<kind>+json` media type. If one record has another scope, or no scope, the result is `scope_mismatch`. The Host does not filter the state by scope, because the request results in `state.requests` do not name their scope. On a shared store, `inspect` is refused in each scope. Use one store for each scope if a reviewer needs `inspect`.

The scope check of `inspect` reads each object at the cursor. The Git store reads the objects with one `git cat-file --batch` process and keeps one object body in memory at a time. An object larger than 16 MiB, or more than 256 MiB of objects in total, gives `store_error` with stage `store`, and `inspect` gives no state. Both stores have these limits. Use one store for each scope, and keep the store below these limits.

On 2026-11-13, the owner accepted the scope check as a security fix for protocol 1.0 and 1.1, with no protocol 1.2 gate. A 1.0 or 1.1 client that read a record of another scope, or called `inspect` on a shared store, now gets `scope_mismatch`. A client that uses one store for each scope sees no change.

The Host does not give the same answer for an unknown reference and for a record of another scope. The owner accepted this decision on 2026-11-13, for these reasons:

- The 1.0 vector `unknown-record` pins `artifact_unavailable` for an unknown reference. A change to that answer changes a stable vector.
- `receipt`, `proposal` and `decide` already give `artifact_unavailable` for an unknown reference and `out_of_scope` for a record of another scope. One equal answer in `record` only does not remove that signal.
- A reference is a SHA-256 content reference. A caller cannot list the references of another scope. A caller can only ask about a reference that it has.

Thus a caller that has a reference can learn that a record with that reference is in the store, but not its content. A caller that can guess all fields of a record can compute its reference and confirm the guess. Use one store for each scope if this signal is not permitted. A later minor version can give one answer for both cases in all reads.

## Refusal codes

The codes in **bold** are new in 1.1, except `invalid_cursor`, which is new in 1.2. The other codes have the meaning of 1.0.

| Code | Stage | Operations | Condition |
|---|---|---|---|
| `unauthenticated` | `authentication` | all | No principal agrees with the prelude token, or the HTTP credential is missing or not valid. |
| `forbidden` | `authority` | all | The principal does not have the role of the operation. |
| `invalid_request` | `protocol` or Host | all | The body does not agree with its schema, the 1.0 request names a 1.1 operation or field, or `reason` is empty. |
| `unsupported_version` | `protocol` | all | The major version is not 1, or the minor version is more than 2. |
| `artifact_unavailable` | `store` | `proposal`, `decide` | The reference is not in the store. |
| `invalid_record` | `store` | `proposal`, `decide` | The reference is a record of another kind. |
| **`invalid_cursor`** | `store` | `proposals` with `limit`, `decision_events` | A page token is not valid, is not available or is for another query context. |
| `unexportable_decision` | `admission` | `decision_events` | A contributing verdict has incomplete or invalid event provenance. A verdict with cited facts and no `fact_reviews` does not contribute. |
| `out_of_scope` | Host | `proposal`, `decide` | The record has another scope or a function that is not installed. |
| **`scope_mismatch`** | `admission` | `record`, `inspect` | The record has another scope or no scope, or the store at the cursor holds such a record. |
| **`index_unavailable`** | `store` | `proposals`, `proposal`, `qualify`, `decide`, `activate` | The store has no `state.reviews`. |
| `qualification_failed` | `admission` | `decide` accept | The report did not pass, or its bindings differ. |
| `untrusted_runner` | `admission` | `decide` accept | The report is not from the installed runner. |
| `implementation_unavailable` | `admission` | `decide` accept | The qualified build is not installed. |
| `stale_qualification` | `admission` | `decide` accept | The source or the plan has been superseded on the interval. |
| `separation_required` | `admission` | `decide` | The actor is the producer or the runner. |
| **`duplicate_verdict`** | `admission` | `decide` | The actor already has a verdict on this snapshot. |
| **`proposal_rejected`** | `admission` | `decide` accept, `activate` | A verdict on this snapshot is a `reject`. |
| `stale_revision` | `store` | `decide` | `expected_revision` is not the current root. |
| `idempotency_conflict` | `store` | `decide` | The `request_key` was used with another payload. |
| `response_too_large` | `protocol` | `proposals`, `proposal` | The result is larger than the frame bound. |

## Protocol version

A 1.2 receiver accepts the minor versions 0, 1 and 2. The rules below record the earlier 1.1 extension. A response has the version of its request when the receiver supports that version. For each other response, the version is `"1.0"`, the lowest version that the receiver supports. Thus a 1.0 client can read each failure. This applies to a request with a missing, invalid or unsupported version, and to a frame that is not a readable request.

The following table records the changes from 1.0 to 1.1:

| File | Change | Reason |
|---|---|---|
| `schema/envelope.schema.json` | `success.protocol` and `failure.protocol` change from `const` `"1.0"` to `enum` `["1.0","1.1"]`. | A 1.1 response is valid. |
| `vectors/envelope-schema.json` | Add a valid case with `"1.1"` and an invalid case with `"1.2"` for `success` and for `failure`. | The new enum needs cases on both sides. |
| `vectors/session-envelope.json` | Step `higher-minor`: change the request version from `"1.1"` to `"1.2"`. The expected `unsupported_version` response with `"1.0"` stays. | 1.1 is now supported, so `"1.1"` no longer tests an unsupported minor version. |
| `vectors/session-envelope.json` | Steps `current-version`, `stream-two-frames`, `stream-reads-1-byte`, `stream-reads-3-bytes`, `stream-short-read-truncated` and `stream-writes-3-bytes`: add `"reviews":{}` to the expected state. | A new store has the review index. The first spec named only `current-version`. Each stream step also sends `inspect` to a new store. |
| `vectors/session-lifecycle.json` | Step `inspect-empty`: add `"reviews":{}` to the expected state. | The same reason. |
| `test_protocol.py` | The runner and its sidecar self-test expect the response version from the rule above: the request version if it is `"1.0"` or `"1.1"`, else `"1.0"`. | The runner must check the 1.1 rule. |

For the 1.1 change, all other expected values in `../vectors/` stay the same, because each other step sends a 1.0 request or an unreadable frame, and gets `"1.0"`. The old vectors stay in the history of the repository. The implementation PR changes them in place and names each step in its description.

The implementation PR also corrected one vector in this directory. In `approval-verdicts.json`, the step `unknown-token` expected `"protocol":"1.1"`. The sidecar does not read a frame with an unknown token, so the version rule above gives `"1.0"`. The step now expects `"1.0"`, and `check_spec.py` checks this case.

The current 1.2 state adds these changes after the 1.1 changes above:

| File | Current 1.2 state |
|---|---|
| `schema/envelope.schema.json` | `success.protocol` and `failure.protocol` use `enum` `["1.0","1.1","1.2"]`. |
| `vectors/envelope-schema.json` | Success and failure cases accept `"1.2"` and refuse `"1.3"`. |
| `vectors/session-envelope.json` | Step `higher-minor` sends `"1.3"`. It expects `unsupported_version` with `"1.0"`. |
| `approval/vectors/approval-version.json` | Unsupported minor cases send `"1.3"`. They expect `unsupported_version` with `"1.0"`. |
| `test_protocol.py` | The runner and its sidecar self-test expect the request version for `"1.0"`, `"1.1"` or `"1.2"`. For each other version, they expect `"1.0"`. |

## Vectors

`vectors/approval-*.json` use the session format of [PROTOCOL.md](../PROTOCOL.md#vectors), with these additions:

- `principals` at the top level maps a principal name to `{"actor", "roles"}`. The runner makes a random token for each principal and writes the credentials file. The `context` has no `actor` or `roles`.
- `clock` at the top level is the start time. A step can have `clock`, a later time. The runner writes the clock file before the step. A step without `clock` uses the time of the previous step.
- Each step has `principal`, the name of one principal. A step with `"principal":"$unknown"` sends a token that is not in the file.
- `{"$record_bundle":{"allowed_state":[…]}}` in a request is the record-filter bundle that [`prepare.py`](../../cel-handoff/tools/prepare.py) made, with that fixed value. The runner makes it as the [sidecar client](../../../examples/sidecar-client/run.py) does.
- `{"$subset":{…}}` in an expected value matches an object that has each of these fields with an equal value. Other fields can be present.
- `{"$type":"object"}` matches any object.
- `{"$wait":{"since":"snap-1","seconds":172800}}` in an expected value matches the integer `seconds`. `since` names the `$capture` of a `propose` step. The checker uses it to compare the hand-written value with the step clocks.
- `protocol_version` at the top level is `1.1` or `1.2`. It names the receiver version that the file needs. The 1.0 runner reads only `../vectors/session-*.json`, so it does not read these files.
- `principal_scopes` at the top level maps a principal name to another scope. The runner serves a second Host with that scope in its context, on the same store as the first Host, on each transport. The steps of that principal go to the second Host. The other principals use the scope of `context`.
- `parity` at the top level names the steps that the runner also sends to a second Host on a separate store, through `HandleAs` in the same process, with the same principal and clock. The record references (`sha256:` values) in the `$capture` values of those steps must be equal on both paths. A cursor is a Git commit of one store, so the cursor values can differ.

`check_spec.py` checks these rules without a Host:

1. Each request body of an approval operation agrees with its schema in this directory. Each other operation has a schema in [`../schema/`](../schema/).
2. Each expected success result of an approval operation agrees with its result schema.
3. Each expected failure code is in the refusal table of this file, and each new code is in a vector or in the coverage limits below.
4. Each `$wait` value equals the clock of its step minus the clock of the `propose` step that it names.
5. A self-test changes a copy of the vectors and checks that the checker reports each change.

`vectors/page-cases.json` is a synthetic fixture with 250 open proposals, numbered in chronological order. Five calls with a limit of 50 return numbers 0 through 249 exactly once. Before page two, number 50 closes and number 250 opens with an earlier submission time. The continuation retains the original sequence; a fresh read sees number 250 first. The file also pins a bad cursor and invalid limits. `check_spec.py` checks the page ranges and includes mutation self-tests. `TestApprovalPageVectors` builds the fixture and checks the responses through the Host, socket framing with credentials, and both HTTP credential modes. It uses an in-memory store, in-memory streams and the HTTP handler, without listeners or CEL. Existing session vectors cover real listeners and Git stores.

`approval-evidence.json` attaches a valid evidence-format example through `propose`, reads it through `proposal` and pins the old list and proposal response shapes. It also refuses a document without the required `producer` member and has direct Host parity steps. `TestApprovalEvidenceVectorsOffline` runs the same attachment/read vector without CEL or listeners, using a describe-only fixture runtime; it makes no qualification claim.

## Coverage limits

- `index_unavailable` needs a store from v0.3.0. The implementation PR adds a Go test with such a store for each of the five operations. The test also checks that the root does not change. No vector covers it.
- The vectors do not contain a migration. The Go tests in `migrate_test.go` start from a store that the v0.3.0 Host made. They cover each refusal code of the migration.
- The vectors do not remove a principal while a connection is open. The implementation PR adds a Go test: a frame after the removal on the same connection gets `unauthenticated`.
- `untrusted_runner`, `implementation_unavailable`, `stale_qualification`, `stale_revision`, `idempotency_conflict` and `response_too_large` keep their 1.0 tests. The approval vectors do not repeat them.
- The vectors do not test the file mode and owner checks of the credentials file. The implementation PR adds Go tests for them.
- The vectors do not prove constant-time comparison. A code review must check it.
- The vectors do not contain HTTP refusals. The Go tests in [`http_test.go`](../../lifecycle-core/protocol/http_test.go) cover a missing claim, a bad signature, a wrong issuer, an expired claim, the other claim checks, a static token in local mode and the request bounds.
- The Host never writes a record without a `scope` field, so no vector contains one. The Go test `TestRecordAndInspectRefuseAnotherScope` puts such a record in a store and checks that `record` and `inspect` give `scope_mismatch`.
- The session vectors install one function. The synthetic page fixture installs 251; `TestProposalListOrder` also checks ties and missing submission times.

## Limits

- Pagination bounds the number of returned items, not the bytes of one item or the cost of reading and sorting the full list at the pinned revision. A page or evidence document can still get `response_too_large`. Page tokens require the retained store revision; there is no server-side session or expiry. Unpaged requests still return the entire list.
- `proposals` shows only the head snapshot of each function and scope. An older snapshot without a verdict is `superseded`. `decide` can still accept it, as in 1.0, if it has the content of the head. [VERDICT.md](../verdict/VERDICT.md) (#127) adds the refusal `stale_verdict` for other content. `proposal` shows its state.
- `record` and `inspect` show if a reference is in the store, as the section on their scope says. `inspect` reads each record object of the store at the cursor, in one Git process.
- Issue [#127](https://github.com/mohanraj00/entelos/issues/127) (a verdict stale after an edit) and issue [#128](https://github.com/mohanraj00/entelos/issues/128) (human and model verdicts) are in v0.5. This version adds no verdict kind and no digest rule. A later minor version can add fields to the decision record.
- A verdict records who decided and when. With `fact_reviews`, it also records the per-fact blind marks. These marks are statements of the review interface.
