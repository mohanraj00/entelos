//go:build unix

package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// killPublisher runs during, stops the git process of the ref transaction
// with SIGKILL, and then accepts.
type killPublisher struct {
	child   func() *os.Process
	during  func()
	records [][]byte
}

func (p *killPublisher) Publish(ctx context.Context, record, signed []byte) error {
	p.records = append(p.records, record)
	p.during()
	if err := p.child().Kill(); err != nil {
		return err
	}
	return nil
}

// The git process stops with SIGKILL during a publish. The lock file of
// the root ref stays, and each later write fails. The recovery refuses
// while the update is in progress. After the update, it finds the stale
// lock file and removes it. The root ref does not change, and the next
// write succeeds.
func TestRecoverAStaleRootLock(t *testing.T) {
	store, err := OpenGitStore(t.TempDir() + "/store")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := raceWorld(t, store)
	_, decision := w.decided("pass")
	if state, err := store.RecoverRootLock(true); err != nil || state["lock"] != "absent" {
		t.Fatalf("a store with no lock file: %v %v", state, err)
	}
	var child *os.Process
	store.prepared = func(p *os.Process) { child = p }
	var inProgress error
	publisher := &killPublisher{child: func() *os.Process { return child }}
	publisher.during = func() { _, inProgress = store.RecoverRootLock(true) }
	w.publisher = publisher
	before, err := store.Root()
	if err != nil {
		t.Fatal(err)
	}
	q := Object{"op": "activate", "expected_revision": before, "request_key": "activate-kill", "decision_ref": decision, "targets": Array{}}
	host := w.host("reviewer", "reviewer", w.runner)
	_, err = host.Handle(copyObject(q))
	if faultCode(t, err) != "store_error" {
		t.Fatalf("the write with a stopped git process: %v", err)
	}
	if faultCode(t, inProgress) != "update_in_progress" {
		t.Fatalf("the recovery during the update: %v", inProgress)
	}
	lock := store.rootLockPath()
	exists := func() bool {
		_, err := os.Stat(lock)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return err == nil
	}
	root := func() string {
		root, err := store.Root()
		if err != nil {
			t.Fatal(err)
		}
		return root
	}
	if f := err.(*Fault); !strings.Contains(f.Message, "run entelos-unlock") {
		t.Fatalf("the message does not name the recovery: %s", f.Message)
	}
	if !exists() || root() != before {
		t.Fatalf("after SIGKILL: lock file %v, root %s, want a lock file and root %s", exists(), root(), before)
	}
	// Each later write fails while the lock file stays.
	store.prepared, w.publisher = nil, nil
	_, err = w.do("reviewer", "reviewer", w.runner, "source", with(window, Object{"text": "other rule", "reason": "other", "supersedes": Array{}}))
	if faultCode(t, err) != "store_error" || root() != before || !strings.Contains(err.(*Fault).Message, "run entelos-unlock") {
		t.Fatalf("a write with a stale lock file: %v", err)
	}
	// The check removes nothing.
	state, err := store.RecoverRootLock(false)
	if err != nil || !equal(state, Object{"lock": "stale", "root": before}) || !exists() {
		t.Fatalf("check: %v %v", state, err)
	}
	state, err = store.RecoverRootLock(true)
	if err != nil || !equal(state, Object{"lock": "removed", "root": before}) || exists() || root() != before {
		t.Fatalf("recovery: %v %v", state, err)
	}
	if state, err := store.RecoverRootLock(true); err != nil || state["lock"] != "absent" {
		t.Fatalf("a second recovery: %v %v", state, err)
	}
	// The next write succeeds. It is the retry of the stopped request, and
	// the consumer gets the same bytes again.
	fast := &testPublisher{}
	w.publisher = fast
	out, err := w.host("reviewer", "reviewer", w.runner).Handle(copyObject(q))
	if err != nil {
		t.Fatal(err)
	}
	event := out.(Object)["result"].(Object)["event_ref"]
	if root() == before || w.activations()[text(event)] == nil {
		t.Fatal("the activation is not the new root")
	}
	if len(fast.records) != 1 || len(publisher.records) != 1 || !bytes.Equal(fast.records[0], publisher.records[0]) {
		t.Fatal("the retry gave other bytes")
	}
}

