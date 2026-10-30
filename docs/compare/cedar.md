# Comparison: Cedar

This page compares Cedar with the Entelos decision path. It uses the record filter `record.visible` from the [quickstart demo](../../examples/quickstart/README.md). It writes the same rule, the correction and the replay question in Cedar where Cedar can express them.

## Version and sources

The comparison uses Cedar SDK and CLI 4.14.0, released on 2026-10-17. Cedar language version 4.6 goes with this release. I read each source below on 2026-10-30. The docs pages do not show a last-updated date.

| Source | What it supports on this page |
|---|---|
| [Release v4.14.0](https://github.com/cedar-policy/cedar/releases/tag/v4.14.0) and [cedar-policy-cli v4.14.0](https://github.com/cedar-policy/cedar/releases/tag/cedar-policy-cli-v4.14.0), both 2026-10-17 | The version. |
| [Document history](https://docs.cedarpolicy.com/other/doc-history.html) | Language version 4.6 for SDK 4.14.0. |
| [Policy syntax](https://docs.cedarpolicy.com/policies/syntax-policy.html) | `permit`, `when`, annotations. Annotations have no effect on evaluation. |
| [Operators](https://docs.cedarpolicy.com/policies/syntax-operators.html) | `datetime("2026-11-15")` and its comparison operators. |
| [CHANGELOG](https://github.com/cedar-policy/cedar/blob/main/cedar-policy/CHANGELOG.md) | `datetime` is stable since SDK 4.4.0 (2025-04-23). |
| [Schema](https://docs.cedarpolicy.com/schema/human-readable-schema.html) | The `.cedarschema` format. |
| [Validation](https://docs.cedarpolicy.com/policies/validation.html) | Strict validation checks. The authorizer does not validate at request time. |
| [Authorization](https://docs.cedarpolicy.com/auth/authorization.html) | Default deny, forbid overrides permit, and the response. |
| [CLI source](https://github.com/cedar-policy/cedar/tree/main/cedar-policy-cli) | The subcommands, and the experimental `symcc` analysis. |

## Run status

I did not run Cedar for this page. The session that wrote it could not run the downloaded 4.14.0 CLI binary. Thus, each Cedar block below is proposed text from the docs. It is not parsed, validated or authorized. The expected answers come from the fixture [lifecycle-scenario.json](../../src/cel-lifecycle/fixtures/lifecycle-scenario.json), not from a Cedar run.

The Entelos answers on this page are tested. The quickstart demo and `verify.py` compare them with the same fixture.

## The rule in Cedar

The Entelos rule: `record.visible` returns `true` only if `state` is equal to the approved `allowed_state` and `archived` is `false`. The approved source tables give these values:

| Stage | Interval | `allowed_state` |
|---|---|---|
| `initial` | 2026-10-01 to 2026-12-01 | `ready` |
| `amendment` | 2026-11-01 to 2026-12-01 | `reviewed` |
| `correction` | 2026-11-01 to 2026-12-01 | `approved` |

Cedar answers an authorization question: can a principal do an action on a resource? The record filter has no principal. In this text, an invented `Demo::Caller` is the principal, and the record is the resource.

### Schema

```cedarschema
namespace Demo {
  entity Caller;
  entity Record = {
    "state": String,
    "archived": Bool,
  };
  action "view" appliesTo {
    principal: Caller,
    resource: Record,
    context: {
      "date": datetime,
    }
  };
}
```

The `datetime` type name in the context is not checked. The docs show the `datetime` constructor in policies, but I did not confirm the type name in a schema.

### Policies before the correction

Cedar has no selection by interval. All satisfied `permit` policies give `Allow`. Thus, the author must split the intervals so that the `amendment` stage replaces the `initial` stage from 2026-11-01:

```cedar
@id("visible-initial")
@source("initial")
permit (principal, action == Demo::Action::"view", resource)
when {
  context.date >= datetime("2026-10-01") &&
  context.date < datetime("2026-11-01") &&
  resource.state == "ready" &&
  !resource.archived
};

@id("visible-amendment")
@source("amendment")
permit (principal, action == Demo::Action::"view", resource)
when {
  context.date >= datetime("2026-11-01") &&
  context.date < datetime("2026-12-01") &&
  resource.state == "reviewed" &&
  !resource.archived
};
```

In Entelos, the author writes one rule and approves one source table for each stage. The lifecycle selects the stage for the date. In Cedar, the date logic is in the policy text, and the caller supplies `context.date`.

### Request and answer

The November call of the scenario, in Cedar terms:

- Principal: `Demo::Caller::"demo"`.
- Action: `Demo::Action::"view"`.
- Resource: `Demo::Record::"r1"` with `state` `"reviewed"` and `archived` `false` in the entities data.
- Context: `{"date": datetime("2026-11-15")}`.

Expected answer from the fixture: `Allow`, because `visible-amendment` is satisfied. The Cedar response has the decision and diagnostics. The diagnostics give the IDs of the determining policies (here `visible-amendment`) and any errors. The response has no request ID, input hash, policy set version or receipt.

### The correction

The `amendment` table had a transcription error. The approved visible state is `approved`, not `reviewed`. In Cedar, the author edits the `visible-amendment` policy or replaces it with a new policy:

```cedar
@id("visible-correction")
@source("correction")
permit (principal, action == Demo::Action::"view", resource)
when {
  context.date >= datetime("2026-11-01") &&
  context.date < datetime("2026-12-01") &&
  resource.state == "approved" &&
  !resource.archived
};
```

The application then loads the new policy set. The same request now gives `Deny`, which is an implicit deny with no determining policy. Cedar keeps no record that the earlier set gave `Allow`.

### The replay question

The question: "On 2026-11-15, the caller got `Allow` for a `reviewed` record. Which rule gave that answer, and does the same rule give it again?"

Cedar cannot answer this from its own data. To answer it, the application must keep these items for each call:

1. The policy set text, for example as a Git commit.
2. The schema and the entities data.
3. The request (principal, action, resource and context).
4. The CLI or SDK version.
5. The decision and the diagnostics.

With these items, the application can run the old request on the old policy set again (replay). It can also run the old request on the corrected set (restatement). Cedar does not link the two answers. Entelos does this in the lifecycle: `replay` uses the pinned selection of the old receipt, and `restate` writes a new receipt that refers to the old one. See [CORRECTION.md](../CORRECTION.md).

## Decision path

"Built in" means Cedar 4.14.0 does it. "Possible with other tools" means an application can add it with Git, a database or a managed service. "Not covered" means Cedar has no part in this step.

| Step | Entelos (tested in the scenario) | Cedar 4.14.0 | Status in Cedar |
|---|---|---|---|
| Source | The reviewer records the source table with `source`. | A policy is text. The `@source` annotation can carry a reference, but annotations have no effect on evaluation. | Possible with other tools |
| Proposal | The author proposes a bundle with `propose`. | No proposal record. A Git branch or pull request can hold the new policy text. | Possible with other tools |
| Qualification | The runner qualifies the bundle against the reviewer's plan cases. | Strict schema validation checks entity types, actions, attributes and operator types. The CLI `run-tests` subcommand runs tests with expected decisions. Cedar records no qualification result. | Built in (checks), not covered (record) |
| Approval | The reviewer decides with `decide`. A passed test does not grant serving authority. | No approval. Pull request review or a managed store can add it. | Possible with other tools |
| Dated activation | The reviewer activates a snapshot with a date interval. | No activation event. A `datetime` condition in the policy and a caller date in the context can limit when a policy applies. | Possible with other tools |
| Preparation | A worker loads the snapshot and records `ack`. | The application parses the policies into a policy set. The authorizer does not validate at request time, so the application must validate before it loads. | Built in (parse and validate), not covered (load record) |
| Execution | `invoke` returns the value and a receipt with the input identity and selection. | `authorize` returns `Allow` or `Deny`, the determining policy IDs and errors. No receipt. | Built in (answer), possible with other tools (record) |
| Feedback/correction | A correction is a new proposal, approval and activation. The old receipt does not change. | The author edits or replaces the policy text. Cedar keeps no history. | Possible with other tools |
| Replay or restatement | `replay` gives the old answer from the pinned selection. `restate` gives a new answer under a later cursor and links to the old receipt. | The application can run the old request again on retained files. The experimental `symcc` subcommand can check whether two policy sets are equivalent and give a counterexample. | Possible with other tools |

## Where Cedar is the better choice

- **The decision is authorization.** If the rule asks "can this principal do this action on this resource", Cedar has the right model. It has principals, entity hierarchies with `in`, and `forbid` policies that override `permit`. Entelos has no principal or entity model. Its current profile takes string and bool inputs and gives a bool.
- **The author needs checked types before deployment.** Strict validation against a schema finds unknown attributes, unknown actions and type errors before the policy is used. The docs say that the validator's soundness is formally proved in Lean. Entelos checks declared caller fields and types, but has no proof of this kind.
- **The author must compare two rule versions for all inputs.** The `symcc` subcommand can check whether an old and a new policy set are equivalent, or whether one implies the other. It can give a counterexample request. This is experimental, it needs the external cvc5 solver, and it is only in the experimental CLI build. Entelos compares versions only on recorded inputs and plan cases.
- **The inputs need more types.** Cedar has sets, records, entity references, `decimal`, `ipaddr` and `datetime`. The Entelos profile has only string and bool.

## Where Entelos adds something

- The receipt binds each answer to the input identity, the selected snapshot, the bundle and the engine build.
- Approval and dated activation are records, separate from the rule text.
- A correction does not change an earlier answer. Replay and restatement are lifecycle operations with explicit links.

An application can put Cedar inside a lifecycle of this kind. Cedar would give the answer, and the lifecycle would give the records. This page does not test that combination.

## Limits

- I did not run Cedar. The Cedar text and the expected Cedar answers are not checked.
- I did not measure speed, effort or maintenance for Cedar or Entelos. This page makes no claim about them.
- Cedar answers authorization requests. This page maps a non-authorization filter onto that model, so some Cedar text (the invented principal and action) has no Entelos equivalent.
- I did not read the Amazon Verified Permissions docs. The Cedar docs only say that it gives a managed policy store. This page makes no claim about its versioning, approval or logs.
