package lifecycle

// The policy verdict of src/protocol/policy/POLICY.md.

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"regexp"
)

const (
	policyFormat = "entelos.policy/1"
	// policyVerdictKind is the store kind of the record that holds a policy
	// verdict and its binding to the scope of the Host.
	policyVerdictKind = "policy_verdict"
	// policyProfile is the profile of a policy bundle.
	policyProfile = "cel-typed/v1"
	maxAuditRate  = 1000000
)

// policyInputNames are the names of the policy inputs table, in table order.
var policyInputNames = []string{"report_passed", "change_class", "evidence_human_reviewed", "evidence_model_reviewed",
	"evidence_all_automatic", "evidence_corrected", "measure_max_bound_bp", "measure_gold_digest"}

var (
	policyName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	// policyRule is schema/policy.schema.json of POLICY.md.
	policyRule = evObject([]string{"function", "activation_ref", "snapshot_ref", "bundle_ref", "engine_build", "suspended", "inputs", "audit_rate_ppm"},
		map[string]*evRule{
			"function": {kind: "string", pattern: policyName}, "activation_ref": evDigest, "snapshot_ref": evDigest,
			"bundle_ref": evDigest, "engine_build": evDigest, "suspended": {kind: "boolean"},
			"inputs":         evList(evString(`^[a-z][a-z0-9_]{0,63}$`), 1, 64),
			"audit_rate_ppm": {kind: "integer", low: 0, high: maxAuditRate}})
	nonceText = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// checkPolicyContext checks the Host context fields policy_functions and
// requirements (POLICY.md, Policy). This version permits one policy
// function, and the Host must serve it.
func checkPolicyContext(ctx Object) error {
	if raw, has := ctx["policy_functions"]; has {
		list, ok := raw.(Array)
		valid := ok && len(list) == 1
		for _, x := range list {
			s, isString := x.(string)
			valid = valid && isString && policyName.MatchString(s) && within(s, ctx["functions"])
		}
		if !valid {
			return NewFault("configuration_error", "host policy_functions must be a list of one function name that the host serves", "")
		}
	}
	if raw, has := ctx["requirements"]; has {
		list, ok := raw.(Array)
		seen := Array{}
		for _, x := range list {
			entry, isObject := x.(Object)
			head := lookupAny(entry, "head")
			if !isObject || len(entry) != 2 || !headShape(head) || !withinStrings(entry["requirement"], []string{"human", "policy"}) || within(head, seen) {
				ok = false
				break
			}
			seen = append(seen, head)
		}
		if !ok {
			return NewFault("configuration_error", "host requirements must be a list of distinct heads with the requirement human or policy", "")
		}
	}
	return nil
}

// headShape reports if a value is a head of CONTENT.md: a cel-bundle head
// with function and scope, or a content head with collection and partition.
func headShape(value any) bool {
	head, ok := value.(Object)
	if !ok || len(head) != 3 {
		return false
	}
	fields := []string{"collection", "partition"}
	if head["kind"] == "cel-bundle" {
		fields = []string{"function", "scope"}
	} else if head["kind"] != contentKind {
		return false
	}
	for _, name := range fields {
		if s, isString := head[name].(string); !isString || s == "" {
			return false
		}
	}
	return true
}

// snapshotHead gives the head of the subject of a snapshot.
func snapshotHead(snapshot Object) Object {
	if snapshotKind(snapshot) == contentKind {
		return Object{"kind": contentKind, "collection": item(snapshot, "collection"), "partition": item(snapshot, "partition")}
	}
	return Object{"kind": "cel-bundle", "function": item(snapshot, "function"), "scope": item(snapshot, "scope")}
}

// changesPolicy reports if a head is the head of a policy function.
func changesPolicy(functions Array, head Object) bool {
	return head["kind"] == "cel-bundle" && within(head["function"], functions)
}

// headRequirement gives the verdict requirement of a head. A head with no
// entry has the requirement human.
func headRequirement(requirements Array, head Object) string {
	for _, x := range requirements {
		if entry, _ := x.(Object); equal(entry["head"], head) {
			return text(entry["requirement"])
		}
	}
	return "human"
}

// policyCase is one item on one date, with the values that the Host read.
type policyCase struct {
	date         string
	functions    Array
	requirements Array
	// policy is the policy of the activation that selects the date, or nil.
	policy Object
	// typed is false if the policy bundle is not a cel-typed/v1 rule with a
	// string output. Then rule 5 fails.
	typed bool
	item  Object
	head  Object
	// values are the policy inputs that the Host built. A missing name has
	// no value.
	values Object
	// call evaluates the policy. It gives the output or an error code.
	call func(inputs Object) (output, code any)
	// nonce gives 64 lowercase hex digits for an accept.
	nonce func() string
}

// judgePolicy applies the rules of POLICY.md in their order and gives the
// verdict record. inputs is the input object of the call, or nil if no call
// occurred.
func judgePolicy(c policyCase) (verdict, inputs Object) {
	verdict = Object{"format": policyFormat, "kind": "policy", "outcome": "route", "date": c.date, "item": c.item, "receipt": nil, "audit": nil}
	route := func(reason string) (Object, Object) {
		verdict["reason"] = reason
		return verdict, inputs
	}
	switch {
	case changesPolicy(c.functions, c.head):
		return route("policy_change")
	case headRequirement(c.requirements, c.head) != "policy":
		return route("human_required")
	case c.policy == nil:
		return route("no_policy")
	case c.policy["suspended"] != false:
		return route("policy_suspended")
	case !c.typed || !policyRule.agrees(c.policy) || !within(c.policy["function"], c.functions):
		return route("invalid_policy")
	}
	names := toStrings(c.policy["inputs"].(Array))
	for _, name := range names {
		if !withinStrings(name, policyInputNames) {
			return route("unknown_field")
		}
	}
	inputs = Object{}
	for _, name := range names {
		value, has := c.values[name]
		if !has {
			inputs = nil
			return route("missing_input")
		}
		inputs[name] = value
	}
	output, code := c.call(inputs)
	if _, isString := output.(string); code == nil && !isString {
		output, code = nil, "output_type"
	}
	if code != nil {
		output = nil
	}
	verdict["receipt"] = Object{"policy_function": c.policy["function"], "activation_ref": c.policy["activation_ref"],
		"policy_snapshot_ref": c.policy["snapshot_ref"], "bundle_ref": c.policy["bundle_ref"], "engine_build": c.policy["engine_build"],
		"inputs_ref": InputDigest(inputs), "output": output, "error": code}
	outcome, reason := policyOutcome(output, code)
	verdict["outcome"], verdict["reason"] = outcome, reason
	if outcome == "accept" {
		nonce := c.nonce()
		rate := c.policy["audit_rate_ppm"]
		verdict["audit"] = Object{"rate_ppm": rate, "nonce": nonce, "selected": auditSelected(nonce, rate)}
	}
	return verdict, inputs
}

// policyOutcome applies rules 8 and 9 to the result of a call.
func policyOutcome(output, code any) (outcome, reason string) {
	switch {
	case code != nil:
		return "route", "policy_error"
	case withinStrings(output, []string{"accept", "refuse", "route"}):
		return output.(string), "policy_output"
	}
	return "route", "invalid_output"
}

// auditSelected applies the draw of POLICY.md, Audit sample: the first 4
// bytes of the nonce, as an unsigned big-endian integer, select the accept
// if draw * 1000000 < rate * 2^32. It uses integers only.
func auditSelected(nonce string, rate any) bool {
	raw, err := hex.DecodeString(nonce)
	n, ok := evidenceInteger(rate)
	if err != nil || len(raw) != 32 || !ok || n < 0 || n > maxAuditRate {
		return false
	}
	draw := uint64(binary.BigEndian.Uint32(raw[:4]))
	return draw*maxAuditRate < uint64(n)<<32
}

// newNonce gives 32 bytes from a cryptographic random source, as 64
// lowercase hex digits.
func newNonce() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}

