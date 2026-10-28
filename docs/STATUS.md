# Current work

The source reference contains a primitive typed CEL callable, a local qualified Git lifecycle, record-filter and PR CI examples, and an ordinary-function comparison. APIs and record formats are unstable. See [SUPPORT.md](../SUPPORT.md) for the tested environment and operating limits.

## Shared workflow

Purpose is in [CONSTITUTION.md](../CONSTITUTION.md), workflow in [AGENTS.md](../AGENTS.md). Codex, Claude Code and other agent entry points share those files and the same skill source. `tools/check_project.py` checks that layout, links and source checksums.

The source-check workflow checks this project structure and source inventory. The full runtime runner is `python3 -B verify.py --work "$TASK_WORK"`; broad platform/runtime CI support is not established by a layout check.

## Next work

Use [BACKLOG.md](BACKLOG.md) and the linked GitHub issues. Start with full-suite CI reproduction. Keep caller walkthrough and input-correction documentation bounded to the existing examples. Numeric/structured profiles, production DBs, authentication gateways and resident-worker coordination are outside the current implemented scope.

## Handoff rule

Record the selected issue, final commit, behavior/checks and unresolved limits. A future worker must be able to identify the next task without reconstructing a conversation. Preserve meaningful failures and mark unmeasured claims as unmeasured.
