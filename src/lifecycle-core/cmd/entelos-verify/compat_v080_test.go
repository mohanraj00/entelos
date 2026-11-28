package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// The command verifies each signed record that the v0.8.0 Host wrote. The
// records and the public key set come from
// testdata/migration/v080-store.json. See "Compatibility promise for 1.x
// releases" in docs/STABILITY.md.
func TestVerifiesTheSignedRecordsOfV080(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "migration", "v080-store.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(lifecycle.Object)
	if doc["made_by"].(lifecycle.Object)["tag"] != "v0.8.0" {
		t.Fatalf("unexpected fixture header %v", doc["made_by"])
	}
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(keys, lifecycle.Encode(doc["keys"]), 0o644); err != nil {
		t.Fatal(err)
	}
	kinds := map[any]int{}
	for i, x := range doc["signed"].(lifecycle.Array) {
		signed := x.(lifecycle.Object)
		path := filepath.Join(dir, "signed.json")
		if err := os.WriteFile(path, lifecycle.Encode(signed["document"]), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		exit := run([]string{"--keys", keys, path}, &stdout, &stderr)
		out, err := lifecycle.Decode(stdout.Bytes())
		if err != nil {
			t.Fatalf("signed record %d: output %q", i, stdout.String())
		}
		result := out.(lifecycle.Object)
		if exit != 0 || result["status"] != "ok" || result["data"].(lifecycle.Object)["record_kind"] != signed["record_kind"] {
			t.Errorf("signed record of %s: exit %d, output %s", signed["record_ref"], exit, stdout.String())
		}
		kinds[signed["record_kind"]]++
	}
	if kinds["decision"] != 2 || kinds["event"] != 3 || kinds["activation"] != 2 || kinds["suspension"] != 1 {
		t.Fatalf("signed record kinds %v", kinds)
	}
}
