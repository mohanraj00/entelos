package example_test

import (
	"encoding/json"
	"example.org/cel-handoff/example"
	"fmt"
	"os"
	"testing"
)

func TestOrdinaryCodeBaseline(t *testing.T) {
	raw, err := os.ReadFile("../testdata/scope-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Fixed map[string]string `json:"fixed_bindings"`
		Cases []struct {
			Name   string         `json:"case"`
			Inputs map[string]any `json:"inputs"`
			Status string         `json:"status"`
			Value  bool           `json:"value"`
		} `json:"expected_calls"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Cases {
		if c.Status != "ok" {
			continue
		}
		got := example.Visible(c.Inputs["state"].(string), c.Inputs["archived"].(bool), f.Fixed["allowed_state"])
		fmt.Printf("%s: %v\n", c.Name, got)
		if got != c.Value {
			t.Fatalf("%s: got %v want %v", c.Name, got, c.Value)
		}
	}
}
