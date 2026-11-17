package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"runtime"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

//go:embed runner.go
var runnerSource string

type item struct {
	ID     string `json:"item_id"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

type manifest struct {
	Items []item `json:"items"`
}

type contentRunner struct{ files fs.FS }

func digest(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }

func (*contentRunner) Identity() (string, error) {
	return digest([]byte(runnerSource + "\x00" + runtime.Version())), nil
}

func (r *contentRunner) Expected(lifecycle.Candidate) string {
	identity, _ := r.Identity()
	return identity
}

// The Host checks the manifest schema. The application checks the item files.
func (r *contentRunner) Qualify(in lifecycle.QualificationInput) (lifecycle.Verdict, error) {
	raw, ok := in.Files["content"]
	if !ok || digest(raw) != in.Candidate.BundleRef {
		return lifecycle.Verdict{}, lifecycle.NewFault("artifact_mismatch", "manifest identity differs", "runner")
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return lifecycle.Verdict{}, err
	}
	var cases []item
	if err := json.Unmarshal(lifecycle.Encode(in.Cases), &cases); err != nil {
		return lifecycle.Verdict{}, lifecycle.NewFault("invalid_plan", "expected item digests and sizes", "runner")
	}
	expected := make(map[string]item, len(cases))
	for _, c := range cases {
		if _, duplicate := expected[c.ID]; duplicate {
			return lifecycle.Verdict{}, lifecycle.NewFault("invalid_plan", "duplicate item case", "runner")
		}
		expected[c.ID] = c
	}
	status := "passed"
	checks := lifecycle.Array{}
	for _, item := range m.Items {
		// A Host-validated item_id has no slash. Read only this folder.
		body, err := fs.ReadFile(r.files, item.ID+".md")
		c, covered := expected[item.ID]
		matches := covered && err == nil && len(body) == c.Size && digest(body) == c.Digest &&
			len(body) == item.Size && digest(body) == item.Digest
		checks = append(checks, lifecycle.Object{"item_id": item.ID, "passed": matches})
		if !matches {
			status = "failed"
		}
	}
	return lifecycle.Verdict{Outcome: lifecycle.Object{"status": status,
		"bundle_ref": in.Candidate.BundleRef, "implementation_ref": r.Expected(in.Candidate), "items": checks}}, nil
}
