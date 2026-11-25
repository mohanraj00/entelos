package lifecycle

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// expandEvidence is expand() of src/protocol/evidence/check_spec.py.
func expandEvidence(value any) any {
	switch v := value.(type) {
	case Object:
		if t, ok := v["$text"].(Object); ok && len(v) == 1 {
			return strings.Repeat(t["char"].(string), intOf(t["count"]))
		}
		if r, ok := v["$repeat"].(Object); ok && len(v) == 1 {
			out := Array{}
			for i := 0; i < intOf(r["count"]); i++ {
				if item, has := r["item"]; has {
					out = append(out, expandEvidence(item))
					continue
				}
				fact := expandEvidence(r["fact"]).(Object)
				fact["fact_id"] = strings.ReplaceAll(r["fact"].(Object)["fact_id"].(string), "{i}", strconv.Itoa(i))
				out = append(out, fact)
			}
			return out
		}
		out := Object{}
		for k, x := range v {
			out[k] = expandEvidence(x)
		}
		return out
	case Array:
		out := Array{}
		for _, x := range v {
			out = append(out, expandEvidence(x))
		}
		return out
	}
	return value
}

// TestEvidenceAdmissionVectors runs each case of the evidence vectors. The
// test encodes and decodes each document again, so the values have the
// types that the Host gets from a request.
func TestEvidenceAdmissionVectors(t *testing.T) {
	raw, err := os.ReadFile("../protocol/evidence/vectors/evidence-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := must(Decode(raw)).(Object)["cases"].(Array)
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	for _, x := range cases {
		c := x.(Object)
		name := c["name"].(string)
		doc := must(Decode(Encode(expandEvidence(c["evidence"]))))
		inputs, code, rule := admitEvidence(doc)
		got := Object{"valid": code == ""}
		switch {
		case code == "":
			got["inputs"] = inputs
		case rule == "":
			got["code"] = code
		default:
			got["code"], got["rule"] = code, rule
		}
		if !equal(got, c["expect"]) {
			t.Errorf("%s: expected %s, got %s", name, Encode(c["expect"]), Encode(got))
		}
	}
	t.Logf("%d cases", len(cases))
}

// TestEvidenceRead checks the evidence read on retained documents: a valid
// document, a document that a rule refuses, and no document. The read
// writes nothing.
func TestEvidenceRead(t *testing.T) {
	raw, err := os.ReadFile("../protocol/evidence/vectors/evidence-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]Object{}
	for _, c := range must(Decode(raw)).(Object)["cases"].(Array) {
		cases[c.(Object)["name"].(string)] = c.(Object)
	}
	doc := func(name string) any { return must(Decode(Encode(expandEvidence(cases[name]["evidence"])))) }
	w := newWorld(t, &CELRunner{Runtime: &fakeEngine{build: engineBuild}})
	source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "invented source", "reason": "first"}))["source_ref"]
	base := ""
	propose := func(evidence any) string {
		fields := with(window, Object{"base_snapshot": base, "source_ref": source, "bundle": bundle("rule"), "reason": "propose"})
		if evidence != nil {
			fields["evidence"] = evidence
		}
		base = w.must("author", "author", "propose", fields)["snapshot_ref"].(string)
		return base
	}
	human := propose(doc("valid-human-confirmed"))
	open := propose(doc("open-review"))
	legacy := propose(doc("schema-legacy-full-data"))
	none := propose(nil)
	root := must(w.store.Read(nil)).Cursor
	read := func(ref string) Object {
		out, err := w.host("reviewer", "reviewer", w.runner).Handle(Object{"op": "evidence", "snapshot_ref": ref})
		if err != nil {
			t.Fatal(err)
		}
		return out.(Object)
	}
	got := read(human)
	if got["admission"] != "valid" || !equal(got["inputs"], cases["valid-human-confirmed"]["expect"].(Object)["inputs"]) {
		t.Fatalf("valid document: %v", got)
	}
	if got := read(open); got["admission"] != "invalid" || got["code"] != "invalid_evidence" || got["rule"] != "open_review" || got["inputs"] != nil {
		t.Fatalf("refused document: %v", got)
	}
	if got := read(legacy); got["admission"] != "invalid" || got["code"] != "invalid_evidence" || got["rule"] != "schema" || got["inputs"] != nil {
		t.Fatalf("legacy fractional document: %v", got)
	}
	if got := read(none); got["admission"] != "absent" || len(got) != 3 {
		t.Fatalf("no document: %v", got)
	}
	if must(w.store.Read(nil)).Cursor != root {
		t.Fatal("the evidence read wrote state")
	}
	_, err = w.host("reader", "reader", w.runner).Handle(Object{"op": "evidence", "snapshot_ref": human})
	if faultCode(t, err) != "forbidden" {
		t.Fatal("a reader got the evidence read")
	}
}
