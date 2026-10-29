# Review and merge rules

These rules apply to every change to `main`. [AGENTS.md](../AGENTS.md) owns the general workflow.

## Protected main

- Every change goes through a pull request. Direct pushes, force pushes and branch deletion are blocked.
- A PR needs one approval from a code owner. A new push removes earlier approvals.
- The required checks are `shared-project`, `ci-ok`, `acceptance` and `security-review`.
- The maintainer is the only code owner. GitHub does not let an author approve their own PR, so the maintainer merges their own PRs with the admin bypass. The cross-harness review below replaces the approval for these PRs.

## Cross-harness review

A harness does not review its own change. The branch prefix names the harness that wrote the change: `claude/`, `codex/` or `agy/`. The reviews run on GitHub. No harness reviews a PR by default, so the PR author asks in a PR comment.

| Builder | Code review comment | Security review comment |
|---|---|---|
| Claude Code | `@codex review` | `@codex security review` |
| Codex | `@claude review this PR. Use the Review guidelines in AGENTS.md.` | `@claude do a security review of this PR. Use the Review guidelines in AGENTS.md.` |
| agy | `@claude review this PR. Use the Review guidelines in AGENTS.md.` | `@claude do a security review of this PR. Use the Review guidelines in AGENTS.md.` |

Follow these rules for each request:

1. Ask when CI is green and the PR is ready.
2. On the next line of the comment, write `Head: <sha>` with the short SHA of the PR head commit.
3. Ask one time for each head commit. Before you ask, read the PR comments. If a request names the current head, do not ask again.
4. If you push fixes after a review, ask again for the new head. Ask at most 3 times for each PR. After that, ask the maintainer to review.
5. Fix each real finding, or write on the PR why it is not a defect.

Codex reviews through the Codex GitHub integration. Claude Code reviews through the [Claude review](../.github/workflows/claude-review.yml) workflow, which needs the `CLAUDE_CODE_OAUTH_TOKEN` secret. That workflow checks out `main` and fetches the PR commits as Git objects only, so the PR cannot supply Claude's instructions. Claude reads the diff with `git diff`, `git show` and `git log`, and posts with the action's comment tools. No tool gets a token: the workflow removes credentials from tool subprocesses, runs them in a PID namespace and denies reads of `/proc`. The OAuth token is a static secret. If you suspect a leak, revoke it and make a new one.

### Local fallback: agy

If the GitHub reviewer is not available (no quota, an outage or no integration), run agy on your machine:

```sh
python3 -B tools/cross_review.py --work "$REVIEW_WORK"
```

Post the report on the PR. It starts with `Review (agy <version>):` and names the base and head commits. If agy is not available, ask the maintainer for a manual review.

The PR is untrusted input to agy:

- agy runs headless and with `--sandbox` in an empty directory. It gets a new HOME that holds only its login token and an empty allow list. Thus, your global agy allow rules and MCP servers do not apply, and the PR cannot supply settings or instruction files.
- The prompt quotes `CONSTITUTION.md` and `AGENTS.md` from the base revision, not from the PR.
- agy gets only basic environment variables and its own login key. Read the report before you post it, and remove anything that is not a review finding.
- The tool resolves the base and head to commit SHAs once. If HEAD changes during the review, the report says so and the tool exits 1.
- If the diff is larger than 90000 bytes, the tool cuts it and says so in the report. Get a manual review of the rest.
- The full output and exit status stay in the work directory, which must be outside the checkout.

## Security review

[tools/classify_change.py](../tools/classify_change.py) owns the list of security-relevant paths. The list includes workflows, tools, the runner, parsers and command boundaries, Git storage, dependency lock files and agent instruction files.

If a PR changes one of these paths, the `security-review` check fails until the PR has the `security-reviewed` label. A move counts both its old and new path. Agent instruction files and settings count at any directory depth. To get the label:

1. Ask for the security review in a PR comment, as the table above shows. If the GitHub reviewer is not available, run `python3 -B tools/cross_review.py --security yes --work "$REVIEW_WORK"` and post the report.
2. Fix or answer each finding.
3. Ask the maintainer to add the `security-reviewed` label. Only the maintainer adds it.

When you add the label, the workflow posts an approval record with the PR head and base at that time. The label counts only while the PR has that head and base. After a push or a base change, the next run removes the label. Review the new commits, then add the label again.

The `PR policy` workflow uses the `pull_request_target` event. GitHub runs that workflow and `tools/pr_policy.py` from the base branch, so a PR cannot change its own gate. The workflow reads only the PR file list, the labels and its own approval records. It never checks out or runs the PR head. Do not change this.

The workflow posts the approval records with the workflow token. A fork PR gets a read-only token, so it cannot write a record. A user with write access can change a workflow to write one, so the gates trust writers.

## PRs from forks

GitHub does not run workflows for a PR from an outside contributor until a maintainer approves the run. Before you approve the run, read the workflow and tool changes. Do not approve a run that changes `.github/` or `tools/` until you understand the change.

The `acceptance` check fails for a fork PR until the PR has the `accepted` label. The label counts only for the head and base in its approval record, as for the security label. Before you add the label, make sure that:

- the PR links an issue with a concrete workflow and completion criteria;
- the change contains no private application material, credentials, customer data or generated binaries;
- new dependencies are pinned and their licences are recorded in [THIRD_PARTY.md](../licenses/THIRD_PARTY.md);
- the PR keeps the Apache-2.0 terms and the NOTICE attribution;
- the cross-harness review and, if needed, the security review are posted.

If the contributor pushes again, read the new commits before you approve the run and add the label again.

## CI scope

The `CI` workflow runs the project check on every PR and every push to `main`. It runs the full suite only when code paths change: `src/`, `examples/`, `tests/`, `tools/`, `verify.py` or the CI workflow. Other changes, for example docs or other workflows, skip the full suite. The `ci-ok` job passes if each needed job passed or was skipped.

The `CI` workflow uses the `pull_request` event and runs the PR code without secrets. Thus a PR can change its own CI scope. The trusted `security-review` gate covers this: a change to `.github/` or `tools/` needs a security review.
