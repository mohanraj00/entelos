package lifecycle

// The read-only decision stream of src/protocol/events/EVENTS.md.
import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const maxEventPosition = int64(9007199254740991)

var decisionEventFormat = regexp.MustCompile(`^entelos\.decision-event/(0|[1-9][0-9]*)$`)
var decisionEventCursor = regexp.MustCompile(`^ev1:([0-9a-f]{64}):(0|[1-9][0-9]{0,15})$`)

var eventRule = evObject([]string{"format", "cursor", "decision_ref", "disposition", "fact_digest", "evidence_receipt_digest", "actor_kind", "blind"}, map[string]*evRule{
	"format": {kind: "string", enum: []string{"entelos.decision-event/1"}},
	"cursor": evString(decisionEventCursor.String()), "decision_ref": evDigest,
	"disposition": {kind: "string", enum: []string{"approve", "reject", "correct"}},
	"fact_digest": evDigest, "evidence_receipt_digest": evDigest,
	"actor_kind": {kind: "string", enum: verdictKinds}, "blind": {kind: "boolean"},
	"corrected_value": {kind: "string", null: true, maxLen: 65536},
})

// CheckDecisionEvent applies the five event rules in order. It returns the
// vector result: valid, and code and rule on refusal. It writes nothing.
func CheckDecisionEvent(value any) Object {
	bad := func(rule string) Object { return Object{"valid": false, "code": "invalid_event", "rule": rule} }
	event, ok := value.(Object)
	format, _ := event["format"].(string)
	m := decisionEventFormat.FindStringSubmatch(format)
	if !ok || m == nil {
		return bad("format")
	}
	if m[1] != "1" {
		return Object{"valid": false, "code": "unsupported_event_format"}
	}
	if !eventRule.agrees(event) {
		return bad("schema")
	}
	_, pos, valid := eventPosition(event["cursor"])
	if !valid || pos == 0 {
		return bad("schema")
	}
	_, corrected := event["corrected_value"]
	if corrected != (event["disposition"] == "correct") {
		return bad("correction")
	}
	if event["actor_kind"] != "human" && (corrected || event["blind"] == true) {
		return bad("provenance")
	}
	return Object{"valid": true}
}

