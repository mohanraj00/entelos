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

Entelos does this with [CEL](https://cel.dev) rules and a local Git history. The demo runs one invented rule through an answer, a rule change, a replay and a restatement. One run on a Mac took about 1.5 minutes.

### 1. Install the tools

| Tool | Version | Check | Install |
|---|---|---|---|
| Go | 1.27.2 or later | `go version` | [go.dev/dl](https://go.dev/dl/) |
| Python | 3 (tested with 3.14.6) | `python3 --version` | [python.org/downloads](https://www.python.org/downloads/) |
| Git | tested with 2.54.0 | `git --version` | [git-scm.com/downloads](https://git-scm.com/downloads) |

You also need network access. The demo downloads the pinned Go modules on each run. macOS and Linux are tested. Windows is not tested. Before it starts, the demo checks that Go and Git are installed and that Go is 1.27.2 or later. It does not check the Git or Python version.

### 2. Get the code

```sh
git clone https://github.com/mohanraj00/entelos.git
cd entelos
```

### 3. Run the demo

The demo writes to a new directory outside this checkout. The directory must not exist. The demo does not overwrite an earlier run.

```sh
export TASK_WORK="$(mktemp -d)/run"
python3 -B examples/quickstart/demo.py --work "$TASK_WORK"
```

### 4. Read the output

This is the output of one run. The hashes in your run can differ.

```text
Entelos demo: each answer stays explainable after its rule changes.
The rule record.visible answers one question: can a user see this record?
  Rule:   state == allowed_state && !archived
  Values: allowed_state is "ready" from 2026-10-01, then "reviewed" from 2026-11-01.

  1. Answer       Is a "reviewed", not archived record visible on 2026-11-15? -> true (expected true)
                  A receipt in the Git store saves the exact rule version, input and answer.
  2. Rule change  A reviewer changes allowed_state from "reviewed" to "approved".
                  This makes a new rule version. The old version and the receipt stay.
  3. Replay       Why was the answer true? Run the receipt on its old rule version. -> true (expected true)
  4. Restatement  Under the new rule, what is the answer for the same record and date? -> false (expected false)
                  This is a new, separate answer. The first answer stays in the history.

  The expected values come from the fixture file, not from this run.
  Input:   {"state":"reviewed","archived":false}
  Records: receipt sha256:47121ec901a7..., old rule sha256:66e12e967a63...,
           new rule sha256:9e30a2cd3dac..., change event sha256:78730c528391...
  Build:   sha256:94ac3b7b4d34... (cel-go v0.32.0)
Result: passed. Details: <TASK_WORK>/summary.json
Next: read "After the demo" in README.md.
```

- **Answer:** the service asks the rule a question. The receipt keeps the exact rule version and input.
- **Rule change:** a reviewer changes the rule value. The change is a new version. Nothing old is edited. [CORRECTION.md](docs/CORRECTION.md) calls this path a rule restatement, and it also explains a fact correction.
- **Replay:** you can still show why the first answer was `true`, because its rule version still exists.
- **Restatement:** the same question under the new rule gives `false`. You now have two answers, and each one has its own record.

The `expected` values come from a fixture file, not from the demo. `Result: passed` means that each answer agrees with the fixture. [examples/quickstart/README.md](examples/quickstart/README.md) explains `summary.json` and the failure output.

## After the demo

| If you want to | Read |
|---|---|
| Know how a service calls a rule, why not a plain function and log, and what a correction date means | [COMPARISON.md](docs/COMPARISON.md) |
| See the same steps with the commands and records | [WALKTHROUGH.md](docs/WALKTHROUGH.md) |
| Know the difference between a fact correction and a rule change | [CORRECTION.md](docs/CORRECTION.md) |
| Call or author a rule from your own service | [CONTRACTS.md](docs/CONTRACTS.md), then [INTEGRATION.md](docs/INTEGRATION.md) |
| Compare Entelos with a tool you know | [OPA](docs/compare/opa.md), [Cedar](docs/compare/cedar.md), [GoRules ZEN](docs/compare/zen.md), [feature flags](docs/compare/flags.md), [ordinary code and Git](docs/compare/code-git.md) |
| Know what Entelos does not do | [Limits](#limits) and [SUPPORT.md](docs/SUPPORT.md) |
| Run all the checks | [Run all checks](#run-all-checks) |
| Ask a question or report a problem | [GitHub issues](https://github.com/mohanraj00/entelos/issues) |

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

Use Python, Go and Git. See [SUPPORT.md](docs/SUPPORT.md) for the tested toolchain and limits. Use a new work directory, not the demo directory. One run on a Mac took about 10 minutes, and the runner prints its result only at the end.

```sh
export TASK_WORK="$(mktemp -d)/run"
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
