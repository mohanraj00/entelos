<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/entelos-logo-dark.svg">
    <img alt="Entelos" src="docs/brand/entelos-logo-light.svg" width="360">
  </picture>
</p>

# Entelos

**Entelos: grow toward the goal.**

> **Limits:** This is local reference code with unstable APIs and records. It is not a hosted service, an authentication boundary or a production database. Rules accept only string and bool inputs and give a bool result. See [Limits](#limits) and [SUPPORT.md](docs/SUPPORT.md).

## The problem

A service often asks a rule for an answer, and people change that rule over time. Later, someone asks why the service gave a specific answer on a specific date. Often nobody can show the exact rule, inputs and approval behind that answer. If a correction changes the old rule in place, the earlier answer has no explanation. I want each answer tied to its exact rule version, inputs and approval, and each correction recorded as a new, separate answer.

## Run the demo

Entelos is a runnable reference that does this with [CEL](https://cel.dev) rules and a local Git history. Use Go, Python 3, Git and network access. Set `TASK_WORK` to a path outside this checkout that does not exist yet. The demo makes this directory, and it does not overwrite an earlier run. Then run the demo:

```sh
export TASK_WORK="$(mktemp -d)/run"
python3 -B examples/quickstart/demo.py --work "$TASK_WORK"
```

The demo builds the host, runs one invented record filter through answer, correction, replay and restatement, and compares each answer with the fixture. This is the output of one run. The hashes in your run can differ.

```text
Record-filter loop for record.visible
  Build identity: sha256:94ac3b7b4d34... (cel-go v0.32.0)
  1. Answer:      on 2026-11-15, {"state":"reviewed","archived":false} -> true (fixture expects true)
                  receipt sha256:e890475b3547..., snapshot sha256:66e12e967a63...
  2. Correction:  visible state "reviewed" -> "approved", activated as event sha256:78730c528391...
                  new snapshot sha256:9e30a2cd3dac...; the earlier receipt stays
  3. Replay:      the same receipt on its pinned snapshot -> true (fixture expects true)
  4. Restatement: the same input under the correction -> false (fixture expects false)
Result: passed. Details: <TASK_WORK>/summary.json
```

[examples/quickstart/README.md](examples/quickstart/README.md) explains each line and the failure output.

## How a decision moves

![Decision path from source to replay or restatement](docs/diagrams/decision-path.svg)

- [Caller and correction walkthrough](docs/WALKTHROUGH.md): the same record filter, step by step, with the commands and records.
- [Fact correction versus rule restatement](docs/CORRECTION.md): the two reasons that a recorded answer can be wrong, and how the lifecycle handles each.
- [Contract map](docs/CONTRACTS.md): the executable interfaces for authors and callers.
- [Diagram notes](docs/diagrams/README.md): what each part of the diagram shows and does not show.
- [Limits](#limits) and [SUPPORT.md](docs/SUPPORT.md): what the reference does not do.

## What the examples show

I reuse CEL and Git; I do not implement another expression language. The record-filter example combines a typed CEL callable with a local Git history. A second example models pull-request CI eligibility and compares CEL with ordinary typed functions.

- One environment drives CEL declarations and a stable required caller schema. Fixed parameters cannot be supplied by callers.
- An exact snapshot is qualified against host-installed cases. A reviewer decides and activates it for an explicit scope and date interval.
- Git publishes accepted state and outbox work through one root compare-and-swap. Historical reads stay at a captured root.
- A correction keeps its earlier evidence, cases and receipts. Replay keeps the original input/build/answer; restatement names a later root and records a separate answer.
- A routed manual task records correction work and fences an earlier owner after takeover.

The record-filter example changes a parameter. The CI example changes a formula and projects nested event data into a different primitive contract. The ordinary-code comparison shares the lifecycle intentionally; it does not prove that CEL or this lifecycle is always the better choice.

## Limits

The callable profile accepts named string/bool variables and a boolean result. Numbers, structured wire types and custom extensions are unsupported. Internal expressions use the pinned CEL standard language. Resource ceilings are configured example bounds, not measured production-safe defaults.

This is local reference code with unstable APIs and records. Host context and repository writers are trusted. It is not a network authentication boundary, distributed database, resident-worker coordinator or hosted service. Preparation acknowledgement records a successful ephemeral preparation; each call prepares and checks its exact closure again.

CI ingress verification is simulated. No webhook is authenticated and no pipeline is launched. Its raw-event audit sidecar is not atomic with the Git receipt. The application supplies source truth, independent expectations, admission/learning policy, authentication, retention and side effects. See [INTEGRATION.md](docs/INTEGRATION.md), [SUPPORT.md](docs/SUPPORT.md) and [SECURITY.md](SECURITY.md).

The complete source-to-correction workflow is reproducible here. Production adapters and operations remain separate work. Ordinary code plus Git is appropriate when these explicit contracts and histories are unnecessary.

## Run all checks

Use Python, Go and Git. See [SUPPORT.md](docs/SUPPORT.md) for the tested toolchain and limits. Choose a new scratch directory outside this checkout:

```sh
python3 -B verify.py --work "$TASK_WORK"
```

The runner fetches the pinned public CEL author tools, builds the host and executes the examples and tests. Network access is needed for public source/module downloads. No model account, database, Kubernetes cluster or private repository is needed. Builds, logs, bare Git stores and generated inputs stay under the supplied work directory. Do not use a directory containing real data.

If the author-tools checkout is already available at the documented revision, pass `--skills /path/to/skills`. The runner requires a clean checkout. It never pushes, creates a remote repository or publishes a package.

The work directory contains `results.json`, command logs and the example stores. To discover the CI function after a successful run:

```sh
printf '%s\n' '{"op":"describe","function":"ci.pr_should_run","date":"2026-11-15"}' | python3 -B src/cel-lifecycle/cli.py --repo "$TASK_WORK/reuse/cel/state.git" --context "$TASK_WORK/reuse/cel/caller.json" --runtime "$TASK_WORK/bin/cel-handoff" --work "$TASK_WORK/reuse/cel"
```

`src/cel-lifecycle/client.py` uses that discovery result, ordinary inputs and a retry key to invoke a function. A caller does not submit the expression, fixed parameters or test expectations. Authors can use the existing CEL prompt/compile/eval tools and submit an exact proposal/base through the same local command boundary.

## Work in this project

Start with [CONSTITUTION.md](CONSTITUTION.md), [AGENTS.md](AGENTS.md) and [current work](docs/STATUS.md). Codex and Claude Code share those rules and the same skill source. [Tracked issues](docs/BACKLOG.md) define the bounded next work. [Contract map](docs/CONTRACTS.md) points to the executable interfaces.

## Name, source and license

Entelos (from Greek *en* + *telos*, "the goal within") is the name of this versioned decision reference. The repository is `mohanraj00/entelos`; its earlier name was `versioned-decision-reference`. [Brand assets](docs/brand/README.md) holds the mark, the logo and the colours.

The project source is licensed under [Apache-2.0](LICENSE). Keep the [project attribution](NOTICE) and required notices. Dependencies retain their own licenses; see [THIRD_PARTY.md](licenses/THIRD_PARTY.md). The [licence decision](docs/LICENSING.md) records the change from MIT. The workflow examples are neutral invented policies informed by the public sources listed in [PROVENANCE.md](docs/PROVENANCE.md).
