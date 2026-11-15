// Command entelos-verify checks one signed record file against a public key
// set file. It calls no service. It writes one JSON line:
// {"status":"ok","data":...} or a fault record. The exit status is 0 for a
// valid record, 1 for a refusal and 2 for a usage error. See
// src/protocol/signed/SIGNED.md.
//
// Usage: entelos-verify --keys KEYSET SIGNED
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// maxFile is more than the largest valid signed record document.
const maxFile = 2 << 20

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("entelos-verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	keysPath := flags.String("keys", "", "the public key set file of the signer")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *keysPath == "" || flags.NArg() != 1 {
		fmt.Fprintln(stderr, "entelos-verify: --keys and one signed record file are required")
		return 2
	}
	write := func(value any) { stdout.Write(lifecycle.Encode(value)) }
	result, err := verify(*keysPath, flags.Arg(0))
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

func verify(keysPath, signedPath string) (lifecycle.Object, error) {
	rawKeys, err := read(keysPath)
	if err != nil {
		return nil, err
	}
	keys, err := lifecycle.ParseKeySet(rawKeys)
	if err != nil {
		return nil, err
	}
	raw, err := read(signedPath)
	if err != nil {
		return nil, err
	}
	return lifecycle.VerifySigned(raw, keys)
}

func read(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxFile {
		return nil, errors.New(path + " is too large")
	}
	return raw, nil
}
