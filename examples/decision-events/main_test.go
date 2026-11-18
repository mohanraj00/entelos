package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestResumeAcrossRuns(t *testing.T) {
	work := t.TempDir()
	var out bytes.Buffer
	if err := run(work, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "Event:") != 3 || !strings.Contains(out.String(), "Second fact: green.") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := run(work, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Event:") {
		t.Fatal("a restart repeated events", out.String())
	}
}
