//go:build !unix

package lifecycle

import "os"

// holdUpdate takes no lock on this system. The update marker needs flock.
func (s *GitStore) holdUpdate() (*os.File, error) { return nil, nil }

// RecoverRootLock is available only on a Unix system, because the check
// for an update in progress needs flock.
func (s *GitStore) RecoverRootLock(remove bool) (Object, error) {
	return nil, NewFault("store_error", "the recovery of the root ref lock needs a Unix system", "store")
}
