# Verdict kinds

This file specifies how the Host keeps human, model and policy verdicts apart. Issue [#128](https://github.com/mohanraj00/entelos/issues/128) tracks it. Each verdict has a kind and the identity of its actor. A Host policy can require a count of each kind before `activate`. The `approval` read of [VERDICT.md](VERDICT.md) reports the count of each kind.

Status: **implemented** in the Go lifecycle Host. The vectors are in [vectors/kinds-cases.json](vectors/kinds-cases.json), and `TestKindVectors` runs them. They cover each combination of the table in [Combinations](#combinations).

## Terms

| Term | Meaning |
|---|---|
| Verdict | A `decision` record that `decide` wrote ([APPROVAL.md](../approval/APPROVAL.md)), or an `accept` policy verdict of [POLICY.md](../policy/POLICY.md). |
| Kind | One value from the closed set `human`, `model` and `policy`. A `human` verdict comes from a person. A `model` verdict comes from a model. A `policy` verdict comes from an automatic policy check. |
| Identity | The `actor` of the verdict: the name of the person, the model or the policy check. The credential of the request gives it, as in 1.0. |
| Actor kinds | The Host context field `actor_kinds`. It maps an actor to its kind. |
| Count | The number of distinct identities of one kind that have a `current` verdict on one snapshot, with a qualification that `activate` accepts. |
| Verdict policy | The Host context field `verdict_policy`. It gives the minimum count of each kind that `activate` requires. |

## Decision

1. The application states the kind of each actor in the Host context: `"actor_kinds": {"<actor>": "human" | "model" | "policy"}`. A request cannot name a kind. The `decide` schema has no `kind` field, so a body with `kind` is `invalid_request`, as in 1.1.
2. `decide` writes the field `kind` in the decision record when `actor_kinds` names the actor. A decision record of an actor that `actor_kinds` does not name has no `kind` field.
3. A verdict counts only for its own kind. A `policy` verdict and a `model` verdict never count as a `human` verdict. A verdict without a `kind` field counts for no kind.
4. The Host context can have the field `verdict_policy`: `{"human": n, "model": n, "policy": n}`. Each value is an integer from 0 to 16. A missing kind is 0.
5. `activate` refuses a current verdict when a count is less than the verdict policy. The code is `verdicts_required`.
6. The `approval` read reports the count of each kind and the verdict policy. `approved` is `true` only when the state is `current` and each count meets the policy.

The reason for decision 1: the kind is a statement about the actor. The application authenticates the actor, so the application also states the kind. A kind in the request body would let a model declare that it is a person. The credentials file `entelos.credentials/1` is a stable surface with a closed set of roles ([STABILITY.md](../../../docs/STABILITY.md)). A Host context field states the kind and does not change that file.

The reason for decision 3: a policy that requires a person must not pass with a model or a policy check only.

### Relation to the policy verdict

[POLICY.md](../policy/POLICY.md) (proposed, #154) defines a policy verdict as a record of format `entelos.policy/1` that the Host writes. This file changes its decision 1 in one point: a `decide` record is a human verdict only when `actor_kinds` names its actor as `human`. Its decision 7 stays: a policy verdict never counts as a human verdict.

The kind `policy` has two sources:

- a `decide` record of an actor that `actor_kinds` names as `policy`, for a policy check that the application runs and that calls `decide` with its own credential;
- an `accept` policy verdict of POLICY.md. `policy_decide` writes it as a decision record with the kind `policy` and the field `policy_verdict_ref`. Its identity is the policy function.

If the verdict requirement of POLICY.md for the head is `human`, the Host ignores a policy verdict of the second source. It does not count for a kind.

## Host context

The Host checks the two fields when it starts. Each error is `configuration_error`:

- `actor_kinds` is not an object, an actor name is empty, or a value is not `human`, `model` or `policy`.
- `verdict_policy` is not an object, it has a key that is not a kind, or a value is not an integer from 0 to 16.

A Host context without `actor_kinds` names no actor. A Host context without `verdict_policy` requires 0 of each kind.

## Counts

The Host counts the verdicts of one snapshot of one subject, at one root:

1. Take each `accept` verdict on the snapshot.
2. Keep each verdict whose state is `current` ([VERDICT.md](VERDICT.md#verdict-state)).
3. Keep each verdict that `activate` accepts as its own verdict: its qualification report is still valid, and the decision bindings agree with the report. Thus a verdict with a stale qualification does not count.
4. For each kind, count the distinct `actor` values of the verdicts of that kind.

A `reject` verdict does not count. It makes the verdicts `rejected`, as in VERDICT.md.

Verdicts on two snapshots of the same content do not combine. Two such snapshots can have a different `stale_mode` or interval. Thus a `human` verdict on a `fail_closed` snapshot does not satisfy the policy for a `continue_stale` snapshot. Each snapshot needs its own verdicts. The separation check still compares all the snapshots of the same content.

## Separation

`decide` keeps check 4 of [APPROVAL.md](../approval/APPROVAL.md): the actor differs from the producer and the runner. This check compares the identity, so it applies to each kind. A model cannot judge a proposal that it produced, and a policy check cannot judge a report that it ran.

`decide` gets one new check. For `accept`, it is check 8, after check 7 of VERDICT.md. For `reject`, it is check 5, after the duplicate check:

- No verdict of this actor on a snapshot of the subject with the same candidate reference has another kind, or has no kind when this verdict has one: `separation_required`.

Thus one identity counts for one kind only. If the application changes the kind of an actor, the earlier verdicts of the actor keep their kind, and the actor cannot add a verdict of the new kind on the same content.

## Changes to `decide`

The decision record gets the field `kind` when `actor_kinds` names the actor:

```json
{"format":"entelos.record/1","function":"…","scope":"…","from":"…","until":"…",
 "snapshot_ref":"…","report_ref":"…","kind":"human","actor":"reviewer-a",
 "reason":"…","disposition":"accept","runner_policy":"fixture-runner",
 "decided_at":"2026-11-22T09:00:00Z"}
```

The `decide` result schema does not close the fields of the decision record, so the new field agrees with it. Without `actor_kinds`, the record bytes do not change. Thus the stable approval vectors keep their bytes.

## Changes to `activate`

After the checks of VERDICT.md, `activate` finds the counts of the snapshot of the verdict at the current root. If a count is less than the verdict policy, the code is `verdicts_required`. The message names each kind that has too few verdicts.

Without `verdict_policy`, `activate` needs one `current` verdict, as before. A verdict without a kind can then activate.

## Changes to the `approval` read

The result gets two fields:

```json
{"cursor":"…","approved":false,"state":"current",
 "counts":{"human":0,"model":2,"policy":1},
 "required":{"human":1,"model":1,"policy":0},
 "verdicts":[…]}
```

The read asks for a candidate reference, and the counts are of one snapshot of that reference. They are the counts of the first snapshot in the order of the proposals list that meets the verdict policy. If no snapshot meets the policy, they are the counts of the last snapshot with a counted verdict. If no verdict counts, each count is 0. `counts` has the three kinds, also when a count is 0. `required` is the verdict policy of the Host, with 0 for a missing kind. `approved` is `true` only when `state` is `current` and each count is at least its required value.

## Combinations

The vectors of the Host code cover these combinations, with the policy `{"human": 1, "model": 1}`:

| Current verdicts | `counts` (human, model, policy) | `approved` | Result |
|---|---|---|---|
| One `model` | 0, 1, 0 | `false` | `activate`: `verdicts_required` |
| Two `model` from two actors | 0, 2, 0 | `false` | `activate`: `verdicts_required` |
| One `policy` | 0, 0, 1 | `false` | `activate`: `verdicts_required` |
| One `model` and one `policy` | 0, 1, 1 | `false` | `activate`: `verdicts_required` |
| One `human` | 1, 0, 0 | `false` | `activate`: `verdicts_required` |
| One `human` and one `model` | 1, 1, 0 | `true` | `activate` succeeds |
| One `human` and one `model`, then an edit | 0, 0, 0 for the old reference | `false` | `activate`: `stale_verdict` |
| One actor as `model`, then the same actor as `human` | 0, 1, 0 | `false` | `decide`: `separation_required` |

More vectors cover verdicts on two snapshots of the same content, a verdict without a kind, a body with a `kind` field, an invalid `actor_kinds` and an invalid `verdict_policy`.

## Refusal codes

| Code | Stage | Operations | Meaning |
|---|---|---|---|
| **`verdicts_required`** | `admission` | `activate` | A count of current verdicts is less than the verdict policy. |
| `separation_required` | `admission` | `decide` | Also: the actor has a verdict of another kind on the same content. |

## Wire protocol

The decision record gets one optional field. The approval schemas and the credentials file do not change. The refusal code `verdicts_required` is new. The sidecar and the JSONL Host read the two fields from the `--context` file, and the Go API reads them from the Host context. The sidecar has no command-line setting for them. Its wire protocol has the `approval` read from version 1.3 (#213).

## Limits

- The application states the kind. The Host does not check that a `human` actor is a person.
- The verdict policy is one policy for each Host. A policy for each subject is not in this file.
- The count uses the `actor` string. Two names for one person count as two identities.
- A verdict of an earlier Host has no kind, so it counts for no kind.
- The Go Host calls one policy function, so the `policy` count of a snapshot from the second source is 0 or 1.
