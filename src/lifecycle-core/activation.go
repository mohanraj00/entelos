package lifecycle

// The activation record and the suspension record of
// src/protocol/activation/ACTIVATION.md.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	activationKind = "activation"
	suspensionKind = "suspension"
)

// publishedRecords gives, for each write that makes a published record, the
// record kind and the state field that maps the event reference to the
// record reference.
var publishedRecords = map[string]struct{ kind, field string }{
	"activate": {activationKind, "activations"},
	"suspend":  {suspensionKind, "suspensions"},
}

// activationFields are the fields of the record table. activationOptional
// are the fields of a content candidate only.
var (
	activationFields = map[string]bool{"format": true, "scope": true, "function": true, "event_ref": true, "request_key": true,
		"candidate": true, "base": true, "collection": true, "partition": true, "from": true, "until": true, "source_ref": true,
		"snapshot_ref": true, "report_ref": true, "decision_ref": true, "producer": true, "runner": true, "approver": true,
		"activator": true, "targets": true, "supersedes": true, "applicability": true}
	activationOptional = map[string]bool{"collection": true, "partition": true}
	suspensionFields   = map[string]bool{"format": true, "scope": true, "function": true, "event_ref": true, "request_key": true,
		"from": true, "until": true, "targets": true, "actor": true}
	candidateFields = map[string]bool{"kind": true, "ref": true}
	activationBase  = regexp.MustCompile(`^(sha256:[0-9a-f]{64})?$`)
	activationDate  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// Publisher gives a signed activation record or a signed suspension record
// to a consumer, for example a store with gated collections. The embedding
// host installs it.
//
// The Host calls Publish in the activate write and in the suspend write,
// after it signs the records and before it commits them. The store makes
// the call after it owns the root update, so a write that loses the
// compare-and-swap does not publish. The store holds its lock during the
// call, so Publish must not call the Host or the store. record is the
// activation record or the suspension record, and signed is its signed
// record, as the store keeps them. The record_kind of the signed record
// gives the kind. An error refuses the write, and the Host stores nothing.
// A commit can fail after an accept. A retry of the same request then gives
// the same bytes again, so the consumer must accept a record that it
// already has.
//
// ctx has the deadline of the call. See SetPublishTimeout. At the deadline
// the Host stops the write with publish_timeout and stores nothing. It
// does not wait for Publish. Thus Publish must stop its work when ctx is
// done. A call that continues after the deadline can still give the record
// to the consumer, as a commit that fails after an accept does.
type Publisher interface {
	Publish(ctx context.Context, record, signed []byte) error
}

// DefaultPublishTimeout is the time limit of one publisher call, if the
// embedding host sets no other limit.
const DefaultPublishTimeout = 10 * time.Second

// SetPublishTimeout sets the time limit of one publisher call. A limit of
// zero or less sets DefaultPublishTimeout. A Host that As gives has the
// limit of its source at that time.
func (h *Host) SetPublishTimeout(limit time.Duration) { h.publishTimeout = limit }

// SetPublisher installs the publisher of the Host. Nil removes it. A Host
// with a publisher and no signing key refuses each activate and each
// suspend with publish_unsigned. A Host with a publisher and a store of
// another package refuses them with publish_unsupported. A Host that As
// gives has the publisher of its source at that time.
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

// suspensionRecord maps one suspend event to the suspension record of
// ACTIVATION.md, or nil for another event. The record copies values only
// and has no time field, so the same event gives the same bytes. It does
// not copy the reason of the event.
func suspensionRecord(event Object, eventRef, requestKey string) Object {
	if !equal(event["kind"], "suspend") {
		return nil
	}
	return Object{"format": RecordFormat, "scope": event["scope"], "function": event["function"], "event_ref": eventRef,
		"request_key": requestKey, "from": event["from"], "until": event["until"], "targets": event["targets"], "actor": event["actor"]}
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
	putPublished(v, "activate", eventRef, activationRecord(event, eventRef, snapshot, base, report, decision, requestKey))
}

// putSuspension stores the suspension record of a suspend event when the
// Host has a signing key. The state field suspensions maps the event
// reference to the record reference. signStaged then signs the record.
func (h *Host) putSuspension(v *View, event Object, eventRef string, key any) {
	if h.signer == nil {
		return
	}
	requestKey, _ := key.(string)
	putPublished(v, "suspend", eventRef, suspensionRecord(event, eventRef, requestKey))
}

// putPublished stores the published record of the write op and adds it to
// the state field of its kind.
func putPublished(v *View, op, eventRef string, record Object) {
	published := publishedRecords[op]
	index, ok := v.State[published.field].(Object)
	if !ok {
		if _, has := v.State[published.field]; has {
			fail("invalid_store", published.field+" is not an object", "store")
		}
		index = Object{}
		v.State[published.field] = index
	}
	index[eventRef] = v.Put(published.kind, record)
}

// publish gives the signed activation record or the signed suspension
// record of this write to the publisher. The bytes are read now. The store
// calls the publisher after it owns the root update, so a write that loses
// the compare-and-swap does not publish. A refusal stops the write.
func (h *Host) publish(v *View, op string, value any) {
	published, has := publishedRecords[op]
	if h.publisher == nil || !has {
		return
	}
	ref := item(item(v.State, published.field), text(item(value, "event_ref")))
	signed := item(item(v.State, "signatures"), text(ref))
	record, signedRaw, publisher := getRaw(v, ref, Media(published.kind)), getRaw(v, signed, Media(signedKind)), h.publisher
	limit := h.publishTimeout
	if limit <= 0 {
		limit = DefaultPublishTimeout
	}
	v.beforeCommit(func() error { return callPublisher(publisher, limit, published.kind, record, signedRaw) })
}

// callPublisher makes one publisher call with a time limit. The store holds
// its lock during the call, so the limit is also the longest time that a
// publisher can stop the other writes. At the deadline, the result is
// publish_timeout, and the call does not wait for the publisher.
func callPublisher(publisher Publisher, limit time.Duration, kind string, record, signed []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	type answer struct {
		err      error
		panicked any
	}
	// The buffer lets a late publisher end without a reader.
	done := make(chan answer, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- answer{panicked: r}
			}
		}()
		done <- answer{err: publisher.Publish(ctx, record, signed)}
	}()
	timeout := NewFault("publish_timeout", "the publisher gave no answer for the "+kind+" record in "+limit.String()+"; the store commits nothing", "publish")
	var got answer
	select {
	case got = <-done:
	case <-ctx.Done():
		// An answer that came at the deadline counts.
		select {
		case got = <-done:
		default:
			return timeout
		}
	}
	if got.panicked != nil {
		// runSteps recovers this, as it does for a step with no time limit.
		panic(got.panicked)
	}
	if got.err == nil {
		return nil
	}
	// A publisher that stopped at the deadline gives the error of ctx.
	if ctx.Err() != nil && errors.Is(got.err, context.DeadlineExceeded) {
		return timeout
	}
	return NewFault("publish_refused", "the publisher refused the "+kind+" record: "+got.err.Error(), "publish")
}

