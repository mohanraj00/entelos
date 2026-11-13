# Changelog

This file lists the changes for each snapshot tag. A tag is a stable snapshot to cite and copy. It is not a package release. Up to v0.3.0, a tag does not promise a stable API or record format.

## Unreleased

- The `record` and `inspect` reads check the scope (#160). `record` refuses
  a record of another scope, or a record without a scope, with the new code
  `scope_mismatch`. `inspect` refuses with `scope_mismatch` if the store at
  the cursor holds such a record. The refusal has no record content. An
  unknown reference stays `artifact_unavailable`, so a caller that has a
  reference can still learn that it is in the store. The rule applies on
  each binding. This changes results of protocol 1.0 and 1.1. The owner
  accepted it as a security fix. The scope scan of `inspect` keeps one object
  in memory at a time, and gives `store_error` for an object over 16 MiB or
  for more than 256 MiB in total. The new vector file `approval-scope.json`
  has 11 steps.

- The new command `entelos-migrate` builds the review index of approval API
  1.1 for a store from v0.3.0 (#159). It adds one commit with
  `state.reviews` and changes no record. `--dry-run` writes nothing, and a
  second run writes nothing. It refuses an index that does not agree with
  the history. The Go package `lifecycle` adds the experimental
  `MigrateReviewIndex`.

  Upgrade steps for a v0.3.0 store:

  1. Stop each Host on the store.
  2. Make a copy of the bare repository.
  3. Run `entelos-migrate --repo <store.git> --dry-run` and examine the index.
  4. Run `entelos-migrate --repo <store.git>`. The action must be `migrated`.
  5. Start the Host. A v0.3.0 report names the v0.3.0 lifecycle build, so
     qualify an open proposal again before an accept.

  See [APPROVAL.md](src/protocol/approval/APPROVAL.md#migration-from-v030).

- [STABILITY.md](docs/STABILITY.md) names the stable surfaces (#147): the
  exported Go API, the wire protocol 1.0 and 1.1 and the record format
  `entelos.record/1`. It gives the promise for 0.x releases and the
  deprecation path. `make check` runs the pin in `tools/stability/`.

- [POLICY.md](src/protocol/policy/POLICY.md) proposes the policy verdict
  `entelos.policy/1` (#154): accept, refuse or route to a person, with
  fail-closed routes, a `suspend` stop control, an audit draw and a
  replayable receipt. A policy verdict never satisfies a human verdict
  requirement. `make check` runs its spec checker. No Host writes it yet.

- [examples/approval-dashboard](examples/approval-dashboard/README.md) adds an
  optional approval dashboard (#157) on the HTTP approval API with a local
  static token. It uses the mugham UI kit at the tag `v0.1.0`, pinned in
  `mugham.json`. The new sidecar flag `--http-static` serves its public files
  from the origin of the API. `make verify` runs its browser test: the list,
  the evidence view, one verdict and one API refusal.
  The Go package `protocol` adds the experimental `StaticHandler` and
  `NewStaticHandler`.

## v0.3.0 (2026-11-11)

- The Host rejects wrong field types, and both stores refuse state that is too deep to read (#136).
- Retrying a write requires current access to the affected functions (#133).
  An authorized retry still returns its original result and cursor.
- The standalone verifier supports spaces in the path to the WASM launcher.
- The sidecar requires a token on each request (#131). Each request frame
  starts with the prelude `ENT1` and the 64 hex digits of the token. The
  operator configures and rotates the token with `--token-file`. A missing or
  wrong token gives `unauthenticated` with the stage `authentication`. See
  [PROTOCOL.md](src/protocol/PROTOCOL.md#unix-socket-authentication).
- Go callers create the socket server with `protocol.NewServer(adapter,
  tokenFile)`. This replaces `protocol.Server{Adapter: ...}`.
- [ADD_CONSUMER.md](docs/ADD_CONSUMER.md) tells how to add a consumer example
  (#132). The [ci-trigger README](examples/ci-trigger/README.md) describes the
  PR CI example (#111).

### Compatibility and limits

- Record format remains `entelos.record/1`. The v0.1.0 store format is
  unsupported. Keep the original lifecycle and engine builds with the store
  for exact replay. See [compatibility and retention](docs/SUPPORT.md#compatibility-and-retention).
- This is a breaking change for sidecar clients. A v0.2.0 client must add
  the token prelude before it calls the v0.3.0 sidecar. The JSON messages
  remain at version 1.0.
- Use trusted local directories and a separate store for each scope. The
  callable profile accepts string and bool inputs and returns a bool.
- A local Codex security review covered the full Go approval and record path
  at `9b8261d` (#136). It found one P2 issue, and #137 fixed it. The review
  read the source. Its sandbox stopped `go test` and `go vet`, so the fix PR
  ran them.

## v0.2.0 (2026-11-06)

This snapshot replaces the Python lifecycle with a Go core and adds a local sidecar and a browser executor. It does not read stores from v0.1.0.

### Reference code

- The Go lifecycle core in `src/lifecycle-core` is now the only lifecycle Host (#95). The Python Host, its stores, its JSONL CLI and the protocol reference adapter are removed.
- `src/cel-lifecycle` keeps the specs, the storage port vectors and the scenario fixture.
- `scenario_test.go` runs the lifecycle scenario in the Go tests. Its expected values come from the fixture.
- `src/lifecycle-core/client/` holds the Python callers of the Go JSONL host: the scenario driver and the generic caller. The examples use them.
- `src/protocol/test_protocol.py` runs the session vectors only through the Go sidecar socket.
- `verify.py` has no parity stage. The `go-lifecycle-core` check runs the Go scenario, the storage port vectors and the Host tests.
- Each stored record has the field `format: "entelos.record/1"` (#78). A read of a record without this field, or with an unknown major version, fails closed with `unsupported_record_format`. The new `record-format` storage vector tests this rule. Stores from v0.1.0 are not migrated. See [SUPPORT.md](docs/SUPPORT.md).
- An expired or revoked activation fails closed by default (#82). The new diagnostics are `activation_expired` and `activation_revoked`. A proposal can declare `stale_mode` `continue_stale`. Then the loaded activation gives the answer, and the receipt has `stale` `true`. Each receipt records `stale_mode`, `stale` and `stale_reason`. See [CONTRACTS.md](docs/CONTRACTS.md#expiry-and-revocation).
- Each receipt records its provenance (#81): `executor_id`, `log_cursor`, `inputs_ref` and `selection.snapshot_ref`. The new `ingest` operation records an executor log, idempotent on the executor ID and the cursor. It keeps each answer and records the Host stale view in `host_stale_reason`. See [CONTRACTS.md](docs/CONTRACTS.md#receipt-provenance-and-ingest).
- At ingest, the Host compares the response `implementation_ref` with the snapshot `engine_build` (#108). A different or missing value gives `artifact_mismatch` and stops the ingest. This also applies to an entry from before this change.
- A storage port has conformance vectors and an in-memory adapter (#79). Both stores pass the storage vectors.
- A qualification runner interface separates the runner from the Host (#80).
- Wire protocol 1.0 is the versioned protocol for a client (#83). See [PROTOCOL.md](src/protocol/PROTOCOL.md).
- `entelos-sidecar` serves this protocol on a local Unix socket (#84). The new example is [sidecar-client](examples/sidecar-client/README.md).
- A WebAssembly build of the executor runs in a browser and keeps a receipt log (#93). The new example is [browser-executor](examples/browser-executor/README.md).

### Docs

- [Decision 0001](docs/decisions/0001-technology.md) records the technology choice (#85). [Decision 0002](docs/decisions/0002-browser-executor.md) records the browser executor design.
- The README, [SUPPORT.md](docs/SUPPORT.md) and [SECURITY.md](SECURITY.md) state the limits of the sidecar, ingest and the browser executor.

### Limits

- This snapshot does not read a v0.1.0 store, and there is no migration tool. See [SUPPORT.md](docs/SUPPORT.md).
- APIs, the protocol and the record format are unstable. The only callable profile is string/bool input and bool output.
- The sidecar has no authentication. The only access control is the file mode of the socket and of its directory. v0.3.0 adds a token (#131).
- An ingested receipt is the statement of the executor. The Host does not run the rule again, and a content reference is not a signature.
- The M5 security review covered the v0.1.0 Python path. Security-relevant M6 pull requests had a security review one at a time. No one repeated the review for the full Go path as one review.
- The `inspect` and `record` reads do not check the scope. Use one store for each scope.
- A `request_key` with a NUL byte gives `store_error` on the Git store. The in-memory store accepts this key.
- Linux and macOS are tested. Windows is not tested. `verify.py` needs a Chromium browser.

## v0.1.0 (2026-11-04)

The first snapshot. It contains the runnable reference and its docs.

### Reference code

- `src/cel-handoff`: a Go host that prepares and evaluates typed CEL rules. Rules accept named string and bool inputs and give a bool result.
- `src/cel-lifecycle`: a Python lifecycle that records each proposal, qualification, approval, dated activation, answer, replay and restatement in a local Git store.
- Three consumer examples that reuse `src/` without copies: `record-filter`, `ci-trigger` and `tier-eligibility`.
- `verify.py`: the full check runner. CI runs it on `ubuntu-24.04` and `macos-15`.

### Docs

- The README states the problem first, gives one demo command and puts the limits at the top.
- [WALKTHROUGH.md](docs/WALKTHROUGH.md), [CORRECTION.md](docs/CORRECTION.md) and the [decision path diagram](docs/diagrams/decision-path.svg).
- [COMPARISON.md](docs/COMPARISON.md) compares OPA/Rego, Cedar, GoRules ZEN, feature-flag services and ordinary code plus Git.
- [CONTRACTS.md](docs/CONTRACTS.md) gives the callable cases and the author fixture fields.
- `CONTRIBUTING.md`, issue templates and a review path for a contributor without an agent harness.

### Security fixes

The M5 security review of the approval and record path found five issues. #74 fixed them.

- An answer now requires the lifecycle build that the qualification report names. Before, only the report check compared the build.
- The reviewer who approves must be different from the producer and the policy runner.
- A replay receipt records `matches_original`, which shows if the new answer is the same as the recorded answer.
- A wrong JSON type in `inputs_json` and very deep JSON nesting now give a fault. Before, they stopped the CLI stream.

### Limits

- The record format has no version field. A later snapshot can change the records, and new code may not load an old store. See [SUPPORT.md](docs/SUPPORT.md).
- APIs are unstable. The only callable profile is string/bool input and bool output.
- Linux and macOS are tested. Windows is not tested.
- The `inspect` and `record` reads do not check the scope. Use one store for each scope.
- A `request_key` with a NUL byte gives `store_error`. The state does not change.
