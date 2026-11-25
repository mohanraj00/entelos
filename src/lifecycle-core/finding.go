package lifecycle

// The finding import of FINDING.md. A finding record of another tool becomes
// a candidate qualification case. A reviewer decides on each candidate, and
// only a plan can give an accepted candidate to the runner.

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	findingName       = "nookku.finding"
	findingMajor      = "1"
	findingCaseFormat = "entelos.finding-case/1"
	// maxFindingBytes is the bound of the text of one finding record.
	maxFindingBytes  = 256 << 10
	kindCandidate    = "case_candidate"
	kindCaseDecision = "case_decision"
	findingMismatch  = "answer_key_mismatch"
	findingIssue     = "issue_row"
	maxFindingTurn   = 999_999_999
	maxSubjectLength = 512
)

// The reader rules of FINDING.md, in their order.
const (
	ruleTooLarge     = "too_large"
	ruleNotJSON      = "not_json"
	ruleNotFinding   = "not_finding"
	ruleVersionForm  = "version_form"
	ruleVersionMajor = "version_major"
	ruleMissingField = "missing_field"
	ruleMissingKey   = "missing_key"
	ruleFieldValue   = "field_value"
	ruleTurn         = "turn_not_in_script"
	ruleSealDigest   = "seal_digest"
	ruleSealText     = "seal_text"
	ruleSealCoverage = "seal_coverage"
	ruleSealSources  = "seal_sources"
)

