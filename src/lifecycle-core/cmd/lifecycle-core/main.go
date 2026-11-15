// Command lifecycle-core is the JSONL host of the Go lifecycle core. The host
// selects the context and the runtime; a request cannot supply authority.
// Each input line is one request record. Each output line is one response record.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
	"github.com/mohanraj00/entelos/src/lifecycle-core/internal/private"
)

const maxRequest = 1048576

func main() { os.Exit(run()) }

func run() int {
	repo := flag.String("repo", "", "dedicated bare Git repository")
	context := flag.String("context", "", "trusted host context, not caller input")
	runtime := flag.String("runtime", "", "pinned CEL executable")
	work := flag.String("work", "", "work directory")
	clockFile := flag.String("clock-file", "", "for tests only: a file with the time of the next request")
	signingKey := flag.String("signing-key", "", "private file with the signing key of approval and activation records")
	flag.Parse()
	if *repo == "" || *context == "" || *runtime == "" || *work == "" {
		fmt.Fprintln(os.Stderr, "lifecycle-core: --repo, --context, --runtime and --work are required")
		return 2
	}
	host, err := open(*repo, *context, *runtime, *work)
	if err != nil {
		os.Stdout.Write(lifecycle.Encode(configuration(err)))
		return 1
	}
	if *clockFile != "" {
		host.Clock = lifecycle.FileClock{Path: *clockFile}
	}
	if *signingKey != "" {
		signer, err := readSigner(*signingKey)
		if err != nil {
			os.Stdout.Write(lifecycle.Encode(configuration(err)))
			return 1
		}
		host.SetSigner(signer)
	}
	return serve(host, os.Stdin, os.Stdout)
}

// serve is the JSONL path shared by the command and its stream tests.
func serve(host *lifecycle.Host, input io.Reader, output io.Writer) int {
	out := bufio.NewWriter(output)
	defer out.Flush()
	write := func(value any) {
		out.Write(lifecycle.Encode(value))
		out.Flush()
	}
	in := bufio.NewReaderSize(input, maxRequest+1)
	for {
		raw, err := readLine(in)
		if len(raw) == 0 && err != nil {
			break
		}
		if len(raw) > maxRequest {
			write(lifecycle.NewFault("invalid_request", "request exceeds reference byte bound; stream closed", "").Record())
			return 1
		}
		request, derr := lifecycle.Decode(raw)
		var result any
		if derr == nil {
			result, derr = host.Handle(request)
		}
		var fault *lifecycle.Fault
		switch {
		case derr == nil:
			write(lifecycle.Object{"status": "ok", "data": result})
		case errors.As(derr, &fault):
			write(fault.Record())
		default:
			write(configuration(derr))
			return 1
		}
	}
	return 0
}

// readLine reads one line of at most maxRequest+1 bytes, like readline(size).
func readLine(in *bufio.Reader) ([]byte, error) {
	var line []byte
	for len(line) <= maxRequest {
		b, err := in.ReadByte()
		if err != nil {
			return line, err
		}
		line = append(line, b)
		if b == '\n' {
			return line, nil
		}
	}
	return line, nil
}

func open(repo, contextPath, binary, work string) (*lifecycle.Host, error) {
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		return nil, err
	}
	context, err := lifecycle.Decode(raw)
	if err != nil {
		return nil, err
	}
	store, err := lifecycle.OpenGitStore(repo)
	if err != nil {
		return nil, err
	}
	rt, err := lifecycle.NewCELRuntime(binary, filepath.Join(work, "tmp"))
	if err != nil {
		return nil, err
	}
	return lifecycle.NewHost(store, rt, context)
}

// readSigner reads the private signing key file.
func readSigner(path string) (*lifecycle.Signer, error) {
	raw, err := private.Read(path, "signing key file", 4096)
	if err != nil {
		return nil, err
	}
	return lifecycle.NewSigner(raw)
}

func configuration(err error) lifecycle.Object {
	var fault *lifecycle.Fault
	if errors.As(err, &fault) {
		return fault.Record()
	}
	return lifecycle.Object{"status": "configuration_error", "message": err.Error()}
}
