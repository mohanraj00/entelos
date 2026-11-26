package lifecycle

import (
	"os"
	"testing"
)

// TestFindingSeam reads the cases that tests/test_nookku_seam.py writes from finding records of
// nookku. Each case goes through the reader rules and the Host import. Then the library record
// goes through the example of FINDING.md: candidate, approval, plan and two qualifications.
// The test skips when ENTELOS_FINDING_SEAM is not set. The Python test sets it and fails on a skip.
func TestFindingSeam(t *testing.T) {
	path := os.Getenv("ENTELOS_FINDING_SEAM")
	if path == "" {
		t.Skip("set ENTELOS_FINDING_SEAM to the cases file of tests/test_nookku_seam.py")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := must(Decode(raw)).(Object)
	cases := doc["cases"].(Array)
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	w := newCatalogWorld(t)
	for _, x := range cases {
		c := x.(Object)
		name, finding, want := c["name"], c["finding"].(string), c["expect"].(Object)
		if _, rule := readFinding([]byte(finding)); rule != want["rule"] {
			t.Errorf("%s: got the rule %q, want %q", name, rule, want["rule"])
		}
		cursor := w.view().Cursor
		imported, err := w.importFinding(finding)
		if code := want["code"].(string); code != "" {
			if err == nil || faultCode(t, err) != code {
				t.Errorf("%s: got %v, want the fault %s", name, err, code)
			}
			if w.view().Cursor != cursor {
				t.Errorf("%s: the refused import published state", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		candidate := imported["candidate"].(Object)
		origin, mapped := candidate["origin"].(Object), candidate["case"].(Object)
		got := Object{"input_digest": candidate["input_digest"], "id": origin["id"], "subject": origin["subject"],
			"seal_digest": origin["seal_digest"], "author_type": origin["adjudication"].(Object)["author_type"],
			"messages": mapped["messages"], "expected": mapped["expected"]}
		for key, value := range got {
			if !equal(value, want[key]) {
				t.Errorf("%s: %s: got %s, want %s", name, key, Encode(value), Encode(want[key]))
			}
		}
	}
	t.Run("library", func(t *testing.T) { findingFlow(t, doc["library"].(string)) })
}
