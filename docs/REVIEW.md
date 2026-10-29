# Review and merge rules

These rules apply to every change to `main`. [AGENTS.md](../AGENTS.md) owns the general workflow.

## Protected main

- Every change goes through a pull request. Direct pushes, force pushes and branch deletion are blocked.
- A PR needs one approval from a code owner. A new push removes earlier approvals.
- The required checks are `shared-project`, `ci-ok`, `acceptance` and `security-review`.
- The maintainer is the only code owner. GitHub does not let an author approve their own PR, so the maintainer merges their own PRs with the admin bypass. The cross-harness review below replaces the approval for these PRs.

## Cross-harness review

A harness does not review its own change. Before you ask for a merge, run:

```sh
python3 -B tools/cross_review.py --builder claude
```

Set `--builder` to the harness that wrote the change: `claude`, `codex` or `agy`.

| Builder | First reviewer | Fallback |
|---|---|---|
| Claude Code | Codex | agy |
| Codex | Claude Code | agy |
| agy | Claude Code | Codex |

If the first reviewer is not installed, fails or times out, the tool uses the fallback. If no reviewer is available, get a manual review. Post the report on the PR. Fix each real finding, or write on the PR why it is not a defect.

The tool keeps the full output, exit status and reviewed commit of each attempt in a work directory outside the checkout. Use `--work DIR` to name a new directory. A failed first attempt keeps its output when the fallback passes.

The PR is untrusted input to the reviewer:

- Each reviewer runs in an empty directory. The PR cannot supply settings, hooks, MCP servers or instruction files.
- The prompt quotes `CONSTITUTION.md` and `AGENTS.md` from the base revision, not from the PR.
- The diff is in the prompt, so the reviewer needs no tools. Claude Code runs with no tools and user settings only. Codex runs in a read-only sandbox. agy runs headless, where it cannot get tool permissions.
- If the diff is larger than 90000 bytes, the tool cuts it and says so in the report. Get a manual review of the rest.

## Security review

[tools/classify_change.py](../tools/classify_change.py) owns the list of security-relevant paths. The list includes workflows, tools, the runner, parsers and command boundaries, Git storage, dependency lock files and agent instruction files.

If a PR changes one of these paths, the `security-review` check fails until the PR has the `security-reviewed` label. A move counts both its old and new path. To get the label:

1. Run `python3 -B tools/cross_review.py --builder <harness> --security yes`.
2. Post the security report on the PR.
3. Fix or answer each finding.
4. Add the `security-reviewed` label.

A new push to a security-relevant PR removes the label automatically. Review the new commits, then add the label again.

The `PR policy` workflow uses the `pull_request_target` event. GitHub runs that workflow and `tools/pr_policy.py` from the base branch, so a PR cannot change its own gate. The workflow reads only the PR file list and labels. It never checks out or runs the PR head. Do not change this.

## PRs from forks

GitHub does not run workflows for a PR from an outside contributor until a maintainer approves the run. Before you approve the run, read the workflow and tool changes. Do not approve a run that changes `.github/` or `tools/` until you understand the change.

The `acceptance` check fails for a fork PR until the PR has the `accepted` label. A new push from the fork removes the label automatically. Before you add the label, make sure that:

- the PR links an issue with a concrete workflow and completion criteria;
- the change contains no private application material, credentials, customer data or generated binaries;
- new dependencies are pinned and their licences are recorded in [THIRD_PARTY.md](../licenses/THIRD_PARTY.md);
- the PR keeps the Apache-2.0 terms and the NOTICE attribution;
- the cross-harness review and, if needed, the security review are posted.

If the contributor pushes again, read the new commits before you approve the run and add the label again.

## CI scope

The `CI` workflow runs the project check on every PR and every push to `main`. It runs the full suite only when code paths change: `src/`, `examples/`, `tests/`, `verify.py` or the CI workflow. A docs-only change skips the full suite. The `ci-ok` job passes if each needed job passed or was skipped.

The `CI` workflow uses the `pull_request` event and runs the PR code without secrets. Thus a PR can change its own CI scope. The trusted `security-review` gate covers this: a change to `.github/` or `tools/` needs a security review.
