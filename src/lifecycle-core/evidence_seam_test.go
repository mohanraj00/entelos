package lifecycle

import (
	"os"
	"testing"
)

// TestEvidenceSeam reads the cases that tests/test_groundgate_seam.py writes from a groundgate
// export. Each case goes through propose with the evidence attachment and the Host evidence read.
// The test skips when ENTELOS_EVIDENCE_SEAM is not set. The Python test sets it and fails on a skip.
func TestEvidenceSeam(t *testing.T) {
	path := os.Getenv("ENTELOS_EVIDENCE_SEAM")
	if path == "" {
		t.Skip("set ENTELOS_EVIDENCE_SEAM to the cases file of tests/test_groundgate_seam.py")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := must(Decode(raw)).(Object)["cases"].(Array)
	if len(cases) == 0 {
		t.Fatal("no cases")
	}
	w := newWorld(t, &CELRunner{Runtime: &fakeEngine{build: engineBuild}})
	source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "invented source", "reason": "first"}))["source_ref"]
	base := ""
	for _, x := range cases {
		c := x.(Object)
		fields := with(window, Object{"base_snapshot": base, "source_ref": source, "bundle": bundle("rule"),
			"reason": "propose", "evidence": c["evidence"]})
		base = w.must("author", "author", "propose", fields)["snapshot_ref"].(string)
		out, err := w.host("reviewer", "reviewer", w.runner).Handle(Object{"op": "evidence", "snapshot_ref": base})
		if err != nil {
			t.Fatal(err)
		}
		got := out.(Object)
		want := c["expect"].(Object)
		for _, k := range []string{"admission", "code", "rule", "inputs"} {
			if !equal(got[k], want[k]) {
				t.Errorf("%s: %s: expected %s, got %s", c["name"], k, Encode(want[k]), Encode(got[k]))
			}
		}
	}
}
