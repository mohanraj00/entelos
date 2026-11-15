package lifecycle

// Signed records. src/protocol/signed/SIGNED.md is the specification. A
// signature covers the exact bytes of one approval or activation record, and
// a consumer verifies it offline with a public key set.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"regexp"
	"sort"
	"strconv"
)

const (
	// SignatureDomain starts each signed message. The number after the
	// slash is the major version of the message layout.
	SignatureDomain = "entelos.signature/1"
	// KeySetFormat is the format of a public key set document.
	KeySetFormat = "entelos.keyset/1"
	// SigningKeyFormat is the format of a private signing key document.
	SigningKeyFormat = "entelos.signing-key/1"

	signatureAlgorithm = "ed25519"
	signedKind         = "signed"
	// maxSignedRecord is the largest record that gets a signed record. The
	// base64 text of 512 KiB has 699,052 characters. Thus the signed record
	// fits in one protocol frame of 1 MiB, and the record operation can
	// give it on each transport.
	maxSignedRecord = 512 << 10
	maxKeySetKeys   = 64
	maxSignedText   = 256
)

// signedKinds are the record kinds that the Host signs: an approval record
// is a decision, and an activation record is an event.
var signedKinds = map[string]bool{"decision": true, "event": true}

var (
	keySetFormatPattern = regexp.MustCompile(`^entelos\.keyset/([1-9][0-9]{0,8})$`)
	algorithmPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	keyIDPattern        = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}:[0-9a-f]{64}$`)
)

func signFault(code, rule, message string) *Fault {
	if rule != "" {
		message = rule + ": " + message
	}
	return NewFault(code, message, "signature")
}

// keyID is the identity of an Ed25519 public key. It is not a signature.
func keyID(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return signatureAlgorithm + ":" + hex.EncodeToString(sum[:])
}

// signedMessage is the byte string that a signature covers: the domain, a
// zero byte, the media type of the record kind, a zero byte and the exact
// record bytes.
func signedMessage(kind string, record []byte) []byte {
	envelope := Envelope(Media(kind), record)
	out := make([]byte, 0, len(SignatureDomain)+1+len(envelope))
	out = append(out, SignatureDomain...)
	out = append(out, 0)
	return append(out, envelope...)
}

// decodeBase64 reads canonical base64 with padding. It refuses white space
// and each text that the encoder does not give.
func decodeBase64(value string) ([]byte, bool) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != value {
		return nil, false
	}
	return raw, true
}

// exactFields reports if doc has each required field as a string, and no
// field outside required and optional.
func exactFields(doc Object, required []string, optional ...string) bool {
	count := 0
	for _, name := range required {
		if _, ok := doc[name].(string); !ok {
			return false
		}
		count++
	}
	for _, name := range optional {
		if value, has := doc[name]; has {
			if _, ok := value.(string); !ok {
				return false
			}
			count++
		}
	}
	return count == len(doc)
}

func boundedText(value string) bool {
	return value != "" && len([]rune(value)) <= maxSignedText
}

// Signer holds one private signing key. The embedding host installs it. A
// request never selects it.
type Signer struct {
	id  string
	key ed25519.PrivateKey
}

// NewSigningKey makes a private signing key document from 32 random bytes
// of random, for example crypto/rand.Reader.
func NewSigningKey(random io.Reader) ([]byte, error) {
	public, private, err := ed25519.GenerateKey(random)
	if err != nil {
		return nil, err
	}
	return Encode(Object{"format": SigningKeyFormat, "algorithm": signatureAlgorithm, "key_id": keyID(public),
		"seed": base64.StdEncoding.EncodeToString(private.Seed())}), nil
}

// NewSigner reads a private signing key document.
func NewSigner(raw []byte) (*Signer, error) {
	bad := func(message string) (*Signer, error) {
		return nil, NewFault("configuration_error", "signing key: "+message, "")
	}
	value, err := Decode(raw)
	doc, ok := value.(Object)
	if err != nil || !ok {
		return bad("the document is not a JSON object")
	}
	if !exactFields(doc, []string{"format", "algorithm", "key_id", "seed"}) {
		return bad("the fields are format, algorithm, key_id and seed, each a string")
	}
	if doc["format"] != SigningKeyFormat || doc["algorithm"] != signatureAlgorithm {
		return bad("unsupported format or algorithm")
	}
	seed, ok := decodeBase64(doc["seed"].(string))
	if !ok || len(seed) != ed25519.SeedSize {
		return bad("seed must be 32 bytes in base64")
	}
	key := ed25519.NewKeyFromSeed(seed)
	id := keyID(key.Public().(ed25519.PublicKey))
	if doc["key_id"] != id {
		return bad("key_id is not the identity of this key")
	}
	return &Signer{id: id, key: key}, nil
}

// KeyID gives the identifier that each signature of this signer names.
func (s *Signer) KeyID() string { return s.id }

// PublicKey gives the key set entry of this signer, with the status active.
func (s *Signer) PublicKey() Object {
	return Object{"key_id": s.id, "algorithm": signatureAlgorithm, "status": "active",
		"public_key": base64.StdEncoding.EncodeToString(s.key.Public().(ed25519.PublicKey))}
}

// sign gives the signed record of the exact bytes of one record. Put sets
// its format.
func (s *Signer) sign(scope, function, kind string, record []byte) Object {
	return Object{"scope": scope, "function": function, "record_kind": kind,
		"record_ref": ContentRef(Envelope(Media(kind), record)), "record": base64.StdEncoding.EncodeToString(record),
		"algorithm": signatureAlgorithm, "key_id": s.id,
		"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(s.key, signedMessage(kind, record)))}
}

// SetSigner installs the signing key of the Host. The Host then signs each
// decision and event record that it writes. Nil removes the key. A Host
// that As gives has the signer of its source at that time.
func (h *Host) SetSigner(s *Signer) { h.signer = s }

// signStaged signs each decision and event record that this transaction
// staged. The state maps each record reference to its signed record. A
// record that has a signed record keeps it.
func (h *Host) signStaged(v *View) {
	if h.signer == nil {
		return
	}
	kinds := map[string]string{}
	var refs []string
	for ref, body := range v.pending {
		media, _, _ := bytes.Cut(body, []byte{0})
		for kind := range signedKinds {
			if string(media) == Media(kind) {
				kinds[ref] = kind
				refs = append(refs, ref)
			}
		}
	}
	if len(refs) == 0 {
		return
	}
	sort.Strings(refs)
	index, ok := v.State["signatures"].(Object)
	if !ok {
		if _, has := v.State["signatures"]; has {
			fail("invalid_store", "signatures is not an object", "store")
		}
		index = Object{}
		v.State["signatures"] = index
	}
	for _, ref := range refs {
		if _, has := index[ref]; has {
			continue
		}
		_, raw, _ := bytes.Cut(v.pending[ref], []byte{0})
		// Refuse the write. A record without its signed record, or with a
		// signed record that no transport can give, is not acceptable.
		if len(raw) > maxSignedRecord {
			fail("record_too_large", "the record has "+strconv.Itoa(len(raw))+" bytes; a signing Host accepts a decision or event record of "+
				strconv.Itoa(maxSignedRecord)+" bytes maximum", "signature")
		}
		record := must(Decode(raw))
		index[ref] = v.Put(signedKind, h.signer.sign(text(item(record, "scope")), text(item(record, "function")), kinds[ref], raw))
	}
}

type publicKey struct {
	key    ed25519.PublicKey
	status string
}

// KeySet is the public key set of one signer.
type KeySet struct {
	signer string
	keys   map[string]publicKey
}

// ParseKeySet reads and checks a public key set document.
func ParseKeySet(raw []byte) (*KeySet, error) {
	bad := func(rule, message string) (*KeySet, error) { return nil, signFault("invalid_keyset", rule, message) }
	value, err := Decode(raw)
	doc, ok := value.(Object)
	if err != nil || !ok {
		return bad("document", "the key set is not a JSON object")
	}
	format, has := doc["format"]
	if !has {
		return nil, signFault("unsupported_keyset_format", "", "the key set has no format")
	}
	name, _ := format.(string)
	m := keySetFormatPattern.FindStringSubmatch(name)
	if m == nil {
		return bad("format", "malformed key set format")
	}
	if m[1] != "1" {
		return nil, signFault("unsupported_keyset_format", "", "unknown key set format "+name)
	}
	signer, _ := doc["signer"].(string)
	list, isList := doc["keys"].(Array)
	if len(doc) != 3 || !boundedText(signer) || !isList || len(list) == 0 || len(list) > maxKeySetKeys {
		return bad("schema", "the fields are format, signer and 1 to 64 keys")
	}
	// The schema check of each key comes before the other key checks.
	entries := make([]Object, 0, len(list))
	for _, x := range list {
		entry, ok := x.(Object)
		if !ok || !exactFields(entry, []string{"key_id", "algorithm", "public_key", "status"}, "note") {
			return bad("schema", "a key has key_id, algorithm, public_key, status and an optional note, each a string")
		}
		status := entry["status"].(string)
		if !keyIDPattern.MatchString(entry["key_id"].(string)) || !algorithmPattern.MatchString(entry["algorithm"].(string)) ||
			(status != "active" && status != "retired" && status != "revoked") {
			return bad("schema", "malformed key_id, algorithm or status")
		}
		if note, has := entry["note"].(string); has && !boundedText(note) {
			return bad("schema", "a note has 1 to 256 characters")
		}
		entries = append(entries, entry)
	}
	set := &KeySet{signer: signer, keys: map[string]publicKey{}}
	for _, entry := range entries {
		id, algorithm := entry["key_id"].(string), entry["algorithm"].(string)
		if algorithm != signatureAlgorithm {
			return nil, signFault("unsupported_algorithm", "", "the key set has a key of algorithm "+algorithm)
		}
		public, ok := decodeBase64(entry["public_key"].(string))
		if !ok || len(public) != ed25519.PublicKeySize {
			return bad("key_encoding", "public_key must be 32 bytes in base64")
		}
		if keyID(public) != id {
			return bad("key_id", "key_id is not the identity of public_key")
		}
		if _, dup := set.keys[id]; dup {
			return bad("duplicate_key", "two keys have the same key_id")
		}
		set.keys[id] = publicKey{key: public, status: entry["status"].(string)}
	}
	return set, nil
}

// Signer gives the signer name of the key set.
func (k *KeySet) Signer() string { return k.signer }

// VerifySigned checks one signed record document against a key set. It
// calls no service. On success it gives the record and the facts that the
// signature covers. Each refusal is a Fault of the stage signature.
func VerifySigned(raw []byte, keys *KeySet) (Object, error) {
	bad := func(rule, message string) (Object, error) {
		return nil, signFault("invalid_signed_record", rule, message)
	}
	if keys == nil {
		return nil, signFault("invalid_keyset", "document", "no key set")
	}
	value, err := Decode(raw)
	doc, ok := value.(Object)
	if err != nil || !ok {
		return bad("document", "the signed record is not a JSON object")
	}
	if fault := checkSignedFormat(doc, "format"); fault != nil {
		return nil, fault
	}
	if !exactFields(doc, []string{"format", "scope", "function", "record_kind", "record_ref", "record", "algorithm", "key_id", "signature"}) {
		return bad("schema", "a field is missing, is not a string or is not permitted")
	}
	scope, function, kind := doc["scope"].(string), doc["function"].(string), doc["record_kind"].(string)
	ref, algorithm, id := doc["record_ref"].(string), doc["algorithm"].(string), doc["key_id"].(string)
	encoded := doc["record"].(string)
	if !boundedText(scope) || !boundedText(function) || !signedKinds[kind] || !refPattern.MatchString(ref) ||
		!algorithmPattern.MatchString(algorithm) || !keyIDPattern.MatchString(id) ||
		encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(maxSignedRecord) {
		return bad("schema", "a field value is malformed or too long")
	}
	if algorithm != signatureAlgorithm {
		return nil, signFault("unsupported_algorithm", "", "unknown signature algorithm "+algorithm)
	}
	record, ok := decodeBase64(encoded)
	if !ok {
		return bad("record_encoding", "record is not canonical base64")
	}
	signature, ok := decodeBase64(doc["signature"].(string))
	if !ok || len(signature) != ed25519.SignatureSize {
		return bad("signature_encoding", "signature must be 64 bytes in base64")
	}
	if ContentRef(Envelope(Media(kind), record)) != ref {
		return nil, signFault("record_mismatch", "ref", "record_ref is not the identity of the record bytes")
	}
	key, known := keys.keys[id]
	if !known {
		return nil, signFault("unknown_key", "", "the key set has no key "+id)
	}
	if key.status == "revoked" {
		return nil, signFault("revoked_key", "", "the key "+id+" is revoked")
	}
	if !ed25519.Verify(key.key, signedMessage(kind, record), signature) {
		return nil, signFault("signature_invalid", "", "the signature does not match the record bytes and the key")
	}
	// From here, the record bytes are the bytes that the signer signed.
	inner, err := Decode(record)
	body, ok := inner.(Object)
	if err != nil || !ok {
		return bad("record_json", "the record bytes are not a strict JSON object")
	}
	if fault := checkSignedFormat(body, "record_format"); fault != nil {
		return nil, fault
	}
	if body["scope"] != scope || body["function"] != function {
		return nil, signFault("record_mismatch", "binding", "scope or function differs from the record")
	}
	return Object{"record_kind": kind, "record_ref": ref, "scope": scope, "function": function, "signer": keys.signer,
		"key_id": id, "key_status": key.status, "record": body}, nil
}

// checkSignedFormat applies the record format rule: a missing format and an
// unknown major version fail closed.
func checkSignedFormat(doc Object, rule string) *Fault {
	format, has := doc["format"]
	if !has {
		return signFault("unsupported_record_format", "", "the record has no format")
	}
	name, _ := format.(string)
	m := formatPattern.FindStringSubmatch(name)
	if m == nil {
		return signFault("invalid_signed_record", rule, "malformed record format")
	}
	if m[1] != "1" {
		return signFault("unsupported_record_format", "", "unknown record format "+name)
	}
	return nil
}
