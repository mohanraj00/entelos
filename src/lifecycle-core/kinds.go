package lifecycle

// The verdict kinds of src/protocol/verdict/KINDS.md.

import (
	"math/big"
	"sort"
	"strings"
)

// verdictKinds is the closed set of verdict kinds, in the order of the
// counts.
var verdictKinds = []string{"human", "model", "policy"}

// maxVerdictCount is the largest value of the verdict policy.
const maxVerdictCount = 16

// checkKindContext checks the Host context fields actor_kinds and
// verdict_policy (KINDS.md, Host context).
func checkKindContext(ctx Object) error {
	if raw, has := ctx["actor_kinds"]; has {
		kinds, ok := raw.(Object)
		if !ok {
			return NewFault("configuration_error", "host actor_kinds must be an object", "")
		}
		for actor, kind := range kinds {
			if actor == "" || !withinStrings(kind, verdictKinds) {
				return NewFault("configuration_error", "host actor_kinds must map a nonempty actor to human, model or policy", "")
			}
		}
	}
	if raw, has := ctx["verdict_policy"]; has {
		policy, ok := raw.(Object)
		if !ok {
			return NewFault("configuration_error", "host verdict_policy must be an object", "")
		}
		for kind, count := range policy {
			if _, valid := policyCount(count); !withinStrings(kind, verdictKinds) || !valid {
				return NewFault("configuration_error", "host verdict_policy must map human, model or policy to an integer from 0 to 16", "")
			}
		}
	}
	return nil
}

// policyCount gives the value of an integer from 0 to 16. A number with a
// fraction, a string or a boolean is not valid.
func policyCount(value any) (int, bool) {
	switch value.(type) {
	case Int, int:
	default:
		return 0, false
	}
	n, ok := number(value)
	if !ok || !n.IsInt() || n.Sign() < 0 || n.Cmp(big.NewRat(maxVerdictCount, 1)) > 0 {
		return 0, false
	}
	return int(n.Num().Int64()), true
}

// kindOf gives the kind that actor_kinds states for an actor, or "".
func (h *Host) kindOf(actor string) string {
	kinds, _ := h.Context["actor_kinds"].(Object)
	kind, _ := kinds[actor].(string)
	return kind
}

// required gives the verdict policy, with 0 for a missing kind.
func (h *Host) required() Object {
	policy, _ := h.Context["verdict_policy"].(Object)
	out := Object{}
	for _, kind := range verdictKinds {
		n, _ := policyCount(policy[kind])
		out[kind] = n
	}
	return out
}

// lacking gives the kinds whose count is less than the verdict policy.
func lacking(counts, required Object) []string {
	var out []string
	for _, kind := range verdictKinds {
		if counts[kind].(int) < required[kind].(int) {
			out = append(out, kind)
		}
	}
	return out
}

// subjectSnapshot is a snapshot of one subject with one candidate reference.
type subjectSnapshot struct {
	ref, submitted string
	snapshot       Object
}

// sameCandidate gives the snapshots of a subject in the Host scope whose
// candidate reference is the stated reference, in the order of the
// proposals list of APPROVAL.md.
func (h *Host) sameCandidate(v *View, index Object, function any, candidate Object) []subjectSnapshot {
	var out []subjectSnapshot
	for snapshotRef := range index {
		// The index covers the whole store. Skip the snapshots of other
		// scopes and other subjects before the candidate check, as the
		// proposals list does.
		snapshot := get(v, snapshotRef, "snapshot")
		if !equal(item(snapshot, "scope"), h.ctx("scope")) || !equal(snapshot["function"], function) {
			continue
		}
		if equal(candidateRef(snapshot), candidate) {
			submitted, _ := snapshot["submitted_at"].(string)
			out = append(out, subjectSnapshot{snapshotRef, submitted, snapshot})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.submitted == "") != (b.submitted == "") {
			return b.submitted == ""
		}
		if a.submitted != b.submitted {
			return a.submitted < b.submitted
		}
		return a.ref < b.ref
	})
	return out
}

// countKinds gives the count of each kind: the number of distinct actors of
// that kind with a current verdict. A verdict without a kind counts for no
// kind (KINDS.md decision 3).
func countKinds(current []Object) Object {
	actors := map[string]map[string]bool{}
	for _, decision := range current {
		kind, _ := lookup(decision, "kind", "").(string)
		if !withinStrings(kind, verdictKinds) {
			continue
		}
		if actors[kind] == nil {
			actors[kind] = map[string]bool{}
		}
		actors[kind][text(item(decision, "actor"))] = true
	}
	out := Object{}
	for _, kind := range verdictKinds {
		out[kind] = len(actors[kind])
	}
	return out
}

// candidateCounts gives the counts of a candidate reference of a subject at
// the view (KINDS.md, Counts).
func (h *Host) candidateCounts(v *View, index Object, function any, candidate Object) Object {
	var current []Object
	for _, s := range h.sameCandidate(v, index, function, candidate) {
		for _, decisionRef := range elements(reviewEntry(index, s.ref, false)["decisions"]) {
			decision := get(v, decisionRef, "decision")
			if h.verdictState(v, index, decision, s.snapshot) == verdictCurrent {
				current = append(current, decision)
			}
		}
	}
	return countKinds(current)
}

// checkVerdictKinds refuses a verdict when the actor has a verdict of
// another kind, or no kind, on the same content (KINDS.md, Separation).
func (h *Host) checkVerdictKinds(v *View, index Object, actor, kind string, snapshot Object) {
	for _, s := range h.sameCandidate(v, index, item(snapshot, "function"), candidateRef(snapshot)) {
		for _, d := range h.verdicts(v, reviewEntry(index, s.ref, false)) {
			earlier, _ := lookup(d, "kind", "").(string)
			if equal(item(d, "actor"), actor) && earlier != kind {
				fail("separation_required", "this actor has a verdict of another kind on the same content", "admission")
			}
		}
	}
}

// checkVerdictPolicy refuses an activation when a count is less than the
// verdict policy (KINDS.md, Changes to activate).
func (h *Host) checkVerdictPolicy(v *View, index Object, snapshot Object) {
	counts := h.candidateCounts(v, index, item(snapshot, "function"), candidateRef(snapshot))
	if missing := lacking(counts, h.required()); len(missing) > 0 {
		fail("verdicts_required", "too few current verdicts of the kind "+strings.Join(missing, ", "), "admission")
	}
}
