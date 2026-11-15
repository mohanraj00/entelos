package lifecycle

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func signedVectors(t *testing.T) Object {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "signed", "vectors", "signed-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return doc.(Object)
}

func vectorCase(t *testing.T, doc Object, name string) Object {
	t.Helper()
	for _, c := range doc["cases"].(Array) {
		if c.(Object)["name"] == name {
			return c.(Object)
		}
	}
	t.Fatalf("no vector case %s", name)
	return nil
}

// verifyVector gives the result of one case in the shape of its expect field.
func verifyVector(sets Object, c Object) Object {
	refusal := func(err error) Object {
		fault, ok := err.(*Fault)
		if !ok || fault.Stage != "signature" {
			return Object{"error": err.Error()}
		}
		out := Object{"valid": false, "code": fault.Code}
		// A code with rules has the rule name at the start of its message.
		switch fault.Code {
		case "invalid_keyset", "invalid_signed_record", "record_mismatch":
			out["rule"], _, _ = strings.Cut(fault.Message, ": ")
		}
		return out
	}
	keys, err := ParseKeySet(Encode(sets[c["key_set"].(string)]))
	if err != nil {
		return refusal(err)
	}
	got, err := VerifySigned(Encode(c["signed"]), keys)
	if err != nil {
		return refusal(err)
	}
	out := Object{"valid": true}
	for _, name := range []string{"record_kind", "record_ref", "signer", "key_id", "key_status"} {
		out[name] = got[name]
	}
	return out
}

// The Go verifier gives the stated result of each vector. A person wrote
// the expected values from SIGNED.md.
func TestSignedVectors(t *testing.T) {
	doc := signedVectors(t)
	sets := doc["key_sets"].(Object)
	cases := doc["cases"].(Array)
	if len(cases) < 40 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, x := range cases {
		c := x.(Object)
		if got := verifyVector(sets, c); !equal(got, c["expect"]) {
			t.Errorf("%s: got %s, want %s", c["name"], Encode(got), Encode(c["expect"]))
		}
	}
}

// The Go signer gives the same signed record as the Python reference for
// the same seed and record. The result holds the record that was signed.
func TestSignerReproducesTheValidVectors(t *testing.T) {
	doc := signedVectors(t)
	key := doc["keys"].(Object)["a"].(Object)
	signer, err := NewSigner(Encode(Object{"format": SigningKeyFormat, "algorithm": "ed25519", "key_id": key["key_id"], "seed": key["seed"]}))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ParseKeySet(Encode(doc["key_sets"].(Object)["current"]))
	if err != nil {
		t.Fatal(err)
	}
	for name, kind := range map[string]string{"valid-approval": "decision", "valid-activation": "event"} {
		record := doc["records"].(Object)[kind].(Object)
		want := vectorCase(t, doc, name)["signed"].(Object)
		got := signer.sign(record["scope"].(string), record["function"].(string), kind, Encode(record))
		got["format"] = RecordFormat
		if !equal(got, want) {
			t.Errorf("%s: signer gave %s", name, Encode(got))
		}
		result, err := VerifySigned(Encode(want), keys)
		if err != nil {
			t.Fatal(err)
		}
		if !equal(result["record"], record) {
			t.Errorf("%s: result record %s", name, Encode(result["record"]))
		}
	}
}

func newTestSigner(t *testing.T) (*Signer, *KeySet) {
	t.Helper()
	raw, err := NewSigningKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(raw)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ParseKeySet(Encode(Object{"format": KeySetFormat, "signer": "test-host", "keys": Array{signer.PublicKey()}}))
	if err != nil {
		t.Fatal(err)
	}
	return signer, keys
}

// approve runs one candidate to an approval and an activation.
func (w *world) approve(rule string) (decision, event string) {
	snapshot, plan, _ := w.candidate(rule)
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	decision = w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "fixture verdict"})["decision_ref"].(string)
	event = w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})["event_ref"].(string)
	return decision, event
}

func (w *world) read(q Object) (Object, error) {
	out, err := w.host("reviewer", "reviewer", w.runner).Handle(q)
	if err != nil {
		return nil, err
	}
	return out.(Object), nil
}

