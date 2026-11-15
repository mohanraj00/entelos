package lifecycle

import (
	"math"
	"strings"
	"unicode/utf8"
)

const (
	maxEvidenceDepth = 32
	maxEvidenceBytes = 256 * 1024
)

// checkEvidence checks storage bounds before walking the fixed evidence shape.
// It does not apply the admission rules or verify a producer's claims.
func checkEvidence(value any) bool {
	remaining := maxEvidenceBytes - 1 // Encode adds one final newline.
	return evidenceBounded(value, 0, &remaining) && evidenceType(value, "document") && value.(Object)["format"] == "entelos.evidence/1"
}

// Count the core encoding without building an unbounded byte slice. Check
// depth before recursion, including arbitrary attached values and cyclic input
// from direct callers. Only values that the record codec can read are allowed.
func evidenceBounded(value any, depth int, remaining *int) bool {
	if *remaining < 0 {
		return false
	}
	switch v := value.(type) {
	case Object:
		if depth >= maxEvidenceDepth {
			return false
		}
		*remaining -= 2
		first := true
		for k, x := range v {
			*remaining-- // colon
			if !first {
				*remaining--
			}
			first = false
			if !evidenceBounded(k, depth+1, remaining) || !evidenceBounded(x, depth+1, remaining) {
				return false
			}
		}
	case Array:
		if depth >= maxEvidenceDepth {
			return false
		}
		*remaining -= 2
		for i, x := range v {
			if i > 0 {
				*remaining--
			}
			if !evidenceBounded(x, depth+1, remaining) {
				return false
			}
		}
	case string:
		if len(v) > *remaining || !utf8.ValidString(v) {
			return false
		}
		*remaining -= 2
		for i := 0; i < len(v); i++ {
			switch v[i] {
			case '"', '\\', '\b', '\f', '\n', '\r', '\t':
				*remaining -= 2
			default:
				if v[i] < 0x20 {
					*remaining -= 6
				} else {
					*remaining--
				}
			}
			if *remaining < 0 {
				return false
			}
		}
	case Int:
		if len(v.Text) > maxIntDigits+1 || len(v.Text) > *remaining {
			return false
		}
		parsed, err := Decode([]byte(v.Text))
		if err != nil || parsed != v {
			return false
		}
		*remaining -= len(v.Text)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
		*remaining -= len(Encode(v)) - 1
	case nil, bool, int:
		*remaining -= len(Encode(v)) - 1
	default:
		return false
	}
	return *remaining >= 0
}

// These are only the required members and JSON types of evidence.schema.json.
// Patterns, value ranges, digests and admission decisions are not checked here.
func evidenceType(value any, kind string) bool {
	if strings.HasPrefix(kind, "?") {
		return value == nil || evidenceType(value, kind[1:])
	}
	if strings.HasPrefix(kind, "[]") {
		list, ok := value.(Array)
		if !ok {
			return false
		}
		for _, x := range list {
			if !evidenceType(x, kind[2:]) {
				return false
			}
		}
		return true
	}
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number", "integer":
		switch v := value.(type) {
		case Int, int:
			return true
		case float64:
			return kind == "number" || math.Trunc(v) == v
		}
		return false
	case "object":
		_, ok := value.(Object)
		return ok
	}
	obj, ok := value.(Object)
	if !ok {
		return false
	}
	var required string
	var fields map[string]string
	switch kind {
	case "document":
		required = "format producer facts"
		fields = map[string]string{"format": "string", "producer": "producer", "facts": "[]fact"}
	case "producer":
		required = "id version digest_scheme"
		fields = map[string]string{"id": "string", "version": "string", "digest_scheme": "string", "producer_data": "attached"}
	case "attached":
		required = "digest value"
		fields = map[string]string{"digest": "string", "value": "object"}
	case "fact":
		required = "fact_id receipt_digest decision_digest source_kind source_digest text_digest spans outcome codes data"
		fields = map[string]string{"fact_id": "string", "receipt_digest": "string", "decision_digest": "string", "packet_digest": "?string", "profile_digest": "?string", "completeness": "attached", "source_kind": "string", "source_digest": "?string", "text_digest": "?string", "url": "?string", "ref": "?string", "spans": "[]span", "pins": "[]pin", "outcome": "string", "codes": "[]string", "data": "data", "review": "review", "producer_data": "attached"}
	case "span":
		required = "start end"
		fields = map[string]string{"start": "integer", "end": "integer", "page": "integer", "box": "box"}
	case "box":
		required = "x0 y0 x1 y1"
		fields = map[string]string{"x0": "number", "y0": "number", "x1": "number", "y1": "number"}
	case "data":
		required = "field value"
		fields = map[string]string{"field": "string", "key": "?string", "value": "?string", "unit": "?string", "parts": "[]part", "schema_digest": "string", "policy_digest": "string", "packs": "[]string", "judgments": "[]judgment"}
	case "part":
		required = "role start end passed"
		fields = map[string]string{"role": "string", "start": "?integer", "end": "?integer", "passed": "boolean"}
	case "judgment":
		required = "question p"
		fields = map[string]string{"question": "string", "answer": "?string", "p": "number"}
	case "pin":
		required = "url digest retrieved edition"
		fields = map[string]string{"url": "string", "digest": "?string", "retrieved": "?string", "edition": "?string"}
	case "review":
		required = "status answers"
		fields = map[string]string{"status": "string", "answers": "[]answer", "task_digest": "?string", "resolution_digest": "string", "correction_receipt_digest": "string"}
	case "answer":
		required = "reviewer reviewer_kind verdict time ms"
		fields = map[string]string{"reviewer": "string", "reviewer_kind": "string", "verdict": "string", "time": "string", "ms": "integer", "entry_digest": "string", "producer_data": "attached"}
	default:
		return false
	}
	for _, name := range strings.Fields(required) {
		if _, has := obj[name]; !has {
			return false
		}
	}
	for name, x := range obj {
		typ, known := fields[name]
		if !known || !evidenceType(x, typ) {
			return false
		}
	}
	return true
}
