# Contributing

You need only Git, Go, Python and a GitHub account. You do not need Codex, Claude Code or agy. The maintainer does the review steps that use those tools.

[Review and merge rules](docs/REVIEW.md) owns the detailed rules. This page gives the steps from an issue to a merge.

## Steps

1. Select an issue.
   - Use an open issue with a concrete workflow and completion criteria.
   - Starter issues have the `good first issue` label.
   - If no issue agrees with your change, open one first. Use a form: a defect, a workflow question or a docs problem. Give the concrete workflow, a small neutral example and the change in behavior.
   - Write a comment on the issue before you start, so that two people do not do the same work.
2. Make the change on a fork.
   - Fork `mohanraj00/entelos` on GitHub and clone your fork.
   - Make a branch for the issue, for example `git checkout -b docs/fix-step-3`.
   - Keep the change to the issue. Keep application-specific source authority, policy and payloads separate from reusable mechanics.
   - Keep intended failure cases beside the successful cases.
3. Run the local checks.
   - Install the tools in the [tools table](README.md#1-install-the-tools).
   - After you change a source file, record the new bytes: `python3 -B tools/check_project.py --write-checksums`. Read the diff of `checksums.json`. It must show only the files that you changed.
   - Run the project check: `python3 -B tools/check_project.py`. It must print no error.
   - If you change `src/`, `examples/`, `tests/`, `tools/`, `verify.py`, a fixture or a dependency, also run the full suite. Use a new work directory outside the checkout, for example `python3 -B verify.py --work "$(mktemp -d)/run"`. One run on a Mac took about 10 minutes.
   - If a check fails, keep the output. Write the failure in the PR. Do not remove a failure case to make a check pass.
4. Open the PR.
   - Push the branch to your fork, then open a PR to `main`.
   - Fill in the [PR template](.github/pull_request_template.md). It asks for the issue, the changed behavior, the checks that you ran and the limits.
   - Write "Closes #N" with the issue number.
   - Write each check that you ran and its result. If you did not run the full suite, write the reason.
   - Write what you did not test and what is not supported.
   - A dependency, profile or record change also needs an identity and compatibility assessment.
5. Wait for the checks and the review. The table below shows who does each step.
6. Answer the review findings.
   - Fix each real finding in a new commit on the same branch, then push. You do not need a force-push.
   - If a finding is not a defect, write the reason in a reply on the PR.
   - Run the local checks again before each push.
   - Each push makes a new head. The maintainer must approve the CI run, review and add the labels again for the new head.

## Who does each step after you open the PR

| Step | Who | What happens |
|---|---|---|
| Approve the CI run | Maintainer | GitHub does not run the CI workflow for a fork PR until the maintainer approves the run. |
| `shared-project` check | CI | Runs the project check on every PR. |
| `ci-ok` check | CI | Runs the full suite if the PR changes a code path. Passes if each needed job passed or was skipped. |
| Code review | Maintainer | Reviews the change, or asks a harness for a review in a PR comment. You do not ask a harness. |
| `security-review` check | Maintainer | Fails if the PR changes a security-relevant path. [tools/classify_change.py](tools/classify_change.py) decides which paths are security-relevant. The maintainer does or asks for the security review. |
| `acceptance` check | Maintainer | Fails for a fork PR until the maintainer does the acceptance review and adds the `accepted` label. The label applies only to the current head. |
| Approval and merge | Maintainer | A PR needs one approval from the code owner. A new push removes earlier approvals. |

To see if your change touches a security-relevant path, compare it with the main repository:

```sh
git remote add upstream https://github.com/mohanraj00/entelos.git
git fetch upstream
python3 -B tools/classify_change.py upstream/main HEAD
```

If the output shows `"security": true`, the PR needs a security review, and the review takes more time.

## Do not submit

- Generated binaries, caches or local Git stores.
- Credentials, customer material or raw private evaluation data.
- Unrelated framework, language or deployment work without a demonstrated integration need.

To report a possible vulnerability, use the private route in [SECURITY.md](SECURITY.md). Do not open a public issue for it.

## Licence

Project source is licensed under [Apache-2.0](LICENSE). Unless explicitly stated otherwise, contributions intentionally submitted for inclusion use those terms, as described in section 5 of the licence. Keep the [project attribution](NOTICE) and [third-party notices](licenses/THIRD_PARTY.md). The [licence decision](docs/LICENSING.md) records the change from MIT.
