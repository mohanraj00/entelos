package protocol

import (
	"bytes"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"time"
)

const authMarker = "ENT1"
const tokenSize = 64
const authWait = 30 * time.Second

var errToken = errors.New("sidecar token required or invalid")
var errTokenFormat = errors.New("token file must contain 64 lowercase hex digits and an optional newline")

func readToken(path string) ([]byte, error) {
	// Check the opened file so atomic replacement cannot race a separate stat.
	// Nonblocking open lets us reject a FIFO without waiting for a writer.
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
		return nil, errors.New("token file must be a regular file with mode 0600 owned by the sidecar user")
	}
	raw, err := io.ReadAll(io.LimitReader(f, tokenSize+2))
	if err != nil {
		return nil, err
	}
	token := bytes.TrimSuffix(raw, []byte("\n"))
	if len(token) != tokenSize {
		return nil, errTokenFormat
	}
	for _, c := range token {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, errTokenFormat
		}
	}
	return token, nil
}

func authenticate(conn net.Conn, path string) error {
	var marker [len(authMarker)]byte
	// An authenticated connection can be idle between requests. Start
	// timing the next prelude as soon as its first byte arrives.
	if _, err := io.ReadFull(conn, marker[:1]); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Now().Add(authWait)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})
	if _, err := io.ReadFull(conn, marker[1:]); err != nil {
		return errToken
	}
	if string(marker[:]) != authMarker {
		return errToken
	}
	var presented [tokenSize]byte
	if _, err := io.ReadFull(conn, presented[:]); err != nil {
		return errToken
	}
	expected, err := readToken(path)
	if err != nil || subtle.ConstantTimeCompare(expected, presented[:]) != 1 {
		return errToken
	}
	return nil
}
