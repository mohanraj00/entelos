package protocol

// The issuer file and the signed claim of the HTTP binding in
// approval/APPROVAL.md. A claim is a compact JWS with the EdDSA algorithm
// (Ed25519). The verifier accepts no other algorithm.

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// IssuerFormat is the format value of an issuer file.
const IssuerFormat = "entelos.issuer/1"

// MaxClaim is the byte bound of one compact claim.
const MaxClaim = 8192

// maxIssuerFile is the byte bound of an issuer file.
const maxIssuerFile = 1048576

// maxLifetime is the upper bound of max_lifetime_seconds.
const maxLifetime = 86400

// clockSkew is the time that iat and nbf can be in the future.
const clockSkew = 60

// maxName is the code point bound of an issuer, an audience, a key ID, a
// subject and a tenant.
const maxName = 256

var b64 = base64.RawURLEncoding.Strict()

var kidPattern = regexp.MustCompile(`\A[A-Za-z0-9._-]{1,64}\z`)

// errClaim is the text of each refusal of a claim. The sidecar log can show
// the reason; the response does not, so that a client cannot probe the
// configuration.
var errClaim = errors.New("signed claim required or invalid")

type issuer struct {
	issuer   string
	audience string
	lifetime int64
	keys     map[string]ed25519.PublicKey
	roles    map[string][]string
}

// readIssuer reads and checks the issuer file. A file that is not private
// or not valid gives an error.
func readIssuer(path string) (*issuer, error) {
	f, err := openPrivate(path, "issuer file")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxIssuerFile+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxIssuerFile {
		return nil, errors.New("issuer file exceeds the byte bound")
	}
	return parseIssuer(raw)
}

func validName(s string) bool {
	return s != "" && len([]rune(s)) <= maxName && !strings.ContainsAny(s, "\x00")
}

func parseIssuer(raw []byte) (*issuer, error) {
	value, err := lifecycle.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("issuer file: %w", err)
	}
	doc, ok := value.(lifecycle.Object)
	if !ok || len(doc) != 6 || doc["format"] != IssuerFormat {
		return nil, errors.New("issuer file must be {format: " + IssuerFormat + ", issuer, audience, max_lifetime_seconds, keys, principals}")
	}
	out := &issuer{keys: map[string]ed25519.PublicKey{}, roles: map[string][]string{}}
	var isIssuer, isAudience bool
	out.issuer, isIssuer = doc["issuer"].(string)
	out.audience, isAudience = doc["audience"].(string)
	if !isIssuer || !validName(out.issuer) || !isAudience || !validName(out.audience) {
		return nil, errors.New("issuer file: issuer and audience must be nonempty strings")
	}
	lifetime, ok := intValue(doc["max_lifetime_seconds"])
	if !ok || lifetime < 1 || lifetime > maxLifetime {
		return nil, fmt.Errorf("issuer file: max_lifetime_seconds must be an integer from 1 to %d", maxLifetime)
	}
	out.lifetime = lifetime
	keys, ok := doc["keys"].(lifecycle.Array)
	if !ok || len(keys) == 0 {
		return nil, errors.New("issuer file needs a nonempty keys list")
	}
	for i, x := range keys {
		k, ok := x.(lifecycle.Object)
		kid, isKid := k["kid"].(string)
		xs, isX := k["x"].(string)
		if !ok || len(k) != 4 || k["kty"] != "OKP" || k["crv"] != "Ed25519" || !isKid || !kidPattern.MatchString(kid) || !isX {
			return nil, fmt.Errorf("key %d must be {kty: OKP, crv: Ed25519, kid, x} only", i)
		}
		pub, err := b64.DecodeString(xs)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("key %d: x must be 32 bytes in base64url without padding", i)
		}
		if _, dup := out.keys[kid]; dup {
			return nil, fmt.Errorf("key %d: kid must be unique", i)
		}
		out.keys[kid] = ed25519.PublicKey(pub)
	}
	list, ok := doc["principals"].(lifecycle.Array)
	if !ok || len(list) == 0 {
		return nil, errors.New("issuer file needs a nonempty principals list")
	}
	known := map[string]bool{}
	for _, r := range lifecycle.RoleNames() {
		known[r] = true
	}
	for i, x := range list {
		entry, ok := x.(lifecycle.Object)
		subject, isSubject := entry["subject"].(string)
		roles, isRoles := entry["roles"].(lifecycle.Array)
		if !ok || len(entry) != 2 || !isSubject || !validName(subject) || !isRoles || len(roles) == 0 {
			return nil, fmt.Errorf("principal %d must have subject and roles only", i)
		}
		if _, dup := out.roles[subject]; dup {
			return nil, fmt.Errorf("principal %d: subject must be unique", i)
		}
		names := []string{}
		for _, r := range roles {
			name, isName := r.(string)
			if !isName || !known[name] {
				return nil, fmt.Errorf("principal %d: unknown role", i)
			}
			if slices.Contains(names, name) {
				return nil, fmt.Errorf("principal %d: role %q is in the list two times", i, name)
			}
			names = append(names, name)
		}
		out.roles[subject] = names
	}
	return out, nil
}

