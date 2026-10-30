# Comparison with OPA/Rego

This page compares [Open Policy Agent](https://www.openpolicyagent.org/) (OPA) and its policy language Rego with the Entelos decision path. It uses the `record.visible` record filter from the [quickstart demo](../../examples/quickstart/README.md). It writes the same rule, the correction and the replay question in Rego.

## Version and sources

- OPA version: 1.22.0, [release page](https://github.com/open-policy-agent/opa/releases/tag/v1.22.0), released on 2026-10-29. Build commit `d4c8d0c8812290c3e3715ca4cf958f75283f6ec4`, Rego version v1.
- I read these primary docs on 2026-10-30. The pages show no version number. Thus, they can change after that date.
  - [Policy language](https://www.openpolicyagent.org/docs/policy-language)
  - [Policy testing](https://www.openpolicyagent.org/docs/policy-testing)
  - [Bundles](https://www.openpolicyagent.org/docs/management-bundles)
  - [Decision logs](https://www.openpolicyagent.org/docs/management-decision-logs)
  - [Status API](https://www.openpolicyagent.org/docs/management-status)
  - [CLI](https://www.openpolicyagent.org/docs/cli)
- Entelos: the `main` branch at merge commit `784e0dc`. The scenario is [lifecycle-scenario.json](../../src/cel-lifecycle/fixtures/lifecycle-scenario.json). [CORRECTION.md](../CORRECTION.md) explains replay and restatement.

## The rule in Rego

The Entelos rule returns `true` only if `state` is equal to the approved `allowed_state` for the date and `archived` is `false`. In Rego, the dated table is bundle data, and the caller sends the date:

```rego
package record

default visible := false

visible if {
	some row in data.record.rows
	row.from <= input.date
	input.date < row.until
	input.state == row.allowed_state
	input.archived == false
}
```

The Entelos fixture has an `initial` interval (2026-10-01 to 2026-12-01, `ready`) and an `amendment` interval (2026-11-01 to 2026-12-01, `reviewed`) that replaces part of it. The Rego data has no stage order. Thus, I split the table into two rows that do not overlap. Each interval includes its start date and excludes its end date, as in the fixture.

| Bundle | `.manifest` revision | Row 1 | Row 2 |
|---|---|---|---|
| `amend` | `amendment-1` | 2026-10-01 to 2026-11-01, `ready` | 2026-11-01 to 2026-12-01, `reviewed` |
| `correct` | `correction-1` | 2026-10-01 to 2026-11-01, `ready` | 2026-11-01 to 2026-12-01, `approved` |

The correction is a second bundle with a new revision string. The rule text does not change.

## What I ran

I ran OPA 1.22.0 (`opa_darwin_arm64`) on macOS arm64 on 2026-10-30. Before the run, the SHA-256 of the binary agreed with the published `.sha256` file. The binary, the policy, the bundles and the output stayed in a scratch directory outside this checkout. This page adds no dependency.

The commands, in sequence:

1. `opa test policy -v` ran three tests with `with data.record.rows as ...` overrides. Each test states its expected value directly from the fixture table. Result: `PASS: 3/3`.
2. `opa build -b amend -o amend.tar.gz` and `opa build -b correct -o correct.tar.gz` made the two bundles.
3. `opa eval -b amend.tar.gz -i input.json 'data.record.visible'` with input `{"date":"2026-11-15","state":"reviewed","archived":false}` gave `true`. The fixture expects `true` (`future_november_reviewed`).
4. `opa eval -b correct.tar.gz` with the same input gave `false`. The fixture expects `false` (`later_restatement`).
5. `opa run --server --set decision_logs.console=true -b amend.tar.gz`, then a `POST /v1/data/record/visible` with the same input. The response was `true` with a `decision_id`.

The decision log entry of step 5 had these fields, with others:

```json
{
 "bundles": {"amend.tar.gz": {"revision": "amendment-1"}},
 "decision_id": "989bd9ad-1abe-4dcb-ab59-acda8e965268",
 "input": {"archived": false, "date": "2026-11-15", "state": "reviewed"},
 "labels": {"version": "1.22.0"},
 "path": "record/visible",
 "result": true,
 "timestamp": "2026-10-30T00:33:48.845149Z"
}
```

These runs show the rule and the two answers. They do not test a bundle server, bundle signing, a decision log upload service or the Status API.

## The replay question in OPA

The question: on 2026-11-15, the caller got `true` for the `reviewed` record. After the correction, what was the earlier answer, and what is the answer now?

- **Replay.** The decision log keeps the input, the result and the bundle revision `amendment-1`. To get the earlier answer again, the operator must find a bundle with that revision and run `opa eval` with the logged input. Step 3 does this by hand. OPA does not keep earlier bundles, and it does not have a replay command that takes a `decision_id`. If the policy uses a non-deterministic built-in function, the decision log can keep its results in `nd_builtin_cache`.
- **Restatement.** Step 4 runs the same input against the `correction-1` bundle. The answer is `false`. OPA records no link from this answer to the earlier decision.
- **Identity.** The bundle revision is a string that the author writes in `.manifest`. It is not a content hash of the bundle bytes. Two different bundles can have the same revision. In Entelos, a receipt names the snapshot, bundle and engine build by content identity.

## The decision path

Each row gives one step of the path. "Built in" means OPA 1.22.0 or its documented management APIs do the step. "Possible with other tools" means OPA supplies a part, and Git, CI or application code must supply the rest. "Not covered" means that I found no OPA feature for the step in the docs above.

| Step | OPA/Rego | Notes | Entelos |
|---|---|---|---|
| Source | Built in | Rego files and JSON data. OPA does not record who has source authority. | CEL source and source tables in a Git store. The application supplies source authority. |
| Proposal | Possible with other tools | A Git pull request or a change to the bundle source. OPA has no proposal record. | `propose` records a candidate snapshot with its source, interval and bundle. |
| Qualification | Possible with other tools | `opa test`, `opa check` and test coverage are built in. A CI job must bind a test pass to the exact bundle bytes. OPA records no such binding. | `qualify` runs a reviewer's case plan on the exact snapshot and records a report. |
| Approval | Possible with other tools | Bundle signing shows that a trusted key signed the bundle. A signature is not an approval record. A Git review can supply the approval. | A reviewer's `decide` records the approval of a qualification report. A content hash is an identity, not a signature. |
| Dated activation | Possible with other tools | OPA activates a bundle when it loads it. The bundle docs do not describe an activation date. Dated rows in the data, as above, give a dated answer. A release schedule on the bundle server can also control the date. | A reviewer's `activate` event selects the snapshot for its interval. |
| Preparation | Built in | `opa build` makes a bundle. It can also compile the policy to Wasm. | `prepare.py` makes the bundle for the pinned host build. A worker's `load` loads the active snapshot. |
| Execution | Built in | The REST API, `opa eval`, the Go SDK and Wasm. Input and output can be any JSON value. | `invoke` on the host. The current profile accepts string and bool inputs and gives a bool output. |
| Feedback and correction | Possible with other tools | Decision logs keep the input, the result, the bundle revision and a `decision_id`. A correction is a new bundle revision. OPA does not link a correction to the earlier decisions. | A receipt for each answer. `observe` attaches feedback. A correction is a new proposal and activation. |
| Replay or restatement | Possible with other tools | The operator keeps earlier bundles and runs `opa eval` with the logged input. OPA stores no earlier bundle and writes no record that links the new answer to the earlier one. | `replay` uses the pinned snapshot. `restate` writes a new receipt that links to the earlier receipt. |

## Where OPA/Rego is the better choice

- **Authorization across many services with structured input.** Rego reads nested JSON input and data, and it can return any JSON value. The current Entelos profile accepts only string and bool inputs and gives a bool output.
- **Kubernetes admission and infrastructure policy.** OPA has a documented integration path for these uses, for example through OPA Gatekeeper. Entelos has no such integration.
- **Policy distribution to many agents.** Bundle polling, bundle signing, the Status API and decision log upload are built in. Entelos is a local reference on a Git store. It does not distribute bundles.
- **A mature language toolchain.** OPA has `opa test`, `opa fmt`, `opa check`, coverage and Wasm compilation. Entelos reuses CEL and does not supply an editor or a formatter.

If the main need is the record of each answer, a dated activation, and replay or restatement that links to the earlier answer, OPA supplies parts. The application must then supply the bundle archive, the approval record and the link records.

## Limits

- I did not measure speed, effort or code size. This page makes no claim about them.
- The Rego rule splits the fixture intervals into rows that do not overlap. It does not model the Entelos stage order.
- I ran OPA on one machine, without a bundle server, a decision log service or bundle signing. The rows for those features come from the docs, not from a run.
- The decision log in step 5 went to the console. A production log service, its retention and its masking rules were not tested.
