package handoff_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	h "example.org/cel-handoff"
	"fmt"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) h.Files {
	t.Helper()
	env := []byte(`{"name":"record-filter","variables":[{"name":"state","type":"string"},{"name":"archived","type":"bool"},{"name":"allowed_state","type":"string"}]}`)
	source := []byte(`state == allowed_state && !archived`)
	fixed := []byte(`{"allowed_state":"ready"}`)
	m := map[string]any{"function_id": "record.visible", "profile": "cel-string-bool/v1", "caller_variables": []string{"state", "archived"}, "implementation_ref": h.BuildID(), "environment_ref": digest("application/json", env), "source_ref": digest("text/plain", source), "fixed_ref": digest("application/json", fixed), "limits": map[string]any{"max_request_bytes": 4096, "max_string_utf8_bytes": 256, "max_source_utf8_bytes": 4096, "parser_recursion_limit": 64, "evaluation_cost_limit": 1000}}
	raw, _ := json.Marshal(m)
	return h.Files{Manifest: raw, Environment: env, Source: source, Fixed: fixed}
}
func load(t *testing.T, f h.Files) *h.Bundle {
	t.Helper()
	b, d := h.Load(f)
	if d != nil {
		t.Fatalf("load: %+v", d)
	}
	return b
}
func request(ref string, inputs any) []byte {
	b, _ := json.Marshal(map[string]any{"bundle_ref": ref, "inputs": inputs})
	return b
}
func TestCallerCanDiscoverAndInvokeFixedRule(t *testing.T) {
	b := load(t, fixture(t))
	d := b.Describe()
	got := b.Run(context.Background(), request(d.BundleRef, map[string]any{"state": "ready", "archived": false}))
	if got.Status != "ok" || got.Value == nil || !*got.Value {
		t.Fatalf("matching record: %+v", got)
	}
	props := d.Inputs["properties"].(map[string]any)
	if len(props) != 2 || props["allowed_state"] != nil {
		t.Fatalf("caller sees fixed data: %v", props)
	}
	fmt.Println("caller can discover and invoke")
}

func digest(media string, body []byte) string {
	sum := sha256.Sum256(append([]byte(media+"\x00"), body...))
	return fmt.Sprintf("sha256:%x", sum)
}

func TestStrictCallerFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/scope-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name   string         `json:"case"`
			Inputs map[string]any `json:"inputs"`
			Status string         `json:"status"`
			Value  *bool          `json:"value"`
			Field  string         `json:"field"`
		} `json:"expected_calls"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	b := load(t, fixture(t))
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got := b.Run(context.Background(), request(b.Describe().BundleRef, c.Inputs))
			if got.Status != c.Status {
				t.Fatalf("status %s want %s: %+v", got.Status, c.Status, got)
			}
			if c.Value != nil && (got.Value == nil || *got.Value != *c.Value) {
				t.Fatalf("value %+v want %v", got, *c.Value)
			}
			if c.Field != "" && (got.Diagnostic == nil || got.Diagnostic.Field != c.Field) {
				t.Fatalf("field: %+v", got)
			}
		})
	}
	for _, raw := range []string{
		`{"bundle_ref":"REF","inputs":{"state":"ready","archived":false,"archived":true}}`,
		`{"bundle_ref":"REF","inputs":{"state":"ready","archived":false},"expr":"true"}`,
		`{"bundle_ref":"REF","inputs":{"state":"ready","archived":false}} {}`,
		`{"bundle_ref":"REF","inputs":{"state":"\ud800","archived":false}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			got := b.Run(context.Background(), []byte(strings.ReplaceAll(raw, "REF", b.Describe().BundleRef)))
			if got.Status != "invalid_input" {
				t.Fatalf("ambiguous JSON accepted: %+v", got)
			}
		})
	}
	t.Run("oversize", func(t *testing.T) {
		got := b.Run(context.Background(), request(b.Describe().BundleRef, map[string]any{"state": strings.Repeat("é", 129), "archived": false}))
		if got.Status != "invalid_input" {
			t.Fatalf("UTF8 byte bound ignored: %+v", got)
		}
	})
}

