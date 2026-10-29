# Support scope

I maintain this as a source reference, not a stable SDK, hosted product or package-registry release. There is no support SLA or promised upgrade schedule.

The supported work is reproducing the supplied examples, fixing defects in documented contract/lifecycle behavior and reviewing dependency-pin updates with the same checks. Report a small neutral reproducer, source revision, tool versions and expected/actual behavior. Real customer data, credentials and confidential source material do not belong in reports.

## Compatibility and retention

The reviewed local run uses Go 1.27.2, Python 3.14.6 and Git 2.54.0 on macOS arm64. The `Full suite` GitHub Actions workflow runs the same verifier on the `ubuntu-24.04` runner (x86_64) with Go 1.27.2, Python 3.14.6 and the runner's Git (2.56.0 in the first run). Other toolchain/OS combinations have not been tested. The concurrency test requires POSIX fork; Windows is outside this reference's tested scope. The runner is not a cross-platform support claim.

CEL Go is pinned to v0.32.0 and the author CLI source to commit 5eac36c53cd8971fa1e5548374238183e0d8b302. The module lockfiles select the dependency closure. A pin or runtime-source change requires new build identity, fresh qualification and the relevant checks. Keep the old build/inputs/artifacts for exact replay. The Git store does not archive executable toolchains for you.

APIs, JSON records and the local storage format may change before a release contract is chosen. Do not assume new code can load or exactly replay old stores. Migration and compatibility replay need their own explicit design and tests. Content hashes are not signatures or source authority.

## Operating limits

The implementation is for modest local histories and trusted host adapters. Each receipt commits through the same root. Successful-request recovery searches first-parent history. Neither operation is a high-volume service design. Backups, retention, filesystem durability, sensitive-data access and cleanup of unreachable objects are operator responsibilities.

Fault injection checks process-level publication and lost replies. It does not establish power-loss durability, consensus across clones or production DB portability. Worker state is a preparation acknowledgement, not live deployment health. Tasks have manual claims/checkpoints/fences, without lease expiry or a scheduler. A full numeric allowance model, structured wire types, custom capabilities, a production DB, remote synchronization and automatic learning are outside the maintained example scope.

For a contribution, keep it tied to a concrete workflow and state what is tested. An adapter is not supported merely because it implements a similarly named method. New guarantees require independent expectations and relevant conformance checks.
