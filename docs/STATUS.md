# Current work

## Name

The project name is Entelos, with the tagline "Entelos: grow toward the goal." On 2026-10-29, the GitHub repository changed from `mohanraj00/versioned-decision-reference` to `mohanraj00/entelos`. GitHub redirects the old URL. The Go module path and the NOTICE attribution are unchanged. On 2026-10-29, `entelos` was free on PyPI, npm and crates.io, and the .dev and .io domains were free. The GitHub user and the .com, .ai and .org domains were taken. No trademark database was searched.

## Source reference

The source reference contains a primitive typed CEL callable, a local qualified Git lifecycle, record-filter and PR CI examples, and an ordinary-function comparison. APIs and record formats are unstable. See [SUPPORT.md](SUPPORT.md) for the tested environment and operating limits.

## Licence

Project source uses [Apache-2.0](../LICENSE); [NOTICE](../NOTICE) retains project attribution. The [licence decision](LICENSING.md) records the maintainer choice and earlier MIT publication. Runtime/example/dependency bytes are unchanged by that change.

## Shared workflow

Purpose is in [CONSTITUTION.md](../CONSTITUTION.md), workflow in [AGENTS.md](../AGENTS.md). Codex (`AGENTS.md`), Claude Code (`CLAUDE.md`) and agy (`GEMINI.md`) share those files and the same skill source. A different harness reviews each change; see [REVIEW.md](REVIEW.md). `tools/check_project.py` checks that layout, links and source checksums.

The `CI` workflow checks the project structure and source inventory on every change. For code changes only, its `standalone` job runs `verify.py` on one Linux runner and uploads the compact results and command logs for 14 days. See [SUPPORT.md](SUPPORT.md) for the observed toolchain. One passing runner does not establish broad platform support.

## Layout

On 2026-10-29, the code moved to `src/` and `examples/`, the boundary test moved to `tests/`, and the loose reference docs moved to `docs/` and `licenses/`. `GEMINI.md` replaced `Agent.md`. Behavior did not change; the full suite must pass on the new paths.

## Next work

Use [BACKLOG.md](BACKLOG.md) and the linked GitHub issues. On 2026-10-30, the M1 work was complete in `main` (merge commit `e844246`):

- [WALKTHROUGH.md](WALKTHROUGH.md) gives the typed caller and the correction (issue #3).
- [CORRECTION.md](CORRECTION.md) explains a fact correction and a rule restatement (issue #4).
- `examples/quickstart/demo.py` runs the answer, correction, replay and restatement in one command. `verify.py` compares its results with the fixture and checks the failure evidence (issues #15 and #27).
- The README states the problem first and shows the [decision path diagram](diagrams/decision-path.svg) (issues #16 and #17).
- [FIRST_CONTACT.md](FIRST_CONTACT.md) and the first-contact issue form are the kit for the exit test (issues #18 and #25).

The M1 exit test has not run. The maintainer runs the first-contact sessions and records the results on [issue #10](https://github.com/mohanraj00/entelos/issues/10). M2 starts from the first three questions of those engineers. Keep caller walkthrough and input-correction documentation bounded to the existing examples. Numeric/structured profiles, production DBs, authentication gateways and resident-worker coordination are outside the current implemented scope.

## Handoff rule

Record the selected issue, final commit, behavior/checks and unresolved limits. A future worker must be able to identify the next task without reconstructing a conversation. Preserve meaningful failures and mark unmeasured claims as unmeasured.