// verdictRequirement applies the rules of POLICY.md, Verdict requirement.
// A verdict is {"kind": "human", "disposition"} for a decide record, or
// {"kind": "policy", "outcome"} for a policy verdict.
func verdictRequirement(requirement string, changes bool, verdicts []Object) string {
	if changes {
		requirement = "human"
	}
	var remaining []Object
	for _, v := range verdicts {
		if v["kind"] == "human" || requirement == "policy" {
			remaining = append(remaining, v)
		}
	}
	for _, v := range remaining {
		if v["disposition"] == "reject" || v["outcome"] == "refuse" {
			return "rejected"
		}
	}
	for _, v := range remaining {
		if v["disposition"] == "accept" || v["outcome"] == "accept" {
			return "satisfied"
		}
	}
	return "awaiting_human"
}

// replayPolicy applies the replay checks of POLICY.md to a stored verdict.
// stored is nil if the store has no policy snapshot of the receipt. call is
// the call of the replay.
func replayPolicy(verdict, inputs, stored Object, build string, call func(Object) (any, any)) Object {
	receipt, _ := verdict["receipt"].(Object)
	switch {
	case receipt == nil:
		return Object{"replay": "refused", "code": "no_receipt"}
	case inputs == nil || InputDigest(inputs) != receipt["inputs_ref"]:
		return Object{"replay": "refused", "code": "input_mismatch"}
	case stored == nil || !equal(stored["bundle_ref"], receipt["bundle_ref"]):
		return Object{"replay": "refused", "code": "artifact_unavailable"}
	case build != receipt["engine_build"]:
		return Object{"replay": "refused", "code": "implementation_unavailable"}
	}
	output, code := call(inputs)
	if _, isString := output.(string); code == nil && !isString {
		output, code = nil, "output_type"
	}
	if code != nil {
		output = nil
	}
	outcome, _ := policyOutcome(output, code)
	same := outcome == verdict["outcome"] && equal(output, receipt["output"]) && equal(code, receipt["error"])
	if audit, has := verdict["audit"].(Object); same && has {
		same = auditSelected(text(lookup(audit, "nonce", "")), audit["rate_ppm"]) == audit["selected"]
	}
	if same {
		return Object{"replay": "same"}
	}
	return Object{"replay": "differs", "outcome": outcome}
}

