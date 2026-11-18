# Verdict binding

This file specifies how a verdict binds to the content that it judged. Issue [#127](https://github.com/mohanraj00/entelos/issues/127) tracks it. A verdict judged the candidate reference of its snapshot. When a later proposal of the same subject has other content, the verdict is stale. A stale verdict cannot approve, and it stays in the history.

Status: **implemented** in the Go lifecycle Host. The minimum vectors are in [vectors/verdict-cases.json](vectors/verdict-cases.json), and `TestVerdictVectors` runs them. A later job adds the full conformance vectors.

## Terms

| Term | Meaning |
|---|---|
| Verdict | A `decision` record that `decide` wrote ([APPROVAL.md](../approval/APPROVAL.md)). |
| Subject | The `function` and `scope` values of a proposal. A content head has the function `content:<collection>/<partition>` ([CONTENT.md](../content/CONTENT.md)). |
| Candidate reference | An object `{"kind", "ref"}`. For a content snapshot, it is the `candidate` field of the snapshot. For a CEL snapshot, it is `{"kind": "cel-bundle", "ref": <bundle_ref>}`. |
| Head | The last proposal of a subject: the snapshot that `state.heads` names for the subject. |
| Edit | A new proposal of the same subject. `propose` already requires that its `base_snapshot` is the head. The new snapshot becomes the head. |

## Decision

1. The decision record does not change. Its `snapshot_ref` is the content hash of the snapshot, and the snapshot names the candidate reference. Thus the record binds the verdict to the content that it judged.
2. The state of a verdict is a function of the store at a cursor. The rules below give it. The Host does not store the state.
3. `decide` refuses an `accept` on a snapshot whose candidate reference differs from the candidate reference of the head. The code is `stale_verdict`.
4. `activate` refuses a verdict that is not `current`. The code is `stale_verdict`, or `proposal_rejected` as in 1.2.
5. The Host deletes no verdict and changes no verdict. A stale verdict stays in the store, in `state.reviews` and in the `proposal` result.
6. A new read operation, `approval`, answers if a candidate reference has a current approval for a subject. It is read-only. The consumer enforces the answer.

The reason for decision 1: the approval vectors pin the bytes of the decision record, and [STABILITY.md](../../../docs/STABILITY.md) makes these vectors a stable surface. A copy of the candidate reference in the record would change those bytes and add no binding. The `approval` read finds the verdicts by candidate reference, so a consumer does not read the snapshot.

The reason for decision 3: an `accept` on old content after an edit would make a verdict that is stale at once.

## Verdict state

The Host gives each `accept` verdict one state, in this order. The first rule that applies gives the state.

| State | Rule |
|---|---|
| `rejected` | A verdict on the same snapshot has `disposition` `reject`. |
| `rejected` | The head has a `reject` verdict, and the head has the same candidate reference as the verdict. |
| `stale` | The candidate reference of the head differs from the candidate reference of the verdict. |
| `current` | No rule above applies. |

The candidate reference of a verdict is the candidate reference of its snapshot.

A `reject` verdict has no state in this table. It never approves.

The state uses the candidate reference, not the snapshot reference. Thus a verdict is `current` again when a later proposal has the same content as the verdict (a return to the old content). The verdict judged those bytes. `activate` still checks the report of the verdict with each check of 1.2, so a superseded source or plan still gives `stale_qualification`.

## Changes to `decide`

For `accept`, a new check 7 comes after check 6 of [APPROVAL.md](../approval/APPROVAL.md):

7. The candidate reference of the snapshot equals the candidate reference of the head: `stale_verdict`.

For `reject`, nothing changes. A reviewer can reject old content.

## Changes to `activate`

After the checks of 1.2, `activate` finds the state of the verdict at the current root. A `rejected` state gives `proposal_rejected`, as in 1.2. A `stale` state gives `stale_verdict`. Only a `current` verdict activates.

## The `approval` read

Role: `reader`, as `resolve`. Body: `{"function": string, "candidate": {"kind", "ref"}, "cursor"?: string}`. The function is the subject in the host scope. A function that the Host does not have gives `out_of_scope`. A `candidate` that is not an object with the two string fields `kind` and `ref` gives `invalid_request`.

Result: `{"cursor", "approved", "state", "counts", "required", "verdicts"}`. [KINDS.md](KINDS.md#changes-to-the-approval-read) gives `counts` and `required`, and the verdict policy that `approved` also needs.

`verdicts` lists each verdict on a snapshot of the subject with the stated candidate reference. Each item is `{"decision_ref", "decision", "state"}`. The item `state` is the verdict state of an `accept`, or `rejected` for a `reject`. The order is the order of the `proposals` list: by `submitted_at` of the snapshot, the oldest first, then by `snapshot_ref`. A snapshot without `submitted_at` comes last. The verdicts of one snapshot are in write order.

The Host gives `state` from the items, with these rules in this order:

| Rule | `state` | `approved` |
|---|---|---|
| 1. An item has the state `current`. | `current` | `true` |
| 2. An item has the state `rejected`. | `rejected` | `false` |
| 3. An item has the state `stale`. | `stale` | `false` |
| 4. The list is empty. | `unknown` | `false` |

Thus a reference that no proposal has, and a proposal with no verdict, both give `unknown`. With `cursor`, the Host answers at that earlier root. An answer at an earlier root can be `current` for content that is `stale` now.

## Refusal code

| Code | Stage | Operations | Meaning |
|---|---|---|---|
| **`stale_verdict`** | `admission` | `decide` accept, `activate` | The content of the head differs from the content that the verdict judged. |

## Wire protocol

The decision record and the approval schemas do not change, so the stable approval vectors keep their bytes. The refusal code is new. The `approval` read is a Host operation. The sidecar wire protocol does not have it yet.

## Limits

- The state follows the head of the subject. A rejected or failed later proposal makes an earlier verdict stale, until a new proposal returns to the old content.
- `proposal` and `proposals` do not show the verdict state. The `approval` read shows it.
- The `approval` read looks at each snapshot in `state.reviews`. Its cost grows with the number of reviewed snapshots in the store.
- The verdict state does not change an activation that occurred before the edit. A new activation needs a current verdict.
- Issue [#128](https://github.com/mohanraj00/entelos/issues/128) adds the verdict kinds. This file does not count verdicts.
