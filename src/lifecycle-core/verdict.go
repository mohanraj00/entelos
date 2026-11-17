package lifecycle

// The verdict binding of src/protocol/verdict/VERDICT.md.

import "sort"

// hostOnlyOps are the operations of Handle that the wire protocol does not
// have yet. The approval read of VERDICT.md is one.
var hostOnlyOps = map[string]struct {
	role   string
	fields []string
}{
	"approval": {"reader", []string{"function", "candidate", "cursor"}},
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
	type proposal struct {
		ref, submitted string
		snapshot       Object
	}
	var proposals []proposal
	for snapshotRef := range index {
		snapshot := h.scoped(v, snapshotRef, "snapshot")
		if equal(snapshot["function"], function) && equal(candidateRef(snapshot), asked) {
			submitted, _ := snapshot["submitted_at"].(string)
			proposals = append(proposals, proposal{snapshotRef, submitted, snapshot})
		}
	}
	// The order of the proposals list of APPROVAL.md.
	sort.Slice(proposals, func(i, j int) bool {
		a, b := proposals[i], proposals[j]
		if (a.submitted == "") != (b.submitted == "") {
			return b.submitted == ""
		}
		if a.submitted != b.submitted {
			return a.submitted < b.submitted
		}
		return a.ref < b.ref
	})
	items := Array{}
	found := map[string]bool{}
	for _, p := range proposals {
		for _, decisionRef := range elements(reviewEntry(index, p.ref, false)["decisions"]) {
			decision := get(v, decisionRef, "decision")
			state := h.verdictState(v, index, decision, p.snapshot)
			found[state] = true
			items = append(items, Object{"decision_ref": decisionRef, "decision": decision, "state": state})
		}
	}
	state := verdictUnknown
	for _, s := range []string{verdictCurrent, verdictRejected, verdictStale} {
		if found[s] {
			state = s
			break
		}
	}
	return Object{"cursor": v.Cursor, "approved": state == verdictCurrent, "state": state, "verdicts": items}
}
