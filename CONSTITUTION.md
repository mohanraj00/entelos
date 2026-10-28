# Versioned decision reference: purpose

I want an authored decision, its approval and each recorded answer to remain tied to exact inputs and revisions. A correction must preserve what was answered earlier and make the new answer explicit.

This is a runnable source reference built on CEL and Git. Its users are engineers who embed changing rules in services and need a clear author/caller contract and decision history. The record-filter and PR CI examples keep that work concrete.

## Scope

Keep the complete path visible: source -> proposal -> qualification -> approval -> dated activation -> preparation -> execution -> feedback/correction -> replay or restatement.

Reuse existing language and storage tools. Applications supply source authority, rule/data content, independent expectations, admission/learning policy, authentication, retention and effects. A passing test does not grant serving authority. Content hashes are identities, not signatures.

The current callable profile is string/bool input and bool output. APIs and records are unstable. Source-reference support, compatibility and operating limits are defined in [SUPPORT.md](SUPPORT.md) and [SECURITY.md](SECURITY.md). A new profile or guarantee needs an explicit workflow, contract and relevant conformance checks.

## Evidence

Distinguish proposed, implemented, tested and deployed behavior. Preserve meaningful failures beside successful cases. Bind a check to the exact source/build/input and explain what it exercised. Compilation or agreement is narrower than correctness.

Compare competent existing tools and ordinary code. Concrete workflow value is sufficient; adoption commitments are not a prerequisite. Do not infer speed, hours saved, maintenance reduction or general superiority from test counts or code size. Run a benchmark only when it answers a named unresolved question.

Keep examples neutral and invented. Private application modules, domain packs, customer data, credentials and raw private evaluations do not belong in this repository. Use Apache-2.0 for project source. Preserve project copyright, licence and NOTICE attribution, plus dependency licences and notices.

This file owns purpose and boundaries. [AGENTS.md](AGENTS.md) owns workflow. Harness entry points must import those shared rules rather than copy them.
