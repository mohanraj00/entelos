This record is the text of a read-only security review by agy (Antigravity), run 3, with a session-only allow list of read commands, on the commit 5d65d23. The only change is the link form. Issue [#251](https://github.com/mohanraj00/entelos/issues/251) tracks finding F1.

## Release candidate

- Commit: 5d65d2362312707149041c85cd15324324987667
- Date: 2026-11-28
- Reviewer: agy (Antigravity), read-only

## Method

The reviewer read the source code and the unit tests for each path in scope. The reviewer used only read commands from the repository root. The reviewer did not run the code or the test suites. The reviewer evaluated trust boundaries, attacker inputs, and validation checks against the project threat model.

## Scope 1: Go lifecycle core and migration

### Files and tests read
- Source files:
  - [src/lifecycle-core/host.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/host.go)
  - [src/lifecycle-core/gitstore.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/gitstore.go)
  - [src/lifecycle-core/gitstore_lock_unix.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/gitstore_lock_unix.go)
  - [src/lifecycle-core/store.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/store.go)
  - [src/lifecycle-core/migrate.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/migrate.go)
  - [src/lifecycle-core/cmd/entelos-migrate/main.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-migrate/main.go)
  - [src/lifecycle-core/cmd/entelos-unlock/main.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-unlock/main.go)
  - [src/lifecycle-core/approval.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/approval.go)
  - [src/lifecycle-core/kinds.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/kinds.go)
  - [src/lifecycle-core/runtime.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/runtime.go)
- Test files:
  - [src/lifecycle-core/host_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/host_test.go)
  - [src/lifecycle-core/gitstore_lock_unix_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/gitstore_lock_unix_test.go)
  - [src/lifecycle-core/migrate_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/migrate_test.go)

### Read commands used
- `grep -n "func (h \*Host)" src/lifecycle-core/host.go`
- `sed -n '220,340p' src/lifecycle-core/host.go`
- `sed -n '1113,1250p' src/lifecycle-core/host.go`
- `sed -n '1240,1420p' src/lifecycle-core/host.go`
- `sed -n '720,775p' src/lifecycle-core/host.go`
- `sed -n '343,380p' src/lifecycle-core/host.go`
- `grep -n "marker" src/lifecycle-core/gitstore.go`
- `grep -n "holdUpdate" src/lifecycle-core/gitstore*`
- `cat src/lifecycle-core/gitstore_lock_unix.go`
- `cat src/lifecycle-core/migrate.go`
- `cat src/lifecycle-core/cmd/entelos-migrate/main.go`
- `grep -n "func (h \*Host) decide" src/lifecycle-core/*.go`
- `sed -n '390,460p' src/lifecycle-core/approval.go`
- `sed -n '110,150p' src/lifecycle-core/gitstore.go`
- `grep -n "env" src/lifecycle-core/gitstore.go`
- `sed -n '440,470p' src/lifecycle-core/gitstore.go`
- `sed -n '470,520p' src/lifecycle-core/gitstore.go`
- `cat src/lifecycle-core/cmd/entelos-unlock/main.go`
- `grep -n "func decisionBound" src/lifecycle-core/*.go`
- `sed -n '190,210p' src/lifecycle-core/kinds.go`
- `cat src/lifecycle-core/runtime.go`

### Trust boundary
The Host executes in a trusted local process for one operating system user. The store directory belongs to that user. The caller supplies operations, request keys, revisions, parameters, and bodies. The Host enforces roles, subject scopes, separation of duty, and build identity checks.

### Findings
No finding.

The Host strictly verifies that the actor role matches the operation specification. In `decide`, the Host requires reviewer separation from candidate producers and runners. It blocks duplicate verdicts and enforces reject priority. In `activate`, the Host verifies passed reports, matching lifecycle and engine builds, and applicability confirmation. In `replay`, it enforces exact input byte matching and lifecycle build identity. In `inspect`, the Host checks that all store records match the trusted scope. The update marker uses mode 0600 and effective user ownership. It takes shared locks during ref updates and exclusive locks during recovery. Git subprocess execution sets fixed configuration overrides and runs with a sanitized environment. Migration checks store history and indexes review entries atomically.

## Scope 2: Sidecar binding, protocol, and authentication

### Files and tests read
- Source files:
  - [src/lifecycle-core/cmd/entelos-sidecar/main.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-sidecar/main.go)
  - [src/lifecycle-core/cmd/entelos-sidecar/AUTH.md](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-sidecar/AUTH.md)
  - [src/lifecycle-core/protocol/socket.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/socket.go)
  - [src/lifecycle-core/protocol/auth.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/auth.go)
  - [src/lifecycle-core/protocol/credentials.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/credentials.go)
  - [src/lifecycle-core/protocol/claims.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/claims.go)
  - [src/lifecycle-core/protocol/http.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/http.go)
  - [src/lifecycle-core/protocol/adapter.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/adapter.go)
  - [src/lifecycle-core/protocol/static.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/static.go)
  - [src/lifecycle-core/content.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/content.go)
  - [src/lifecycle-core/decision_events.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/decision_events.go)
- Test files:
  - [src/lifecycle-core/protocol/auth_matrix_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/auth_matrix_test.go)
  - [src/lifecycle-core/protocol/content_wire_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/content_wire_test.go)
  - [src/lifecycle-core/protocol/socket_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/socket_test.go)
  - [src/lifecycle-core/protocol/http_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/http_test.go)

### Read commands used
- `cat src/lifecycle-core/cmd/entelos-sidecar/main.go`
- `cat src/lifecycle-core/protocol/socket.go`
- `cat src/lifecycle-core/protocol/auth.go`
- `cat src/lifecycle-core/protocol/credentials.go`
- `cat src/lifecycle-core/protocol/claims.go`
- `cat src/lifecycle-core/protocol/http.go`
- `cat src/lifecycle-core/protocol/adapter.go`
- `sed -n '130,200p' src/lifecycle-core/protocol/adapter.go`
- `grep -n "unsupportedSubject" src/lifecycle-core/protocol/adapter.go`
- `sed -n '215,230p' src/lifecycle-core/protocol/adapter.go`
- `cat src/lifecycle-core/cmd/entelos-sidecar/AUTH.md`
- `cat src/lifecycle-core/protocol/auth_matrix_test.go`
- `grep -n "func celOnly" src/lifecycle-core/*.go`
- `sed -n '418,435p' src/lifecycle-core/content.go`
- `grep -n "func (h \*Host) proposals" src/lifecycle-core/*.go`
- `sed -n '252,300p' src/lifecycle-core/approval.go`
- `sed -n '300,350p' src/lifecycle-core/approval.go`
- `sed -n '1,50p' src/lifecycle-core/decision_events.go`
- `cat src/lifecycle-core/protocol/static.go`
- `sed -n '90,120p' src/lifecycle-core/approval.go`
- `sed -n '70,95p' src/lifecycle-core/approval.go`
- `sed -n '135,170p' src/lifecycle-core/protocol/http.go`
- `sed -n '255,280p' src/lifecycle-core/protocol/adapter.go`
- `grep -n "Getuid" src/lifecycle-core/*.go src/lifecycle-core/*/*.go`
- `grep -n "Geteuid" src/lifecycle-core/*.go src/lifecycle-core/*/*.go`

### Trust boundary
The sidecar sits between unauthenticated clients and the lifecycle Host. The client sends framed JSON on a Unix socket or HTTP POST requests to `/v1/request`. The sidecar authenticates credentials before reading frames or bodies. It maps credentials to operator-defined principals and roles. It validates JSON schemas, enforces protocol limits, and serializes requests to the Host.

### Findings

#### Finding F1
- Title: Lock file owner check in Listen uses real UID instead of effective UID
- Severity: P3
- Location: [src/lifecycle-core/protocol/socket.go:49](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/socket.go#L49)
- Reachability: Yes. A local operator who executes the sidecar in an environment where real UID differs from effective UID (for example, under privilege elevation or privilege separation).
- Scenario: `Listen` checks `int(st.Uid) != os.Getuid()` on `<socket>.lock`. In contrast, `openPrivate` in `src/lifecycle-core/protocol/auth.go:36` and `openMarker` in `src/lifecycle-core/gitstore_lock_unix.go:36` check `int(st.Uid) != os.Geteuid()`. When the effective UID differs from the real UID, `Listen` rejects a lock file owned by the effective process owner.
- Fix: Replace `os.Getuid()` with `os.Geteuid()` in [src/lifecycle-core/protocol/socket.go:49](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/socket.go#L49).

## Scope 3: Signed records and offline verification

### Files and tests read
- Source files:
  - [src/protocol/signed/SIGNED.md](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/protocol/signed/SIGNED.md)
  - [src/lifecycle-core/sign.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/sign.go)
  - [src/lifecycle-core/cmd/entelos-keygen/main.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-keygen/main.go)
  - [src/lifecycle-core/cmd/entelos-verify/main.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-verify/main.go)
- Test files:
  - [src/lifecycle-core/sign_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/sign_test.go)
  - [src/lifecycle-core/cmd/entelos-verify/compat_v080_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/cmd/entelos-verify/compat_v080_test.go)

### Read commands used
- `cat src/protocol/signed/SIGNED.md`
- `cat src/lifecycle-core/sign.go`
- `cat src/lifecycle-core/cmd/entelos-keygen/main.go`
- `cat src/lifecycle-core/cmd/entelos-verify/main.go`
- `sed -n '180,240p' src/lifecycle-core/sign.go`
- `grep -n "func ParseKeySet" src/lifecycle-core/sign.go`
- `sed -n '255,310p' src/lifecycle-core/sign.go`
- `grep -n "func (s \*Signer) PublicKey" src/lifecycle-core/sign.go`
- `sed -n '165,180p' src/lifecycle-core/sign.go`

### Trust boundary
A signing key file is a private local file of the store owner. The key set is a public configuration file. The consumer checks signed records offline without calling network services. The signed document contains the base64 record bytes, an Ed25519 signature, domain separation metadata, and binding copies of scope and function.

### Findings
No finding.

`entelos-keygen` creates private keys with mode 0600 and exclusive flags. It prints public keys only. `entelos-verify` bounds input file sizes to 2 MiB. `VerifySigned` checks the signature domain, media type, and exact record bytes. It validates that the record reference equals the SHA-256 digest of the envelope. It checks key status and rejects revoked keys. It validates that the outer scope and function match the inner record. The Host signs staged records before commit. It verifies each signed record with its public key before storage.

## Scope 4: Publisher call and evidence admission

### Files and tests read
- Source files:
  - [src/protocol/evidence/EVIDENCE.md](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/protocol/evidence/EVIDENCE.md)
  - [src/lifecycle-core/activation.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/activation.go)
  - [src/lifecycle-core/evidence_admission.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/evidence_admission.go)
  - [src/lifecycle-core/evidence.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/evidence.go)
  - [src/lifecycle-core/content.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/content.go)
- Test files:
  - [src/lifecycle-core/activation_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/activation_test.go)
  - [src/lifecycle-core/evidence_admission_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/evidence_admission_test.go)
  - [src/lifecycle-core/evidence_test.go](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/evidence_test.go)

### Read commands used
- `cat src/protocol/evidence/EVIDENCE.md`
- `cat src/lifecycle-core/activation.go`
- `sed -n '165,220p' src/lifecycle-core/activation.go`
- `sed -n '220,250p' src/lifecycle-core/activation.go`
- `cat src/lifecycle-core/evidence_admission.go`
- `grep -n "checkSnapshotEvidence" src/lifecycle-core/*.go`
- `sed -n '580,610p' src/lifecycle-core/content.go`
- `sed -n '1,60p' src/lifecycle-core/evidence.go`
- `grep -n "func Test" src/lifecycle-core/activation_test.go`
- `grep -n "func Test" src/lifecycle-core/evidence_admission_test.go`

### Trust boundary
The publisher is an optional component installed by the embedding host. It receives signed activation or suspension records before commit. The publisher can refuse a publication or exceed its deadline. Evidence documents come from external producers via proposal requests.

### Findings
No finding.

The Host requires a signing key and a store with commit hooks before publishing. The publisher executes under a deadline (default 10 seconds). If the publisher exceeds the timeout or returns an error, the Host aborts the commit and stores nothing. Panics inside the publisher are caught and abort the transaction. For evidence admission, `propose` enforces fixed depth (32) and size (256 KiB) limits on documents. `admitEvidence` strictly verifies schema conformance, integer domains, unit markers, span ordering, review statuses, and fact consistency. Content proposals enforce evidence admission at propose, accept, and activate stages.

## Findings

| ID | Title | Severity | File:Line | Reachable |
|---|---|---|---|---|
| F1 | Lock file owner check in Listen uses real UID instead of effective UID | P3 | [src/lifecycle-core/protocol/socket.go:49](https://github.com/mohanraj00/entelos/blob/5d65d2362312707149041c85cd15324324987667/src/lifecycle-core/protocol/socket.go#L49) | Yes |

## Limits

The review did not cover live network execution with an external TLS proxy. The review did not execute dynamic fuzzing or live concurrency load tests. The review did not cover external CEL rule implementations or third-party host adapters outside this repository.

Verdict: P0 0, P1 0, P2 0, P3 1