// intValue gives the value of a JSON integer in the int64 range.
func intValue(v any) (int64, bool) {
	n, ok := v.(lifecycle.Int)
	if !ok {
		return 0, false
	}
	x, err := strconv.ParseInt(n.Text, 10, 64)
	return x, err == nil
}

// verifyClaim checks one compact claim at the time now and gives its
// principal. tenant is the scope of the Host context. Each failure gives an
// error with the reason for the sidecar log.
func (is *issuer) verifyClaim(token string, now time.Time, tenant string) (*Principal, error) {
	if len(token) > MaxClaim {
		return nil, errors.New("claim exceeds the byte bound")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("claim is not a compact JWS")
	}
	header, err := decodeSegment(parts[0])
	if err != nil {
		return nil, fmt.Errorf("claim header: %w", err)
	}
	// Only alg, kid and typ are permitted, so crit, jku, jwk, x5u and b64
	// cannot change the verification.
	for k := range header {
		if k != "alg" && k != "kid" && k != "typ" {
			return nil, fmt.Errorf("claim header has a field that is not permitted: %q", k)
		}
	}
	if header["alg"] != "EdDSA" {
		return nil, errors.New("claim algorithm must be EdDSA")
	}
	if typ, has := header["typ"]; has && typ != "JWT" {
		return nil, errors.New("claim typ must be JWT")
	}
	kid, _ := header["kid"].(string)
	key, known := is.keys[kid]
	if !known {
		return nil, errors.New("claim kid is not in the key set")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, errors.New("claim signature is not 64 bytes in base64url")
	}
	if !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, errors.New("claim signature does not verify")
	}
	// The payload is read only after the signature verifies.
	body, err := decodeSegment(parts[1])
	if err != nil {
		return nil, fmt.Errorf("claim payload: %w", err)
	}
	if iss, _ := body["iss"].(string); iss != is.issuer {
		return nil, errors.New("claim issuer is not the configured issuer")
	}
	if !hasAudience(body["aud"], is.audience) {
		return nil, errors.New("claim audience does not contain the configured audience")
	}
	exp, okExp := intValue(body["exp"])
	iat, okIat := intValue(body["iat"])
	if !okExp || !okIat {
		return nil, errors.New("claim needs integer exp and iat")
	}
	t := now.Unix()
	switch {
	case t >= exp:
		return nil, errors.New("claim expired")
	case iat > t+clockSkew:
		return nil, errors.New("claim iat is in the future")
	case iat < 0:
		return nil, errors.New("claim iat is negative")
	// iat is from 0 to t+clockSkew here, so iat+is.lifetime cannot overflow
	// and exp-iat is not calculated.
	case exp <= iat || exp > iat+is.lifetime:
		return nil, errors.New("claim lifetime is longer than max_lifetime_seconds or not positive")
	}
	if raw, has := body["nbf"]; has {
		nbf, ok := intValue(raw)
		switch {
		case !ok:
			return nil, errors.New("claim nbf must be an integer")
		case nbf > t+clockSkew:
			return nil, errors.New("claim is not valid yet")
		}
	}
	claimTenant, _ := body["tenant"].(string)
	if claimTenant == "" || claimTenant != tenant {
		return nil, errors.New("claim tenant is not the scope of this sidecar")
	}
	subject, _ := body["sub"].(string)
	roles, known := is.roles[subject]
	if !validName(subject) || !known {
		return nil, errors.New("claim subject is not a configured principal")
	}
	return &Principal{Actor: subject, Roles: append([]string{}, roles...)}, nil
}

func hasAudience(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case lifecycle.Array:
		for _, x := range v {
			if s, ok := x.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// decodeSegment decodes one base64url segment as one strict JSON object.
func decodeSegment(s string) (lifecycle.Object, error) {
	raw, err := b64.DecodeString(s)
	if err != nil {
		return nil, errors.New("not base64url without padding")
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		return nil, errors.New("not strict JSON")
	}
	object, ok := value.(lifecycle.Object)
	if !ok {
		return nil, errors.New("not a JSON object")
	}
	return object, nil
}

// staticPrincipal checks a static token of local mode against the
// credentials file, as the socket does for a prelude token.
func staticPrincipal(token, path string) (*Principal, error) {
	if len(token) != tokenSize {
		return nil, errToken
	}
	for i := 0; i < len(token); i++ {
		c := token[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, errToken
		}
	}
	return principalFor([]byte(token), path)
}
