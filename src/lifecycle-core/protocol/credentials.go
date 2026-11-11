package protocol

// The credentials file of approval/APPROVAL.md. Each entry gives one
// principal: the SHA-256 digest of its token, its actor and its roles.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"

	lifecycle "example.org/lifecycle-core"
)

// CredentialsFormat is the format value of a credentials file.
const CredentialsFormat = "entelos.credentials/1"

// maxCredentials is the byte bound of a credentials file.
const maxCredentials = 1048576

var digestPattern = regexp.MustCompile(`\A[0-9a-f]{64}\z`)

type credential struct {
	digest [32]byte
	actor  string
	roles  []string
}

// readCredentials reads and checks the credentials file. A file that is not
// private, not valid, or that has a duplicate digest, a duplicate actor or
// an unknown role gives an error.
func readCredentials(path string) ([]credential, error) {
	f, err := openPrivate(path, "credentials file")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxCredentials+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxCredentials {
		return nil, errors.New("credentials file exceeds the byte bound")
	}
	return parseCredentials(raw)
}

func parseCredentials(raw []byte) ([]credential, error) {
	value, err := lifecycle.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("credentials file: %w", err)
	}
	doc, ok := value.(lifecycle.Object)
	if !ok || len(doc) != 2 || doc["format"] != CredentialsFormat {
		return nil, errors.New("credentials file must be {format: " + CredentialsFormat + ", principals: [...]}")
	}
	list, ok := doc["principals"].(lifecycle.Array)
	if !ok || len(list) == 0 {
		return nil, errors.New("credentials file needs a nonempty principals list")
	}
	known := map[string]bool{}
	for _, r := range lifecycle.RoleNames() {
		known[r] = true
	}
	digests, actors := map[string]bool{}, map[string]bool{}
	var out []credential
	for i, x := range list {
		entry, ok := x.(lifecycle.Object)
		digest, isDigest := entry["token_sha256"].(string)
		actor, isActor := entry["actor"].(string)
		roles, isRoles := entry["roles"].(lifecycle.Array)
		if !ok || len(entry) != 3 || !isDigest || !digestPattern.MatchString(digest) || !isActor || actor == "" || !isRoles || len(roles) == 0 {
			return nil, fmt.Errorf("principal %d must have token_sha256 (64 lowercase hex digits), actor and roles only", i)
		}
		if digests[digest] || actors[actor] {
			return nil, fmt.Errorf("principal %d: token_sha256 and actor must be unique", i)
		}
		digests[digest], actors[actor] = true, true
		c := credential{actor: actor}
		hex.Decode(c.digest[:], []byte(digest))
		for _, r := range roles {
			name, isName := r.(string)
			if !isName || !known[name] {
				return nil, fmt.Errorf("principal %d: unknown role", i)
			}
			c.roles = append(c.roles, name)
		}
		out = append(out, c)
	}
	return out, nil
}
