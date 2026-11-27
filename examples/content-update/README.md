# Content update

This example runs a synthetic product FAQ through the real Go lifecycle Host.
It uses an in-memory store and a runner that the application supplies.
The Go module uses the standard library and the local lifecycle-core module.
Use Go 1.27.2 or later. From the repository root, run:

```sh
go -C examples/content-update run .
```

The reviewer installs plan cases from [expected.json](expected.json).
Each case gives an item identifier, SHA-256 digest and size.
The example then proposes [manifest.json](manifest.json), which lists two FAQ
files by digest and size. It first qualifies [refused/](refused/). The charging
file has the same size but a different digest. Qualification fails, and the
Host refuses approval. It then qualifies [items/](items/) against the same plan
and manifest. The example reads the failed report from the Host to check that
the store still has it. A separate reviewer accepts the passed report and
activates the manifest for November 2026. The last step selects the active
candidate at 2026-11-15. It checks each file against that manifest before it
prints the item text.

[runner.go](runner.go) reads the installed plan cases from `QualificationInput.Cases`.
An item passes only if its bytes agree with both its plan case and its manifest
entry. An item with no plan case fails. A plan case with no manifest item also
fails, so a candidate cannot drop an item. In this example, expected.json simulates
a plan that the reviewer owns. It is in the same folder and commit as the
candidate, so it does not prove independence. In a real deployment, the plan
must come from a source outside the candidate. The runner identity binds its source and Go
version. The program embeds the fixtures so each run uses the same bytes.

Run the workflow, plan mismatch and item read tests with:

```sh
go -C examples/content-update test ./...
```

Limits: The store disappears when the process ends. The actors are trusted
Host contexts, with no login flow or signing key. The runner checks file identity
and size, not the meaning of the text. The application owns item storage and
delivery. A dated content read selects a manifest and writes no answer receipt.
The shipped commands use `CELRunner`, which refuses content candidates.
Protocol 1.3 accepts `propose.candidate` on the wire, but the sidecar cannot
qualify a content candidate with `CELRunner`. See
[CONTENT.md](../../src/protocol/content/CONTENT.md).
