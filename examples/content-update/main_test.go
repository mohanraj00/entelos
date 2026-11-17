package main

import (
	"bytes"
	"encoding/json"
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
		"3. Qualify: passed", "Retained report:", "status failed.",
		"4. Decide:", "5. Activate:", "6. Read at 2026-11-15:",
		"Read checks passed: item digests and sizes agree with the active manifest.",
		"charging: Charge the sample device with the supplied cable.",
		"returns: Return the sample device within 30 days."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, &out)
		}
	}
}

func reviewerCases(t *testing.T) lifecycle.Array {
	t.Helper()
	raw, err := files.ReadFile("expected.json")
	if err != nil {
		t.Fatal(err)
	}
	cases, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return cases.(lifecycle.Array)
}

func itemFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	items := fstest.MapFS{}
	for _, id := range []string{"charging", "returns"} {
		body, err := fs.ReadFile(files, "items/"+id+".md")
		if err != nil {
			t.Fatal(err)
		}
		items[id+".md"] = &fstest.MapFile{Data: body}
	}
	return items
}

func TestItemChecks(t *testing.T) {
	raw, err := files.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"charging", "returns"} {
		for _, change := range []string{"digest", "size", "missing"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				items := itemFiles(t)
				runner := &contentRunner{files: items}
				input := lifecycle.QualificationInput{Candidate: lifecycle.Candidate{BundleRef: digest(raw)},
					Files: map[string][]byte{"content": raw}, Cases: reviewerCases(t)}
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

func TestPlanChecks(t *testing.T) {
	raw, err := files.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"charging", "returns"} {
		for _, change := range []string{"manifest-and-files", "missing-case", "case-size", "missing-item"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				items := itemFiles(t)
				cases := reviewerCases(t)
				candidate := raw
				switch change {
				case "manifest-and-files":
					// The file and candidate agree. The reviewer case stays unchanged.
					items[name+".md"].Data[0] ^= 1
					var m manifest
					if err := json.Unmarshal(raw, &m); err != nil {
						t.Fatal(err)
					}
					candidate = bytes.Replace(raw, []byte(m.Items[index].Digest),
						[]byte(digest(items[name+".md"].Data)), 1)
				case "missing-case":
					cases = append(cases[:index], cases[index+1:]...)
				case "case-size":
					cases[index].(lifecycle.Object)["size"] = len(items[name+".md"].Data) + 1
				case "missing-item":
					// The candidate drops an item that the plan expects.
					var m map[string]any
					if err := json.Unmarshal(raw, &m); err != nil {
						t.Fatal(err)
					}
					list := m["items"].([]any)
					m["items"] = append(list[:index:index], list[index+1:]...)
					if candidate, err = json.Marshal(m); err != nil {
						t.Fatal(err)
					}
				}
				runner := &contentRunner{files: items}
				verdict, err := runner.Qualify(lifecycle.QualificationInput{
					Candidate: lifecycle.Candidate{BundleRef: digest(candidate)},
					Files:     map[string][]byte{"content": candidate}, Cases: cases})
				if err != nil {
					t.Fatal(err)
				}
				outcome := verdict.Outcome.(lifecycle.Object)
				failed := false
				for _, check := range outcome["items"].(lifecycle.Array) {
					check := check.(lifecycle.Object)
					failed = failed || check["item_id"] == name && check["passed"] == false
				}
				if outcome["status"] != "failed" || !failed {
					t.Fatalf("plan mismatch must fail %s: %v", name, outcome)
				}
			})
		}
	}
}

func TestReadChecks(t *testing.T) {
	raw, err := files.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"charging", "returns"} {
		for _, change := range []string{"digest", "size", "missing"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				items := itemFiles(t)
				active := raw
				switch change {
				case "digest":
					items[name+".md"].Data[0] ^= 1
				case "size":
					old := fmt.Sprintf(`"size": %d`, len(items[name+".md"].Data))
					next := fmt.Sprintf(`"size": %d`, len(items[name+".md"].Data)+1)
					active = bytes.Replace(raw, []byte(old), []byte(next), 1)
				case "missing":
					delete(items, name+".md")
				}
				var out bytes.Buffer
				if err := printItems(&out, items, active); err == nil {
					t.Fatal("read must refuse an item that differs from the active manifest")
				}
				if strings.Contains(out.String(), name+":") || strings.Contains(out.String(), "Read checks passed") {
					t.Fatalf("read printed an unchecked item or claimed success: %s", &out)
				}
			})
		}
	}
}
