// Command entelos-keygen makes a new private signing key file for signed
// records. It writes one JSON line with the key set entry of the new key.
// The file is new, and has the mode 0600. See src/protocol/signed/SIGNED.md.
//
// Usage: entelos-keygen --out FILE
package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("entelos-keygen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "the new signing key file; it must not exist")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *out == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "entelos-keygen: --out is required, and no other arguments are permitted")
		return 2
	}
	entry, err := generate(*out)
	if err != nil {
		fmt.Fprintln(stderr, "entelos-keygen:", err)
		return 1
	}
	stdout.Write(lifecycle.Encode(lifecycle.Object{"status": "ok", "data": lifecycle.Object{"key": entry}}))
	return 0
}

func generate(path string) (lifecycle.Object, error) {
	raw, err := lifecycle.NewSigningKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := lifecycle.NewSigner(raw)
	if err != nil {
		return nil, err
	}
	// O_EXCL never replaces a key that is in use.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	return signer.PublicKey(), nil
}
