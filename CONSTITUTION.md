# Entelos constitution

Version 1, 2026-11-09.

## Purpose

I want an authored decision, its approval and each recorded answer to remain tied to exact inputs and revisions. A correction must preserve what was answered earlier and make the new answer explicit.

This is a runnable source reference built on CEL and Git. Its users are engineers who embed changing rules in services and need a clear author/caller contract and decision history. The record-filter and PR CI examples keep that work concrete.

## Boundaries

Keep the complete path visible: source -> proposal -> qualification -> approval -> dated activation -> preparation -> execution -> feedback/correction -> replay or restatement.

Reuse existing language and storage tools. Applications supply source authority, rule/data content, independent expectations, admission/learning policy, authentication, retention and effects. A passing test does not grant serving authority. Content hashes are identities, not signatures.

The current callable profile is string/bool input and bool output. APIs and records are unstable. Source-reference support, compatibility and operating limits are defined in [SUPPORT.md](docs/SUPPORT.md) and [SECURITY.md](SECURITY.md). A new profile or guarantee needs an explicit workflow, contract and relevant conformance checks.

## Principles

Map the change to its question before adding machinery. Keep authoring and calling usable through explicit contracts and structured diagnostics. Separate domain projection/authority/policy from reusable mechanics. Use independently stated expectations; never label a candidate's own output as its oracle.

## Evidence

Distinguish proposed, implemented, tested and deployed behavior. Preserve meaningful failures beside successful cases. Bind a check to the exact source/build/input and explain what it exercised. Compilation or agreement is narrower than correctness.

Compare competent existing tools and ordinary code. Concrete workflow value is sufficient; adoption commitments are not a prerequisite. Do not infer speed, hours saved, maintenance reduction or general superiority from test counts or code size. Run a benchmark only when it answers a named unresolved question.

## Contributions

Keep examples neutral and invented. Private application modules, domain packs, customer data, credentials and raw private evaluations do not belong in this repository. Use Apache-2.0 for project source. Preserve project copyright, licence and NOTICE attribution, plus dependency licences and notices.

## Governance

<!-- governance:start v1 -->
This project follows the project standard in `docs/STANDARD.md`. That file is an exact copy from `mohanraj00/.github`. The standard owns the rules for builds, reviews, approvals and CI. A project exception to the standard is valid only when the standard records it.

The documents own these topics:

- `CONSTITUTION.md`: the purpose, the boundaries, the principles, the evidence rules, the contribution rules and the version log.
- `docs/STANDARD.md`: the builds, the reviews, the approvals and CI.
- `AGENTS.md`: the layout, the check command and the review guidelines.
- `CLAUDE.md` and `GEMINI.md`: imports only.

These rules apply to each change:

1. A harness does not review its own change. The Codex security review is the one exception, as the standard says.
2. The agents use the owner login. An agent never approves a PR and never merges a level 1 PR. An agent never adds the labels `accepted`, `security-reviewed`, `cross-reviewed` or `ready-for-review`.
3. The owner merges each level 1 PR.
4. Each tag, release, package publication, visibility change and outreach step needs a separate owner approval.
<!-- governance:end v1 -->

## Amendment

Change this constitution when the owner changes direction or evidence changes a decision. A change is a level 1 PR. Record the reason in the change, and add an entry to the version log.

Change the governance block only with a new version of the template in `mohanraj00/.github`.

## Version log

### Version 1, 2026-11-09

Align the constitution with template v1: rename the title and `## Scope`, move the design principles from `AGENTS.md`, move the contribution rules out of `## Evidence`, and add the governance block and the amendment text. Reason: `docs/STANDARD.md` requires the same sections and governance block in each constitution.
