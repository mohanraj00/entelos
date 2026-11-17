# Content update

This example runs a synthetic product FAQ through the real Go lifecycle Host.
It uses an in-memory store and a runner that the application supplies.
The Go module uses the standard library and the local lifecycle-core module.
Use Go 1.27.2 or later and Git. From the repository root, run:

```sh
go -C examples/content-update run .
```

The example proposes [manifest.json](manifest.json), which lists two FAQ files
by SHA-256 digest and size. It first qualifies [refused/](refused/). The charging
file has the same size but a different digest. Qualification fails, and the
Host refuses approval. It then qualifies [items/](items/) against the same
manifest. A separate reviewer accepts it and activates it for November 2026.
The last step selects the active candidate at 2026-11-15 and prints its text.

[runner.go](runner.go) checks each item file against the manifest.
Its identity binds its source and Go version. The manifest holds fixed expected
digests and sizes. The example never replaces them with the files under test.
The program embeds these files so each run uses the same bytes.

Run the example and the digest, size and missing-file tests with:

```sh
go -C examples/content-update test ./...
```

Limits: The store disappears when the process ends. The actors are trusted
Host contexts, with no login flow or signing key. The runner checks file identity
and size, not the meaning of the text. The application owns item storage and
delivery. A dated content read selects a manifest and writes no answer receipt.
The shipped commands use `CELRunner`, which refuses content candidates.
The wire protocol does not accept `propose.candidate` yet. See
[CONTENT.md](../../src/protocol/content/CONTENT.md).
