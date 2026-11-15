package lifecycle

// The activation record of src/protocol/activation/ACTIVATION.md.

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

const activationKind = "activation"

// activationFields are the fields of the record table. activationOptional
// are the fields of a content candidate only.
var (
	activationFields = map[string]bool{"format": true, "scope": true, "function": true, "event_ref": true, "request_key": true,
		"candidate": true, "base": true, "collection": true, "partition": true, "from": true, "until": true, "source_ref": true,
		"snapshot_ref": true, "report_ref": true, "decision_ref": true, "producer": true, "runner": true, "approver": true,
		"activator": true, "targets": true, "supersedes": true, "applicability": true}
	activationOptional = map[string]bool{"collection": true, "partition": true}
	candidateFields    = map[string]bool{"kind": true, "ref": true}
	activationBase     = regexp.MustCompile(`^(sha256:[0-9a-f]{64})?$`)
	activationDate     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// Publisher gives a signed activation record to a consumer, for example a
// store with gated collections. The embedding host installs it.
//
// The Host calls Publish in the activate write, after it signs the records
// and before it commits them. record is the activation record and signed is
// its signed record, as the store keeps them. An error refuses the
// activation, and the Host stores nothing. A commit can fail after an
// accept. A retry of the same request then gives the same bytes again, so
// the consumer must accept a record that it already has.
type Publisher interface {
	Publish(record, signed []byte) error
}

// SetPublisher installs the publisher of the Host. Nil removes it. A Host
// with a publisher and no signing key refuses each activate with
// publish_unsigned. A Host that As gives has the publisher of its source at
// that time.
func (h *Host) SetPublisher(p Publisher) { h.publisher = p }

// activationRecord maps one enable event and the records that it names to
// the activation record of ACTIVATION.md, or nil for another event. base
// is the base snapshot of a CEL snapshot, or nil. The record copies values only and has no time
// field, so the same records give the same bytes.
func activationRecord(event Object, eventRef string, snapshot, base, report, decision Object, requestKey string) Object {
	// A suspend event has no activation record.
	if !equal(event["kind"], "enable") {
		return nil
	}
	out := Object{"format": RecordFormat, "scope": event["scope"], "function": event["function"], "event_ref": eventRef,
		"request_key": requestKey}
	if candidate, has := snapshot["candidate"]; has {
		out["candidate"], out["base"] = candidate, snapshot["base"]
		out["collection"], out["partition"] = snapshot["collection"], snapshot["partition"]
	} else {
		out["candidate"], out["base"] = Object{"kind": "cel-bundle", "ref": snapshot["bundle_ref"]}, ""
		if base != nil {
			out["base"] = base["bundle_ref"]
		}
	}
	return merge(out, Object{"from": event["from"], "until": event["until"], "source_ref": snapshot["source_ref"],
		"snapshot_ref": event["snapshot_ref"], "report_ref": event["report_ref"], "decision_ref": event["decision_ref"],
		"producer": snapshot["producer"], "runner": report["runner"], "approver": decision["actor"], "activator": event["actor"],
		"targets": event["targets"], "supersedes": lookup(event, "supersedes", Array{}), "applicability": lookup(event, "applicability", Object{})})
}

// putActivation stores the activation record of an enable event when the
// Host has a signing key. The state field activations maps the event
// reference to the record reference. signStaged then signs the record.
func (h *Host) putActivation(v *View, event Object, eventRef string, snapshot, report, decision Object, key any) {
	if h.signer == nil {
		return
	}
	var base Object
	if ref := lookup(snapshot, "base_snapshot", nil); truthy(ref) {
		base = get(v, ref, "snapshot")
	}
	requestKey, _ := key.(string)
	index, ok := v.State["activations"].(Object)
	if !ok {
		if _, has := v.State["activations"]; has {
			fail("invalid_store", "activations is not an object", "store")
		}
		index = Object{}
		v.State["activations"] = index
	}
	index[eventRef] = v.Put(activationKind, activationRecord(event, eventRef, snapshot, base, report, decision, requestKey))
}

// publish gives the signed activation record of this write to the
// publisher. A refusal stops the write.
func (h *Host) publish(v *View, value any) {
	if h.publisher == nil {
		return
	}
	ref := item(item(v.State, "activations"), text(item(value, "event_ref")))
	signed := item(item(v.State, "signatures"), text(ref))
	if err := h.publisher.Publish(getRaw(v, ref, Media(activationKind)), getRaw(v, signed, Media(signedKind))); err != nil {
		fail("publish_refused", "the publisher refused the activation record: "+err.Error(), "publish")
	}
}

func activationFault(code, rule, message string) *Fault {
	if rule != "" {
		message = rule + ": " + message
	}
	return NewFault(code, message, "activation")
}

// VerifyActivation checks a signed activation record with the checks of
// SIGNED.md and then with the rules of ACTIVATION.md. It makes no call to a
// Host. A valid record gives the verifier result, with the activation
// record as the record.
func VerifyActivation(signed []byte, keys *KeySet) (Object, error) {
	result, err := VerifySigned(signed, keys)
	if err != nil {
		return nil, err
	}
	// VerifySigned accepted the document, so the record is base64 text.
	doc := must(Decode(signed)).(Object)
	raw, err := base64.StdEncoding.DecodeString(doc["record"].(string))
	if err != nil {
		return nil, activationFault("invalid_activation", "json", "the record is not base64 text")
	}
	record, err := checkActivation(text(result["record_kind"]), raw)
	if err != nil {
		return nil, err
	}
	result["record"] = record
	return result, nil
}

// checkActivation applies the rules of ACTIVATION.md to the record bytes,
// in their order. The first rule that fails gives the result.
func checkActivation(recordKind string, raw []byte) (Object, error) {
	if recordKind != activationKind {
		return nil, activationFault("unexpected_record_kind", "", "the record kind is "+recordKind+", not activation")
	}
	value, err := Decode(raw)
	doc, ok := value.(Object)
	if err != nil || !ok {
		return nil, activationFault("invalid_activation", "json", "the record is not one strict JSON object")
	}
	if !equal(doc["format"], RecordFormat) {
		return nil, activationFault("unsupported_record_format", "", "the record format is not "+RecordFormat)
	}
	if field := unknownActivationField(raw); field != "" {
		return nil, activationFault("invalid_activation", "unknown_field", field)
	}
	if !activationShape(doc) {
		return nil, activationFault("invalid_activation", "schema", "the record does not agree with the schema")
	}
	content := equal(item(doc["candidate"], "kind"), contentKind)
	_, collection := doc["collection"]
	_, partition := doc["partition"]
	if content != collection || content != partition {
		return nil, activationFault("invalid_activation", "candidate_fields", "collection and partition are only for a content candidate")
	}
	if content && doc["function"] != contentPrefix+doc["collection"].(string)+"/"+doc["partition"].(string) {
		return nil, activationFault("invalid_activation", "subject", "the function is not the subject of the collection and the partition")
	}
	from, until := doc["from"].(string), doc["until"].(string)
	if !calendarDate(from) || !calendarDate(until) || from >= until {
		return nil, activationFault("invalid_activation", "interval", "the interval is not two real dates in order")
	}
	return doc, nil
}

func calendarDate(s string) bool {
	parsed, err := time.Parse("2006-01-02", s)
	return err == nil && parsed.Format("2006-01-02") == s
}

// unknownActivationField gives the path of the first member name that the
// record table does not define, in the order of the text, or "". The walk
// enters the record and its candidate. The names in applicability are data.
func unknownActivationField(raw []byte) string {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	root, err := readKeys(dec)
	if err != nil {
		return ""
	}
	for _, name := range root.names {
		if !activationFields[name] {
			return name
		}
		if node := root.members[name]; name == "candidate" && node.object {
			for _, inner := range node.names {
				if !candidateFields[inner] {
					return "candidate." + inner
				}
			}
		}
	}
	return ""
}

// activationShape applies the schema of the activation record. Rule
// unknown_field ran before, but the member names are checked again on the
// decoded values.
func activationShape(doc Object) bool {
	str := func(value any, pattern *regexp.Regexp) bool {
		s, ok := value.(string)
		return ok && pattern.MatchString(s)
	}
	nonEmpty := func(value any) bool {
		s, ok := value.(string)
		return ok && s != ""
	}
	refs := func(value any) bool {
		list, ok := value.(Array)
		if !ok {
			return false
		}
		for _, x := range list {
			if !str(x, refPattern) {
				return false
			}
		}
		return true
	}
	for name := range doc {
		if !activationFields[name] {
			return false
		}
	}
	for name := range activationFields {
		if _, has := doc[name]; !has && !activationOptional[name] {
			return false
		}
	}
	for _, name := range []string{"scope", "function", "producer", "runner", "approver", "activator"} {
		if !nonEmpty(doc[name]) {
			return false
		}
	}
	for _, name := range []string{"event_ref", "source_ref", "snapshot_ref", "report_ref", "decision_ref"} {
		if !str(doc[name], refPattern) {
			return false
		}
	}
	if _, ok := doc["request_key"].(string); !ok || !str(doc["base"], activationBase) ||
		!str(doc["from"], activationDate) || !str(doc["until"], activationDate) || !refs(doc["targets"]) || !refs(doc["supersedes"]) {
		return false
	}
	for _, name := range []string{"collection", "partition"} {
		if value, has := doc[name]; has && !str(value, contentName) {
			return false
		}
	}
	candidate, ok := doc["candidate"].(Object)
	if !ok || len(candidate) != 2 || !str(candidate["ref"], refPattern) ||
		(!equal(candidate["kind"], "cel-bundle") && !equal(candidate["kind"], contentKind)) {
		return false
	}
	applicability, ok := doc["applicability"].(Object)
	if !ok {
		return false
	}
	// The schema permits an item with no names. The rules of CONTENT.md do
	// not, but this record does not apply them.
	for _, values := range applicability {
		if !applicabilityShape(values) {
			return false
		}
	}
	return true
}
