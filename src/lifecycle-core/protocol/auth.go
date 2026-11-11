package protocol

import (
	"bytes"
	"crypto/sha256"
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

// openPrivate opens a regular file with mode 0600 of the sidecar user.
func openPrivate(path, label string) (*os.File, error) {
	// Check the opened file so atomic replacement cannot race a separate stat.
	// Nonblocking open lets us reject a FIFO without waiting for a writer.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(st.Uid) != os.Geteuid() || info.Mode().Perm() != 0o600 {
		f.Close()
		return nil, errors.New(label + " must be a regular file with mode 0600 owned by the sidecar user")
	}
	return f, nil
}

func readToken(path string) ([]byte, error) {
	f, err := openPrivate(path, "token file")
	if err != nil {
		return nil, err
	}
	defer f.Close()
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

// readPrelude reads the marker and the token of one frame prelude.
func readPrelude(conn net.Conn) ([tokenSize]byte, error) {
	var marker [len(authMarker)]byte
	var presented [tokenSize]byte
	// An authenticated connection can be idle between requests. Start
	// timing the next prelude as soon as its first byte arrives.
	if _, err := io.ReadFull(conn, marker[:1]); err != nil {
		return presented, err
	}
	if err := conn.SetReadDeadline(time.Now().Add(authWait)); err != nil {
		return presented, err
	}
	defer conn.SetReadDeadline(time.Time{})
	if _, err := io.ReadFull(conn, marker[1:]); err != nil {
		return presented, errToken
	}
	if string(marker[:]) != authMarker {
		return presented, errToken
	}
	if _, err := io.ReadFull(conn, presented[:]); err != nil {
		return presented, errToken
	}
	return presented, nil
}

func authenticate(conn net.Conn, path string) error {
	presented, err := readPrelude(conn)
	if err != nil {
		return err
	}
	expected, err := readToken(path)
	if err != nil || subtle.ConstantTimeCompare(expected, presented[:]) != 1 {
		return errToken
	}
	return nil
}

// authenticatePrincipal reads the credentials file again for this frame and
// gives the principal of the prelude token. It compares the token digest
// with each entry in constant time, and it compares all entries.
func authenticatePrincipal(conn net.Conn, path string) (*Principal, error) {
	presented, err := readPrelude(conn)
	if err != nil {
		return nil, err
	}
	entries, err := readCredentials(path)
	if err != nil {
		return nil, errToken
	}
	digest := sha256.Sum256(presented[:])
	found := 0
	index := 0
	for i, e := range entries {
		same := subtle.ConstantTimeCompare(digest[:], e.digest[:])
		index = subtle.ConstantTimeSelect(same, i, index)
		found |= same
	}
	if found != 1 {
		return nil, errToken
	}
	e := entries[index]
	return &Principal{Actor: e.actor, Roles: append([]string{}, e.roles...)}, nil
}
