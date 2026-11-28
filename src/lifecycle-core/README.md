# Go lifecycle core

This module is the lifecycle core. It is the only lifecycle Host. The specs, the conformance vectors and the fixtures are in `src/cel-lifecycle/`. Milestone M6 ([#88](https://github.com/mohanraj00/entelos/issues/88)) moved the core from Python to Go. Issue [#95](https://github.com/mohanraj00/entelos/issues/95) removed the Python core.

## Contents

- `host.go`: the Host: the operations, roles, faults and records.
- `store.go`: the storage port (`Read`, `Transact`, `History`) and the read view. `View.Put` writes the record format `entelos.record/1`. `View.Get` is the decode path of each record read, and it fails closed on an unknown record format.
- `gitstore.go`: the local Git adapter. It publishes state only through a compare-and-swap of `refs/lifecycle/state`.
- `memstore.go`: an in-memory adapter with the same publication contract. It is not durable.
- `canon.go`: the record codec. Content references depend on its bytes.
- `runtime.go`: the CEL adapter. It calls the pinned `cel-handoff` executable.
- `runner.go`: the qualification runner contract. [RUNNER.md](RUNNER.md) is the spec. `CELRunner` is the first runner.
- `step.go`: the step outcome contract of a workflow agent step ([#113](https://github.com/mohanraj00/entelos/issues/113)). [STEP.md](STEP.md) is the spec. A step result becomes the inputs of an `invoke`, and the Host has no step operation. The core has no step runner. [examples/agent-step-koodam](../../examples/agent-step-koodam/README.md) is one optional adapter.
- `finding.go`: the finding import contract ([#112](https://github.com/mohanraj00/entelos/issues/112)). [FINDING.md](FINDING.md) is the spec. The Host-only write `import_finding` makes a candidate case from a finding record of an agent test. The Host-only write `decide_case` records the decision of a reviewer. A plan imports only an accepted candidate.
- `cmd/lifecycle-core`: the JSONL host. It reads one request record on each stdin line and writes one response record on each stdout line. The arguments are `--repo`, `--context`, `--runtime` and `--work`.
- `protocol/`: the wire protocol 1.0 of [PROTOCOL.md](../protocol/PROTOCOL.md) in front of the Host: frames, the RFC 8785 encoder, the schema validator and the Unix socket listener.
- `cmd/entelos-sidecar`: the local sidecar ([#84](https://github.com/mohanraj00/entelos/issues/84)). It serves the protocol on a Unix domain socket.
- `migrate.go` and `cmd/entelos-migrate`: the migration of a v0.3.0 store to the review index of approval API 1.1 ([#159](https://github.com/mohanraj00/entelos/issues/159)). The arguments are `--repo` and the optional `--dry-run`. See [APPROVAL.md](../protocol/approval/APPROVAL.md#migration-from-v030).
- `sign.go`: signed approval and activation records ([#155](https://github.com/mohanraj00/entelos/issues/155)). After `Host.SetSigner`, the Host signs each `decision`, `event`, `activation` and `suspension` record that it writes. `VerifySigned` checks a signed record against a public key set, with no service. [SIGNED.md](../protocol/signed/SIGNED.md) is the spec. `lifecycle-core` and `entelos-sidecar` take the key file with `--signing-key`.
- `activation.go`: the activation record of [ACTIVATION.md](../protocol/activation/ACTIVATION.md) ([#105](https://github.com/mohanraj00/entelos/issues/105)). A signing Host writes one with each `enable` event. `VerifyActivation` checks a signed activation record offline. `Host.SetPublisher` installs an optional `Publisher`, which gets the record before the commit and can refuse it. The file also has the suspension record of the same spec ([#211](https://github.com/mohanraj00/entelos/issues/211)). A signing Host writes one with each `suspend` event, the `Publisher` gets it with the same rules, and `VerifySuspension` checks it offline. A publisher call has a time limit ([#212](https://github.com/mohanraj00/entelos/issues/212)): `Publish` gets a context with a deadline, and `Host.SetPublishTimeout` sets the limit.
- `gitstore_lock_unix.go` and `cmd/entelos-unlock`: the recovery of a stale lock file of the root ref ([#212](https://github.com/mohanraj00/entelos/issues/212)). `GitStore.RecoverRootLock` removes the lock file only when no update of the root ref is in progress. [ACTIVATION.md](../protocol/activation/ACTIVATION.md#stale-lock-of-the-root-ref) gives the procedure.
- Supersession ([#129](https://github.com/mohanraj00/entelos/issues/129)): `activate` takes `supersedes`, and `host.go` removes a superseded update from the selection at each date from the start of its successor. [SUPERSESSION.md](../protocol/supersession/SUPERSESSION.md) is the spec.
- `cmd/entelos-verify`: the offline check of a signed record file. The arguments are `--keys` and the file. `cmd/entelos-keygen` makes a new signing key file. Its argument is `--out`.
- `internal/private`: the read of a secret file with mode `0600`.
- `client/`: Python callers of the JSONL host. They contain no Host and no store. `lifecycle_client.py` encodes the requests and starts the host for each request. `scenario.py` runs the neutral lifecycle scenario. `caller.py` discovers a callable, then invokes it. The examples use these files.

## Dependencies

The module uses only the Go standard library. It does not import `cel-go`, because CEL evaluation stays in the pinned `cel-handoff` executable. Thus this module adds no dependency and no licence. It needs Go 1.27.2 and `git` on the `PATH`.

## Verification

The `go-lifecycle-core` check in `verify.py` does these steps:

1. Build `cmd/lifecycle-core`.
2. Run `go test` and `go vet`.

`scenario_test.go` runs the neutral lifecycle scenario on a new Git store, with the pinned CEL executable. It compares each answer, replay and restatement with the `expected` field of `src/cel-lifecycle/fixtures/lifecycle-scenario.json`. `LIFECYCLE_CEL_BINARY` names the CEL executable and `LIFECYCLE_CEL_BUNDLE` names the prepared record bundle. If these are not set, the scenario and the Host tests skip. `verify.py` also sets `LIFECYCLE_REQUIRE_SCENARIO`, so that a skip is a failure. `LIFECYCLE_SCENARIO_RESULTS` names a file for the checks, the pins and the trace of the scenario.

The Host tests in `host_test.go` start from a copy of the Git repository that the scenario made. The pins name the records of that scenario. `TestUnknownRecordFormatFailsClosed` plants receipts with an unknown, a missing and a malformed format. The `receipt`, `record` and `replay` operations must each give a structured diagnostic and publish no state. `TestRecordAndInspectRefuseAnotherScope` checks that `record` and `inspect` give `scope_mismatch` in another scope and for a record without a scope, and that the Git batch read and the in-memory read give the same scopes. `TestScopeScanStreamsAndHasLimits` checks the object and total limits of the scan on both stores and through `inspect`, and each malformed batch output, with a size check before the body read. The stale mode tests check that `fail_closed` is the default, that an invalid `stale_mode` is refused, that a caller cannot select the mode, that `continue_stale` gives a stale receipt on revocation, and that a reader finds each stale receipt with `inspect` and `receipt`. `typed_test.go` runs the typed vector `src/cel-lifecycle/conformance/typed/admission-record.json` through the Host on a new Git store. The rule has an int, a date and a list input and a record output. The test qualifies the bundle with the cases of `typed-cases-pass` in `case-checks.json`, then approves, activates and loads it. It invokes each of the 29 run steps and compares each answer with the vector. A refused input gives a receipt with the canonical `inputs_ref`. The test replays each receipt and checks that the replay gives the same response bytes. It needs `LIFECYCLE_CEL_BINARY`. `schema_test.go` checks that each schema file in `src/protocol/schema/` agrees with the Host table.

`vectors_test.go` runs each storage port vector in `src/cel-lifecycle/conformance/storage/` on a new Git store and a new in-memory store. `ingest_test.go` runs each ingest vector in `src/cel-lifecycle/conformance/ingest/` on both adapters. It also has the negative ingest tests.

The `go test` step also runs the `protocol/` tests:

- `protocol/vectors_test.go` runs each file in `src/protocol/vectors/` in the process: the session steps on a new Git store, with short reads and short writes, the RFC 8785 cases and the envelope schema cases.
- `protocol/adapter_test.go` uses stub Hosts. A panic, an error that is not a `Fault`, a float, an integer outside ±(2^53 − 1) and a Go type outside the value domain each give one `internal_error` frame with no error text. A result over the frame bound gives `response_too_large`.
- `protocol/socket_test.go` tests the socket rules below.

The `sidecar-protocol` check in `verify.py` builds `cmd/entelos-sidecar`. Then it runs `src/protocol/test_protocol.py --sidecar`, which runs each session vector file through the socket of a new sidecar process. The `sidecar-client` check runs [examples/sidecar-client](../../examples/sidecar-client/README.md).

## Sidecar

```sh
entelos-sidecar --socket <path> --repo <store.git> --context <context.json> --runtime <cel-handoff> --work <dir> --schema src/protocol/schema --token-file <path>
entelos-sidecar ... --credentials-file <path>
```

- The host context comes from the `--context` file. A request cannot supply the actor, the roles or the scope. The context fields are `actor`, `roles`, `scope`, `runner_actor`, `worker`, `functions` and the optional boolean `upload_inputs`, the ingest upload policy. See [CONTRACTS.md](../../docs/CONTRACTS.md#receipt-provenance-and-ingest). The optional fields `actor_kinds` and `verdict_policy` give the kind of each actor and the count of each kind that `activate` requires. See [KINDS.md](../protocol/verdict/KINDS.md).
- The sidecar reads the schemas from `--schema` at start. If an operation schema does not agree with the Host read operations, the sidecar does not start.
- Create the token in a private directory, for example with `(umask 077; set -C; openssl rand -hex 32 > sidecar.token)`. The token file must be a regular file owned by the sidecar user with mode 0600. It contains exactly 64 lowercase hex digits and may end with one newline. The sidecar refuses to start with an invalid file. Pass only the file path on the command line.
- The client sends the [token prelude](../protocol/PROTOCOL.md#unix-socket-authentication) before each request. To rotate, create a new token file in the same directory, then rename it over the old file. The sidecar opens the file for each authentication check, including on existing connections. An unavailable or invalid file denies new requests. A request that has passed the token check can finish.
- With `--credentials-file` in place of `--token-file`, each reviewer has a token of its own. The file format, the checks and the rotation are in [APPROVAL.md](../protocol/approval/APPROVAL.md). The context file then has no `actor` and no `roles`, because each request runs as the principal of its token. The sidecar also reads the approval schemas from `../approval/schema` next to `--schema`, and accepts protocol 1.1.
- `--http-listen 127.0.0.1:<port>` adds the [HTTP binding](../protocol/approval/APPROVAL.md#http-binding) next to the socket. It needs `--credentials-file`, and one of `--issuer-file <path>` (signed claims) or `--http-local-tokens` (the static tokens of the credentials file, loopback only). The address must be loopback unless `--http-allow-remote` is given. The sidecar has no TLS, so a remote address needs a TLS front proxy. The stdout line then also has `"http":"<address>"`. `--http-static <directory>` serves the public files of that directory with `GET`, so a browser client has the same origin as the API ([static files](../protocol/approval/APPROVAL.md#static-files)).
- [AUTH.md](cmd/entelos-sidecar/AUTH.md) gives the authentication of each operation of each protocol version on each transport. `TestSidecarAuthMatrix` checks it.
- `--clock-file <path>` replaces the system clock with the time in that file. Use it only in tests.
- A Go embedding that serves a listener constructs it with `protocol.NewServer(adapter, tokenFile)` or `protocol.NewCredentialServer(adapter, credentialsFile)`. This replaces construction with a `Server{Adapter: ...}` literal.
- The socket file has mode 0600. The umask is 0177 while the sidecar makes the socket, so the mode is 0600 from the start.
- If the socket path exists, the sidecar removes it only if it is a stale socket: a socket file with no listener. If the path is another file type or a socket with a listener, the sidecar does not start.
- While the sidecar starts, it holds an exclusive lock on `<path>.lock` for the stale check, the removal and the bind. It releases the lock when the socket listens. Thus two sidecars that start at the same time at one stale socket cannot both remove it. The lock file stays, with mode 0600. If the lock path is not a regular file that the current user owns and can read and write, or another process holds the lock for 10 seconds, the sidecar does not start.
- The socket path has a maximum of 103 bytes, the `sun_path` limit of macOS. For a longer directory, start the sidecar in the socket directory and give a relative path.
- When the socket listens, the sidecar writes one line to stdout: `{"socket":"<path>","status":"listening"}`.
- Each connection has its own goroutine. The Host gets one request at a time.
- On SIGINT or SIGTERM, the sidecar closes the open connections, removes the socket file and exits with status 0.

## Build identity

The lifecycle build identity uses the names and bytes of the Go source files in this directory. It excludes test files and subdirectories. New Host execution receipts record the Go and Git versions that the Host reads when it starts. The receipt field is `host_tools`.

If only the Go or Git version changes, the lifecycle build identity stays the same. Qualification reports and accept decisions for this build identity remain valid. The Host does not qualify the snapshot again. To get a new qualification after a tool version changes, run `qualify` again. The runner and engine build checks still apply.

Reports and receipts with the old identity remain readable. Qualify again, accept the new report, and replace the old activation before you resume service. Keep the original build for exact replay of an old receipt. See the [lifecycle build rules](RUNNER.md#lifecycle-build).

## Limits

- Records that hold the lifecycle build (reports, receipts and decisions) get new content references when a core source changes.
- The scenario runs only on the Git adapter. The in-memory adapter has the storage vectors, store tests and one in-process Host test.
- Windows is not tested. The sidecar is POSIX only.
- Put the socket and the token in a directory with mode 0700. Every client holding the token uses the host context selected for that socket. Treat the socket directory and the account that runs the sidecar as trusted.
- The socket vectors cannot make the sidecar do short writes. The in-process Go tests cover short reads and short writes.
