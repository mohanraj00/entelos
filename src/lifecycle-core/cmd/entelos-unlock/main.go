// Command entelos-unlock examines the lock file of the root ref of a Git
// store, and removes a stale lock file. A git process that stops with
// SIGKILL during a write leaves this file, and each later write then fails
// with store_error. The command writes one JSON line:
// {"status":"ok","data":{"lock":...,"root":...}} or a fault record. The
// exit status is 0 for a result, 1 for a refusal and 2 for a usage error.
// See "Stale lock of the root ref" in src/protocol/activation/ACTIVATION.md.
//
// Usage: entelos-unlock --repo DIR [--check]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("entelos-unlock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", "", "the bare Git repository of the store")
	check := flags.Bool("check", false, "report the state of the lock file and remove nothing")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *repo == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "entelos-unlock: --repo is required, and no other arguments are permitted")
		return 2
	}
	write := func(value any) { stdout.Write(lifecycle.Encode(value)) }
	// OpenGitStore makes a missing repository. A recovery must not do that.
	if _, err := os.Stat(*repo); err != nil {
		write(lifecycle.NewFault("invalid_store", "the store does not exist: "+*repo, "store").Record())
		return 1
	}
	store, err := lifecycle.OpenGitStore(*repo)
	var result lifecycle.Object
	if err == nil {
		result, err = store.RecoverRootLock(!*check)
	}
	var fault *lifecycle.Fault
	switch {
	case err == nil:
		write(lifecycle.Object{"status": "ok", "data": result})
		return 0
	case errors.As(err, &fault):
		write(fault.Record())
	default:
		write(lifecycle.Object{"status": "configuration_error", "message": err.Error()})
	}
	return 1
}