func activationFault(code, rule, message string) *Fault {
	return recordFault(activationKind, code, rule, message)
}

// recordFault gives a refusal of the rules of one published record kind.
// The stage is the kind, and the message starts with the rule name.
func recordFault(kind, code, rule, message string) *Fault {
	if rule != "" {
		message = rule + ": " + message
	}
	return NewFault(code, message, kind)
}

// VerifyActivation checks a signed activation record with the checks of
// SIGNED.md and then with the rules of ACTIVATION.md. It makes no call to a
// Host. A valid record gives the verifier result, with the activation
// record as the record. A suspension record is not an activation record:
// the result is unexpected_record_kind.
func VerifyActivation(signed []byte, keys *KeySet) (Object, error) {
	return verifyPublished(signed, keys, activationKind, checkActivation)
}

// VerifySuspension checks a signed suspension record with the checks of
// SIGNED.md and then with the suspension rules of ACTIVATION.md. It makes
// no call to a Host. A valid record gives the verifier result, with the
// suspension record as the record. An activation record is not a
// suspension record: the result is unexpected_record_kind.
func VerifySuspension(signed []byte, keys *KeySet) (Object, error) {
	return verifyPublished(signed, keys, suspensionKind, checkSuspension)
}

func verifyPublished(signed []byte, keys *KeySet, kind string, check func(string, []byte) (Object, error)) (Object, error) {
	result, err := VerifySigned(signed, keys)
	if err != nil {
		return nil, err
	}
	// VerifySigned accepted the document, so the record is base64 text.
	doc := must(Decode(signed)).(Object)
	raw, err := base64.StdEncoding.DecodeString(doc["record"].(string))
	if err != nil {
		return nil, recordFault(kind, "invalid_"+kind, "json", "the record is not base64 text")
	}
	record, err := check(text(result["record_kind"]), raw)
	if err != nil {
		return nil, err
	}
	result["record"] = record
	return result, nil
}

