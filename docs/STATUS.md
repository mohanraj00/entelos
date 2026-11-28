# Current work

This page describes the state after the v1.0.0 release.

## Current release

The current release is `v1.0.0`, with the Go module tag `src/lifecycle-core/v1.0.0`. The [CHANGELOG](../CHANGELOG.md) gives the changes, the upgrade steps and the limits of each release.

[STABILITY.md](STABILITY.md) names the stable surfaces and the experimental surfaces of this release. [SUPPORT.md](SUPPORT.md) gives the tested environment and the operating limits.

## Current work

The current work is the v1.1 release ([milestone v1.1](https://github.com/mohanraj00/entelos/milestone/10)). The open issues of the milestone own the live status. [BACKLOG.md](BACKLOG.md) lists them.

## How to work here

[CONSTITUTION.md](../CONSTITUTION.md) gives the purpose and the rules. [AGENTS.md](../AGENTS.md) gives the layout, the check command and the review rules. [REVIEW.md](REVIEW.md) gives the review requests. Codex, Claude Code and agy share these files and the same skill source.

The `CI` workflow checks the project structure and the source inventory on each change. For code changes, its `standalone` job runs `verify.py` on `ubuntu-24.04` and `macos-15`. Two passing runners do not show broad platform support.

## History

The [CHANGELOG](../CHANGELOG.md), the [decision records](decisions/) and the closed GitHub issues keep the history of each release. This page does not repeat it.

## Handoff rule

At the end of a task, record the issue, the final commit, the checks and the limits that stay. A future worker must find the next task without the conversation. Keep the failed runs, and mark each claim that no check measured.
