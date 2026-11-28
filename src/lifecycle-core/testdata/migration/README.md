# Migration and compatibility fixtures

## Store of v0.3.0

`v030-store.json` is a synthetic store that the v0.3.0 Host made. `migrate_test.go` loads it into a new bare Git repository and checks each blob against its Git object ID.

The store has two proposals in the scope `demo`:

- `record.visible`: qualified, accepted and activated.
- `record.archived`: qualified, with no verdict.

`refs` in the file names the snapshot, report, decision and event references of each proposal.

### How the fixture was made

`make_fixture.sh` takes the source tree of the tag `v0.3.0` from Git, copies `make_v030_store_test.go` into it, and runs that test there. The test uses the v0.3.0 Host, a Git store, a fake describe engine and the fixture runner of the v0.3.0 tests. Then it exports the first-parent history with Git plumbing. The current code does not write any part of the fixture.

```sh
sh src/lifecycle-core/testdata/migration/make_fixture.sh <new directory outside the checkout>
```

Run it from the repository root. The go tool ignores this directory, so `make_v030_store_test.go` does not run with the current code.

Two runs on 2026-11-13 with Go 1.27.2 gave the same bytes. The reports hold the v0.3.0 lifecycle build, which depends on the Go and Git versions. Thus another toolchain can give other bytes. The tests do not depend on that value.

## Store of v0.8.0

`v080-store.json` is a synthetic store that the v0.8.0 Host made. `v080-wire.json` has the answers of the v0.8.0 adapter on that store. They check the [compatibility promise for 1.x releases](../../../../docs/STABILITY.md#compatibility-promise-for-1x-releases). The tests are in `compat_v080_test.go`, `protocol/compat_v080_test.go` and `cmd/entelos-verify/compat_v080_test.go`.

The store has three proposals in the scope `demo`:

- `record.visible`: qualified, accepted and activated. It has one answer and one replay of that answer.
- `record.hidden`: qualified, accepted and activated, then suspended.
- `record.archived`: qualified, with no verdict. This is the open proposal.

The Host had a signing key. Thus the store has a signed record for each decision and each event, two activation records and one suspension record.

`v080-store.json` has these fields:

| Field | Contents |
|---|---|
| `made_by` | The tag, the commit, the Go version and the Git version of the run. |
| `builds` | The lifecycle build of v0.8.0, and the builds of the test engine and the test runner. |
| `clock` | The clock value of each write. |
| `keys` | The public key set of the signer, in the format `entelos.keyset/1`. The file has no private key. |
| `refs` | The references of the sources, plans, snapshots, bundles, reports, decisions, events and receipts, by name. |
| `layout` | The refs of the repository and the name of the update marker. |
| `signed` | Each signed record: the reference of its record, its record kind and the document. The document is the file contents for `entelos-verify`. |
| `reads` | 30 reads: the actor, the role and the request, with the response or the fault record of the v0.8.0 Host. |
| `commits` | The first-parent history. Each entry has the commit ID, the raw commit object and the tree. |
| `blobs` | Each blob of the trees, by its Git object ID. |

A test loads the blobs, the trees and the raw commits into a new bare repository. Each object must get the Git object ID that the file states. Thus the loaded store has the cursors of the v0.8.0 store.

`v080-wire.json` has one entry for each protocol version from 1.0 to 1.3. Each entry has the same 17 steps: the request, the response and the close flag. The first 12 steps are reads on the state of the store fixture. Then one `decide` has a stale revision, one `decide` writes a verdict on the open proposal, and one repeats that request. The last two steps read the list and the proposal after the verdict. A commit has the time of its run. Thus the commit ID of the verdict is different in each run, and the test accepts one new commit ID at these places. The field `context` is the Host context, and `clock` is the clock value.

### How the fixtures were made

`make_v080_fixture.sh` takes the source tree of the tag `v0.8.0` from Git. It copies `make_v080_store_test.go` and `make_v080_wire_test.go` into that tree and runs them there. The first test uses the v0.8.0 Host with a Git store, a test engine and the fixture runner of the v0.8.0 tests. The second test uses the v0.8.0 protocol adapter. The current code does not write any part of the fixtures.

```sh
sh src/lifecycle-core/testdata/migration/make_v080_fixture.sh <new directory outside the checkout>
```

Run it from the repository root. The run of 2026-11-27 used Go 1.27.2 and Git 2.54.0. A new run gives other bytes, because the signing key is new and each commit has the time of the run. Do not make the fixtures again without a reason: a new fixture is a new expectation.

### Use by a consumer

To test a consumer on these vectors, load `v080-store.json` as the tests do, or verify each `signed` document with the `keys` document:

```sh
entelos-verify --keys <keys.json> <signed.json>
```

The test engine gives `true` for each execution, and the runner is a test fixture. Thus the fixtures show the records, the reads, the verdict write and the signatures. They do not show a CEL result.
