package lifecycle

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evidenceFixture(t *testing.T) Object {
	t.Helper()
	raw, err := os.ReadFile("../protocol/approval/vectors/approval-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	return must(Decode(raw)).(Object)["steps"].(Array)[3].(Object)["request"].(Object)["body"].(Object)["evidence"].(Object)
}

func TestEvidenceBounds(t *testing.T) {
	doc := evidenceFixture(t)
	value := Object{"padding": ""}
	doc["producer"].(Object)["producer_data"] = Object{"digest": "fixture", "value": value}
	padding := maxEvidenceBytes - len(Encode(doc))
	value["padding"] = strings.Repeat("x", padding)
	if len(Encode(doc)) != maxEvidenceBytes || !checkEvidence(doc) {
		t.Fatal("exact byte bound refused")
	}
	value["padding"] = strings.Repeat("x", padding+1)
	if checkEvidence(doc) {
		t.Fatal("excess byte accepted")
	}
	// The budget must use encoded bytes, including escapes, UTF-8 and newline.
	for _, text := range []string{"a\"\\\b\f\n\r\t\x00\x1f", "தமிழ்", ""} {
		value["padding"] = text
		remaining := len(Encode(doc)) - 1
		if !evidenceBounded(doc, 0, &remaining) || remaining != 0 {
			t.Fatalf("wrong encoded size for %q: %d", text, remaining)
		}
	}
	value["padding"] = nestedArrays(maxEvidenceDepth - 4)
	if !checkEvidence(doc) {
		t.Fatal("exact depth bound refused")
	}
	value["padding"] = nestedArrays(maxEvidenceDepth - 3)
	if checkEvidence(doc) {
		t.Fatal("excess depth accepted")
	}
	value["padding"] = value
	if checkEvidence(doc) {
		t.Fatal("cyclic direct input accepted")
	}
	for _, bad := range []any{math.NaN(), math.Inf(1), make(chan int), Int{"01"}, "\xff"} {
		value["padding"] = bad
		if checkEvidence(doc) {
			t.Fatalf("non-JSON value %T accepted", bad)
		}
	}
}

// Use the source schema, not the Host's type table, to state the expected
// required fields and valid JSON types, including optional nested members.
func TestEvidenceStructureFromSchema(t *testing.T) {
	raw, err := os.ReadFile("../protocol/evidence/schema/evidence.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	root := must(Decode(raw)).(Object)
	defs := root["$defs"].(Object)
	var sample func(Object) any
	sample = func(s Object) any {
		if ref, ok := s["$ref"].(string); ok {
			return sample(defs[strings.TrimPrefix(ref, "#/$defs/")].(Object))
		}
		if c, ok := s["const"]; ok {
			return c
		}
		typ := s["type"]
		if types, ok := typ.(Array); ok {
			typ = types[0]
		}
		switch typ {
		case "object":
			out := Object{}
			if props, ok := s["properties"].(Object); ok {
				for k, p := range props {
					out[k] = sample(p.(Object))
				}
			}
			return out
		case "array":
			return Array{sample(s["items"].(Object))}
		case "string":
			return "fixture"
		case "integer", "number":
			return Int{"1"}
		case "boolean":
			return true
		}
		t.Fatalf("unknown schema: %v", s)
		return nil
	}
	for name, raw := range merge(copyObject(defs), Object{"document": root}) {
		s := raw.(Object)
		if s["type"] != "object" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			obj := sample(s).(Object)
			if !evidenceType(obj, name) {
				t.Fatal("schema structure refused")
			}
			for _, raw := range s["required"].(Array) {
				key := raw.(string)
				prior := obj[key]
				delete(obj, key)
				if evidenceType(obj, name) {
					t.Errorf("missing %s accepted", key)
				}
				obj[key] = prior
			}
			for key, p := range s["properties"].(Object) {
				prop := p.(Object)
				if ref, ok := prop["$ref"].(string); ok {
					prop = defs[strings.TrimPrefix(ref, "#/$defs/")].(Object)
				}
				kinds, ok := prop["type"].(Array)
				if !ok {
					kinds = Array{prop["type"]}
				}
				prior := obj[key]
				for _, bad := range []struct {
					kind  string
					value any
				}{{"null", nil}, {"string", "text"}, {"boolean", false}, {"integer", Int{"1"}}, {"number", 1.5}, {"object", Object{}}, {"array", Array{}}} {
					if within(bad.kind, kinds) || (bad.kind == "integer" && within("number", kinds)) {
						continue
					}
					obj[key] = bad.value
					if evidenceType(obj, name) {
						t.Errorf("%s accepted type %s", key, bad.kind)
					}
				}
				obj[key] = prior
			}
		})
	}
}

func TestEvidenceVectorBounds(t *testing.T) {
	count := 0
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case Object:
			if v["format"] == "entelos.evidence/1" {
				remaining := maxEvidenceBytes - 1
				if !evidenceBounded(v, 0, &remaining) {
					t.Error("vector exceeds bounds")
				}
				count++
			}
			for _, x := range v {
				walk(x)
			}
		case Array:
			for _, x := range v {
				walk(x)
			}
		}
	}
	for _, dir := range []string{"evidence", "approval"} {
		paths, err := filepath.Glob(filepath.Join("..", "protocol", dir, "vectors", "*.json"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("vectors: %v", err)
		}
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			walk(must(Decode(raw)))
		}
	}
	if count == 0 {
		t.Fatal("no evidence documents checked")
	}
	t.Logf("%d vector documents fit the bounds", count)
}
