// Command entelos-migrate builds the review index of approval API 1.1 for a
// store from v0.3.0. It writes one JSON line: {"status":"ok","data":...} or a
// fault record. See "Migration from v0.3.0" in
// src/protocol/approval/APPROVAL.md.
//
// Usage: entelos-migrate --repo DIR [--dry-run]
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
	flags := flag.NewFlagSet("entelos-migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", "", "the bare Git repository of the store")
	dryRun := flags.Bool("dry-run", false, "list the writes and write nothing")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *repo == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "entelos-migrate: --repo is required, and no other arguments are permitted")
		return 2
	}
	write := func(value any) { stdout.Write(lifecycle.Encode(value)) }
	// OpenGitStore makes a missing repository. A migration must not do that.
	if _, err := os.Stat(*repo); err != nil {
		write(lifecycle.NewFault("invalid_store", "the store does not exist: "+*repo, "store").Record())
		return 1
	}
	store, err := lifecycle.OpenGitStore(*repo)
	var result lifecycle.Object
	if err == nil {
		result, err = lifecycle.MigrateReviewIndex(store, *dryRun)
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
