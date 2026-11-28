# Support scope

I maintain this as a source reference and a Go dependency, not a hosted product or package-registry release. [STABILITY.md](STABILITY.md) names the stable surfaces. There is no support SLA or promised upgrade schedule.

The supported work is reproducing the supplied examples, fixing defects in documented contract/lifecycle behavior and reviewing dependency-pin updates with the same checks. Report a small neutral reproducer, source revision, tool versions and expected/actual behavior. Real customer data, credentials and confidential source material do not belong in reports.

## Edge executor freeze

On 2026-11-13, the maintainer froze the edge executor line until v1.0 ([#162](https://github.com/mohanraj00/entelos/issues/162)). The line has these parts: the WASM build of the executor, the browser bridge, the IndexedDB receipt log, `ingest` of executor logs, and the planned gomobile package.

- The code and its tests stay. I do not remove them.
- No new work starts on this line until v1.0. I do not add a feature, an example or a platform.
- I still fix a security defect in the frozen code. I also keep the tests in CI.
- The frozen line gets no support beyond this. Report a security defect through [SECURITY.md](../SECURITY.md).

## Compatibility and retention

The reviewed local run uses Go 1.27.2, Python 3.14.6 and Git 2.54.0 on macOS arm64. The `standalone` job of the `CI` GitHub Actions workflow runs the same verifier on the `ubuntu-24.04` runner (x86_64) with Go 1.27.2, Python 3.14.6 and the runner's Git (2.56.0 in the first run). The same job also runs `verify.py` on the `macos-15` runner (arm64) with the same Go and Python versions and the runner's Git. Other toolchain/OS combinations have not been tested. The concurrency test requires POSIX fork; Windows is outside this reference's tested scope. The browser checks need a Chromium browser. `ENTELOS_BROWSER` names it, and CI uses the Chrome of the runner image. The sidecar is POSIX only. These runners are not a cross-platform support claim.

CEL Go is pinned to v0.32.0 and the author CLI source to commit 5eac36c53cd8971fa1e5548374238183e0d8b302. The module lockfiles select the dependency closure. A pin or runtime-source change requires new build identity, fresh qualification and the relevant checks. Keep the old build/inputs/artifacts for exact replay. The Git store does not archive executable toolchains for you.

[STABILITY.md](STABILITY.md) is the compatibility statement for the Go packages, the wire protocol and the record format. It names the stable surfaces and the experimental surfaces, the promise for 0.x releases and the deprecation path. A surface that the stability pin does not check can change in any release. Content hashes are not signatures or source authority.

Each stored record has a `format` field. The current value is `entelos.record/1`. The number after the slash is the major version. A reader reads only the major version that it knows. If a record has no `format` field or an unknown major version, the read fails closed with the diagnostic `unsupported_record_format`. If the `format` value has a different shape, the read fails with `invalid_record`. The storage port spec and the `record-format` storage vector define this rule.

v0.2.0 and v0.3.0 use the same record major. Record readability and execution compatibility are separate. A new lifecycle or engine build needs fresh qualification and approval before activation and load. For exact replay of an earlier receipt, retain the lifecycle and engine builds that the receipt names, along with its inputs and bundle. A retry of a stored write requires current access to the affected functions.

Stores from v0.1.0 are not migrated. The v0.1.0 snapshot declared the records unstable, and its records have no `format` field. The current code does not read them; each read of such a record gives `unsupported_record_format`. There is no migration tool. To keep a v0.1.0 store readable, keep the v0.1.0 code and build with it. Compatibility replay across record formats needs its own explicit design and tests.

## Operating limits

The implementation is for modest local histories and trusted host adapters. Each receipt commits through the same root. Successful-request recovery searches first-parent history. Neither operation is a high-volume service design. Backups, retention, filesystem durability, sensitive-data access and cleanup of unreachable objects are operator responsibilities.

### Git store volume

I use three limits for one Git store: a store size of 1 GB, a write p95 of 1 second and a retry of the first request of 300 seconds. The supported volume is 1000 receipts for each store and one writer at about 1 write per second ([#164](https://github.com/mohanraj00/entelos/issues/164)). The limit comes from one run of `TestGitStoreVolume` in `src/lifecycle-core/volume_test.go` on commit `d7f63f3`. The run was on an Apple M4 with 10 cores, 24 GiB of memory and an APFS SSD, with macOS 27.0.1 and Go 1.27.2. Other work also ran on the machine. The 1-minute load average was 10.75 at the start and 8.74 at the end. `make check` does not run the test. It runs only when `ENTELOS_STORE_VOLUME` names an output directory.

| Receipts | Store size | `state.json` | Write p50 | Write p95 | `inspect` | `decision_events` | Retry of the first request |
|---|---|---|---|---|---|---|---|
| 100 | 2.3 MB | 0.2 MB | 0.39 s | 0.77 s | 0.10 s | 0.11 s | 7.9 s |
| 500 | 51 MB | 1.0 MB | 0.40 s | 0.56 s | 0.18 s | 0.16 s | 49 s |
| 1000 | 209 MB | 2.0 MB | 0.36 s | 0.61 s | 0.37 s | 0.36 s | 194 s |
| 2000 | 831 MB | 4.0 MB | 0.64 s | 1.27 s | 0.60 s | 0.41 s | not run |
| 4600 | 4.37 GB | 9.3 MB | 0.70 s | 0.95 s | 0.87 s | 0.45 s | not run |

Each write commits the full `state.json`, so the store size grows with the square of the receipt count. The retry of an earlier request walks the first-parent history, so its time grows with the history. The run skipped the retry after 1000 receipts, because it took more than 60 seconds. It stopped the receipt phase when the store passed 4 GiB.

At 4600 receipts, one writer got about 1.2 writes per second at each offered rate from 5 to 100 per second. A higher offered rate gives a backlog, not more writes. Concurrent writers give fewer writes: 2 writers got 0.75 writes per second, and 4 writers got 0.45 writes per second. Of the 1541 attempts of the 4 writers, 1141 got `stale_revision` and retried.

The retry limit is the first limit that a store passes. From 500 to 1000 receipts, the retry time grew from 49 s to 194 s, about 4 times. If it grows with the square of the receipt count, it passes 300 seconds at about 1250 receipts. That estimate is not a measurement. The store size passes 1 GB between 2000 and 4600 receipts, and the write p95 passed 1 second at 2000 receipts. The supported volume of 1000 receipts keeps a margin of about 20 percent below the first limit.

Above 1000 receipts, writes and reads still give correct results, but the store size and the retry time grow fast. Do these steps:

1. Start a new store for a new history, and keep the old store for replay.
2. Make retries only for recent requests, or accept a longer retry time.
3. For a larger history, use an adapter for a different store. A second store adapter is follow-up work ([#246](https://github.com/mohanraj00/entelos/issues/246)).

Fault injection checks process-level publication and lost replies. It does not establish power-loss durability, consensus across clones or production DB portability. Worker state is a preparation acknowledgement, not live deployment health. Tasks have manual claims/checkpoints/fences, without lease expiry or a scheduler. Activation expiry compares the invocation date with the activation interval. The Host reads no clock, and there is no clock synchronization policy beyond the local clock of the executor that sets the date. A full numeric allowance model, structured wire types, custom capabilities, a production DB, remote synchronization and automatic learning are outside the maintained example scope.

For a contribution, keep it tied to a concrete workflow and state what is tested. An adapter is not supported merely because it implements a similarly named method. New guarantees require independent expectations and relevant conformance checks.
