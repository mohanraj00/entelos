# Tracked work

GitHub issues own live status. This map identifies bounded work for this source reference.

| Issue | Scope |
|---|---|
| [Switch project licensing to Apache-2.0](https://github.com/mohanraj00/entelos/issues/5) | Licence, attribution, documentation and verification; prior history and dependency terms retained. |
| [Bootstrap the runnable reference and shared agent workflow](https://github.com/mohanraj00/entelos/issues/1) | Initial publication, shared project workflow and verification. |
| [Run the full standalone reference suite in GitHub Actions](https://github.com/mohanraj00/entelos/issues/2) | Done: the `Full suite` workflow runs `verify.py` on `ubuntu-24.04`. |
| [Add a clean-run typed caller and correction walkthrough](https://github.com/mohanraj00/entelos/issues/3) | Next: existing neutral author-to-caller and correction/replay commands. |
| [Explain supplied-fact correction versus rule restatement](https://github.com/mohanraj00/entelos/issues/4) | Application input revisions versus pinned replay and rule restatement. |

## Milestones to a public announcement

On 2026-10-29, the maintainer chose to announce Entelos as a source reference to read and copy. It is not a library to depend on. A milestone is complete only when its exit test passes. The announcement itself needs specific maintainer authorization.

| Milestone | Work | Exit test |
|---|---|---|
| M0 Bootstrap | Done: runnable reference, two examples, full-suite CI, Apache-2.0, cross-harness review, rename. | Met on 2026-10-29. |
| [M1 First contact](https://github.com/mohanraj00/entelos/issues/10) | Issues 3 and 4. One demo command that shows an answer, a correction, a replay and a restatement. A README that states the problem first. One diagram of the complete path. | Two or three engineers who do not know the project run the demo from a clean clone without help. Each states the problem in one sentence. Their stop points are recorded. |
| [M2 Comparison](https://github.com/mohanraj00/entelos/issues/11) | Compare OPA/Rego, Cedar, GoRules ZEN, feature-flag services and ordinary code plus Git. State where each one is the better choice. | The comparison answers the first three questions from the M1 engineers. |
| [M3 Copy proof](https://github.com/mohanraj00/entelos/issues/12) | A third neutral example, written from the docs only, that copies or reuses `src/`. | The example passes `verify.py`. Each `src/` change that it needed is recorded as a finding. |
| [M4 Contributor path](https://github.com/mohanraj00/entelos/issues/13) | `CONTRIBUTING.md`, issue templates, three to five starter issues, a review path for a human contributor without an agent harness, CI on Linux and macOS. | One outside pull request merges through the documented path. |
| [M5 Snapshot and announcement](https://github.com/mohanraj00/entelos/issues/14) | A record format version field, a compatibility statement, a changelog, a `v0.1.0` tag, a security review of the approval and record path. A quiet share with five to ten engineers before the public post. | The tag passes on both operating systems. Known limits are at the top of the README. |

A reference to copy needs no package release. A tag gives a stable snapshot that a reader can cite and copy.

No issue here authorizes an unscoped model benchmark, a new domain policy, a public package release or production deployment. Choose the smallest change that completes the issue's stated workflow and preserve its failures/limits.
