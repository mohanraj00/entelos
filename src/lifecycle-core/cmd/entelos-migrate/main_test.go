package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// The command refuses a missing repository and does not make one.
func TestMissingRepositoryIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.git")
	var out, errs bytes.Buffer
	if code := run([]string{"--repo", path, "--dry-run"}, &out, &errs); code != 1 || !strings.Contains(out.String(), `"invalid_store"`) {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the command made a repository")
	}
}

func TestUsage(t *testing.T) {
	var out, errs bytes.Buffer
	for _, args := range [][]string{{}, {"--repo", "x", "extra"}, {"--unknown"}} {
		if code := run(args, &out, &errs); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

// An empty store needs no migration.
func TestEmptyStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.git")
	if _, err := lifecycle.OpenGitStore(path); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := run([]string{"--repo", path}, &out, &errs); code != 0 || !strings.Contains(out.String(), `"action":"empty"`) {
		t.Fatalf("exit %d: %s %s", code, out.String(), errs.String())
	}
}