// isPolicyDecision reports if a decision record is the decision of a policy
// verdict.
func isPolicyDecision(decision Object) bool {
	_, has := decision["policy_verdict_ref"]
	return has
}

// requirementOf gives the requirement of the snapshot of a decision, after
// rule 1 of the requirement rules.
func (h *Host) requirementOf(snapshot Object) string {
	functions, _ := h.Context["policy_functions"].(Array)
	requirements, _ := h.Context["requirements"].(Array)
	head := snapshotHead(snapshot)
	if changesPolicy(functions, head) {
		return "human"
	}
	return headRequirement(requirements, head)
}

// ignored reports if the requirement rules ignore a decision: a decision of
// a policy verdict on a snapshot whose requirement is human (rule 2).
func (h *Host) ignored(v *View, decision Object) bool {
	return isPolicyDecision(decision) && h.requirementOf(get(v, item(decision, "snapshot_ref"), "snapshot")) == "human"
}

// checkRequirement applies the requirement rules to the verdicts of the
// snapshot of an activated decision.
func (h *Host) checkRequirement(v *View, index Object, decision, snapshot Object) {
	var verdicts []Object
	for _, ref := range elements(reviewEntry(index, text(item(decision, "snapshot_ref")), false)["decisions"]) {
		d := get(v, ref, "decision")
		if isPolicyDecision(d) {
			outcome := "accept"
			if d["disposition"] == "reject" {
				outcome = "refuse"
			}
			verdicts = append(verdicts, Object{"kind": "policy", "outcome": outcome})
		} else {
			verdicts = append(verdicts, Object{"kind": "human", "disposition": d["disposition"]})
		}
	}
	functions, _ := h.Context["policy_functions"].(Array)
	requirements, _ := h.Context["requirements"].(Array)
	head := snapshotHead(snapshot)
	switch verdictRequirement(headRequirement(requirements, head), changesPolicy(functions, head), verdicts) {
	case "rejected":
		fail("proposal_rejected", "a verdict on this snapshot is a reject or a policy refuse", "admission")
	case "awaiting_human":
		fail("human_verdict_required", "the verdict requirement of this head needs a human verdict", "admission")
	}
	if h.ignored(v, decision) {
		fail("human_verdict_required", "a policy verdict does not satisfy the human verdict requirement of this head", "admission")
	}
}

// recoverFault runs f and gives the code of a *Fault that f raises, or
// invalid_request for a malformed value. It gives "" if f returns.
func recoverFault(f func()) (code string) {
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case *Fault:
				code = e.Code
			case *requestError:
				code = "invalid_request"
			default:
				panic(r)
			}
		}
	}()
	f()
	return ""
}

