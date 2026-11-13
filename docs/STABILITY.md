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

The sidecar wire protocol has the versions 1.0 ([PROTOCOL.md](../src/protocol/PROTOCOL.md)) and 1.1 ([APPROVAL.md](../src/protocol/approval/APPROVAL.md)) on the Unix domain socket. These files are the stable surface:

- `src/protocol/schema/*.json`: the envelope and the operation schemas.
- `src/protocol/vectors/*.json`: the session, RFC 8785 and envelope vectors.
- `src/protocol/approval/schema/*.json` and `src/protocol/approval/vectors/*.json`: the 1.1 approval schemas and vectors.

The pin records the SHA-256 of each of these files. An added, removed or changed file fails the pin. The pin also checks that the envelope schema accepts exactly the protocol versions `1.0` and `1.1`, and that the Go constant `protocol.Version` is `1.0`.

### Record format

Stored records have the format `entelos.record/1`. The pin checks that the Go constant `lifecycle.RecordFormat` is `entelos.record/1`. It records the SHA-256 of each storage port vector in `src/cel-lifecycle/conformance/storage/` and each ingest vector in `src/cel-lifecycle/conformance/ingest/`. These vectors include the `record-format` and `content-identity` vectors. The reviewer credentials file has the format `entelos.credentials/1`, and the pin checks the constant `protocol.CredentialsFormat`.

## Experimental surfaces

These surfaces are experimental:

- Each line of `api.txt` that starts with `experimental`. Today these are the test hooks `FileClock`, `GitStore.Git`, `GitStore.RunGit` and `GitResult`, the package variables `FileNames`, `Mimes` and `Reads`, `handoff.Variable`, which has the typed profile fields, and the HTTP binding declarations.
- Each new exported declaration. It starts as experimental. A later change can promote it to stable.
- The typed profile `cel-typed/v1` and its vectors in `src/cel-lifecycle/conformance/typed/`.
- The executor vectors in `src/cel-lifecycle/conformance/executor/`, the browser executor module and the WASM bridge.
- The commands and their flags: `cel-handoff`, the `lifecycle-core` JSONL host and `entelos-sidecar`.
- The Python callers in `src/lifecycle-core/client/`, the tools, `verify.py` and the examples.
- The text of diagnostic messages, and each diagnostic code that no pinned vector contains.
- The content proposal `entelos.content/1` ([CONTENT.md](../src/protocol/content/CONTENT.md), [#153](https://github.com/mohanraj00/entelos/issues/153)) with its schemas and vectors in `src/protocol/content/`. It is a proposed spec, and no Host code implements it.
- The policy verdict `entelos.policy/1` ([POLICY.md](../src/protocol/policy/POLICY.md), [#154](https://github.com/mohanraj00/entelos/issues/154)) with its schemas and vectors in `src/protocol/policy/`. It is a proposed spec, and no Host code implements it.
- The HTTP binding of the approval API ([APPROVAL.md](../src/protocol/approval/APPROVAL.md#http-binding), [#156](https://github.com/mohanraj00/entelos/issues/156)): the `HTTP*` declarations, `ListenHTTP`, `HTTPPath`, `IssuerFormat` and `MaxClaim` in `protocol`, the path `/v1/request` and the issuer file format `entelos.issuer/1`.
- The evidence format `entelos.evidence/1` ([#148](https://github.com/mohanraj00/entelos/issues/148)). When this statement was written, it was an open PR, not merged code.

The pin does not read the prose of PROTOCOL.md, APPROVAL.md, the storage port spec or the other docs.

## Compatibility promise for 0.x releases

A release is a tag that the owner creates. In a 0.x minor release (for example v0.4.0 to v0.5.0) or a patch release (for example v0.4.0 to v0.4.1), a stable surface can change only in these ways:

- Add an exported declaration. It starts as experimental.
- Promote an experimental declaration to stable.
- Add a protocol minor version, for example 1.2, with its own schemas and vectors. The earlier versions stay.
- Add a vector file to a pinned set.
- Mark a stable declaration as deprecated.
- Remove a declaration that was deprecated for the full notice period.

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
