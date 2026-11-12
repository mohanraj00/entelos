package handoff_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	h "github.com/mohanraj00/entelos/src/cel-handoff"
)

const typedVectorDir = "../cel-lifecycle/conformance/typed"

// typedVector is one file of the format typed-profile-vector/v1. The
// expected values come from the specification, not from this engine.
type typedVector struct {
	Format      string                     `json:"format"`
	Description string                     `json:"description"`
	ID          string                     `json:"id"`
	Bundle      map[string]json.RawMessage `json:"bundle"`
	Steps       []struct {
		Name       string                     `json:"name"`
		Op         string                     `json:"op"`
		Replace    map[string]json.RawMessage `json:"replace"`
		InputsText string                     `json:"inputs_text"`
		Cases      json.RawMessage            `json:"cases"`
		Expect     struct {
			OK         *bool `json:"ok"`
			Diagnostic *struct {
				Code, Stage, Field string
			} `json:"diagnostic"`
			Inputs json.RawMessage `json:"inputs"`
			Output json.RawMessage `json:"output"`
			Status string          `json:"status"`
			Value  json.RawMessage `json:"value"`
			Field  *string         `json:"field"`
			Cases  []bool          `json:"cases"`
		} `json:"expect"`
	} `json:"steps"`
}

// typedFiles binds the vector bundle to the build under test. The keys of
// replace change the same keys of the bundle for one step only.
func typedFiles(t *testing.T, bundle, replace map[string]json.RawMessage) h.Files {
	t.Helper()
	merged := map[string]json.RawMessage{}
	for k, v := range bundle {
		merged[k] = v
	}
	for k, v := range replace {
		merged[k] = v
	}
	var source string
	if err := json.Unmarshal(merged["source"], &source); err != nil {
		t.Fatalf("source: %v", err)
	}
	env, fixed := compactJSON(t, merged["environment"]), compactJSON(t, merged["fixed"])
	m := map[string]any{"function_id": merged["function_id"], "profile": merged["profile"], "caller_variables": merged["caller_variables"], "limits": merged["limits"],
		"implementation_ref": h.BuildID(), "environment_ref": digest("application/json", env), "source_ref": digest("text/plain", []byte(source)), "fixed_ref": digest("application/json", fixed)}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return h.Files{Manifest: raw, Environment: env, Source: []byte(source), Fixed: fixed}
}

