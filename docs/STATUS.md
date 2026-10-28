# Current work

## Name

The project name is Entelos, with the tagline "Entelos: grow toward the goal." On 2026-10-29, the GitHub repository changed from `mohanraj00/versioned-decision-reference` to `mohanraj00/entelos`. GitHub redirects the old URL. The Go module path and the NOTICE attribution are unchanged. On 2026-10-29, `entelos` was free on PyPI, npm and crates.io, and the .dev and .io domains were free. The GitHub user and the .com, .ai and .org domains were taken. No trademark database was searched.

## Source reference

The source reference contains a primitive typed CEL callable, a local qualified Git lifecycle, record-filter and PR CI examples, and an ordinary-function comparison. APIs and record formats are unstable. See [SUPPORT.md](../SUPPORT.md) for the tested environment and operating limits.

## Licence

Project source uses [Apache-2.0](../LICENSE); [NOTICE](../NOTICE) retains project attribution. The [licence decision](LICENSING.md) records the maintainer choice and earlier MIT publication. Runtime/example/dependency bytes are unchanged by that change.

## Shared workflow

Purpose is in [CONSTITUTION.md](../CONSTITUTION.md), workflow in [AGENTS.md](../AGENTS.md). Codex, Claude Code and other agent entry points share those files and the same skill source. `tools/check_project.py` checks that layout, links and source checksums.

The source-check workflow checks this project structure and source inventory. The full runtime runner is `python3 -B verify.py --work "$TASK_WORK"`; broad platform/runtime CI support is not established by a layout check.

## Next work

Use [BACKLOG.md](BACKLOG.md) and the linked GitHub issues. Start with full-suite CI reproduction. Keep caller walkthrough and input-correction documentation bounded to the existing examples. Numeric/structured profiles, production DBs, authentication gateways and resident-worker coordination are outside the current implemented scope.

## Handoff rule

Record the selected issue, final commit, behavior/checks and unresolved limits. A future worker must be able to identify the next task without reconstructing a conversation. Preserve meaningful failures and mark unmeasured claims as unmeasured.
