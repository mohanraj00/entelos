package lifecycle

// The approval API 1.1 of src/protocol/approval/APPROVAL.md: the clock port,
// the review index, the proposal states and the two approval reads.

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TimeLayout is the clock format of APPROVAL.md: UTC, whole seconds.
const TimeLayout = "2006-01-02T15:04:05Z"

// Clock is the time port of the Host. The Host records the time on a new
// snapshot and a new decision, and returns it on an approval read. The time
// selects nothing and refuses nothing.
type Clock interface {
	Now() (string, error)
}

// SystemClock is the UTC system clock in whole seconds.
type SystemClock struct{}

// Now gives the current UTC time.
func (SystemClock) Now() (string, error) { return time.Now().UTC().Format(TimeLayout), nil }

// FileClock reads one time value from a file for each call. It is for
// conformance runs only. A test runner writes the file before each request.
type FileClock struct{ Path string }

// Now gives the time value in the file.
func (c FileClock) Now() (string, error) {
	raw, err := os.ReadFile(c.Path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(string(raw), "\n")
	if err := CheckTime(value); err != nil {
		return "", err
	}
	return value, nil
}

// CheckTime accepts only the exact clock format.
func CheckTime(value string) error {
	t, err := time.Parse(TimeLayout, value)
	if err != nil || t.Format(TimeLayout) != value {
		return errors.New("time must have the form YYYY-MM-DDTHH:MM:SSZ")
	}
	return nil
}

func (h *Host) now() string {
	clock := h.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	return must(clock.Now())
}

// RoleNames gives each Host role name, sorted.
func RoleNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, spec := range schema {
		if !seen[spec.role] {
			seen[spec.role] = true
			out = append(out, spec.role)
		}
	}
	sort.Strings(out)
	return out
}

// As gives a Host for one principal. The copy shares the store, the runtime,
// the runner and the clock. Only the actor and the roles change.
func (h *Host) As(actor string, roles []string) *Host {
	ctx := copyObject(h.Context)
	list := Array{}
	for _, r := range roles {
		list = append(list, r)
	}
	ctx["actor"], ctx["roles"] = actor, list
	other := *h
	other.Context = ctx
	return &other
}

// HandleAs runs one request as one principal.
func (h *Host) HandleAs(actor string, roles []string, request any) (any, error) {
	return h.As(actor, roles).Handle(request)
}

func headKey(function, scope any) string {
	return strings.TrimSpace(string(Encode(Array{function, scope})))
}

// prepareIndex gives the empty store its review index before its first
// write. A store with history and without the index stays without it.
func prepareIndex(v *View) {
	if _, has := v.State["reviews"]; !has && v.Cursor == "" {
		v.State["reviews"] = Object{}
	}
}

// reviews gives the review index, or refuses on a store without it.
func reviews(v *View) Object {
	prepareIndex(v)
	index, ok := v.State["reviews"].(Object)
	if !ok {
		fail("index_unavailable", "this store has no review index; it cannot get a new report, verdict or activation", "store")
	}
	return index
}

// reviewEntry gives the index entry of a snapshot. With add, it makes a
// missing entry.
func reviewEntry(index Object, snapshotRef string, add bool) Object {
	entry, ok := index[snapshotRef].(Object)
	if !ok {
		entry = Object{"reports": Array{}, "decisions": Array{}}
		if add {
			index[snapshotRef] = entry
		}
	}
	return entry
}

func appendEntry(entry Object, field, ref string) {
	entry[field] = append(stringList(entry[field]), ref)
}

func (h *Host) verdicts(v *View, entry Object) []Object {
	var out []Object
	for _, ref := range elements(entry["decisions"]) {
		out = append(out, get(v, ref, "decision"))
	}
	return out
}

func hasReject(decisions []Object) bool {
	for _, d := range decisions {
		if equal(item(d, "disposition"), "reject") {
			return true
		}
	}
	return false
}

// reportRefusal is the code of the first accept check of decide that fails
// among checks 1, 2 and 6, or an empty string.
func (h *Host) reportRefusal(v *View, reportRef any, rejected bool) (code string) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				fault, ok := r.(*Fault)
				if !ok {
					panic(r)
				}
				code = fault.Code
			}
		}()
		h.reportValid(v, reportRef)
	}()
	if code == "" && rejected {
		code = "proposal_rejected"
	}
	return code
}

// proposalItem gives the proposal item of APPROVAL.md for one snapshot.
func (h *Host) proposalItem(v *View, index Object, snapshotRef string, snapshot Object, asOf string) Object {
	entry := reviewEntry(index, snapshotRef, false)
	decisions := h.verdicts(v, entry)
	reports := elements(entry["reports"])
	rejected := hasReject(decisions)
	head, _ := object(item(v.State, "heads"))[headKey(item(snapshot, "function"), item(snapshot, "scope"))].(string)
	state, refusal := "", ""
	switch {
	case rejected:
		state = "rejected"
	case len(decisions) > 0:
		state = "accepted"
	case head != snapshotRef:
		state = "superseded"
	case len(reports) == 0:
		state = "awaiting_qualification"
	default:
		if refusal = h.reportRefusal(v, reports[len(reports)-1], false); refusal == "" {
			state = "awaiting_verdict"
		} else {
			state = "not_approvable"
		}
	}
	out := pick(snapshot, "function", "scope", "from", "until", "producer", "reason", "bundle_ref")
	out["snapshot_ref"] = snapshotRef
	out["stale_mode"] = lookup(snapshot, "stale_mode", failClosed)
	out["state"], out["refusal"] = state, refusal
	out["report_refs"] = append(Array{}, reports...)
	out["decision_refs"] = append(Array{}, elements(entry["decisions"])...)
	if submitted, ok := snapshot["submitted_at"].(string); ok {
		out["submitted_at"] = submitted
		from, err1 := time.Parse(TimeLayout, submitted)
		to, err2 := time.Parse(TimeLayout, asOf)
		if err1 != nil || err2 != nil {
			fail("invalid_record", "snapshot submitted_at is not a clock value", "store")
		}
		out["wait_seconds"] = Int{strconv.FormatInt(int64(to.Sub(from)/time.Second), 10)}
	}
	return out
}

