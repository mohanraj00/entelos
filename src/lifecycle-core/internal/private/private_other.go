//go:build !unix

package private

import "errors"

// Read refuses each file. This platform has no check of the owner and the
// mode of a file, so a secret file is not supported.
func Read(path, label string, max int64) ([]byte, error) {
	return nil, errors.New(label + " is supported only on a Unix system")
}
