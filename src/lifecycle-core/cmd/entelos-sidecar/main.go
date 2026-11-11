// Command entelos-sidecar serves the wire protocol 1.0 of src/protocol and
// the approval API 1.1 of src/protocol/approval on a local Unix domain
// socket, in front of the Go lifecycle Host. The host context comes from a
// file that the operator selects; a request cannot supply it. Each socket
// request requires a credential of the operator: the one token of
// --token-file, or one principal of --credentials-file.
//
// --clock-file is for conformance tests only. The Host then reads its time
// from that file for each request, not from the system clock.
//
// When the socket listens, the command writes one JSON line to stdout:
// {"socket":"<path>","status":"listening"}. On SIGINT or SIGTERM, it closes
// the socket, removes the socket file and exits with status 0.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
	"github.com/mohanraj00/entelos/src/lifecycle-core/protocol"
)

func main() { os.Exit(run()) }

func run() int {
	socket := flag.String("socket", "", "socket path; it must not exist, or it must be a stale socket")
	repo := flag.String("repo", "", "dedicated bare Git repository")
	contextPath := flag.String("context", "", "trusted host context, not caller input")
	runtime := flag.String("runtime", "", "pinned CEL executable")
	work := flag.String("work", "", "work directory")
	schema := flag.String("schema", "", "the src/protocol/schema directory; the approval schemas are in ../approval/schema")
	tokenFile := flag.String("token-file", "", "private file containing the sidecar token")
	credentialsFile := flag.String("credentials-file", "", "private file with one token digest, actor and roles for each principal")
	clockFile := flag.String("clock-file", "", "for tests only: a file with the time of the next request")
	flag.Parse()
	if *socket == "" || *repo == "" || *contextPath == "" || *runtime == "" || *work == "" || *schema == "" || (*tokenFile == "") == (*credentialsFile == "") {
		fmt.Fprintln(os.Stderr, "entelos-sidecar: --socket, --repo, --context, --runtime, --work, --schema and one of --token-file or --credentials-file are required")
		return 2
	}
	schemas, err := protocol.LoadSchemas(*schema)
	if err != nil {
		return stop(err)
	}
	if err := schemas.LoadApproval(protocol.ApprovalDir(*schema)); err != nil {
		return stop(err)
	}
	host, err := open(*repo, *contextPath, *runtime, *work, *credentialsFile != "")
	if err != nil {
		return stop(err)
	}
	if *clockFile != "" {
		host.Clock = lifecycle.FileClock{Path: *clockFile}
	}
	adapter := protocol.NewAdapter(host, schemas)
	var server *protocol.Server
	if *credentialsFile != "" {
		server, err = protocol.NewCredentialServer(adapter, *credentialsFile)
	} else {
		server, err = protocol.NewServer(adapter, *tokenFile)
	}
	if err != nil {
		return stop(err)
	}
	listener, err := protocol.Listen(*socket)
	if err != nil {
		return stop(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	os.Stdout.Write(lifecycle.Encode(lifecycle.Object{"status": "listening", "socket": *socket}))
	select {
	case <-signals:
	case err = <-done:
		if err != nil {
			fmt.Fprintln(os.Stderr, "entelos-sidecar:", err)
		}
	}
	if shutdownErr := server.Shutdown(listener); shutdownErr != nil {
		return stop(shutdownErr)
	}
	if err != nil {
		return 1
	}
	return 0
}

func stop(err error) int {
	fmt.Fprintln(os.Stderr, "entelos-sidecar:", err)
	return 1
}

// open makes the Host. With principals, the context must not contain actor
// or roles, so each actor comes from one source. Without principals, the
// context must contain both.
func open(repo, contextPath, binary, work string, principals bool) (*lifecycle.Host, error) {
	raw, err := os.ReadFile(contextPath)
	if err != nil {
		return nil, err
	}
	context, err := lifecycle.Decode(raw)
	if err != nil {
		return nil, err
	}
	ctx, _ := context.(lifecycle.Object)
	_, hasActor := ctx["actor"]
	_, hasRoles := ctx["roles"]
	switch {
	case principals && (hasActor || hasRoles):
		return nil, lifecycle.NewFault("configuration_error", "with --credentials-file, the context must not contain actor or roles", "")
	case !principals && (!hasActor || !hasRoles):
		return nil, lifecycle.NewFault("configuration_error", "with --token-file, the context must contain actor and roles", "")
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
