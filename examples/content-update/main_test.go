package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

func TestContentWorkflow(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1. Propose:", "Approval refused: qualification_failed",
		"3. Qualify: passed", "4. Decide:", "5. Activate:", "6. Read at 2026-11-15:",
		"charging: Charge the sample device with the supplied cable.",
		"returns: Return the sample device within 30 days."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, &out)
		}
	}
}

func TestItemChecks(t *testing.T) {
	raw, err := files.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"charging", "returns"} {
		for _, change := range []string{"digest", "size", "missing"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				items := fstest.MapFS{}
				for _, id := range []string{"charging", "returns"} {
					body, err := fs.ReadFile(files, "items/"+id+".md")
					if err != nil {
						t.Fatal(err)
					}
					items[id+".md"] = &fstest.MapFile{Data: body}
				}
				runner := &contentRunner{files: items}
				input := lifecycle.QualificationInput{Candidate: lifecycle.Candidate{BundleRef: digest(raw)},
					Files: map[string][]byte{"content": raw}}
				check := func(want string) {
					t.Helper()
					verdict, err := runner.Qualify(input)
					if err != nil || verdict.Outcome.(lifecycle.Object)["status"] != want {
						t.Fatalf("want %s, got %v, %v", want, verdict, err)
					}
				}
				check("passed")
				switch change {
				case "digest":
					items[name+".md"].Data[0] ^= 1 // Same length, different digest.
				case "size":
					// Change only the expected size, so the digest still agrees.
					old := fmt.Sprintf(`"size": %d`, len(items[name+".md"].Data))
					next := fmt.Sprintf(`"size": %d`, len(items[name+".md"].Data)+1)
					input.Files["content"] = bytes.Replace(raw, []byte(old), []byte(next), 1)
					input.Candidate.BundleRef = digest(input.Files["content"])
				case "missing":
					delete(items, name+".md")
				}
				check("failed")
			})
		}
	}
}
