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

Use [BACKLOG.md](BACKLOG.md) and the linked GitHub issues. The next milestone is [M3 Copy proof](https://github.com/mohanraj00/entelos/issues/12).

### M1 First contact

On 2026-10-30, the M1 work was complete in `main` (merge commit `e844246`):

- [WALKTHROUGH.md](WALKTHROUGH.md) gives the typed caller and the correction (issue #3).
- [CORRECTION.md](CORRECTION.md) explains a fact correction and a rule restatement (issue #4).
- `examples/quickstart/demo.py` runs the answer, correction, replay and restatement in one command. `verify.py` compares its results with the fixture and checks the failure evidence (issues #15 and #27).
- The README states the problem first and shows the [decision path diagram](diagrams/decision-path.svg) (issues #16 and #17).

On 2026-10-30, the maintainer dropped the observed first-contact sessions as the M1 exit test. The scope of that test was not clear, and most of it measured whether the docs were clear enough to run the demo. The maintainer ran the demo and found those problems in one session. The reason and the limits are on [issue #10](https://github.com/mohanraj00/entelos/issues/10). These changes replace the test:

- PR #45 (merge `0375158`, issue #44): a clear error and a README line for the work directory.
- PR #47 (merge `c3a2ece`, issue #46): a tools table, numbered steps, a demo output that explains each step, a Go and Git check before the build, and an "After the demo" table.
- A fresh-context adversarial critic read the new README. Its findings are on PR #47.

No engineer who does not know the project ran the demo. Nobody recorded whether the problem matters to an engineer. [FIRST_CONTACT.md](FIRST_CONTACT.md) and the first-contact issue form stay in the repo, but nobody used them.

### M2 Comparison

On 2026-10-30, the M2 work was complete in `main`:

- Five pages in [compare/](compare/): OPA/Rego, Cedar, GoRules ZEN, feature-flag services and ordinary code plus Git (issues #33 to #37). Each page states where the other tool is the better choice.
- [COMPARISON.md](COMPARISON.md) answers the three questions of the critic pass (PR #48, merge `49d5a7e`, issue #38).

The M2 exit test was "the first three questions from the M1 engineers". The three questions of the critic replace them, because the first-contact sessions did not run.

These parts of M2 were not run:

- The Cedar blocks in [compare/cedar.md](compare/cedar.md) are proposed text from the Cedar docs. Nobody parsed, validated or authorized them.
- Unleash, the GoRules BRMS and the OpenFeature SDK are described from their docs only.
- No call cost, speed or effort was measured.

These are the critic findings from PR #47. The README change for issues #50, #52 and #54 addresses three of them:

- An example of how a service calls a rule from its own code (issue #50).
- A command that opens a receipt from the demo.
- The first-screen order: the tagline and the Limits note come before the problem (issue #52).
- Progress output from `verify.py` during its run of about 10 minutes.
- Jargon in "What the examples show" (issue #54).

Keep caller walkthrough and input-correction documentation bounded to the existing examples. Numeric/structured profiles, production DBs, authentication gateways and resident-worker coordination are outside the current implemented scope.

## Handoff rule

Record the selected issue, final commit, behavior/checks and unresolved limits. A future worker must be able to identify the next task without reconstructing a conversation. Preserve meaningful failures and mark unmeasured claims as unmeasured.
