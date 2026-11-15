package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// The command verifies the vector files from disk: a valid record, a
// changed record, an unknown key and a revoked key.
func TestCommandVerifiesAFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "signed", "vectors", "signed-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(lifecycle.Object)
	dir := t.TempDir()
	keys := filepath.Join(dir, "keyset.json")
	if err := os.WriteFile(keys, lifecycle.Encode(doc["key_sets"].(lifecycle.Object)["current"]), 0o644); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		exit   int
		status string
	}{
		"valid-approval":   {0, "ok"},
		"valid-activation": {0, "ok"},
		"changed-record":   {1, "record_mismatch"},
		"unknown-key":      {1, "unknown_key"},
		"revoked-key":      {1, "revoked_key"},
		"format-major-2":   {1, "unsupported_record_format"},
	}
	seen := 0
	for _, x := range doc["cases"].(lifecycle.Array) {
		c := x.(lifecycle.Object)
		expect, ok := want[c["name"].(string)]
		if !ok {
			continue
		}
		seen++
		path := filepath.Join(dir, c["name"].(string)+".json")
		if err := os.WriteFile(path, lifecycle.Encode(c["signed"]), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		exit := run([]string{"--keys", keys, path}, &stdout, &stderr)
		out, err := lifecycle.Decode(stdout.Bytes())
		if err != nil {
			t.Fatalf("%s: output %q", c["name"], stdout.String())
		}
		if exit != expect.exit || out.(lifecycle.Object)["status"] != expect.status {
			t.Errorf("%s: exit %d, output %s", c["name"], exit, stdout.String())
		}
	}
	if seen != len(want) {
		t.Fatalf("found %d of %d cases", seen, len(want))
	}
	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--keys", keys}, &stdout, &stderr); exit != 2 || !strings.Contains(stderr.String(), "required") {
		t.Fatalf("usage: exit %d, %q", exit, stderr.String())
	}
	if exit := run([]string{"--keys", keys, filepath.Join(dir, "absent.json")}, &stdout, &stderr); exit != 1 {
		t.Fatalf("absent file: exit %d", exit)
	}
}
