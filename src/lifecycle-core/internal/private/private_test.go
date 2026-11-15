package private

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRefusesAFileThatIsNotPrivate(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, err := Read(good, "key file", 16); err != nil || string(raw) != "secret" {
		t.Fatalf("got %q %v", raw, err)
	}
	if _, err := Read(good, "key file", 5); err == nil {
		t.Fatal("a file over the limit was read")
	}
	open := filepath.Join(dir, "open")
	if err := os.WriteFile(open, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{open, link, dir, filepath.Join(dir, "absent")} {
		if _, err := Read(path, "key file", 16); err == nil {
			t.Fatalf("%s was read", path)
		}
	}
}