var (
	findingVersion   = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
	findingSHA       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	findingTolerance = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,9})?$`)
	findingDecimal   = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
	// findingFields are the fields that each record of major version 1 has.
	findingFields = []string{"record", "version", "id", "kind", "test", "script", "turn", "expected", "observed", "issue",
		"adjudication", "subject", "gate", "seal", "nookku"}
	// findingKeys are the keys that each object of a record has.
	findingKeys = map[string][]string{
		"script":       {"messages", "sha256"},
		"turn":         {"number", "input", "reply"},
		"expected":     {"value", "match", "tolerance", "field"},
		"issue":        {"class", "evidence", "text"},
		"adjudication": {"outcome", "reason", "author", "author_type", "ts"},
		"gate":         {"id", "name", "result_sha256"},
		"seal":         {"sha256", "text", "files"},
	}
	// findingSources are the sealed files that a record of each kind names.
	findingSources = map[bool][]string{
		true:  {"manifest.json", "tap.jsonl", "findings.json", "answer_key.json", "adjudications.jsonl"},
		false: {"manifest.json", "tap.jsonl", "evaluation.json"},
	}
)

// findingText is true for a string with a character that is not white space.
// The white space set is the set of the writer of the record.
func findingText(value any) bool {
	s, ok := value.(string)
	return ok && strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }) != ""
}

func findingDigest(value any) bool {
	s, ok := value.(string)
	return ok && findingSHA.MatchString(s)
}

// findingObject gives an object of a record, or the rule that refuses it.
// A key with the value null is a key.
func findingObject(value any, field string) (Object, string) {
	o, ok := value.(Object)
	if !ok {
		return nil, ruleFieldValue
	}
	for _, key := range findingKeys[field] {
		if _, has := o[key]; !has {
			return nil, ruleMissingKey
		}
	}
	return o, ""
}

// readFinding applies the reader rules of FINDING.md to the exact text of
// one finding record. It gives the record, or the first rule that refuses
// the text. It ignores each field that the rules do not name.
func readFinding(raw []byte) (Object, string) {
	if len(raw) > maxFindingBytes {
		return nil, ruleTooLarge
	}
	value, err := Decode(raw)
	record, ok := value.(Object)
	if err != nil || !ok {
		return nil, ruleNotJSON
	}
	if record["record"] != findingName {
		return nil, ruleNotFinding
	}
	version, _ := record["version"].(string)
	parts := findingVersion.FindStringSubmatch(version)
	if parts == nil {
		return nil, ruleVersionForm
	}
	if parts[1] != findingMajor {
		return nil, ruleVersionMajor
	}
	for _, name := range findingFields {
		if _, has := record[name]; !has {
			return nil, ruleMissingField
		}
	}
	kind := record["kind"]
	if kind != findingMismatch && kind != findingIssue {
		return nil, ruleFieldValue
	}
	mismatch := kind == findingMismatch
	for _, name := range []string{"id", "test", "nookku"} {
		if !findingText(record[name]) {
			return nil, ruleFieldValue
		}
	}
	script, rule := findingObject(record["script"], "script")
	if rule != "" {
		return nil, rule
	}
	messages, ok := script["messages"].(Array)
	if !ok || len(messages) == 0 {
		return nil, ruleFieldValue
	}
	for _, m := range messages {
		if _, ok := m.(string); !ok {
			return nil, ruleFieldValue
		}
	}
	if script["sha256"] != nil && !findingDigest(script["sha256"]) {
		return nil, ruleFieldValue
	}
	for _, rule := range []string{turnRule(record["turn"], mismatch, messages), expectedRule(record["expected"], mismatch),
		observedRule(record["observed"], mismatch), issueRule(record["issue"], mismatch),
		adjudicationRule(record["adjudication"], mismatch), subjectRule(record["subject"]), gateRule(record["gate"]),
		sealRule(record, script, mismatch)} {
		if rule != "" {
			return nil, rule
		}
	}
	return record, ""
}

// turnNumber gives the turn of a record, or 0 for a value that is not a
// whole number from 1 to maxFindingTurn.
func turnNumber(value any) int {
	n, ok := value.(Int)
	if !ok || len(n.Text) > 9 {
		return 0
	}
	number, err := strconv.Atoi(n.Text)
	if err != nil || number < 1 || number > maxFindingTurn {
		return 0
	}
	return number
}

func turnRule(value any, mismatch bool, messages Array) string {
	if value == nil {
		if mismatch {
			return ruleFieldValue
		}
		return ""
	}
	turn, rule := findingObject(value, "turn")
	if rule != "" {
		return rule
	}
	number := turnNumber(turn["number"])
	if number == 0 {
		return ruleFieldValue
	}
	if number > len(messages) || turn["input"] != messages[number-1] {
		return ruleTurn
	}
	if _, ok := turn["reply"].(string); !ok && turn["reply"] != nil {
		return ruleFieldValue
	}
	return ""
}

func expectedRule(value any, mismatch bool) string {
	if !mismatch {
		if value != nil {
			return ruleFieldValue
		}
		return ""
	}
	expected, rule := findingObject(value, "expected")
	if rule != "" {
		return rule
	}
	match, tolerance := expected["match"], expected["tolerance"]
	answer, ok := expected["value"].(string)
	if !ok || (match != "exact" && match != "number") {
		return ruleFieldValue
	}
	if match == "number" {
		if t, ok := tolerance.(string); !ok || !findingTolerance.MatchString(t) || !findingDecimal.MatchString(answer) {
			return ruleFieldValue
		}
	} else if tolerance != nil {
		return ruleFieldValue
	}
	if _, ok := expected["field"].(string); !ok {
		return ruleFieldValue
	}
	return ""
}

func observedRule(value any, mismatch bool) string {
	if _, ok := value.(string); value == nil || (mismatch && ok) {
		return ""
	}
	return ruleFieldValue
}

func issueRule(value any, mismatch bool) string {
	if mismatch {
		if value != nil {
			return ruleFieldValue
		}
		return ""
	}
	issue, rule := findingObject(value, "issue")
	if rule != "" {
		return rule
	}
	for _, key := range findingKeys["issue"] {
		if !findingText(issue[key]) {
			return ruleFieldValue
		}
	}
	return ""
}

func adjudicationRule(value any, mismatch bool) string {
	a, rule := findingObject(value, "adjudication")
	if rule != "" {
		return rule
	}
	if a["outcome"] != "agent_wrong" || !findingText(a["reason"]) {
		return ruleFieldValue
	}
	// Only a person adjudicates a mismatch.
	switch a["author_type"] {
	case "human":
		if !findingText(a["author"]) {
			return ruleFieldValue
		}
	case "model":
		if mismatch || a["author"] != nil {
			return ruleFieldValue
		}
	default:
		return ruleFieldValue
	}
	time := false
	switch ts := a["ts"].(type) {
	case Int:
		time = ts.Text == "-0" || !strings.HasPrefix(ts.Text, "-")
	case float64:
		time = ts >= 0 && !math.IsInf(ts, 1)
	}
	if !time && (mismatch || a["ts"] != nil) {
		return ruleFieldValue
	}
	return ""
}

func subjectRule(value any) string {
	subject, ok := value.(string)
	if n := utf8.RuneCountInString(subject); !ok || n < 1 || n > maxSubjectLength {
		return ruleFieldValue
	}
	for _, r := range subject {
		if r < 32 || r == 127 {
			return ruleFieldValue
		}
	}
	return ""
}

func gateRule(value any) string {
	if value == nil {
		return ""
	}
	gate, rule := findingObject(value, "gate")
	if rule != "" {
		return rule
	}
	if !findingText(gate["id"]) || !findingText(gate["name"]) || !findingDigest(gate["result_sha256"]) {
		return ruleFieldValue
	}
	return ""
}

// sealRule applies the seal checks. They show that the record agrees with
// its own seal text. They do not show that the sealed test is unchanged.
func sealRule(record, script Object, mismatch bool) string {
	seal, rule := findingObject(record["seal"], "seal")
	if rule != "" {
		return rule
	}
	text, ok := seal["text"].(string)
	files, isObject := seal["files"].(Object)
	if !ok || !isObject || !findingDigest(seal["sha256"]) {
		return ruleFieldValue
	}
	for _, digest := range files {
		if !findingDigest(digest) {
			return ruleFieldValue
		}
	}
	if sum := sha256.Sum256([]byte(text)); hex.EncodeToString(sum[:]) != seal["sha256"] {
		return ruleSealDigest
	}
	decoded, err := Decode([]byte(text))
	sealed, _ := decoded.(Object)
	sealedFiles, ok := sealed["files"].(Object)
	if err != nil || !ok || sealed["test"] != record["test"] {
		return ruleSealText
	}
	for name, digest := range files {
		if sealedFiles[name] != digest {
			return ruleSealCoverage
		}
	}
	for _, name := range findingSources[mismatch] {
		if _, has := files[name]; !has {
			return ruleSealSources
		}
	}
	if digest := script["sha256"]; digest != nil && files["script.json"] != digest && files["script.jsonl"] != digest {
		return ruleSealSources
	}
	return ""
}

// findingCase maps a record to the case that a runner gets after approval:
// the messages to send, and the expected value with its match rule.
func findingCase(record Object) Object {
	messages := object(record["script"])["messages"].(Array)
	if turn, ok := record["turn"].(Object); ok {
		messages = messages[:turnNumber(turn["number"])]
	}
	out := Object{"format": findingCaseFormat, "name": record["id"], "messages": append(Array{}, messages...),
		"expected": nil, "issue": nil}
	if expected, ok := record["expected"].(Object); ok {
		out["expected"] = pick(expected, findingKeys["expected"]...)
	}
	if issue, ok := record["issue"].(Object); ok {
		out["issue"] = pick(issue, findingKeys["issue"]...)
	}
	return out
}

// findingOrigin gives the fields that tie a candidate to its record. It
// copies no float, so each stored value is in the value domain of the wire.
func findingOrigin(record Object) Object {
	var gate any
	if g, ok := record["gate"].(Object); ok {
		gate = Object{"id": g["id"], "name": g["name"], "result_digest": "sha256:" + g["result_sha256"].(string)}
	}
	return Object{"record": findingName, "version": record["version"], "id": record["id"], "kind": record["kind"],
		"subject": record["subject"], "seal_digest": "sha256:" + object(record["seal"])["sha256"].(string),
		"adjudication": pick(object(record["adjudication"]), "outcome", "reason", "author", "author_type"),
		"gate":         gate, "producer_version": record["nookku"]}
}

// caseIndex gives the state index of the candidates: each candidate
// reference with its decision reference, or an empty string. A store gets
// the index at its first import.
func caseIndex(v *View) Object {
	index, ok := v.State["case_candidates"].(Object)
	if !ok {
		if _, has := v.State["case_candidates"]; has {
			fail("invalid_store", "case_candidates is not an object", "store")
		}
		index = Object{}
		v.State["case_candidates"] = index
	}
	return index
}

// importFinding is the write of FINDING.md clause F3. It writes one
// candidate record and its index entry. It writes no report, no verdict and
// no event.
func (h *Host) importFinding(v *View, q Object) Object {
	source := h.scoped(v, item(q, "source_ref"), "source")
	raw := []byte(text(item(q, "finding")))
	record, rule := readFinding(raw)
	if rule == ruleVersionMajor {
		fail("unsupported_finding_version", "the finding record has a major version that this reader does not know", "import")
	}
	if rule != "" {
		fail("invalid_finding", "the finding record breaks the reader rule "+rule, "import")
	}
	candidate := merge(pick(source, interval4...), Object{"source_ref": q["source_ref"], "importer": h.ctx("actor"),
		"input_digest": ByteDigest(raw), "origin": findingOrigin(record), "case": findingCase(record)})
	ref := v.Put(kindCandidate, candidate)
	if index := caseIndex(v); index[ref] == nil {
		index[ref] = ""
	}
	return Object{"candidate_ref": ref, "candidate": candidate}
}

// decideCase is the write of FINDING.md clause F4: one decision of a
// reviewer on one candidate.
func (h *Host) decideCase(v *View, q Object) Object {
	actor := h.ctx("actor")
	ref := text(item(q, "candidate_ref"))
	candidate := h.scoped(v, ref, kindCandidate)
	disposition := item(q, "disposition")
	if !within(disposition, Array{"accept", "reject"}) {
		fail("invalid_request", "disposition must be accept or reject", "")
	}
	if !truthy(item(q, "reason")) {
		fail("invalid_request", "case decision reason required", "")
	}
	if equal(actor, item(candidate, "importer")) {
		fail("separation_required", "reviewer must differ from importer", "admission")
	}
	index := caseIndex(v)
	if prior, _ := index[ref].(string); prior != "" {
		fail("duplicate_case_decision", "this candidate already has a decision", "admission")
	}
	if disposition == "accept" && item(item(candidate, "case"), "expected") == nil {
		fail("case_without_expectation", "a candidate with no expected value cannot become a case", "admission")
	}
	record := merge(pick(candidate, interval4...), Object{"candidate_ref": ref, "input_digest": item(candidate, "input_digest"),
		"actor": actor, "reason": q["reason"], "disposition": disposition, "decided_at": h.now()})
	if kind := h.kindOf(actor); kind != "" {
		record["kind"] = kind
	}
	decision := v.Put(kindCaseDecision, record)
	index[ref] = decision
	return Object{"case_decision_ref": decision, "case_decision": record}
}

// importedCases gives the imports of a plan request: each candidate with its
// accept decision, and the case of each candidate in the request order. A
// candidate with no accept decision refuses the plan.
func (h *Host) importedCases(v *View, q Object) (imports, cases Array) {
	refs, _ := q["imports"].(Array)
	if len(refs) == 0 {
		return nil, nil
	}
	index := caseIndex(v)
	seen := map[string]bool{}
	for _, x := range refs {
		ref := text(x)
		if seen[ref] {
			fail("invalid_plan", "a candidate is in imports two times", "")
		}
		seen[ref] = true
		candidate := h.scoped(v, ref, kindCandidate)
		if !equal(item(candidate, "source_ref"), item(q, "source_ref")) {
			fail("case_source_mismatch", "the candidate has another source than the plan", "admission")
		}
		decision, _ := index[ref].(string)
		if decision == "" || !equal(item(h.scoped(v, decision, kindCaseDecision), "disposition"), "accept") {
			fail("case_not_approved", "the candidate has no accept decision", "admission")
		}
		imports = append(imports, Object{"candidate_ref": ref, "decision_ref": decision})
		cases = append(cases, item(candidate, "case"))
	}
	return imports, cases
}
