# Migration fixture

`v030-store.json` is a synthetic store that the v0.3.0 Host made. `migrate_test.go` loads it into a new bare Git repository and checks each blob against its Git object ID.

The store has two proposals in the scope `demo`:

- `record.visible`: qualified, accepted and activated.
- `record.archived`: qualified, with no verdict.

`refs` in the file names the snapshot, report, decision and event references of each proposal.

## How the fixture was made

`make_fixture.sh` takes the source tree of the tag `v0.3.0` from Git, copies `make_v030_store_test.go` into it, and runs that test there. The test uses the v0.3.0 Host, a Git store, a fake describe engine and the fixture runner of the v0.3.0 tests. Then it exports the first-parent history with Git plumbing. The current code does not write any part of the fixture.

```sh
sh src/lifecycle-core/testdata/migration/make_fixture.sh <new directory outside the checkout>
```

Run it from the repository root. The go tool ignores this directory, so `make_v030_store_test.go` does not run with the current code.

Two runs on 2026-11-13 with Go 1.27.2 gave the same bytes. The reports hold the v0.3.0 lifecycle build, which depends on the Go and Git versions. Thus another toolchain can give other bytes. The tests do not depend on that value.
