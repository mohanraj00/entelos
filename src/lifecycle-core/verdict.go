package lifecycle

// The verdict binding of src/protocol/verdict/VERDICT.md.

// hostOnlyOps are the operations of Handle that the wire protocol does not
// have yet: the approval read of VERDICT.md, the evidence read of
// EVIDENCE.md, the promotion read of PROMOTION.md and the two writes of
// FINDING.md.
var hostOnlyOps = map[string]struct {
	role   string
	fields []string
}{
	"approval":  {"reader", []string{"function", "candidate", "cursor"}},
	"evidence":  {"reviewer", []string{"snapshot_ref", "cursor"}},
	"promotion": {"reviewer", []string{"report_ref", "cursor"}},
	// The writes of FINDING.md: the import of a finding record as a
	// candidate case, and the decision of a reviewer on a candidate.
	"import_finding": {"author", []string{"source_ref", "finding"}},
	"decide_case":    {"reviewer", []string{"candidate_ref", "disposition", "reason"}},
}

// Verdict states of VERDICT.md.
const (
	verdictCurrent  = "current"
	verdictStale    = "stale"
	verdictRejected = "rejected"
	verdictUnknown  = "unknown"
)

// candidateRef gives the candidate reference of a snapshot: the candidate
// of a content snapshot, or the bundle of a CEL snapshot.
func candidateRef(snapshot Object) Object {
	if c, ok := lookup(snapshot, "candidate", nil).(Object); ok {
		return c
	}
	return Object{"kind": "cel-bundle", "ref": item(snapshot, "bundle_ref")}
}

// headOf gives the head snapshot of the subject of a snapshot, or nil.
func (h *Host) headOf(v *View, snapshot Object) (string, Object) {
	ref, _ := object(item(v.State, "heads"))[headKey(item(snapshot, "function"), item(snapshot, "scope"))].(string)
	if ref == "" {
		return "", nil
	}
	return ref, h.scoped(v, ref, "snapshot")
}

// verdictState gives the state of a verdict at the view, in the rule order
// of VERDICT.md. A reject verdict is rejected.
func (h *Host) verdictState(v *View, index Object, decision, snapshot Object) string {
	if !equal(item(decision, "disposition"), "accept") ||
		hasReject(h.verdicts(v, reviewEntry(index, text(item(decision, "snapshot_ref")), false))) {
		return verdictRejected
	}
	// The verdict judged the candidate of its snapshot (VERDICT.md).
	judged := candidateRef(snapshot)
	headRef, head := h.headOf(v, snapshot)
	if head != nil && equal(candidateRef(head), judged) && hasReject(h.verdicts(v, reviewEntry(index, headRef, false))) {
		return verdictRejected
	}
	if head == nil || !equal(candidateRef(head), judged) {
		return verdictStale
	}
	return verdictCurrent
}

// approval is the read of VERDICT.md: it answers if a candidate reference
// has a current approval for a subject. It writes nothing.
func (h *Host) approval(v *View, q Object) Object {
	function := item(q, "function")
	h.owned(Object{"function": function, "scope": h.ctx("scope")})
	asked, ok := lookup(q, "candidate", nil).(Object)
	kind, _ := asked["kind"].(string)
	ref, _ := asked["ref"].(string)
	if !ok || len(asked) != 2 || kind == "" || ref == "" {
		fail("invalid_request", "candidate must be an object with the strings kind and ref", "")
	}
	index := reviews(v)
	items := Array{}
	found := map[string]bool{}
	required := h.required()
	// KINDS.md: the counts are of one snapshot. They are the counts of the
	// first snapshot that meets the verdict policy, else of the last snapshot
	// with a counted verdict.
	counts, met := countKinds(nil), false
	for _, p := range h.sameCandidate(v, index, function, asked) {
		for _, decisionRef := range elements(reviewEntry(index, p.ref, false)["decisions"]) {
			decision := get(v, decisionRef, "decision")
			state := h.verdictState(v, index, decision, p.snapshot)
			found[state] = true
			items = append(items, Object{"decision_ref": decisionRef, "decision": decision, "state": state})
		}
		if c := h.snapshotCounts(v, index, p); !met && counted(c) {
			counts, met = c, len(lacking(c, required)) == 0
		}
	}
	state := verdictUnknown
	for _, s := range []string{verdictCurrent, verdictRejected, verdictStale} {
		if found[s] {
			state = s
			break
		}
	}
	// KINDS.md: approved needs each count of the verdict policy.
	approved := state == verdictCurrent && len(lacking(counts, required)) == 0
	return Object{"cursor": v.Cursor, "approved": approved, "state": state, "counts": counts, "required": required, "verdicts": items}
}
