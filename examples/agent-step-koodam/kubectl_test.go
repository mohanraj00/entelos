package agentstep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKubectl puts a shell script with the name kubectl first on the PATH.
func fakeKubectl(t *testing.T, script string) Kubectl {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return Kubectl{Kubeconfig: filepath.Join(dir, "kubeconfig")}
}

// The writer keeps the first bytes up to its limit and records the rest.
func TestCappedWriter(t *testing.T) {
	c := &capped{limit: 10}
	for _, part := range []string{"abcdef", "ghijkl", "mnopqr"} {
		if n, err := c.Write([]byte(part)); n != len(part) || err != nil {
			t.Fatalf("write %q: %d, %v", part, n, err)
		}
	}
	if c.buf.String() != "abcdefghij" || !c.over {
		t.Fatalf("kept %q, over %v", c.buf.String(), c.over)
	}
	exact := &capped{limit: 6}
	exact.Write([]byte("abcdef"))
	if exact.buf.String() != "abcdef" || exact.over {
		t.Fatalf("kept %q, over %v", exact.buf.String(), exact.over)
	}
}

// A log that is larger than the output limit is an error, and the command
// still ends. The error text of a failed command is short.
func TestKubectlBoundsItsOutput(t *testing.T) {
	large := fakeKubectl(t, `head -c 1000000 /dev/zero`)
	if out, err := large.Output("steps", "step-abc-run"); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("%d bytes, error %v", len(out), err)
	}
	small := fakeKubectl(t, `printf 'a summary\n'`)
	if out, err := small.Output("steps", "step-abc-run"); err != nil || string(out) != "a summary\n" {
		t.Fatalf("output %q, error %v", out, err)
	}
	noisy := fakeKubectl(t, `head -c 1000000 /dev/zero | tr '\0' 'e' >&2; exit 1`)
	if _, err := noisy.Agent("steps", "step-abc"); err == nil || len(err.Error()) > 600 {
		t.Fatalf("error of %d bytes", len(err.Error()))
	}
	if _, err := (Kubectl{}).Agent("steps", "step-abc"); err == nil {
		t.Fatal("kubectl ran with no kubeconfig")
	}
}