// policyAt gives the policy of POLICY.md for a date: the policy of the
// activation that selects the date, or of the enable event that a suspend
// masks at the date. It gives nil if no activation selects the date. typed
// is false if the bundle is not a typed rule with a string output.
func (h *Host) policyAt(v *View, date string) (policy, event Object, typed bool) {
	functions, _ := h.Context["policy_functions"].(Array)
	if len(functions) == 0 {
		return nil, nil, false
	}
	function := functions[0]
	var order []any
	var active map[any]Object
	if recoverFault(func() { order, active = h.live(v, function, date) }) != "" {
		return nil, nil, false
	}
	suspended := false
	if len(active) > 0 {
		event = active[order[0]]
	} else if event = h.suspendedAt(v, function, date); event != nil {
		suspended = true
	} else {
		return nil, nil, false
	}
	policy = Object{"function": function, "activation_ref": event["ref"], "snapshot_ref": event["snapshot_ref"], "suspended": suspended}
	recoverFault(func() {
		snapshot := h.scoped(v, event["snapshot_ref"], "snapshot")
		policy["bundle_ref"], policy["engine_build"] = snapshot["bundle_ref"], snapshot["engine_build"]
		files := h.files(v, snapshot)
		decoded := func(name string) Object {
			value, err := Decode(files[name])
			o, _ := value.(Object)
			if err != nil || o == nil {
				fail("invalid_policy", "the policy "+name+" is not a JSON object", "")
			}
			return o
		}
		manifest, env, fixed := decoded("manifest"), decoded("environment"), decoded("fixed")
		policy["inputs"] = manifest["caller_variables"]
		if rate, has := fixed["audit_rate_ppm"]; has {
			policy["audit_rate_ppm"] = rate
		}
		typed = manifest["profile"] == policyProfile && equal(env["output"], Object{"type": "string"})
	})
	return policy, event, typed
}

// suspendedAt gives an enable event of a function whose interval covers the
// date and that a suspend event masks at the date, or nil.
func (h *Host) suspendedAt(v *View, function any, date string) Object {
	probe := Object{"function": function, "scope": h.ctx("scope")}
	events := h.events(v)
	for _, suspend := range events {
		if !equal(suspend["kind"], "suspend") || !same(suspend, probe) || !lessEqual(suspend["from"], date) || !less(date, suspend["until"]) {
			continue
		}
		for _, enable := range events {
			if equal(enable["kind"], "enable") && same(enable, probe) && within(enable["ref"], suspend["targets"]) &&
				lessEqual(enable["from"], date) && less(date, enable["until"]) {
				return enable
			}
		}
	}
	return nil
}

// policyValues builds the policy inputs of an item (POLICY.md, Policy
// inputs). A name without a value is missing.
func (h *Host) policyValues(v *View, reportRef string, report, snapshot Object) Object {
	values := Object{"report_passed": h.reportRefusal(v, reportRef, false) == ""}
	if class, ok := report["change_class"].(string); ok {
		values["change_class"] = class
	}
	if doc, has := snapshot["evidence"]; has {
		if recoverFault(func() { checkSnapshotEvidence(snapshot) }) == "" {
			if inputs, code, _ := admitEvidence(doc); code == "" {
				for k, x := range inputs {
					values[k] = x
				}
			}
		}
	}
	candidate := Object{"kind": snapshotKind(snapshot)}
	if snapshotKind(snapshot) == contentKind {
		candidate["manifest"] = string(getRaw(v, item(snapshot, "manifest_ref"), contentMime))
	}
	collections, _ := h.Context["promotion_collections"].(Array)
	if inputs, rule := judgePromotion(collections, candidate, report); rule == "" {
		for k, x := range inputs {
			values[k] = x
		}
	}
	return values
}