func eventPosition(value any) (string, int64, bool) {
	s, _ := value.(string)
	m := decisionEventCursor.FindStringSubmatch(s)
	if m == nil {
		return "", 0, false
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	return m[1], n, err == nil && n <= maxEventPosition
}

// The marker and append index commit together. The parent identifies the
// start revision without a self-reference to its own content hash.
func prepareDecisionStream(v *View) {
	if _, has := v.State["decision_stream"]; !has {
		seed := make([]byte, 32)
		must(rand.Read(seed))
		v.State["decision_stream"] = Object{"namespace": hex.EncodeToString(seed), "after": v.Cursor, "decisions": Array{}}
	}
}

// StartDecisionEvents commits the stream boundary if no Host write has done
// so. Use it before reading an empty store or before upgrading an old store.
// It changes no decision. Concurrent starts use the same idempotency key.
func (h *Host) StartDecisionEvents() error {
	for {
		v, err := h.Store.Read(nil)
		if err != nil {
			return err
		}
		if _, has := v.State["decision_stream"]; has {
			return nil
		}
		_, err = h.Store.Transact(v.Cursor, "start-decision-events/1", Object{"format": "entelos.decision-event/1"}, func(v *View) (any, error) {
			prepareDecisionStream(v)
			return Object{}, nil
		})
		if f, ok := err.(*Fault); ok && f.Code == "stale_revision" {
			continue
		}
		return err
	}
}

var factReviewRule = evObject([]string{"blind"}, map[string]*evRule{
	"blind": {kind: "boolean"}, "corrected_value": {kind: "string", null: true, maxLen: 65536},
})

// Keep the caller's statement in the decision, never in producer evidence.
// Old requests may omit it. A decision with facts and no fact_reviews field
// is then an unmarked verdict, which the export leaves out.
func (h *Host) recordFactReviews(snapshot, decision, q Object) {
	facts := citedFacts(snapshot)
	kind := decision["kind"]
	raw, supplied := q["fact_reviews"]
	if !supplied {
		if kind != "model" && kind != "policy" {
			return
		}
		marks := Array{}
		for range facts {
			marks = append(marks, Object{"blind": false})
		}
		if len(marks) > 0 {
			decision["fact_reviews"] = marks
		}
		return
	}
	marks, ok := raw.(Array)
	if !ok || len(marks) != len(facts) || len(marks) > maxEvidenceFacts || kind != "human" {
		fail("invalid_request", "fact_reviews requires a human actor and one mark per cited fact", "")
	}
	for _, raw := range marks {
		if !factReviewRule.agrees(raw) {
			fail("invalid_request", "invalid fact review", "")
		}
		if _, corrected := raw.(Object)["corrected_value"]; corrected && decision["disposition"] != "accept" {
			fail("invalid_request", "a rejection has no fact corrections", "")
		}
	}
	decision["fact_reviews"] = marks
}

func citedFacts(snapshot Object) Array {
	doc, _ := snapshot["evidence"].(Object)
	facts, _ := doc["facts"].(Array)
	return facts
}

func (h *Host) decisionEvents(q Object) Object {
	// Request before cursor before source checks, on every transport.
	raw, has := q["limit"]
	if value, ok := raw.(int64); ok {
		raw = Int{strconv.FormatInt(value, 10)}
	}
	n, numeric := number(raw)
	validInt := false
	switch raw.(type) {
	case Int, int, int64:
		validInt = true
	}
	if !has || !validInt || !numeric || !n.IsInt() || n.Cmp(big.NewRat(1, 1)) < 0 || n.Cmp(big.NewRat(256, 1)) > 0 {
		fail("invalid_request", "limit must be an integer from 1 to 256", "")
	}
	if cursor, has := q["cursor"]; has {
		if _, ok := cursor.(string); !ok {
			fail("invalid_request", "cursor must be a string", "")
		}
	}
	v := must(h.Store.Read(nil)) // one immutable committed prefix
	stream, ok := v.State["decision_stream"].(Object)
	if !ok {
		fail("invalid_cursor", "the store has no committed decision stream start", "store")
	}
	history := must(h.Store.History(v.Cursor))
	after, ok := stream["after"].(string)
	start := ""
	for i, revision := range history {
		parent := ""
		if i+1 < len(history) {
			parent = history[i+1]
		}
		if parent == after {
			start = revision
			break
		}
	}
	if !ok || start == "" {
		fail("invalid_cursor", "the stream start is not retained", "store")
	}
	// This hash assigns a namespace for the saved seed and fixed selection.
	// It is neither a signature nor an access grant.
	namespace := strings.TrimPrefix(ContentRef(Encode(Array{"entelos.decision-event/1", stream["namespace"], start, h.pageContext()})), "sha256:")
	cursor := func(p int64) string { return fmt.Sprintf("ev1:%s:%d", namespace, p) }
	position := int64(0)
	if raw, has := q["cursor"]; has {
		ns, p, valid := eventPosition(raw)
		if !valid || ns != namespace {
			fail("invalid_cursor", "invalid decision event cursor", "store")
		}
		position = p
	}
	type source struct {
		ref                string
		decision, snapshot Object
		offset, count      int64
	}
	sources := []source{}
	total := int64(0)
	for _, raw := range elements(stream["decisions"]) {
		ref := text(raw)
		d := get(v, ref, "decision")
		if d["scope"] != h.ctx("scope") || !within(d["function"], h.ctxList("functions")) {
			continue
		}
		snapshot := h.scoped(v, d["snapshot_ref"], "snapshot")
		count := int64(len(citedFacts(snapshot)))
		if unmarkedVerdict(d, count) {
			continue
		}
		if count > maxEventPosition-total {
			fail("invalid_cursor", "stream position exceeds the event range", "store")
		}
		sources = append(sources, source{ref, d, snapshot, total, count})
		total += count
	}
	if position > total {
		fail("invalid_cursor", "cursor is after the stream end", "store")
	}
	end := min(position+n.Num().Int64(), total)
	events := Array{}
	for _, s := range sources {
		if s.offset >= end || s.offset+s.count <= position {
			continue
		}
		// Check the whole contributing decision before slicing the page.
		projected := projectDecision(s.ref, s.decision, s.snapshot, s.offset, cursor)
		for i, e := range projected {
			p := s.offset + int64(i) + 1
			if p > position && p <= end {
				events = append(events, e)
			}
		}
	}
	return Object{"events": events, "next_cursor": cursor(end), "stream_start": start}
}

// unmarkedVerdict tells if a decision cites facts and has no fact_reviews
// field (EVENTS.md, Unmarked verdicts). Such a decision gives no events and
// uses no position. The read never supplies its marks or its kind. A record
// that has the field is never unmarked: projectDecision checks it.
func unmarkedVerdict(decision Object, facts int64) bool {
	_, marked := decision["fact_reviews"]
	return facts > 0 && !marked
}

func projectDecision(ref string, decision, snapshot Object, offset int64, cursor func(int64) string) Array {
	bad := func() {
		fail("unexportable_decision", "a contributing decision has incomplete or invalid event provenance", "admission")
	}
	disposition := ""
	switch decision["disposition"] {
	case "accept":
		disposition = "approve"
	case "reject":
		disposition = "reject"
	default:
		bad()
	}
	facts := citedFacts(snapshot)
	marks, ok := decision["fact_reviews"].(Array)
	if _, has := decision["fact_reviews"]; !has && len(facts) == 0 {
		// A fact-free verdict uses no position, so it needs no marks.
		marks, ok = Array{}, true
	}
	if !ok || len(marks) != len(facts) {
		bad()
	}
	out := Array{}
	for i, raw := range facts {
		f, ok := raw.(Object)
		if !ok {
			bad()
		}
		mark, ok := marks[i].(Object)
		if !ok || !factReviewRule.agrees(mark) {
			bad()
		}
		event := Object{"format": "entelos.decision-event/1", "cursor": cursor(offset + int64(i) + 1), "decision_ref": ref,
			"disposition": disposition, "fact_digest": f["decision_digest"], "evidence_receipt_digest": f["receipt_digest"],
			"actor_kind": decision["kind"], "blind": mark["blind"]}
		if value, corrected := mark["corrected_value"]; corrected {
			if disposition != "approve" {
				bad()
			}
			event["disposition"], event["corrected_value"] = "correct", value
		}
		if CheckDecisionEvent(event)["valid"] != true {
			bad()
		}
		out = append(out, event)
	}
	return out
}