// The update marker is a file of the current user with the mode 0600, so
// no other account can open it and hold its lock. The store repairs a
// wider mode, and it refuses a marker that is not a regular file.
func TestUpdateMarkerIsPrivate(t *testing.T) {
	store, err := OpenGitStore(t.TempDir() + "/store")
	if err != nil {
		t.Fatal(err)
	}
	w, _ := raceWorld(t, store)
	w.publisher = nil
	write := func(text string) error {
		_, err := w.do("reviewer", "reviewer", w.runner, "source", with(window, Object{"text": text, "reason": text, "supersedes": Array{}}))
		return err
	}
	marker := store.Path + "/" + updateMarker
	mode := func() os.FileMode {
		info, err := os.Lstat(marker)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode()
	}
	if err := write("first rule"); err != nil || mode() != 0o600 {
		t.Fatalf("first write: %v, marker mode %v", err, mode())
	}
	if err := os.Chmod(marker, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := write("second rule"); err != nil || mode() != 0o600 {
		t.Fatalf("a write with a wide marker: %v, marker mode %v", err, mode())
	}
	if err := os.Chmod(marker, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverRootLock(false); err != nil || mode() != 0o600 {
		t.Fatalf("a recovery with a wide marker: %v, marker mode %v", err, mode())
	}
	// A link is not a marker. The store follows no link.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir() + "/other"
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, marker); err != nil {
		t.Fatal(err)
	}
	before := must(store.Read(nil)).Cursor
	if err := write("third rule"); faultCode(t, err) != "store_error" || must(store.Read(nil)).Cursor != before {
		t.Fatalf("a write with a link as the marker: %v", err)
	}
	if _, err := store.RecoverRootLock(true); faultCode(t, err) != "store_error" {
		t.Fatalf("a recovery with a link as the marker: %v", err)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("the store changed the target of the link: %v", err)
	}
	// A directory is not a marker.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := write("fourth rule"); err == nil || must(store.Read(nil)).Cursor != before {
		t.Fatalf("a write with a directory as the marker: %v", err)
	}
	// A FIFO is not a marker, and the open does not wait for a writer.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := write("fifth rule"); faultCode(t, err) != "invalid_store" || must(store.Read(nil)).Cursor != before {
		t.Fatalf("a write with a FIFO as the marker: %v", err)
	}
}

// A different account cannot open the marker, and the store refuses a
// marker of a different account. The test needs two accounts, and only
// root can act as a second account. README.md gives the same check by hand.
func TestUpdateMarkerRefusesAnotherAccount(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("only root can act as another account")
	}
	const other = 65534
	// The other account can go into the store directory, as with the
	// mode 0755. Thus only the mode of the marker stops the open.
	dir := t.TempDir()
	for _, d := range []string{filepath.Dir(dir), dir} {
		if err := os.Chmod(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenGitStore(dir + "/store")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := store.Path + "/" + updateMarker
	openAsOther := func() error {
		cmd := exec.Command("sh", "-c", `exec 3<"$1"`, "sh", marker)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: other, Gid: other}}
		return cmd.Run()
	}
	// A marker of an earlier version has the mode 0644. The other account
	// can open it, so the path is not the cause of the later refusal.
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(marker, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := openAsOther(); err != nil {
		t.Fatalf("the other account cannot open a marker with the mode 0644: %v", err)
	}
	if _, err := store.RecoverRootLock(false); err != nil {
		t.Fatal(err)
	}
	if err := openAsOther(); err == nil {
		t.Fatal("the other account opened the marker after a use by the store")
	}
	// A marker of the other account is not usable, also with a wide mode.
	if err := os.Chown(marker, other, other); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(marker, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverRootLock(false); faultCode(t, err) != "invalid_store" {
		t.Fatalf("a recovery with a marker of another account: %v", err)
	}
	if info, err := os.Stat(marker); err != nil || info.Mode().Perm() != 0o666 {
		t.Fatalf("the store changed a marker of another account: %v", err)
	}
}

// A lock path that is not a regular file is not a stale lock file.
func TestRecoveryRefusesAnotherFileKind(t *testing.T) {
	store, err := OpenGitStore(t.TempDir() + "/store")
	if err != nil {
		t.Fatal(err)
	}
	lock := store.rootLockPath()
	if err := os.MkdirAll(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverRootLock(true); faultCode(t, err) != "invalid_store" {
		t.Fatalf("got %v", err)
	}
	if info, err := os.Stat(lock); err != nil || !info.IsDir() {
		t.Fatalf("the recovery changed the path: %v", err)
	}
}