func (w *world) signatures() Object {
	out, err := w.read(Object{"op": "inspect"})
	if err != nil {
		w.t.Fatal(err)
	}
	index, _ := out["state"].(Object)["signatures"].(Object)
	return index
}

// A Host with a signing key signs each approval record and each activation
// record. A consumer verifies the signed record offline, and a changed
// record fails.
func TestHostSignsApprovalAndActivationRecords(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	signer, keys := newTestSigner(t)
	w.signer = signer
	decision, event := w.approve("pass")
	suspension := w.must("reviewer", "reviewer", "suspend", with(window, Object{"targets": Array{event}, "reason": "stop"}))["event_ref"].(string)
	index := w.signatures()
	if len(index) != 3 {
		t.Fatalf("signatures %s", Encode(index))
	}
	for ref, kind := range map[string]string{decision: "decision", event: "event", suspension: "event"} {
		signed, err := w.read(Object{"op": "record", "kind": "signed", "ref": index[ref]})
		if err != nil {
			t.Fatal(err)
		}
		result, err := VerifySigned(Encode(signed), keys)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		stored, err := w.read(Object{"op": "record", "kind": kind, "ref": ref})
		if err != nil {
			t.Fatal(err)
		}
		if result["record_ref"] != ref || result["record_kind"] != kind || result["key_id"] != signer.KeyID() ||
			result["key_status"] != "active" || result["signer"] != "test-host" || !equal(result["record"], stored) {
			t.Fatalf("%s: result %s", kind, Encode(result))
		}
		// One changed byte of the record, with the reference of the new bytes.
		raw, _ := base64.StdEncoding.DecodeString(signed["record"].(string))
		changed := bytes.Replace(raw, []byte(`"demo"`), []byte(`"dem0"`), 1)
		forged := copyObject(signed)
		forged["record"] = base64.StdEncoding.EncodeToString(changed)
		if _, err := VerifySigned(Encode(forged), keys); faultCode(t, err) != "record_mismatch" {
			t.Fatalf("changed record: %v", err)
		}
		forged["record_ref"] = ContentRef(Envelope(Media(kind), changed))
		if _, err := VerifySigned(Encode(forged), keys); faultCode(t, err) != "signature_invalid" {
			t.Fatalf("changed record and reference: %v", err)
		}
	}
	// Another key set does not have the key. A revoked key is refused.
	_, other := newTestSigner(t)
	signed, _ := w.read(Object{"op": "record", "kind": "signed", "ref": index[event]})
	if _, err := VerifySigned(Encode(signed), other); faultCode(t, err) != "unknown_key" {
		t.Fatalf("other key set: %v", err)
	}
	entry := signer.PublicKey()
	entry["status"] = "revoked"
	revoked, err := ParseKeySet(Encode(Object{"format": KeySetFormat, "signer": "test-host", "keys": Array{entry}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySigned(Encode(signed), revoked); faultCode(t, err) != "revoked_key" {
		t.Fatalf("revoked key: %v", err)
	}
}

// Without a signing key, the Host writes no signed record and no index.
// After a key rotation, an earlier signed record stays as it was.
func TestSigningKeyIsOptionalAndRotationKeepsEarlierRecords(t *testing.T) {
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	snapshot, plan, _ := w.candidate("pass")
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	out, err := w.read(Object{"op": "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := out["state"].(Object)["signatures"]; has {
		t.Fatal("a write that is not an approval or an activation made a signature index")
	}
	first, firstKeys := newTestSigner(t)
	w.signer = first
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "fixture verdict"})["decision_ref"].(string)
	second, secondKeys := newTestSigner(t)
	w.signer = second
	event := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})["event_ref"].(string)
	index := w.signatures()
	check := func(ref string, keys *KeySet, id string) {
		t.Helper()
		signed, err := w.read(Object{"op": "record", "kind": "signed", "ref": index[ref]})
		if err != nil {
			t.Fatal(err)
		}
		result, err := VerifySigned(Encode(signed), keys)
		if err != nil || result["key_id"] != id {
			t.Fatalf("got %v %v, want key %s", result, err, id)
		}
	}
	check(decision, firstKeys, first.KeyID())
	check(event, secondKeys, second.KeyID())
	// A Host without a key adds no signed record and keeps the index.
	w.signer = nil
	w.must("reviewer", "reviewer", "suspend", with(window, Object{"targets": Array{event}, "reason": "stop"}))
	if after := w.signatures(); !equal(after, index) {
		t.Fatalf("signatures changed without a key: %s", Encode(after))
	}
}

