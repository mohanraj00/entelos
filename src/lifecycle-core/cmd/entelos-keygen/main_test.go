package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// The command writes a private key file that the Host can read, prints its
// key set entry and never replaces a file.
func TestCommandMakesAPrivateKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing-key.json")
	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--out", path}, &stdout, &stderr); exit != 0 {
		t.Fatalf("exit %d: %s", exit, stderr.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file %v %v", info, err)
	}
	raw, _ := os.ReadFile(path)
	signer, err := lifecycle.NewSigner(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := lifecycle.Decode(stdout.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	entry := out.(lifecycle.Object)["data"].(lifecycle.Object)["key"].(lifecycle.Object)
	if entry["key_id"] != signer.KeyID() || entry["status"] != "active" {
		t.Fatalf("entry %v", entry)
	}
	if _, err := lifecycle.ParseKeySet(lifecycle.Encode(lifecycle.Object{"format": lifecycle.KeySetFormat, "signer": "s", "keys": lifecycle.Array{entry}})); err != nil {
		t.Fatal(err)
	}
	if exit := run([]string{"--out", path}, &stdout, &stderr); exit != 1 {
		t.Fatalf("second run: exit %d", exit)
	}
	if again, _ := os.ReadFile(path); !bytes.Equal(again, raw) {
		t.Fatal("the second run changed the key file")
	}
}