// proposalOrder is by submitted_at, the oldest first, then by snapshot_ref.
// An item without submitted_at comes last.
func proposalOrder(items []Object) {
	sort.SliceStable(items, func(i, j int) bool {
		a, hasA := items[i]["submitted_at"].(string)
		b, hasB := items[j]["submitted_at"].(string)
		if hasA != hasB {
			return hasA
		}
		if a != b {
			return a < b
		}
		return items[i]["snapshot_ref"].(string) < items[j]["snapshot_ref"].(string)
	})
}

func (h *Host) proposals(v *View) Object {
	index := reviews(v)
	asOf := h.now()
	items := []Object{}
	for _, raw := range object(item(v.State, "heads")) {
		ref, ok := raw.(string)
		if !ok || ref == "" {
			continue
		}
		snapshot := get(v, ref, "snapshot")
		if !equal(item(snapshot, "scope"), h.ctx("scope")) || !within(item(snapshot, "function"), h.ctxList("functions")) {
			continue
		}
		x := h.proposalItem(v, index, ref, snapshot, asOf)
		if s := x["state"]; s == "awaiting_qualification" || s == "awaiting_verdict" || s == "not_approvable" {
			items = append(items, x)
		}
	}
	proposalOrder(items)
	list := Array{}
	for _, x := range items {
		list = append(list, x)
	}
	return Object{"cursor": v.Cursor, "as_of": asOf, "proposals": list}
}

func (h *Host) proposal(v *View, q Object) Object {
	index := reviews(v)
	ref := text(item(q, "snapshot_ref"))
	snapshot := h.scoped(v, ref, "snapshot")
	asOf := h.now()
	entry := reviewEntry(index, ref, false)
	decisions := h.verdicts(v, entry)
	rejected := hasReject(decisions)
	bundle := Object{}
	for k, raw := range h.files(v, snapshot) {
		bundle[k] = string(raw)
	}
	reports := Array{}
	for _, reportRef := range elements(entry["reports"]) {
		report := get(v, reportRef, "qualification")
		reports = append(reports, Object{"report_ref": reportRef, "report": report, "plan_ref": item(report, "plan_ref"),
			"plan": get(v, item(report, "plan_ref"), "plan"), "refusal": h.reportRefusal(v, reportRef, rejected)})
	}
	verdicts := Array{}
	for i, decisionRef := range elements(entry["decisions"]) {
		verdicts = append(verdicts, Object{"decision_ref": decisionRef, "decision": decisions[i]})
	}
	return Object{"cursor": v.Cursor, "as_of": asOf, "proposal": h.proposalItem(v, index, ref, snapshot, asOf),
		"snapshot": snapshot, "source": get(v, item(snapshot, "source_ref"), "source"), "bundle": bundle,
		"reports": reports, "decisions": verdicts}
}

// decide records an accept or a reject verdict in the order of checks of
// APPROVAL.md.
func (h *Host) decide(v *View, q Object) Object {
	index := reviews(v)
	actor := h.ctx("actor")
	disposition := lookup(q, "disposition", "accept")
	if !within(disposition, Array{"accept", "reject"}) {
		fail("invalid_request", "disposition must be accept or reject", "")
	}
	var report, snapshot Object
	if disposition == "accept" {
		report, snapshot = h.reportValid(v, item(q, "report_ref"))
	} else {
		report = h.scoped(v, item(q, "report_ref"), "qualification")
		snapshot = h.scoped(v, item(report, "snapshot_ref"), "snapshot")
	}
	if !truthy(item(q, "reason")) {
		fail("invalid_request", "admission reason required", "")
	}
	if equal(actor, item(snapshot, "producer")) || equal(actor, item(report, "runner")) {
		fail("separation_required", "reviewer must differ from producer and runner", "admission")
	}
	snapshotRef := text(item(report, "snapshot_ref"))
	entry := reviewEntry(index, snapshotRef, true)
	decisions := h.verdicts(v, entry)
	for _, d := range decisions {
		if equal(item(d, "actor"), actor) {
			fail("duplicate_verdict", "this actor already has a verdict on this snapshot", "admission")
		}
	}
	if disposition == "accept" && hasReject(decisions) {
		fail("proposal_rejected", "a verdict on this snapshot is a reject", "admission")
	}
	record := merge(pick(snapshot, interval4...), Object{"snapshot_ref": snapshotRef, "report_ref": q["report_ref"], "actor": actor,
		"reason": q["reason"], "disposition": disposition, "runner_policy": h.ctx("runner_actor"), "decided_at": h.now()})
	ref := v.Put("decision", record)
	appendEntry(entry, "decisions", ref)
	return Object{"decision_ref": ref, "decision": record}
}