// A signing Host refuses an approval whose record is over the limit, and
// stores nothing. A record at the limit gets a signed record that fits in
// one protocol frame of 1 MiB. The verifier refuses a longer record text.
func TestSignedRecordFitsInOneFrame(t *testing.T) {
	const frame = 1 << 20
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
	signer, keys := newTestSigner(t)
	w.signer = signer
	snapshot, plan, _ := w.candidate("pass")
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	before, _ := w.store.Read(nil)
	_, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": qualified["report_ref"], "reason": strings.Repeat("r", 600000)})
	if fault, ok := err.(*Fault); !ok || fault.Code != "record_too_large" || fault.Stage != "signature" {
		t.Fatalf("long reason: %v", err)
	}
	if after, _ := w.store.Read(nil); after.Cursor != before.Cursor {
		t.Fatal("the refused approval published state")
	}
	// Find the reason length that makes a record of exactly the limit.
	small := w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "r", "disposition": "reject"})
	record := small["decision"].(Object)
	fill := maxSignedRecord - len(Encode(record))
	other := newWorld(t, w.runner)
	other.signer = signer
	snapshot, plan, _ = other.candidate("pass")
	qualified = other.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	if _, err := other.do("reviewer", "reviewer", other.runner, "decide", Object{"report_ref": qualified["report_ref"],
		"reason": strings.Repeat("r", fill+2), "disposition": "reject"}); faultCode(t, err) != "record_too_large" {
		t.Fatalf("one byte over the limit: %v", err)
	}
	decision := other.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"],
		"reason": strings.Repeat("r", fill+1), "disposition": "reject"})["decision_ref"].(string)
	signed, err := other.read(Object{"op": "record", "kind": "signed", "ref": other.signatures()[decision]})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(signed["record"].(string))
	if len(raw) != maxSignedRecord {
		t.Fatalf("record of %d bytes, want the limit %d", len(raw), maxSignedRecord)
	}
	// 4096 bytes is more than the envelope of a protocol response.
	if size := len(Encode(signed)); size+4096 > frame {
		t.Fatalf("the signed record has %d bytes", size)
	}
	if _, err := VerifySigned(Encode(signed), keys); err != nil {
		t.Fatal(err)
	}
	long := signer.sign("demo", "record.visible", "decision", append(raw[:len(raw)-1], []byte("  \n")...))
	long["format"] = RecordFormat
	_, err = VerifySigned(Encode(long), keys)
	if fault, ok := err.(*Fault); !ok || fault.Code != "invalid_signed_record" || !strings.HasPrefix(fault.Message, "schema: ") {
		t.Fatalf("record over the limit: %v", err)
	}
}

func TestSigningKeyFileIsChecked(t *testing.T) {
	raw, err := NewSigningKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := Decode(raw)
	good := value.(Object)
	_, other := newTestSigner(t)
	var otherID string
	for id := range other.keys {
		otherID = id
	}
	cases := map[string]func(Object){
		"format":     func(d Object) { d["format"] = "entelos.signing-key/2" },
		"algorithm":  func(d Object) { d["algorithm"] = "ed448" },
		"key id":     func(d Object) { d["key_id"] = otherID },
		"short seed": func(d Object) { d["seed"] = "AAAA" },
		"extra":      func(d Object) { d["note"] = "x" },
		"missing":    func(d Object) { delete(d, "seed") },
	}
	for name, change := range cases {
		doc := copyObject(good)
		change(doc)
		if _, err := NewSigner(Encode(doc)); faultCode(t, err) != "configuration_error" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := NewSigner([]byte("[]")); faultCode(t, err) != "configuration_error" {
		t.Errorf("array: %v", err)
	}
	if _, err := VerifySigned([]byte("{}"), nil); faultCode(t, err) != "invalid_keyset" {
		t.Errorf("nil key set: %v", err)
	}
}