// compactJSON keeps each number token as it is in the vector file.
func compactJSON(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// sameJSON compares two JSON texts as values, with exact number tokens.
func sameJSON(t *testing.T, want json.RawMessage, got any) bool {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(raw []byte) any {
		var v any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	return reflect.DeepEqual(decode(want), decode(gotRaw))
}

func TestTypedProfileVectors(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(typedVectorDir, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no typed vectors: %v", err)
	}
	steps := 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var v typedVector
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if v.Format != "typed-profile-vector/v1" || len(v.Steps) == 0 {
			t.Fatalf("%s: format %q with %d steps", path, v.Format, len(v.Steps))
		}
		for _, s := range v.Steps {
			steps++
			t.Run(v.ID+"/"+s.Name, func(t *testing.T) {
				b, d := h.Load(typedFiles(t, v.Bundle, s.Replace))
				switch s.Op {
				case "load":
					if s.Expect.Diagnostic != nil {
						want := *s.Expect.Diagnostic
						if d == nil || d.Code != want.Code || d.Stage != want.Stage || d.Field != want.Field {
							t.Fatalf("diagnostic %+v, want %+v", d, want)
						}
						return
					}
					if s.Expect.OK == nil || !*s.Expect.OK || d != nil {
						t.Fatalf("load: %+v", d)
					}
				case "describe":
					if d != nil {
						t.Fatalf("load: %+v", d)
					}
					got := b.Describe()
					if got.Profile != "cel-typed/v1" || !sameJSON(t, s.Expect.Inputs, got.Inputs) || !sameJSON(t, s.Expect.Output, got.Output) {
						gi, _ := json.Marshal(got.Inputs)
						gout, _ := json.Marshal(got.Output)
						t.Fatalf("descriptor %s %s %s", got.Profile, gi, gout)
					}
				case "run":
					if d != nil {
						t.Fatalf("load: %+v", d)
					}
					req := append([]byte(`{"bundle_ref":"`+b.Describe().BundleRef+`","inputs":`), s.InputsText...)
					got := b.Run(context.Background(), append(req, '}'))
					if got.Status != s.Expect.Status {
						t.Fatalf("status %+v, want %s", got, s.Expect.Status)
					}
					if s.Expect.Status == "ok" {
						if got.Value == nil || !sameJSON(t, s.Expect.Value, got.Value) || got.Diagnostic != nil {
							t.Fatalf("value %+v, want %s", got, s.Expect.Value)
						}
						return
					}
					if got.Value != nil || got.Diagnostic == nil || s.Expect.Field == nil || got.Diagnostic.Field != *s.Expect.Field {
						t.Fatalf("diagnostic %+v, want field %v", got.Diagnostic, s.Expect.Field)
					}
				case "check":
					if d != nil {
						t.Fatalf("load: %+v", d)
					}
					var cases []h.Case
					dec := json.NewDecoder(bytes.NewReader(s.Cases))
					dec.UseNumber()
					if err := dec.Decode(&cases); err != nil {
						t.Fatal(err)
					}
					got := b.Check(context.Background(), cases)
					passed := []bool{}
					for _, c := range got.Cases {
						passed = append(passed, c.Passed)
					}
					if got.Status != s.Expect.Status || !reflect.DeepEqual(passed, s.Expect.Cases) {
						t.Fatalf("report %s %v, want %s %v", got.Status, passed, s.Expect.Status, s.Expect.Cases)
					}
				default:
					t.Fatalf("unknown op %q", s.Op)
				}
			})
		}
	}
	if steps != 93 {
		t.Fatalf("ran %d typed steps, want 93", steps)
	}
}

func typedAdmission(t *testing.T) (h.Files, *h.Bundle) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(typedVectorDir, "admission-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v typedVector
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	f := typedFiles(t, v.Bundle, nil)
	return f, load(t, f)
}

// The executor vectors cover only cel-string-bool/v1, so the executor and
// its WASM bridge refuse a typed bundle.
func TestExecutorRefusesTypedProfile(t *testing.T) {
	f, b := typedAdmission(t)
	pin, _ := json.Marshal(map[string]any{"function": "admission.review", "scope": "s", "snapshot_ref": "sha256:" + strings.Repeat("5", 64),
		"bundle_ref": b.Describe().BundleRef, "engine_build": h.BuildID(), "revoked": false, "stale_mode": "fail_closed", "from": "2026-01-01", "until": "2027-01-01"})
	if _, d := h.NewExecutor(pin, f); d == nil || d.Code != "unsupported_profile" || d.Field != "/profile" {
		t.Fatalf("executor accepted a typed bundle: %+v", d)
	}
}

// Check needs an expected value for an ok case. Check encodes the inputs to
// JSON, so a Go float64 812 is the token 812 on the wire.
func TestTypedCheckCases(t *testing.T) {
	_, b := typedAdmission(t)
	inputs := map[string]any{"score": json.Number("812"), "observed_on": "2026-03-14", "sources": []any{"registry", "survey"}}
	ok := map[string]any{"admit": true, "reason": "eligible"}
	got := b.Check(context.Background(), []h.Case{{Name: "token", Inputs: inputs, Status: "ok", Value: ok}})
	if got.Status != "passed" {
		t.Fatalf("valid case failed: %+v", got)
	}
	got = b.Check(context.Background(), []h.Case{{Name: "no-value", Inputs: inputs, Status: "ok"}})
	if got.Status != "failed" {
		t.Fatalf("case without value passed: %+v", got)
	}
	got = b.Check(context.Background(), []h.Case{{Name: "float", Inputs: map[string]any{"score": 812.0}, Status: "needs_input", Field: "/observed_on"}})
	if got.Status != "passed" {
		t.Fatalf("a Go float64 input must encode as the token 812: %+v", got)
	}
}
