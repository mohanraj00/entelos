package lifecycle

import (
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// The admission rules and the policy inputs of
// src/protocol/evidence/EVIDENCE.md. checkEvidence stays the shape check of
// the approval API 1.2 attachment. These rules do not change it.

const (
	maxEvidenceFacts = 256
	maxEvidenceInt   = 1<<53 - 1
	evidenceUnits    = "ppm-milli"
)

var evidenceFormat = regexp.MustCompile(`^entelos\.evidence/(0|[1-9][0-9]*)$`)

// evRule is one schema of evidence.schema.json. A nil list of fields permits
// each object. Else a member that is not in fields is refused.
type evRule struct {
	kind      string // object, array, string, integer, boolean or attached_value
	null      bool   // null also agrees
	pattern   *regexp.Regexp
	text      bool // no control character and one character that is not white space
	maxLen    int  // code points; 0 is no limit
	enum      []string
	low, high int64
	items     *evRule
	minItems  int
	maxItems  int // 0 is no limit
	fields    map[string]*evRule
	required  []string
}

func evString(pattern string) *evRule {
	return &evRule{kind: "string", pattern: regexp.MustCompile(pattern)}
}

func evText(max int, null bool) *evRule {
	return &evRule{kind: "string", null: null, text: true, maxLen: max}
}

func evNullable(r evRule) *evRule { r.null = true; return &r }

func evInteger(low int64, null bool) *evRule {
	return &evRule{kind: "integer", null: null, low: low, high: maxEvidenceInt}
}

func evList(items *evRule, min, max int) *evRule {
	return &evRule{kind: "array", items: items, minItems: min, maxItems: max}
}

func evObject(required []string, fields map[string]*evRule) *evRule {
	return &evRule{kind: "object", fields: fields, required: required}
}

var (
	evName       = evString(`^[A-Za-z0-9._-]{1,128}$`)
	evScheme     = evString(`^[A-Za-z0-9._:/@-]{1,128}$`)
	evCode       = evString(`^[A-Z][A-Z0-9_]{0,63}$`)
	evWord       = evString(`^[a-z][a-z0-9_]{0,63}$`)
	evDigest     = evString(`^sha256:[0-9a-f]{64}$`)
	evDigestOr   = evNullable(*evDigest)
	evTime       = evString(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?Z$`)
	evCoordinate = evInteger(-maxEvidenceInt, false)
	evAttached   = evObject([]string{"digest", "value"}, map[string]*evRule{"digest": evDigest, "value": {kind: "attached_value"}})

	evBox = evObject([]string{"x0", "y0", "x1", "y1"}, map[string]*evRule{"x0": evCoordinate, "y0": evCoordinate, "x1": evCoordinate, "y1": evCoordinate})

	evSpan = evObject([]string{"start", "end"}, map[string]*evRule{
		"start": evInteger(0, false), "end": evInteger(1, false), "page": evInteger(1, false), "box": evBox})

	evPart = evObject([]string{"role", "start", "end", "passed"}, map[string]*evRule{
		"role": evWord, "start": evInteger(0, true), "end": evInteger(1, true), "passed": {kind: "boolean"}})

	evJudgment = evObject([]string{"question", "p"}, map[string]*evRule{
		"question": evWord, "answer": evText(256, true), "p": {kind: "integer", low: 0, high: 1000000}})

	evData = evObject([]string{"field", "value"}, map[string]*evRule{
		"field": evText(256, false), "key": evText(256, true), "value": {kind: "string", null: true, maxLen: 65536},
		"unit": evText(256, true), "parts": evList(evPart, 0, 64), "schema_digest": evDigest, "policy_digest": evDigest,
		"packs": evList(evText(256, false), 0, 64), "judgments": evList(evJudgment, 0, 64)})

	evPin = evObject([]string{"url", "digest", "retrieved", "edition"}, map[string]*evRule{
		"url": evText(4096, false), "digest": evDigestOr, "retrieved": evText(256, true), "edition": evText(256, true)})

	evAnswer = evObject([]string{"reviewer", "reviewer_kind", "verdict", "time", "ms"}, map[string]*evRule{
		"reviewer": evText(256, false), "reviewer_kind": {kind: "string", enum: []string{"human", "model"}}, "verdict": evWord,
		"time": evTime, "ms": evInteger(0, false), "entry_digest": evDigest, "producer_data": evAttached})

	evReview = evObject([]string{"status", "answers"}, map[string]*evRule{
		"status":  {kind: "string", enum: []string{"admitted", "confirmed", "chosen", "corrected", "rejected", "not_in_document", "open", "disputed"}},
		"answers": evList(evAnswer, 1, 16), "task_digest": evDigestOr, "resolution_digest": evDigest, "correction_receipt_digest": evDigest})

	evFact = evObject([]string{"fact_id", "receipt_digest", "decision_digest", "source_kind", "source_digest", "text_digest", "spans", "outcome", "codes", "data"},
		map[string]*evRule{
			"fact_id": evText(256, false), "receipt_digest": evDigest, "decision_digest": evDigest, "packet_digest": evDigestOr,
			"profile_digest": evDigestOr, "completeness": evAttached, "source_kind": evWord, "source_digest": evDigestOr,
			"text_digest": evDigestOr, "url": evText(4096, true), "ref": evText(256, true), "spans": evList(evSpan, 0, 64),
			"pins": evList(evPin, 0, 16), "outcome": {kind: "string", enum: []string{"admitted", "needs_verification", "rejected"}},
			"codes": evList(evCode, 0, 64), "data": evData, "review": evReview, "producer_data": evAttached})

	evDocument = evObject([]string{"format", "producer", "facts"}, map[string]*evRule{
		"format":   {kind: "string", enum: []string{"entelos.evidence/1"}},
		"producer": evObject([]string{"id", "version", "digest_scheme"}, map[string]*evRule{"id": evName, "version": evName, "digest_scheme": evScheme, "producer_data": evAttached}),
		"facts":    evList(evFact, 1, 0)})
)

// agrees applies the schema keywords as the subset validator of
// src/protocol/test_protocol.py does.
func (r *evRule) agrees(value any) bool {
	if value == nil {
		return r.null
	}
	switch r.kind {
	case "string":
		s, ok := value.(string)
		if !ok || !utf8.ValidString(s) || (r.maxLen > 0 && utf8.RuneCountInString(s) > r.maxLen) {
			return false
		}
		if r.pattern != nil && !r.pattern.MatchString(s) || r.text && !evTextOK(s) {
			return false
		}
		if r.enum != nil {
			for _, e := range r.enum {
				if s == e {
					return true
				}
			}
			return false
		}
		return true
	case "integer":
		n, ok := evidenceInteger(value)
		return ok && n >= r.low && n <= r.high
	case "attached_value":
		obj, ok := value.(Object)
		return ok && evidenceValue(obj)
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		list, ok := value.(Array)
		if !ok || len(list) < r.minItems || (r.maxItems > 0 && len(list) > r.maxItems) {
			return false
		}
		for _, x := range list {
			if !r.items.agrees(x) {
				return false
			}
		}
		return true
	case "object":
		obj, ok := value.(Object)
		if !ok {
			return false
		}
		for _, name := range r.required {
			if _, has := obj[name]; !has {
				return false
			}
		}
		if r.fields == nil {
			return true
		}
		for name, x := range obj {
			field, known := r.fields[name]
			if !known || !field.agrees(x) {
				return false
			}
		}
		return true
	}
	return false
}

func evidenceInteger(value any) (int64, bool) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case Int:
		parsed, err := strconv.ParseInt(v.Text, 10, 64)
		if err != nil || strconv.FormatInt(parsed, 10) != v.Text {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	return n, n >= -maxEvidenceInt && n <= maxEvidenceInt
}

// Attached values can have arbitrary keys but use the same number domain.
func evidenceValue(value any) bool {
	switch v := value.(type) {
	case Object:
		for _, child := range v {
			if !evidenceValue(child) {
				return false
			}
		}
	case Array:
		for _, child := range v {
			if !evidenceValue(child) {
				return false
			}
		}
	case nil, bool, string:
	default:
		_, ok := evidenceInteger(v)
		return ok
	}
	return true
}

func evidenceHasUnits(doc Object, facts Array) bool {
	producer := doc["producer"].(Object)
	attached, _ := producer["producer_data"].(Object)
	value, _ := attached["value"].(Object)
	units, _ := value["number_units"].(string)
	if units == evidenceUnits {
		return true
	}
	for _, raw := range facts {
		fact := raw.(Object)
		judgments, _ := fact["data"].(Object)["judgments"].(Array)
		if len(judgments) != 0 {
			return false
		}
		for _, span := range fact["spans"].(Array) {
			if _, present := span.(Object)["box"]; present {
				return false
			}
		}
	}
	return true
}

// evTextOK is the pattern of a text. Go and Python \s differ, so the check
// uses unicode.IsSpace. Outside the control characters, unicode.IsSpace and
// the Python \s of a str pattern match the same characters.
func evTextOK(s string) bool {
	visible := false
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return false
		}
		if !unicode.IsSpace(c) {
			visible = true
		}
	}
	return visible
}

// admitEvidence applies the rules of EVIDENCE.md in their order. Each rule
// examines all facts before the next rule starts. An empty code means that
// the document is valid, and inputs then has the four policy inputs.
func admitEvidence(doc any) (inputs Object, code, rule string) {
	bad := func(rule string) (Object, string, string) { return nil, "invalid_evidence", rule }
	obj, ok := doc.(Object)
	if !ok {
		return bad("format")
	}
	format, ok := obj["format"].(string)
	if !ok {
		return bad("format")
	}
	m := evidenceFormat.FindStringSubmatch(format)
	if m == nil {
		return bad("format")
	}
	if m[1] != "1" {
		return nil, "unsupported_evidence_format", ""
	}
	if !evDocument.agrees(obj) {
		return bad("schema")
	}
	rawFacts := obj["facts"].(Array)
	if !evidenceHasUnits(obj, rawFacts) {
		return bad("schema")
	}
	// The schema passed, so each type assertion below holds.
	var facts []Object
	for _, x := range rawFacts {
		facts = append(facts, x.(Object))
	}
	review := func(f Object) Object { r, _ := f["review"].(Object); return r }
	status := func(f Object) string { s, _ := review(f)["status"].(string); return s }
	spans := func(f Object) []Object {
		var out []Object
		for _, x := range f["spans"].(Array) {
			out = append(out, x.(Object))
		}
		return out
	}
	more := func(end, start any) bool {
		a, _ := evidenceInteger(end)
		b, _ := evidenceInteger(start)
		return a > b
	}
	if len(facts) > maxEvidenceFacts {
		return bad("too_many_facts")
	}
	seen := map[string]bool{}
	for _, f := range facts {
		id := f["fact_id"].(string)
		if seen[id] {
			return bad("duplicate_fact")
		}
		seen[id] = true
	}
	for _, f := range facts {
		for _, s := range spans(f) {
			if !more(s["end"], s["start"]) {
				return bad("span_order")
			}
		}
	}
	for _, f := range facts {
		parts, _ := f["data"].(Object)["parts"].(Array)
		for _, x := range parts {
			p := x.(Object)
			if p["start"] == nil && p["end"] == nil {
				continue
			}
			if p["start"] == nil || p["end"] == nil || !more(p["end"], p["start"]) {
				return bad("span_order")
			}
		}
	}
	for _, f := range facts {
		if len(spans(f)) > 0 && f["text_digest"] == nil {
			return bad("span_without_text")
		}
	}
	for _, f := range facts {
		for _, s := range spans(f) {
			box, has := s["box"].(Object)
			if !has {
				continue
			}
			if _, page := s["page"]; !page || !more(box["x1"], box["x0"]) || !more(box["y1"], box["y0"]) {
				return bad("locator")
			}
		}
	}
	for _, f := range facts {
		if _, has := review(f)["correction_receipt_digest"]; status(f) == "corrected" && !has {
			return bad("correction_receipt_required")
		}
	}
	for _, f := range facts {
		if _, has := review(f)["correction_receipt_digest"]; review(f) != nil && status(f) != "corrected" && has {
			return bad("correction_receipt_forbidden")
		}
	}
	for _, f := range facts {
		if status(f) == "admitted" && f["outcome"] != "admitted" {
			return bad("review_status_outcome")
		}
	}
	for _, f := range facts {
		if f["outcome"] == "needs_verification" && review(f) == nil {
			return bad("unreviewed_fact")
		}
	}
	for _, f := range facts {
		if f["outcome"] == "rejected" && review(f) == nil || status(f) == "rejected" || status(f) == "not_in_document" {
			return bad("rejected_fact")
		}
	}
	for _, f := range facts {
		if status(f) == "open" || status(f) == "disputed" {
			return bad("open_review")
		}
	}
	human, model, reviewed, corrected := false, false, false, false
	for _, f := range facts {
		if review(f) == nil {
			continue
		}
		reviewed = true
		corrected = corrected || status(f) == "corrected"
		for _, x := range review(f)["answers"].(Array) {
			switch x.(Object)["reviewer_kind"] {
			case "human":
				human = true
			case "model":
				model = true
			}
		}
	}
	return Object{"evidence_human_reviewed": human, "evidence_model_reviewed": model,
		"evidence_all_automatic": !reviewed, "evidence_corrected": corrected}, "", ""
}

// evidenceRead is the evidence read of EVIDENCE.md. It applies the rules to
// the evidence document that a snapshot retains, and gives the result and
// the policy inputs. It writes nothing and changes no record.
func (h *Host) evidenceRead(v *View, q Object) Object {
	ref := text(item(q, "snapshot_ref"))
	snapshot := h.scoped(v, ref, "snapshot")
	out := Object{"cursor": v.Cursor, "snapshot_ref": ref}
	doc, present := snapshot["evidence"]
	if !present {
		out["admission"] = "absent"
		return out
	}
	inputs, code, rule := admitEvidence(doc)
	if code != "" {
		out["admission"], out["code"] = "invalid", code
		if rule != "" {
			out["rule"] = rule
		}
		return out
	}
	out["admission"], out["inputs"] = "valid", inputs
	return out
}
