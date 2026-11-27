//go:build unix

package lifecycle

// The update marker of the Git adapter and the recovery of a stale lock
// file of the root ref. See "Stale lock of the root ref" in
// src/protocol/activation/ACTIVATION.md.

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (s *GitStore) openMarker() (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(s.Path, updateMarker), os.O_RDONLY|os.O_CREATE|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return nil, NewFault("store_error", "the update marker of the store does not open: "+err.Error(), "store")
	}
	return f, nil
}

// holdUpdate takes a shared lock on the update marker for one update of the
// root ref. Two updates do not stop each other: the lock of the root ref
// selects the update that wins. The caller gives the file to its git
// process and closes it after that process stops. The operating system
// releases the lock when the last process that has the file closes it, also
// after SIGKILL. A recovery holds the exclusive lock only for a short time,
// so this function tries for 1 second.
//
// On a file system that does not support flock, the result is nil, and the
// update continues with no marker lock. The recovery refuses on such a
// file system, so it never removes a lock file that it cannot examine.
func (s *GitStore) holdUpdate() (*os.File, error) {
	f, err := s.openMarker()
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if flockUnsupported(err) {
			f.Close()
			return nil, nil
		}
		if (err != syscall.EWOULDBLOCK && err != syscall.EINTR) || attempt == 100 {
			f.Close()
			return nil, NewFault("store_error", "no shared lock on the update marker of the store; a recovery can be in progress: "+err.Error(), "store")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// flockUnsupported is true for the errors of a file system that has no
// flock.
func flockUnsupported(err error) bool {
	return err == syscall.ENOTSUP || err == syscall.EOPNOTSUPP || err == syscall.ENOLCK || err == syscall.ENOSYS
}

// RecoverRootLock examines the lock file of the root ref, and removes a
// stale lock file if remove is true. A git process that stops with SIGKILL
// during an update leaves this file, and each later write then fails with
// store_error.
//
// The function takes an exclusive lock on the update marker. If an update
// of the root ref is in progress, in this process or in another process on
// this system, the result is update_in_progress and the lock file stays.
// Else no update of this package owns the lock file, so the file is stale.
// The removal does not change the root ref: an update moves the root only
// when it renames its lock file.
//
// The result has the field lock with the value absent, stale or removed,
// and the field root with the root at that time.
func (s *GitStore) RecoverRootLock(remove bool) (Object, error) {
	f, err := s.openMarker()
	if err != nil {
		return nil, err
	}
	// The close releases the exclusive lock.
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK {
			return nil, NewFault("update_in_progress", "an update of the root ref is in progress; the lock file stays", "store")
		}
		return nil, NewFault("store_error", "no exclusive lock on the update marker of the store: "+err.Error(), "store")
	}
	root, err := s.Root()
	if err != nil {
		return nil, err
	}
	path := s.rootLockPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Object{"lock": "absent", "root": root}, nil
	}
	if err != nil {
		return nil, NewFault("store_error", err.Error(), "store")
	}
	if !info.Mode().IsRegular() {
		return nil, NewFault("invalid_store", "the lock path of the root ref is not a regular file", "store")
	}
	if !remove {
		return Object{"lock": "stale", "root": root}, nil
	}
	if err := os.Remove(path); err != nil {
		return nil, NewFault("store_error", "the stale lock file stays: "+err.Error(), "store")
	}
	after, err := s.Root()
	if err != nil {
		return nil, err
	}
	if after != root {
		return nil, NewFault("store_error", "the root ref changed during the recovery", "store")
	}
	return Object{"lock": "removed", "root": root}, nil
}
