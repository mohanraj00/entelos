//go:build unix

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
	if code := run([]string{"--repo", path, "--check"}, &out, &errs); code != 1 || !strings.Contains(out.String(), `"invalid_store"`) {
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

// The command reports a stale lock file with --check, removes it without
// --check, and then reports that no lock file is there.
func TestCheckAndRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.git")
	store, err := lifecycle.OpenGitStore(path)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(store.Path, filepath.FromSlash(lifecycle.RootRef)+".lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		args []string
		want string
		left bool
	}{
		{[]string{"--repo", path, "--check"}, `"lock":"stale"`, true},
		{[]string{"--repo", path}, `"lock":"removed"`, false},
		{[]string{"--repo", path}, `"lock":"absent"`, false},
	} {
		var out, errs bytes.Buffer
		if code := run(step.args, &out, &errs); code != 0 || !strings.Contains(out.String(), step.want) {
			t.Fatalf("%v: exit %d: %s %s", step.args, code, out.String(), errs.String())
		}
		if _, err := os.Stat(lock); (err == nil) != step.left {
			t.Fatalf("%v: lock file: %v", step.args, err)
		}
	}
}