// checkSuspension applies the suspension rules of ACTIVATION.md to the
// record bytes, in their order. The first rule that fails gives the result.
func checkSuspension(recordKind string, raw []byte) (Object, error) {
	bad := func(rule, message string) (Object, error) {
		return nil, recordFault(suspensionKind, "invalid_suspension", rule, message)
	}
	if recordKind != suspensionKind {
		return nil, recordFault(suspensionKind, "unexpected_record_kind", "", "the record kind is "+recordKind+", not suspension")
	}
	value, err := Decode(raw)
	doc, ok := value.(Object)
	if err != nil || !ok {
		return bad("json", "the record is not one strict JSON object")
	}
	if !equal(doc["format"], RecordFormat) {
		return nil, recordFault(suspensionKind, "unsupported_record_format", "", "the record format is not "+RecordFormat)
	}
	if field := unknownRecordField(raw, suspensionFields, nil); field != "" {
		return bad("unknown_field", field)
	}
	if !suspensionShape(doc) {
		return bad("schema", "the record does not agree with the schema")
	}
	from, until := doc["from"].(string), doc["until"].(string)
	if !calendarDate(from) || !calendarDate(until) || from >= until {
		return bad("interval", "the interval is not two real dates in order")
	}
	return doc, nil
}

// suspensionShape applies the schema of the suspension record.
func suspensionShape(doc Object) bool {
	// A nil pattern is a string with 1 character or more.
	str := func(name string, pattern *regexp.Regexp) bool {
		s, ok := doc[name].(string)
		return ok && (pattern == nil && s != "" || pattern != nil && pattern.MatchString(s))
	}
	if len(doc) != len(suspensionFields) {
		return false
	}
	for name := range doc {
		if !suspensionFields[name] {
			return false
		}
	}
	targets, ok := doc["targets"].(Array)
	if _, key := doc["request_key"].(string); !key || !ok || len(targets) == 0 || !str("scope", nil) || !str("function", nil) ||
		!str("actor", nil) || !str("event_ref", refPattern) || !str("from", activationDate) || !str("until", activationDate) {
		return false
	}
	for _, target := range targets {
		if s, ok := target.(string); !ok || !refPattern.MatchString(s) {
			return false
		}
	}
	return true
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
	if field := unknownRecordField(raw, activationFields, candidateFields); field != "" {
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
	return err == nil && parsed.Year() >= 1 && parsed.Format("2006-01-02") == s
}

// unknownRecordField gives the path of the first member name that the record
// table does not define, in the order of the text, or "". fields are the
// fields of the record. The walk enters the record and, if candidate is
// not nil, its candidate. The names in applicability are data.
func unknownRecordField(raw []byte, fields, candidate map[string]bool) string {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	root, err := readKeys(dec)
	if err != nil {
		return ""
	}
	for _, name := range root.names {
		if !fields[name] {
			return name
		}
		if node := root.members[name]; candidate != nil && name == "candidate" && node.object {
			for _, inner := range node.names {
				if !candidate[inner] {
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