// policyDecide is the write policy_decide of POLICY.md. The Host builds the
// inputs, calls the active policy and writes the verdict. An accept or a
// refuse also gives a decision record of the kind policy, so that activate
// and the verdict rules of VERDICT.md and KINDS.md read it.
func (h *Host) policyDecide(v *View, q Object) Object {
	index := reviews(v)
	reportRef := text(item(q, "report_ref"))
	report := h.scoped(v, reportRef, "qualification")
	snapshotRef := text(item(report, "snapshot_ref"))
	snapshot := h.scoped(v, snapshotRef, "snapshot")
	if _, head := h.headOf(v, snapshot); head == nil || !equal(candidateRef(head), candidateRef(snapshot)) {
		fail("stale_verdict", "the head of this subject has other content than this snapshot", "admission")
	}
	entry := reviewEntry(index, snapshotRef, true)
	if hasReject(h.verdicts(v, entry)) {
		fail("proposal_rejected", "a verdict on this snapshot is a reject", "admission")
	}
	for _, ref := range elements(entry["decisions"]) {
		if isPolicyDecision(get(v, ref, "decision")) {
			fail("duplicate_verdict", "this snapshot already has a policy accept or refuse", "admission")
		}
	}
	now := h.now()
	date := now[:10]
	functions, _ := h.Context["policy_functions"].(Array)
	requirements, _ := h.Context["requirements"].(Array)
	policy, event, typed := h.policyAt(v, date)
	receiptRef := ""
	c := policyCase{date: date, functions: functions, requirements: requirements, policy: policy, typed: typed,
		item:   Object{"snapshot_ref": snapshotRef, "report_ref": reportRef, "candidate": candidateRef(snapshot)},
		head:   snapshotHead(snapshot),
		values: h.policyValues(v, reportRef, report, snapshot), nonce: newNonce}
	c.call = func(inputs Object) (output, code any) {
		var actual any
		if failed := recoverFault(func() {
			out := h.receipt(v, h.selection(v, policy["function"], date, event), Encode(inputs), q["request_key"], "invoke", "", "", "", "")
			receiptRef, actual = text(out["receipt_ref"]), out["actual"]
		}); failed != "" {
			return nil, failed
		}
		if status := lookupAny(actual, "status"); status != "ok" {
			if s, ok := status.(string); ok && s != "" {
				return nil, s
			}
			return nil, "runtime_error"
		}
		return lookupAny(actual, "value"), nil
	}
	verdict, _ := judgePolicy(c)
	record := merge(pick(snapshot, interval4...), Object{"snapshot_ref": snapshotRef, "report_ref": reportRef, "verdict": verdict,
		"requester": h.ctx("actor"), "decided_at": now})
	if receiptRef != "" {
		record["receipt_ref"] = receiptRef
	}
	ref := v.Put(policyVerdictKind, record)
	out := Object{"policy_verdict_ref": ref, "verdict": verdict}
	if outcome := verdict["outcome"]; outcome != "route" {
		disposition := "accept"
		if outcome == "refuse" {
			disposition = "reject"
		}
		actor := text(policy["function"])
		h.checkVerdictKinds(v, index, actor, "policy", snapshot)
		decision := merge(pick(snapshot, interval4...), Object{"snapshot_ref": snapshotRef, "report_ref": reportRef, "kind": "policy",
			"actor": actor, "reason": verdict["reason"], "disposition": disposition, "runner_policy": h.ctx("runner_actor"),
			"decided_at": now, "policy_verdict_ref": ref})
		decisionRef := v.Put("decision", decision)
		appendEntry(entry, "decisions", decisionRef)
		out["decision_ref"] = decisionRef
	}
	return out
}

// policyReplay is the read policy_replay of POLICY.md, Replay. It calls the
// policy again on the stored inputs and writes nothing.
func (h *Host) policyReplay(v *View, q Object) Object {
	ref := text(item(q, "policy_verdict_ref"))
	record := h.scoped(v, ref, policyVerdictKind)
	verdict := object(item(record, "verdict"))
	receipt, _ := verdict["receipt"].(Object)
	var inputs, stored Object
	var files map[string][]byte
	if receipt != nil {
		recoverFault(func() {
			value, err := Decode(getRaw(v, receipt["inputs_ref"], "application/json"))
			if err == nil {
				inputs, _ = value.(Object)
			}
		})
		recoverFault(func() {
			snapshot := h.scoped(v, receipt["policy_snapshot_ref"], "snapshot")
			files = h.files(v, snapshot)
			stored = Object{"snapshot_ref": receipt["policy_snapshot_ref"], "bundle_ref": snapshot["bundle_ref"]}
		})
	}
	build := must(h.Runtime.Build())
	result := replayPolicy(verdict, inputs, stored, build, func(inputs Object) (any, any) {
		var actual any
		if failed := recoverFault(func() { actual = must(h.Runtime.Execute(files, "run", Encode(inputs), nil)) }); failed != "" {
			return nil, failed
		}
		if !equal(lookupAny(actual, "bundle_ref"), receipt["bundle_ref"]) || !equal(lookupAny(actual, "implementation_ref"), build) {
			return nil, "artifact_mismatch"
		}
		if status := lookupAny(actual, "status"); status != "ok" {
			if s, ok := status.(string); ok && s != "" {
				return nil, s
			}
			return nil, "runtime_error"
		}
		return lookupAny(actual, "value"), nil
	})
	return merge(result, Object{"cursor": v.Cursor, "policy_verdict_ref": ref})
}
