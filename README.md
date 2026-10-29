<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/entelos-logo-dark.svg">
    <img alt="Entelos" src="docs/brand/entelos-logo-light.svg" width="360">
  </picture>
</p>

# Entelos

**Entelos: grow toward the goal.**

Entelos (from Greek *en* + *telos*, "the goal within") is the name of this versioned decision reference. The repository is `mohanraj00/entelos`; its earlier name was `versioned-decision-reference`. [Brand assets](docs/brand/README.md) holds the mark, the logo and the colours.

I built this reference to keep an authored decision, its approval, and its recorded answers tied to exact inputs and revisions. It shows how to correct a decision without rewriting the answer that was given earlier.

The reference combines a typed CEL callable with a local Git history. A second example models pull-request CI eligibility and compares CEL with ordinary typed functions. I reuse CEL and Git; I do not implement another expression language.

## Work in this project

Start with [CONSTITUTION.md](CONSTITUTION.md), [AGENTS.md](AGENTS.md) and [current work](docs/STATUS.md). Codex and Claude Code share those rules and the same skill source. [Tracked issues](docs/BACKLOG.md) define the bounded next work. [Contract map](docs/CONTRACTS.md) points to the executable interfaces.

## Run

Use Python, Go and Git. See [SUPPORT.md](SUPPORT.md) for the tested toolchain and limits. Choose a new scratch directory outside this checkout:

```sh
python3 -B verify.py --work "$TASK_WORK"
```

The runner fetches the pinned public CEL author tools, builds the host and executes the examples and tests. Network access is needed for public source/module downloads. No model account, database, Kubernetes cluster or private repository is needed. Builds, logs, bare Git stores and generated inputs stay under the supplied work directory. Do not use a directory containing real data.

If the author-tools checkout is already available at the documented revision, pass `--skills /path/to/skills`. The runner requires a clean checkout. It never pushes, creates a remote repository or publishes a package.

The work directory contains `results.json`, command logs and the example stores. To discover the CI function after a successful run:

```sh
printf '%s\n' '{"op":"describe","function":"ci.pr_should_run","date":"2026-11-15"}' | python3 -B cel-lifecycle/cli.py --repo "$TASK_WORK/reuse/cel/state.git" --context "$TASK_WORK/reuse/cel/caller.json" --runtime "$TASK_WORK/bin/cel-handoff" --work "$TASK_WORK/reuse/cel"
```

`cel-lifecycle/client.py` uses that discovery result, ordinary inputs and a retry key to invoke a function. A caller does not submit the expression, fixed parameters or test expectations. Authors can use the existing CEL prompt/compile/eval tools and submit an exact proposal/base through the same local command boundary.

## What the examples show

- One environment drives CEL declarations and a stable required caller schema. Fixed parameters cannot be supplied by callers.
- An exact snapshot is qualified against host-installed cases. A reviewer decides and activates it for an explicit scope and date interval.
- Git publishes accepted state and outbox work through one root compare-and-swap. Historical reads stay at a captured root.
- A correction keeps its earlier evidence, cases and receipts. Replay keeps the original input/build/answer; restatement names a later root and records a separate answer.
- A routed manual task records correction work and fences an earlier owner after takeover.

The record-filter example changes a parameter. The CI example changes a formula and projects nested event data into a different primitive contract. The ordinary-code comparison shares the lifecycle intentionally; it does not prove that CEL or this lifecycle is always the better choice.

## Boundaries

The callable profile accepts named string/bool variables and a boolean result. Numbers, structured wire types and custom extensions are unsupported. Internal expressions use the pinned CEL standard language. Resource ceilings are configured example bounds, not measured production-safe defaults.

This is local reference code with unstable APIs and records. Host context and repository writers are trusted. It is not a network authentication boundary, distributed database, resident-worker coordinator or hosted service. Preparation acknowledgement records a successful ephemeral preparation; each call prepares and checks its exact closure again.

CI ingress verification is simulated. No webhook is authenticated and no pipeline is launched. Its raw-event audit sidecar is not atomic with the Git receipt. The application supplies source truth, independent expectations, admission/learning policy, authentication, retention and side effects. See [INTEGRATION.md](INTEGRATION.md), [SUPPORT.md](SUPPORT.md) and [SECURITY.md](SECURITY.md).

The complete source-to-correction workflow is reproducible here. Production adapters and operations remain separate work. Ordinary code plus Git is appropriate when these explicit contracts and histories are unnecessary.

## Source and license

The project source is licensed under [Apache-2.0](LICENSE). Keep the [project attribution](NOTICE) and required notices. Dependencies retain their own licenses; see [THIRD_PARTY.md](THIRD_PARTY.md). The [licence decision](docs/LICENSING.md) records the change from MIT. The workflow examples are neutral invented policies informed by the public sources listed in [PROVENANCE.md](PROVENANCE.md).
