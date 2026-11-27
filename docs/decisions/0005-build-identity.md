# 0005 Lifecycle build identity

- Status: accepted
- Date: 2026-11-27
- Decided by: the maintainer, 2026-11-27
- Issue: [#214](https://github.com/mohanraj00/entelos/issues/214)
- Spec: [RUNNER.md](../../src/lifecycle-core/RUNNER.md#lifecycle-build)

## Question

How can a Host use a qualified snapshot after an upgrade of Go or Git when the core source files are the same?

## Decision

1. **The lifecycle build identifies the core source files.** The Host uses the file names and exact bytes of the Go files in the root of `src/lifecycle-core`. It excludes test files.
2. **The receipt records the tool versions.** When the Host starts, it reads `runtime.Version()` and the output of `git --version`. Each new Host execution receipt gives these values in `host_tools.go` and `host_tools.git`. The Host does not compare these fields for admission, execution or replay.
3. **A report with the old build identity needs a new qualification.** The new Host reads the old report and refuses its use with `implementation_unavailable`. Qualify the snapshot on the new Host, get an accept decision, and activate it again. For a CEL bundle, load the new activation before invocation.

## Reasons

An upgrade of a Host tool does not change the core source files. The source reference lets a report remain valid after that upgrade. The receipt records the tool versions that the Host read at startup.

The old reference has one digest for the source files and both tool versions. The Host cannot check the source files alone from that digest. A new qualification binds the report to the new identity.

## Consequences

- If only the Go or Git version changes, qualification reports and accept decisions for this build identity remain valid. The Host does not qualify the snapshot again. To get a new qualification after a tool version changes, run `qualify` again.
- Each stored report and receipt keeps its bytes and its reference.
- Replay of a receipt with the old identity needs the original lifecycle build and its tool versions. Replay of a new receipt checks the core source reference and the pinned engine build.
- The qualification runner and the CEL engine keep their own build checks. If an upgrade also changes either of these identities, its build check still applies.
