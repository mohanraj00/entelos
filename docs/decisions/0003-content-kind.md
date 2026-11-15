# 0003 Candidate kinds and the content subject

- Status: proposed
- Date: 2026-11-15
- Decided by: the maintainer, when the maintainer merges the Host change
- Issue: [#175](https://github.com/mohanraj00/entelos/issues/175), from [#153](https://github.com/mohanraj00/entelos/issues/153)
- Spec: [CONTENT.md](../../src/protocol/content/CONTENT.md)

## Question

A content proposal is a candidate with no CEL file. Its head is a collection and a partition, not a function. How does the Host keep one lifecycle for a CEL bundle and for content, without a second copy of the source, interval, approval, supersession and signature rules?

## Decision

1. **A candidate has a kind: `cel-bundle` or `content`.** A snapshot without a `candidate` field is a CEL bundle. A CEL snapshot keeps the bytes of an earlier Host.
2. **A content head has the subject `content:<collection>/<partition>`.** The Host writes the subject in the `function` field of each lifecycle record of the head. The embedding host installs the subject in the `functions` list of the Host context.
3. **The candidate reference is `{"kind", "ref"}`.** For content, `ref` is the SHA-256 of the exact manifest text. The snapshot and the report record it.
4. **The `bundle_ref` field of a content snapshot holds the candidate `ref`.** Thus the stable runner contract (`Candidate.BundleRef` and the `bundle_ref` of a verdict) binds a passed report to the exact manifest bytes.
5. **Entelos stores the manifest text only.** An application runner qualifies a content candidate. The CEL runner refuses it.
6. **`load`, `invoke`, `describe` and `ingest` run a CEL bundle only.** A content activation gives its candidate reference through `resolve`, at the current root or at an earlier cursor.

## Reasons

| | Subject in `function` | Separate head fields in each record |
|---|---|---|
| Lifecycle rules | The same code for both kinds | A second form of `same`, `owned`, `live` and the event checks |
| Signed records | No change to the signed record format | A new field in the signed record |
| Approval API | Lists a content proposal with no change | A change in the list and in the 1.2 schemas |
| Record reading | A reader must know the `content:` prefix | The kind is explicit |
| Name space | A CEL function name with `content:` is refused | No shared name space |

One set of rules for both kinds keeps one meaning for scope, interval, supersession and approval. The cost is the prefix rule. A CEL bundle can no longer use a function name that starts with `content:`.

## Consequences

- The `propose` field `candidate` is a Host field. The wire protocol gets it in a later minor version, after the approval API 1.2 change.
- The list item of `proposals` gets the fields `candidate` and `evidence_reference` in a later change. `proposal` gives them now, in the snapshot.
- The applicability fields (#106) and the signed activation record (#105) use the candidate reference and the head of this decision.
- A third kind needs a new kind name, a new subject prefix and its own rules in a spec.
