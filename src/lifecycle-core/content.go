package lifecycle

// The content proposal kind entelos.content/1. src/protocol/content/CONTENT.md
// is the spec. The Host keeps the manifest text and its digest. It never
// reads, fetches or stores a content item.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	contentKind = "content"
	celKind     = "cel-bundle"
	// contentPrefix starts the subject of a content head. The subject is
	// the value of the "function" field of each lifecycle record of the
	// head: content:<collection>/<partition>.
	contentPrefix = "content:"
	// contentMime is the media type of a stored manifest text.
	contentMime = "application/vnd.entelos.content-manifest+json"
	// contentFile is the name of the manifest text in the files that a
	// qualification runner gets.
	contentFile = "content"

	maxManifestBytes = 262144
	maxContentItems  = 1024
	maxMetadata      = 64
)

var (
	contentFormat = regexp.MustCompile(`^entelos\.content/(0|[1-9][0-9]*)$`)
	contentName   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	contentDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	contentBase   = regexp.MustCompile(`^(sha256:[0-9a-f]{64})?$`)
	mediaType     = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9][a-z0-9.+-]{0,63}$`)

	manifestFields = map[string]bool{"format": true, "collection": true, "partition": true, "base": true, "items": true,
		"metadata": true, "evidence": true}
	itemFields     = map[string]bool{"item_id": true, "digest": true, "size": true, "media_type": true}
	evidenceFields = map[string]bool{"format": true, "digest": true}
)

// candidateResult is the result of the rules of CONTENT.md for one
// candidate. It has the shape of the "expect" values of the vectors.
type candidateResult struct {
	Valid bool
	Code  string
	Rule  string
	Field string
	Kind  string
	// For a valid content candidate.
	Ref        string
	Collection string
	Partition  string
	Manifest   Object
	Text       string
}

// Object gives the result in the vector shape.
func (r candidateResult) Object() Object {
	if !r.Valid {
		out := Object{"valid": false, "code": r.Code}
		if r.Rule != "" {
			out["rule"] = r.Rule
		}
		if r.Field != "" {
			out["field"] = r.Field
		}
		return out
	}
	out := Object{"valid": true, "kind": r.Kind}
	if r.Kind == contentKind {
		out["ref"] = r.Ref
		out["head"] = Object{"kind": contentKind, "collection": r.Collection, "partition": r.Partition}
	}
	return out
}

// subject is the function value of the records of a content head.
func (r candidateResult) subject() string {
	return contentPrefix + r.Collection + "/" + r.Partition
}

// fault gives the refusal of an invalid candidate. The message names the
// rule and, for rule unknown_field, the path of the field.
func (r candidateResult) fault() *Fault {
	message := "the candidate is not valid"
	if r.Rule != "" {
		message = "rule " + r.Rule
	}
	if r.Field != "" {
		message += ": unknown field " + r.Field
	}
	return NewFault(r.Code, message, "")
}

func refused(code, rule string) candidateResult {
	return candidateResult{Code: code, Rule: rule}
}

// judgeCandidate applies the rules of CONTENT.md in their order. The first
// rule that fails gives the result.
func judgeCandidate(value any) candidateResult {
	candidate, ok := value.(Object)
	kind, isString := lookupAny(candidate, "kind").(string)
	if !ok || !isString {
		return refused("invalid_candidate", "kind")
	}
	if kind != celKind && kind != contentKind {
		return candidateResult{Code: "unsupported_candidate_kind"}
	}
	if !candidateShape(candidate, kind) {
		return refused("invalid_candidate", "candidate_schema")
	}
	if kind == celKind {
		return candidateResult{Valid: true, Kind: kind}
	}
	text := candidate["manifest"].(string)
	if len(text) > maxManifestBytes {
		return refused("invalid_content", "too_large")
	}
	// Decode is strict: it refuses a duplicate member name, a second value,
	// a non-finite number, a lone surrogate and deep nesting.
	decoded, err := Decode([]byte(text))
	doc, isObject := decoded.(Object)
	if err != nil || !isObject {
		return refused("invalid_content", "json")
	}
	format, isString := doc["format"].(string)
	match := contentFormat.FindStringSubmatch(format)
	if !isString || match == nil {
		return refused("invalid_content", "format")
	}
	if match[1] != "1" {
		return candidateResult{Code: "unsupported_content_format"}
	}
	field, walked := unknownField(text)
	if !walked {
		return refused("invalid_content", "json")
	}
	if field != "" {
		return candidateResult{Code: "invalid_content", Rule: "unknown_field", Field: field}
	}
	if !manifestShape(doc) {
		return refused("invalid_content", "schema")
	}
	items := doc["items"].(Array)
	if len(items) > maxContentItems {
		return refused("invalid_content", "too_many_items")
	}
	seen := map[string]bool{}
	for _, x := range items {
		id := x.(Object)["item_id"].(string)
		if seen[id] {
			return refused("invalid_content", "duplicate_item")
		}
		seen[id] = true
	}
	for i := 1; i < len(items); i++ {
		// Go compares strings by UTF-8 bytes, which is the code point order.
		if items[i-1].(Object)["item_id"].(string) >= items[i].(Object)["item_id"].(string) {
			return refused("invalid_content", "item_order")
		}
	}
	metadata, _ := doc["metadata"].(Object)
	if len(metadata) > maxMetadata {
		return refused("invalid_content", "too_many_metadata")
	}
	for name := range metadata {
		if !contentName.MatchString(name) {
			return refused("invalid_content", "metadata_name")
		}
	}
	sum := sha256.Sum256([]byte(text))
	return candidateResult{Valid: true, Kind: contentKind, Ref: "sha256:" + hex.EncodeToString(sum[:]),
		Collection: doc["collection"].(string), Partition: doc["partition"].(string), Manifest: doc, Text: text}
}

// candidateShape applies the candidate schema of the kind.
func candidateShape(candidate Object, kind string) bool {
	if kind == contentKind {
		_, isString := candidate["manifest"].(string)
		return len(candidate) == 2 && isString
	}
	bundle, ok := candidate["bundle"].(Object)
	if len(candidate) != 2 || !ok || len(bundle) != len(FileNames) {
		return false
	}
	for name := range FileNames {
		if _, isString := bundle[name].(string); !isString {
			return false
		}
	}
	return true
}

// manifestShape applies the manifest schema. Rule unknown_field ran before,
// so each object of the manifest table has only known members.
func manifestShape(doc Object) bool {
	str := func(value any, pattern *regexp.Regexp) bool {
		s, ok := value.(string)
		return ok && pattern.MatchString(s)
	}
	// The walk of rule unknown_field ran before. Check the member names
	// again on the decoded values, so that no unknown field passes.
	known := func(value Object, fields map[string]bool) bool {
		for name := range value {
			if !fields[name] {
				return false
			}
		}
		return true
	}
	if !known(doc, manifestFields) {
		return false
	}
	for _, name := range []string{"format", "collection", "partition", "base", "items"} {
		if _, has := doc[name]; !has {
			return false
		}
	}
	if !str(doc["collection"], contentName) || !str(doc["partition"], contentName) || !str(doc["base"], contentBase) {
		return false
	}
	items, ok := doc["items"].(Array)
	if !ok || len(items) == 0 {
		return false
	}
	for _, x := range items {
		entry, ok := x.(Object)
		if !ok || !known(entry, itemFields) || !str(entry["item_id"], contentName) || !str(entry["digest"], contentDigest) {
			return false
		}
		size, ok := entry["size"].(Int)
		if !ok || strings.HasPrefix(size.Text, "-") {
			return false
		}
		if media, has := entry["media_type"]; has && !str(media, mediaType) {
			return false
		}
	}
	if raw, has := doc["metadata"]; has {
		metadata, ok := raw.(Object)
		if !ok {
			return false
		}
		for _, value := range metadata {
			s, ok := value.(string)
			if !ok || utf8.RuneCountInString(s) > 1024 {
				return false
			}
		}
	}
	if raw, has := doc["evidence"]; has {
		evidence, ok := raw.(Object)
		if !ok || !known(evidence, evidenceFields) || !equal(evidence["format"], "entelos.evidence/1") || !str(evidence["digest"], contentDigest) {
			return false
		}
	}
	return true
}

// keyNode keeps the member names of a JSON value in the order of the text.
type keyNode struct {
	names    []string
	members  map[string]*keyNode
	elements []*keyNode
	object   bool
	array    bool
}

// readKeys reads one value. Each number stays text, so no number gives a
// conversion error. Any error stops the walk.
func readKeys(dec *json.Decoder) (*keyNode, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	node := &keyNode{}
	switch token {
	case json.Delim('{'):
		node.object, node.members = true, map[string]*keyNode{}
		for dec.More() {
			name, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := name.(string)
			node.names = append(node.names, key)
			if node.members[key], err = readKeys(dec); err != nil {
				return nil, err
			}
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
	case json.Delim('['):
		node.array = true
		for dec.More() {
			entry, err := readKeys(dec)
			if err != nil {
				return nil, err
			}
			node.elements = append(node.elements, entry)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
	}
	return node, nil
}

// unknownField gives the path of the first member name that the manifest
// table does not define, in the order of the text, or "". The text is a
// strict JSON object. The walk enters only the objects of the table: the
// manifest, each item and the evidence reference. The names in metadata
// are data.
func unknownField(text string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	root, err := readKeys(dec)
	if err != nil {
		return "", false
	}
	first := func(node *keyNode, known map[string]bool, path string) string {
		if !node.object {
			return ""
		}
		for _, name := range node.names {
			if !known[name] {
				return path + name
			}
		}
		return ""
	}
	for _, name := range root.names {
		if !manifestFields[name] {
			return name, true
		}
		value := root.members[name]
		switch name {
		case "items":
			if value.array {
				for i, entry := range value.elements {
					if found := first(entry, itemFields, "items["+strconv.Itoa(i)+"]."); found != "" {
						return found, true
					}
				}
			}
		case "evidence":
			if found := first(value, evidenceFields, "evidence."); found != "" {
				return found, true
			}
		}
	}
	return "", true
}

// hostOnlyFields has the request fields of the Host that no protocol schema
// has yet. The wire protocol gets them in a later minor version.
var hostOnlyFields = map[string][]string{"propose": {"candidate"}}

// requestSubject gives the function value of a request for the access check
// of a retry. For a content proposal, it is the subject of the head that the
// manifest names.
func requestSubject(op string, q Object) any {
	if raw, has := q["candidate"]; op == "propose" && has {
		if result := judgeCandidate(raw); result.Valid && result.Kind == contentKind {
			return result.subject()
		}
	}
	return item(q, "function")
}

// snapshotKind gives the candidate kind of a snapshot. A snapshot without a
// candidate field is a CEL bundle, as from an earlier Host.
func snapshotKind(snapshot Object) string {
	raw, has := snapshot["candidate"]
	if !has {
		return celKind
	}
	kind, _ := lookupAny(raw, "kind").(string)
	if kind != contentKind {
		fail("invalid_record", "unknown snapshot candidate kind", "store")
	}
	return kind
}

// celOnly refuses a content snapshot. Only a CEL bundle is described,
// loaded, invoked or ingested.
func celOnly(snapshot Object) {
	if snapshotKind(snapshot) != celKind {
		fail("unsupported_candidate_kind", "a content candidate is not loaded or invoked; resolve gives its candidate reference", "")
	}
}

// contentSelection is the dated selection of a content activation. It gives
// the candidate reference. The bundle_ref field holds the same reference, so
// that the qualification bindings of a CEL bundle apply without change.
func contentSelection(v *View, function, at any, event, snapshot Object) Object {
	return Object{"function": function, "scope": item(snapshot, "scope"), "date": at, "registry_cursor": v.Cursor,
		"event_ref": item(event, "ref"), "snapshot_ref": item(event, "snapshot_ref"), "bundle_ref": item(snapshot, "bundle_ref"),
		"candidate": item(snapshot, "candidate"), "collection": item(snapshot, "collection"), "partition": item(snapshot, "partition"),
		"report_ref": item(event, "report_ref"), "decision_ref": item(event, "decision_ref")}
}

// proposeContent writes the snapshot of a valid content candidate. The
// manifest names the head. Its base must be the candidate reference of the
// current head.
func (h *Host) proposeContent(v *View, q Object, result candidateResult) Object {
	for _, name := range []string{"function", "base_snapshot"} {
		if _, has := q[name]; has {
			fail("invalid_request", "a content candidate has no "+name+"; the manifest names the head and the base", "")
		}
	}
	target := h.target(Object{"function": result.subject(), "from": item(q, "from"), "until": item(q, "until")})
	staleMode := lookup(q, "stale_mode", failClosed)
	if !within(staleMode, Array{failClosed, continueStale}) {
		fail("invalid_request", "stale_mode must be fail_closed or continue_stale", "")
	}
	heads := object(item(v.State, "heads"))
	key := headKey(target["function"], target["scope"])
	base := ""
	if ref, _ := heads[key].(string); ref != "" {
		head := h.scoped(v, ref, "snapshot")
		if snapshotKind(head) != contentKind {
			fail("invalid_record", "the head of a content subject is not a content snapshot", "store")
		}
		base = text(item(item(head, "candidate"), "ref"))
	}
	if !equal(result.Manifest["base"], base) {
		fail("stale_base", "the manifest base is not the candidate reference of the current head", "")
	}
	source := h.scoped(v, item(q, "source_ref"), "source")
	if !same(source, target) || !covers(source, target) {
		fail("source_not_applicable", "source does not cover the candidate", "")
	}
	candidate := Object{"kind": contentKind, "ref": result.Ref}
	snapshot := merge(target, Object{"candidate": candidate, "collection": result.Collection, "partition": result.Partition,
		"base": base, "manifest_ref": v.PutRaw(contentMime, []byte(result.Text)), "bundle_ref": result.Ref,
		"source_ref": q["source_ref"], "reason": item(q, "reason"), "producer": h.ctx("actor"), "stale_mode": staleMode,
		"submitted_at": h.now()})
	if evidence, has := result.Manifest["evidence"]; has {
		snapshot["evidence_reference"] = evidence
	}
	// The evidence document of the request is kept as for a CEL bundle.
	// The Host does not compare it with the evidence reference of the
	// manifest.
	if evidence, has := q["evidence"]; has {
		snapshot["evidence"] = evidence
	}
	ref := v.Put("snapshot", snapshot)
	heads[key] = ref
	return Object{"snapshot_ref": ref, "candidate": copyObject(candidate),
		"head": Object{"kind": contentKind, "collection": result.Collection, "partition": result.Partition}}
}
