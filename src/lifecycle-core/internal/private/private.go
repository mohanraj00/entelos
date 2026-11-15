// Package private reads a secret file of the current user.
package private

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// Read gives the bytes of a regular file with mode 0600 of the current
// user. It refuses a symbolic link and a file of more than max bytes.
func Read(path, label string, max int64) ([]byte, error) {
	// Check the opened file so a replacement cannot race a separate stat.
	// A nonblocking open refuses a FIFO without a wait for a writer.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(st.Uid) != os.Geteuid() || info.Mode().Perm() != 0o600 {
		return nil, errors.New(label + " must be a regular file with mode 0600 owned by the current user")
	}
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > max {
		return nil, errors.New(label + " is too large")
	}
	return raw, nil
}