func change(t *testing.T, f h.Files, edit func(map[string]any)) h.Files {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(f.Manifest, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	f.Manifest, _ = json.Marshal(m)
	return f
}
func rehash(t *testing.T, f h.Files) h.Files {
	return change(t, f, func(m map[string]any) {
		m["environment_ref"] = digest("application/json", f.Environment)
		m["fixed_ref"] = digest("application/json", f.Fixed)
		m["source_ref"] = digest("text/plain", f.Source)
	})
}
func TestLoadRejectsUnusableClosure(t *testing.T) {
	tests := []struct {
		name, code string
		edit       func(*h.Files)
	}{
		{"unknown-variable", "compile_error", func(f *h.Files) { f.Source = []byte(`missing == state`) }},
		{"wrong-output", "output_type", func(f *h.Files) { f.Source = []byte(`state`) }},
		{"field-type-drift", "compile_error", func(f *h.Files) {
			f.Environment = []byte(strings.Replace(string(f.Environment), `"type":"bool"`, `"type":"string"`, 1))
		}},
		{"unsupported-number", "unsupported_profile", func(f *h.Files) {
			f.Environment = []byte(strings.Replace(string(f.Environment), `"type":"bool"`, `"type":"int"`, 1))
		}},
		{"unsupported-extension", "unsupported_profile", func(f *h.Files) {
			f.Environment = []byte(`{"name":"bad","variables":[],"extensions":[{"name":"strings","version":4}]}`)
		}},
		{"missing-function-implementation", "unsupported_profile", func(f *h.Files) {
			f.Environment = []byte(`{"name":"bad","variables":[],"functions":[{"name":"custom"}]}`)
			f.Source = []byte(`custom()`)
		}},
		{"duplicate-variable", "invalid_environment", func(f *h.Files) {
			f.Environment = []byte(`{"name":"bad","variables":[{"name":"state","type":"string"},{"name":"state","type":"bool"}]}`)
		}},
		{"missing-fixed-binding", "invalid_bindings", func(f *h.Files) { f.Fixed = []byte(`{}`) }},
		{"mistyped-fixed-binding", "invalid_bindings", func(f *h.Files) { f.Fixed = []byte(`{"allowed_state":true}`) }},
		{"extra-fixed-binding", "invalid_bindings", func(f *h.Files) { f.Fixed = []byte(`{"allowed_state":"ready","other":true}`) }},
		{"overlapping-binding", "invalid_bindings", func(f *h.Files) { f.Fixed = []byte(`{"allowed_state":"ready","state":"ready"}`) }},
		{"duplicate-caller", "invalid_bindings", func(f *h.Files) {
			*f = change(t, *f, func(m map[string]any) { m["caller_variables"] = []string{"state", "archived", "state"} })
		}},
		{"unknown-caller", "invalid_bindings", func(f *h.Files) {
			*f = change(t, *f, func(m map[string]any) { m["caller_variables"] = []string{"other", "archived"} })
		}},
		{"missing-build", "implementation_unavailable", func(f *h.Files) { *f = change(t, *f, func(m map[string]any) { m["implementation_ref"] = "" }) }},
		{"wrong-build", "implementation_unavailable", func(f *h.Files) { *f = change(t, *f, func(m map[string]any) { m["implementation_ref"] = "absent" }) }},
		{"unsupported-profile", "unsupported_profile", func(f *h.Files) { *f = change(t, *f, func(m map[string]any) { m["profile"] = "numeric/v1" }) }},
		{"no-limits", "invalid_bundle", func(f *h.Files) { *f = change(t, *f, func(m map[string]any) { m["limits"] = map[string]any{} }) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture(t)
			tc.edit(&f)
			_, d := h.Load(rehash(t, f))
			if d == nil || d.Code != tc.code {
				t.Fatalf("diagnostic %+v want %s", d, tc.code)
			}
		})
	}
	t.Run("changed-bytes-without-new-ref", func(t *testing.T) {
		f := fixture(t)
		f.Source = []byte(`true`)
		_, d := h.Load(f)
		if d == nil || d.Code != "artifact_mismatch" {
			t.Fatalf("digest guard: %+v", d)
		}
	})
}
func TestOldIdentityCannotCallChangedClosure(t *testing.T) {
	f := fixture(t)
	old := load(t, f).Describe().BundleRef
	edits := []func(*h.Files){
		func(f *h.Files) { f.Fixed = []byte(`{"allowed_state":"closed"}`) },
		func(f *h.Files) { f.Source = []byte(`true`) },
		func(f *h.Files) { f.Environment = append(f.Environment, ' ') },
		func(f *h.Files) {
			*f = change(t, *f, func(m map[string]any) { m["limits"].(map[string]any)["evaluation_cost_limit"] = 999 })
		},
	}
	for i, edit := range edits {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f := fixture(t)
			edit(&f)
			b := load(t, rehash(t, f))
			if b.Describe().BundleRef == old {
				t.Fatal("identity did not change")
			}
			got := b.Run(context.Background(), request(old, map[string]any{"state": "ready", "archived": false}))
			if got.Status != "artifact_mismatch" {
				t.Fatalf("silently used new bundle: %+v", got)
			}
		})
	}
	b := load(t, f)
	got := b.Run(context.Background(), request("", map[string]any{"state": "ready", "archived": false}))
	if got.Status != "artifact_mismatch" {
		t.Fatalf("missing expected identity: %+v", got)
	}
}
func TestUnusedFieldStaysRequired(t *testing.T) {
	f := fixture(t)
	f.Source = []byte(`true`)
	b := load(t, rehash(t, f))
	d := b.Describe()
	if len(d.Inputs["properties"].(map[string]any)) != 2 {
		t.Fatal(d)
	}
	got := b.Run(context.Background(), request(d.BundleRef, map[string]any{"state": "ready"}))
	if got.Status != "needs_input" {
		t.Fatalf("AST shrank declared contract: %+v", got)
	}
}
func TestResourceExhaustionAndCancellationAreNotFalse(t *testing.T) {
	f := change(t, fixture(t), func(m map[string]any) { m["limits"].(map[string]any)["evaluation_cost_limit"] = 1 })
	b := load(t, f)
	got := b.Run(context.Background(), request(b.Describe().BundleRef, map[string]any{"state": "ready", "archived": false}))
	if got.Status != "resource_exhausted" || got.Value != nil {
		t.Fatalf("cost exhaustion: %+v", got)
	}
	b = load(t, fixture(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = b.Run(ctx, request(b.Describe().BundleRef, map[string]any{"state": "ready", "archived": false}))
	if got.Status != "cancelled" || got.Value != nil {
		t.Fatalf("cancelled call: %+v", got)
	}
}

func TestCheckRetainsFailedExpectationsWithoutActivation(t *testing.T) {
	b := load(t, fixture(t))
	yes := true
	no := false
	cases := []h.Case{{Name: "known-true", Inputs: map[string]any{"state": "ready", "archived": false}, Status: "ok", Value: &yes}, {Name: "deliberately-wrong-expectation", Inputs: map[string]any{"state": "ready", "archived": false}, Status: "ok", Value: &no}}
	got := b.Check(context.Background(), cases)
	if got.Status != "failed" || len(got.Cases) != 2 || !got.Cases[0].Passed || got.Cases[1].Passed || got.Cases[1].Actual.Value == nil || !*got.Cases[1].Actual.Value {
		t.Fatalf("check hid failure: %+v", got)
	}
	got = b.Check(context.Background(), nil)
	if got.Status != "failed" {
		t.Fatalf("empty check passed: %+v", got)
	}
}

func TestNullInputsAreInvalidAndSizeLimitsApply(t *testing.T) {
	b := load(t, fixture(t))
	raw := []byte(`{"bundle_ref":"` + b.Describe().BundleRef + `","inputs":null}`)
	got := b.Run(context.Background(), raw)
	if got.Status != "invalid_input" || got.Diagnostic.Field != "/inputs" {
		t.Fatalf("null object: %+v", got)
	}
	t.Run("request-byte-limit", func(t *testing.T) {
		got := b.Run(context.Background(), request(b.Describe().BundleRef, map[string]any{"state": strings.Repeat("x", 5000), "archived": false}))
		if got.Status != "invalid_input" {
			t.Fatalf("oversize: %+v", got)
		}
	})
	t.Run("source-byte-limit", func(t *testing.T) {
		f := fixture(t)
		f.Source = []byte("true //" + strings.Repeat("x", 4096))
		_, d := h.Load(rehash(t, f))
		if d == nil || d.Code != "invalid_bundle" {
			t.Fatalf("oversize source: %+v", d)
		}
	})
	t.Run("parser-recursion", func(t *testing.T) {
		f := fixture(t)
		f.Source = []byte(strings.Repeat("(", 100) + "true" + strings.Repeat(")", 100))
		_, d := h.Load(rehash(t, f))
		if d == nil || d.Code != "compile_error" {
			t.Fatalf("parser bound: %+v", d)
		}
	})
}
