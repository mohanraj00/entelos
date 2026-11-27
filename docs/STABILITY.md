# API stability

This statement tells a caller which surfaces of entelos are stable, which surfaces can change, and how a change is announced. Issue [#147](https://github.com/mohanraj00/entelos/issues/147) owns it. It applies from the first tag that contains this file. Earlier tags, up to v0.3.0, promise no stable surface.

The statement promises only what the stability pin checks. The pin is `tools/stability/check_stability.py`, and `make check` runs it. A surface that the pin does not check is not stable, even if this file does not name it.

## Classes

- **Stable**: the pin checks the surface. A 0.x minor release changes it only as the [compatibility promise](#compatibility-promise-for-0x-releases) permits.
- **Experimental**: the surface can change or go away in any release, without notice and without a deprecation period.
- **Deprecated**: a stable surface that a later release will remove. The [deprecation path](#deprecation-path) gives the notice period.

## Stable surfaces

### Go packages

These are the importable packages:

| Import path | Package name |
|---|---|
| `github.com/mohanraj00/entelos/src/lifecycle-core` | `lifecycle` |
| `github.com/mohanraj00/entelos/src/lifecycle-core/protocol` | `protocol` |
| `github.com/mohanraj00/entelos/src/cel-handoff` | `handoff` |

[tools/stability/api.txt](../tools/stability/api.txt) lists each exported declaration of these packages, one on each line. The first word of each line is its class. Each line that starts with `stable` is a stable surface. A line has the name, the kind, the signature of a function or a method, the exported fields of a struct with their tags, the methods of an interface and the value of a constant.

The pin reads the Go source with the Go parser. It does not read doc comments, unexported names, function bodies or the values of variables. Thus the pin checks the shape of the API, not its behavior. The `go test` step of `make verify` checks the behavior against the pinned vectors below.

### Wire protocol

The sidecar wire protocol has the versions 1.0 and 1.3 ([PROTOCOL.md](../src/protocol/PROTOCOL.md)) and approval versions 1.1 and 1.2 ([APPROVAL.md](../src/protocol/approval/APPROVAL.md)) on the Unix domain socket. These files are the stable surface:

- `src/protocol/schema/*.json`: the envelope and the operation schemas.
- `src/protocol/vectors/*.json`: the session, RFC 8785 and envelope vectors.
- `src/protocol/approval/schema/*.json` and `src/protocol/approval/vectors/*.json`: the 1.1 approval schemas and the approval vectors.
- `src/protocol/approval/schema/1.2/*.json`: the 1.2 approval schema overrides.
- `src/protocol/approval/schema/1.3/*.json`: the 1.3 schema overrides of the content workflow (#213).

Version 1.2 is first in the v0.5.0 snapshot. Its schemas include the read `decision_events` and the field `decide.fact_reviews` (#158), so the pin checks them.

In the v0.6.0 snapshot, the read `decision_events` of version 1.2 gives a page with a jump in the positions for a verdict that cites facts and has no per-fact marks (#197), where v0.5.0 refused the page. The schemas, the cursor and the event format do not change. A reader that checks for contiguous positions must accept a jump.

Version 1.3 is first in the v0.8.0 snapshot (#213). Its schemas add `propose.candidate`, `activate.applicability`, the content fields of the `proposal` and `resolve` results, and the reads `evidence`, `promotion` and `approval`. The pin checks them, and the session vectors in `src/protocol/vectors/session-content.json`. The formats inside these fields stay experimental, as the list below says. The schemas pin only their shape on the wire.

The pin records the SHA-256 of each of these files. An added, removed or changed file fails the pin. The pin also checks that the envelope schema accepts exactly the protocol versions `1.0`, `1.1`, `1.2` and `1.3`, and that the Go constant `protocol.Version` is `1.0`.

### Record format

Stored records have the format `entelos.record/1`. The pin checks that the Go constant `lifecycle.RecordFormat` is `entelos.record/1`. It records the SHA-256 of each storage port vector in `src/cel-lifecycle/conformance/storage/` and each ingest vector in `src/cel-lifecycle/conformance/ingest/`. These vectors include the `record-format` and `content-identity` vectors. The reviewer credentials file has the format `entelos.credentials/1`, and the pin checks the constant `protocol.CredentialsFormat`.

## Experimental surfaces

These surfaces are experimental:

- Each line of `api.txt` that starts with `experimental`. These include the test hooks `FileClock`, `GitStore.Git`, `GitStore.RunGit` and `GitResult`, the package variables `FileNames`, `Mimes` and `Reads`, `handoff.Variable`, which has the typed profile fields, and the HTTP binding declarations.
- Each new exported declaration. It starts as experimental. A later change can promote it to stable.
- The field `Verdict.Measure` of the stable type `Verdict` in `lifecycle` (#152). Also the rest of [PROMOTION.md](../src/protocol/promotion/PROMOTION.md): the promotion read, the report field `measure`, the measure summary `entelos.measure/1` with its schema and vectors in `src/protocol/promotion/`, and the Host context field `promotion_collections`. The pin has one line for each struct, so the `Verdict` line is `stable`. This entry makes the new field experimental.
- The typed profile `cel-typed/v1` and its vectors in `src/cel-lifecycle/conformance/typed/`.
- The executor vectors in `src/cel-lifecycle/conformance/executor/`, the browser executor module and the WASM bridge. The edge executor line (the WASM build, the browser bridge, the IndexedDB log, `ingest` of executor logs and the planned gomobile package) is frozen until v1.0 ([#162](https://github.com/mohanraj00/entelos/issues/162)). It is not a stable surface. The names stay, and nothing is removed.
- The commands and their flags: `cel-handoff`, the `lifecycle-core` JSONL host, `entelos-sidecar`, `entelos-migrate`, `entelos-verify`, `entelos-keygen` and `entelos-unlock`.
- Supersession ([SUPERSESSION.md](../src/protocol/supersession/SUPERSESSION.md), [#129](https://github.com/mohanraj00/entelos/issues/129)): the `supersedes` field of `activate` and of the event record, the code `activation_superseded`, and the vectors in `src/protocol/supersession/`.
- Signed records ([SIGNED.md](../src/protocol/signed/SIGNED.md), [#155](https://github.com/mohanraj00/entelos/issues/155)): the signed record of the kind `signed`, the signature domain `entelos.signature/1`, the key set format `entelos.keyset/1`, the signing key file format `entelos.signing-key/1`, the state field `signatures`, the schemas and vectors in `src/protocol/signed/`, and the declarations `Signer`, `KeySet`, `ParseKeySet`, `VerifySigned`, `NewSigner`, `NewSigningKey` and `Host.SetSigner` in `lifecycle`.
- The Python callers in `src/lifecycle-core/client/`, the tools, `verify.py` and the examples.
- The text of diagnostic messages, and each diagnostic code that no pinned vector contains.
- The content proposal `entelos.content/1` ([CONTENT.md](../src/protocol/content/CONTENT.md), [#153](https://github.com/mohanraj00/entelos/issues/153)) with its schemas and vectors in `src/protocol/content/`. The Go Host reads it through the Host field `candidate` of `propose` (#175). The item field `applicability` and the field `applicability` of `activate` (#106) are also experimental. Protocol 1.3 accepts the two fields on the wire (#213). Its schemas are stable, and the manifest format and the meaning of the values stay experimental.
- The policy verdict `entelos.policy/1` ([POLICY.md](../src/protocol/policy/POLICY.md), [#154](https://github.com/mohanraj00/entelos/issues/154)) with its schemas and vectors in `src/protocol/policy/`. The Go Host implements it (#210): the Host-only write `policy_decide`, the Host-only read `policy_replay`, the record kind `policy_verdict`, the decision record field `policy_verdict_ref`, the report field `change_class`, the Host context fields `policy_functions` and `requirements`, and the code `human_verdict_required`. The field `Verdict.ChangeClass` of the stable type `Verdict` is also experimental, as `Verdict.Measure` is.
- The HTTP binding of the approval API ([APPROVAL.md](../src/protocol/approval/APPROVAL.md#http-binding), [#156](https://github.com/mohanraj00/entelos/issues/156)): the `HTTP*` declarations, `ListenHTTP`, `HTTPPath`, `IssuerFormat` and `MaxClaim` in `protocol`, the path `/v1/request` and the issuer file format `entelos.issuer/1`.
- The evidence format `entelos.evidence/1` ([#148](https://github.com/mohanraj00/entelos/issues/148)), and the rules that the read `evidence` applies (#104). Protocol 1.3 has the read on the wire (#213), and its schema is stable.
- The activation record ([ACTIVATION.md](../src/protocol/activation/ACTIVATION.md), [#105](https://github.com/mohanraj00/entelos/issues/105)): the record kind `activation`, the state field `activations`, the codes `publish_refused`, `publish_unsigned` and `publish_unsupported`, and the declarations `VerifyActivation`, `Publisher` and `Host.SetPublisher` in `lifecycle`. The suspension record of the same spec (#211) is also experimental: the record kind `suspension`, the state field `suspensions` and the declaration `VerifySuspension`. So are the publish time limit and the lock recovery (#212): the codes `publish_timeout` and `update_in_progress`, the declarations `Host.SetPublishTimeout`, `DefaultPublishTimeout` and `GitStore.RecoverRootLock`, and the marker file `lifecycle-root-update.flock`.
- The verdict binding and the verdict kinds ([VERDICT.md](../src/protocol/verdict/VERDICT.md), [KINDS.md](../src/protocol/verdict/KINDS.md), #127 and #128): the rules of the read `approval` (its 1.3 schema is stable), the Host context fields `actor_kinds` and `verdict_policy`, and the decision record field `kind`. No pinned vector has the codes `stale_verdict` and `verdicts_required`, so the diagnostic code rule above makes them experimental.
- The decision event export ([EVENTS.md](../src/protocol/events/EVENTS.md), #158): the state field `decision_stream`, the cursor contents and the declarations `CheckDecisionEvent` and `Host.StartDecisionEvents` in `lifecycle`. The 1.2 schemas of the read are stable, as the wire protocol section says.
- The step outcome contract ([STEP.md](../src/lifecycle-core/STEP.md), #113): the declarations `StepRequest`, `StepOutcome`, `StepResult`, `StepRunner`, `ByteDigest`, `StepRequestRef`, `CheckStepResult`, `StepDecisionInputs` and the three status constants in `lifecycle`, the names of the decision inputs and the fault codes. The adapter in `examples/agent-step-koodam` is an example, and it has no promise.

The pin does not read the prose of PROTOCOL.md, APPROVAL.md, the storage port spec or the other docs.

## Compatibility promise for 0.x releases

A release is a tag that the owner creates. In a 0.x minor release (for example v0.4.0 to v0.5.0) or a patch release (for example v0.4.0 to v0.4.1), a stable surface can change only in these ways:

- Add an exported declaration. It starts as experimental.
- Promote an experimental declaration to stable.
- Add a protocol minor version, for example 1.2, with its own schemas and vectors. The earlier versions stay.
- Add a vector file to a pinned set.
- Mark a stable declaration as deprecated.
- Remove a declaration that was deprecated for the full notice period.

Only cases that use the first unsupported minor version can move. They can move only when that minor version becomes supported. This rule applies only to these files:

- `src/protocol/vectors/envelope-schema.json`
- `src/protocol/vectors/session-envelope.json`
- `src/protocol/approval/vectors/approval-version.json`

Each moved case must keep a refusal case at the new first unsupported minor version. Thus no refusal coverage is lost. This change is level 1 and needs owner approval. In all other cases, the vectors of an existing version must not change which messages are valid. Their results must not change.

A 0.x release does not do these things to a stable surface:

- Remove a stable declaration, or change its signature.
- Remove, rename or change the type or the tag of an exported struct field of a stable type.
- Change the value of a pinned constant.
- Change the schemas or the vectors of an existing protocol version, in a way that changes which messages are valid or what the results are.
- Change the record major version, or stop the read of `entelos.record/1`.

The pin does not decide if a change is compatible. It finds each change of a pinned surface. Then the change is a planned change only if the same PR updates the pins and follows these rules. The pins and this file are level 1 paths in `.github/gates.json`, so the owner merges each change to them.

A 1.0 release will replace this promise with a semantic version promise. Record readability and execution compatibility stay separate, as [SUPPORT.md](SUPPORT.md#compatibility-and-retention) says.

## Deprecation path

1. Change the first word of the line in `api.txt` from `stable` to `deprecated`.
2. Add a Go doc comment that starts with `Deprecated:` and names the replacement.
3. Add a CHANGELOG entry under `### Deprecated` in the release section. The entry has the form ``- Deprecated: `lifecycle-core.Host.As`. Use `Host.HandleAs`. Removal: not before v0.6.0.`` The name is the last element of the import path, then the receiver type for a method, then the declaration name.
4. Keep the declaration for the notice period. If v0.y.0 deprecates a declaration, it stays in each v0.y patch release and in v0.(y+1).0. The earliest removal is v0.(y+2).0.

The pin fails if a `deprecated` line has no `- Deprecated:` line with its name in `CHANGELOG.md`. `--write` refuses to remove a `stable` line, so a stable declaration goes through `deprecated` first. For a protocol version, a vector file or the record format, step 3 names the version or the file in place of a Go name.

## Update the pins

For a planned change:

1. Make the change.
2. Run `python3 -B tools/stability/check_stability.py --write`. It records the current API and the file hashes. A new declaration gets the class `experimental`.
3. Edit the classes in `api.txt` if necessary.
4. Review the diff of `tools/stability/`, add the CHANGELOG entry, and run `make check`.

The pin needs Go on the `PATH`, because it reads the source with `go run tools/stability/apidump.go`. The tool uses only the Go standard library.

## Limits

- The pin checks shapes and bytes. It does not prove compatibility, and it does not check behavior. The tests of `make verify` check behavior against the pinned vectors.
- The pin does not check the notice period or the release history. The owner review checks them.
- A content hash in `pins.json` identifies bytes. It is not a signature, and it is not an approval.
