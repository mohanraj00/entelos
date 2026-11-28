# Current work

This page describes the state after the v0.8.0 release.

## Current release

The current release is `v0.8.0`, with the Go module tag `src/lifecycle-core/v0.8.0`. The [CHANGELOG](../CHANGELOG.md) gives the changes, the upgrade steps and the limits of each release.

[STABILITY.md](STABILITY.md) names the stable surfaces and the experimental surfaces of this release. [SUPPORT.md](SUPPORT.md) gives the tested environment and the operating limits.

## Current work

The current work is the v1.0 release ([milestone v1.0](https://github.com/mohanraj00/entelos/milestone/9)). The open issues of the milestone own the live status:

- [#234](https://github.com/mohanraj00/entelos/issues/234): the v1.0 compatibility statement and the migration from v0.8.
- [#235](https://github.com/mohanraj00/entelos/issues/235): simple, short and current docs for v1.0.
- [#236](https://github.com/mohanraj00/entelos/issues/236): a security review of the Go path and the sidecar on the v1.0 release candidate.

The owner creates the `v1.0.0` tag after these issues close. The tag needs a separate owner approval.

[BACKLOG.md](BACKLOG.md) lists the issues after v1.0.

## How to work here

[CONSTITUTION.md](../CONSTITUTION.md) gives the purpose and the rules. [AGENTS.md](../AGENTS.md) gives the layout, the check command and the review rules. [REVIEW.md](REVIEW.md) gives the review requests. Codex, Claude Code and agy share these files and the same skill source.

The `CI` workflow checks the project structure and the source inventory on each change. For code changes, its `standalone` job runs `verify.py` on `ubuntu-24.04` and `macos-15`. Two passing runners do not show broad platform support.

## History

The [CHANGELOG](../CHANGELOG.md), the [decision records](decisions/) and the closed GitHub issues keep the history of each release. This page does not repeat it.

## Handoff rule

At the end of a task, record the issue, the final commit, the checks and the limits that stay. A future worker must find the next task without the conversation. Keep the failed runs, and mark each claim that no check measured.
