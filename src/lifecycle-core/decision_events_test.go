package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func eventVectors(t *testing.T, name string) Object {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "events", "vectors", name))
	if err != nil {
		t.Fatal(err)
	}
	return must(Decode(raw)).(Object)
}

func TestDecisionEventVectors(t *testing.T) {
	for _, raw := range eventVectors(t, "event-cases.json")["cases"].(Array) {
		c := raw.(Object)
		t.Run(c["name"].(string), func(t *testing.T) {
			got := CheckDecisionEvent(expandEvidence(c["event"]))
			if !equal(got, c["expect"]) {
				t.Fatalf("got %s, want %s", Encode(got), Encode(c["expect"]))
			}
		})
	}
}

// Vectors name references, revisions and namespaces symbolically. The
// fixture uses real records and commits. Only those three names are mapped
// for comparison. Facts, marks, positions and expectations stay unchanged.
// Malformed fixture records bypass decide. Every read uses the real Host.
// No expected event is used to construct the store.
func TestDecisionExportVectors(t *testing.T) {
	doc := eventVectors(t, "export-cases.json")
	for _, raw := range doc["cases"].(Array) {
		c := raw.(Object)
		t.Run(c["name"].(string), func(t *testing.T) {
			stream := doc["streams"].(Object)[c["stream"].(string)].(Object)
			store := NewMemoryStore()
			host := eventHost(t, store)
			names := map[string]string{}
			started := false
			for _, raw := range stream["decisions"].(Array) {
				d := raw.(Object)
				before := d["before_stream_start"] == true
				if !before && !started {
					if err := host.StartDecisionEvents(); err != nil {
						t.Fatal(err)
					}
					started = true
				}
				v := must(store.Read(nil))
				_, err := store.Transact(v.Cursor, fmt.Sprint(len(names)), Object{}, func(v *View) (any, error) {
					facts, marks := Array{}, Array{}
					for _, raw := range d["facts"].(Array) {
						f := raw.(Object)
						facts = append(facts, Object{"decision_digest": f["fact_digest"], "receipt_digest": f["evidence_receipt_digest"]})
						mark := Object{}
						for _, k := range []string{"blind", "corrected_value"} {
							if x, has := f[k]; has {
								mark[k] = x
							}
						}
						marks = append(marks, mark)
					}
					snapshot := v.Put("snapshot", Object{"scope": "demo", "function": "record.visible", "evidence": Object{"facts": facts}})
					disposition := d["disposition"]
					if disposition == "approve" {
						disposition = "accept"
					}
					decision := Object{"scope": "demo", "function": "record.visible", "snapshot_ref": snapshot, "disposition": disposition, "fact_reviews": marks}
					// A vector decision with facts and no mark on any fact
					// stands for a record with no fact_reviews field.
					unmarked := len(marks) > 0
					for _, mark := range marks {
						unmarked = unmarked && len(mark.(Object)) == 0
					}
					if unmarked {
						delete(decision, "fact_reviews")
					}
					if kind, has := d["actor_kind"]; has {
						decision["kind"] = kind
					}
					ref := v.Put("decision", decision)
					names[ref] = d["decision_ref"].(string)
					if !before {
						appendEntry(v.State["decision_stream"].(Object), "decisions", ref)
					}
					return Object{}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := host.StartDecisionEvents(); err != nil {
				t.Fatal(err)
			}
			v := must(store.Read(nil))
			marker := v.State["decision_stream"].(Object)
			history := must(store.History(v.Cursor))
			start := ""
			for i, revision := range history {
				parent := ""
				if i+1 < len(history) {
					parent = history[i+1]
				}
				if parent == marker["after"] {
					start = revision
					break
				}
			}
			namespace := strings.TrimPrefix(ContentRef(Encode(Array{"entelos.decision-event/1", marker["namespace"], start, host.pageContext()})), "sha256:")
			q, isObject := c["request"].(Object)
			var request any
			if isObject {
				q = copyObject(q)
				if cursor, ok := q["cursor"].(string); ok {
					q["cursor"] = strings.Replace(cursor, "ev1:"+stream["namespace"].(string)+":", "ev1:"+namespace+":", 1)
				}
				q["op"] = "decision_events"
				request = q
			}
			result, err := readAs(host, request)
			got := Object{"ok": err == nil}
			if err != nil {
				got["code"] = faultCode(t, err)
			} else {
				page := result.(Object)
				if page["stream_start"] != start {
					t.Fatal("read gave another stream start")
				}
				got["stream_start"] = stream["stream_start"]
				got["next_cursor"] = strings.Replace(page["next_cursor"].(string), namespace, stream["namespace"].(string), 1)
				events := Array{}
				for _, raw := range page["events"].(Array) {
					event := copyObject(raw.(Object))
					symbolic, found := names[event["decision_ref"].(string)]
					if !found {
						t.Fatal("unknown exported record")
					}
					event["decision_ref"] = symbolic
					event["cursor"] = strings.Replace(event["cursor"].(string), namespace, stream["namespace"].(string), 1)
					events = append(events, event)
				}
				got["events"] = events
			}
			if !equal(got, c["expect"]) {
				t.Fatalf("got %s, want %s", Encode(got), Encode(c["expect"]))
			}
			if must(store.Read(nil)).Cursor != v.Cursor {
				t.Fatal("read changed the store")
			}
		})
	}
}

func eventHost(t *testing.T, store Store) *Host {
	t.Helper()
	h, err := NewHost(store, nil, Object{"actor": "reviewer", "roles": Array{"reviewer"}, "scope": "demo", "runner_actor": "runner", "worker": "unused", "functions": Array{"record.visible"}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestInspectOmitsDecisionStream(t *testing.T) {
	eachStore(t, func(t *testing.T, store Store) {
		host := eventHost(t, store)
		if err := host.StartDecisionEvents(); err != nil {
			t.Fatal(err)
		}
		before := root(t, store)
		if _, has := before.State["decision_stream"]; !has {
			t.Fatal("store has no decision stream")
		}
		want := copyObject(before.State)
		delete(want, "decision_stream")
		for _, request := range []Object{{"op": "inspect"}, {"op": "inspect", "cursor": before.Cursor}} {
			result, err := host.Handle(request)
			if err != nil {
				t.Fatal(err)
			}
			state := result.(Object)["state"].(Object)
			if _, has := state["decision_stream"]; has {
				t.Fatal("inspect returned decision_stream")
			}
			if result.(Object)["cursor"] != before.Cursor || !equal(state, want) {
				t.Fatal("inspect changed the cursor or other state fields")
			}
			after := root(t, store)
			if after.Cursor != before.Cursor || !equal(after.State, before.State) {
				t.Fatal("inspect changed stored state")
			}
		}
	})
}

func eventEvidence() Object {
	facts := Array{}
	for i := 1; i <= 3; i++ {
		facts = append(facts, Object{"fact_id": fmt.Sprint(i), "decision_digest": "sha256:" + strings.Repeat(fmt.Sprint(i), 64),
			"receipt_digest": "sha256:" + strings.Repeat("9", 64), "source_kind": "document", "source_digest": nil, "text_digest": nil,
			"spans": Array{}, "outcome": "admitted", "codes": Array{}, "data": Object{"field": "color", "value": "private-original"},
			"producer_data": Object{"digest": "sha256:" + strings.Repeat("8", 64), "value": Object{"metadata": "private-metadata"}}})
	}
	return Object{"format": "entelos.evidence/1", "producer": Object{"id": "sample-ledger", "version": "1", "digest_scheme": "sample/1"}, "facts": facts}
}

func eventWorld(t *testing.T, store Store) (*world, string) {
	runner := &fixtureRunner{name: "events", verdicts: map[string]string{}}
	w := newWorld(t, runner)
	w.store = store
	w.context = Object{"actor_kinds": Object{"human": "human", "second": "human", "policy": "policy", "model": "model"}}
	source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "private-source", "reason": "private-reason", "supersedes": Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"case": "one"}}, "origin": "independent"})["plan_ref"]
	proposal := w.must("author", "author", "propose", with(window, Object{"source_ref": source, "base_snapshot": "", "bundle": bundle("events"), "evidence": eventEvidence(), "reason": "private-proposal"}))
	runner.verdicts[proposal["bundle_ref"].(string)] = "passed"
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})
	return w, report["report_ref"].(string)
}

// readAs sends a request with only the event_reader role.
func readAs(h *Host, request any) (any, error) {
	return h.HandleAs("event-reader", []string{"event_reader"}, request)
}

func readEvents(t *testing.T, h *Host, q Object) Object {
	t.Helper()
	result, err := readAs(h, merge(Object{"op": "decision_events", "limit": 256}, q))
	if err != nil {
		t.Fatal(err)
	}
	return result.(Object)
}

func TestDecisionEventsHostCaptureAndResume(t *testing.T) {
	for _, storage := range []string{"memory", "git"} {
		t.Run(storage, func(t *testing.T) {
			var store Store = NewMemoryStore()
			if storage == "git" {
				store = must(OpenGitStore(filepath.Join(t.TempDir(), "events.git")))
			}
			w, report := eventWorld(t, store)
			h := w.host("reviewer", "reviewer", w.runner)
			history := must(store.History(must(store.Read(nil)).Cursor))
			firstRevision := history[len(history)-1]
			if must(store.Read(firstRevision)).State["decision_stream"] == nil {
				t.Fatal("first revision has no stream start")
			}
			marks := Array{Object{"blind": false}, Object{"blind": true, "corrected_value": "green"}, Object{"blind": true}}
			decision := w.must("human", "reviewer", "decide", Object{"report_ref": report, "reason": "private-reason", "fact_reviews": marks})
			page := readEvents(t, h, Object{"limit": 2})
			if page["stream_start"] != firstRevision {
				t.Fatal("wrong saved start")
			}
			if !equal(page, readEvents(t, h, Object{"limit": 2})) {
				t.Fatal("retry differs")
			}
			second := readEvents(t, h, Object{"cursor": page["next_cursor"]})
			if len(second["events"].(Array)) != 1 {
				t.Fatal("resume did not start at third fact")
			}
			end := second["next_cursor"]
			if len(readEvents(t, h, Object{"cursor": end})["events"].(Array)) != 0 {
				t.Fatal("end is not empty")
			}
			w.must("policy", "reviewer", "decide", Object{"report_ref": report, "reason": "policy approval"})
			policy := readEvents(t, h, Object{"cursor": end})
			for _, raw := range policy["events"].(Array) {
				e := raw.(Object)
				if e["actor_kind"] != "policy" || e["blind"] != false {
					t.Fatal("policy provenance lost")
				}
			}
			if len(policy["events"].(Array)) != 3 || policy["stream_start"] != page["stream_start"] {
				t.Fatal("append lost start or events")
			}
			for _, raw := range page["events"].(Array) {
				e := raw.(Object)
				if e["decision_ref"] != decision["decision_ref"] {
					t.Fatal("decision identity changed")
				}
				for k := range e {
					if !withinStrings(k, []string{"format", "cursor", "decision_ref", "disposition", "fact_digest", "evidence_receipt_digest", "actor_kind", "blind", "corrected_value"}) {
						t.Fatal("extra export field", k)
					}
				}
			}
			for _, secret := range []string{"private-proposal", "private-original", "sample-ledger", "private-reason", "private-metadata", "private-source", "\"actor\""} {
				if strings.Contains(string(Encode(page)), secret) {
					t.Fatal("export leaked", secret)
				}
			}
			root := must(store.Read(nil)).Cursor
			h = eventHost(t, store)
			if !equal(page, readEvents(t, h, Object{"limit": 2})) {
				t.Fatal("reopen changed prefix")
			}
			if err := h.StartDecisionEvents(); err != nil {
				t.Fatal(err)
			}
			if must(store.Read(nil)).Cursor != root {
				t.Fatal("restart or read wrote state")
			}
			// A verdict with 3 facts and no marks keeps its positions 7 to 9
			// and gives no events. The read does not stop, and it supplies
			// no mark. The cursor goes through the gap.
			legacy := w.must("second", "reviewer", "decide", Object{"report_ref": report, "reason": "legacy caller without marks"})
			if _, marked := legacy["decision"].(Object)["fact_reviews"]; marked {
				t.Fatal("a decision without marks got marks")
			}
			at := func(p int) string { return strings.TrimSuffix(policy["next_cursor"].(string), "6") + strconv.Itoa(p) }
			gap := readEvents(t, h, Object{"limit": 256, "cursor": policy["next_cursor"]})
			if len(gap["events"].(Array)) != 0 || gap["next_cursor"] != at(9) {
				t.Fatal("an unmarked verdict gave events or lost its positions: ", string(Encode(gap)))
			}
			if inside := readEvents(t, h, Object{"limit": 1, "cursor": at(7)}); len(inside["events"].(Array)) != 0 || inside["next_cursor"] != at(8) {
				t.Fatal("wrong page inside the gap: ", string(Encode(inside)))
			}
			w.must("model", "reviewer", "decide", Object{"report_ref": report, "reason": "model verdict after the unmarked verdict"})
			past := readEvents(t, h, Object{"limit": 256, "cursor": policy["next_cursor"]})
			if len(past["events"].(Array)) != 3 || past["next_cursor"] != at(12) {
				t.Fatal("the read did not continue past the unmarked verdict")
			}
			for i, raw := range past["events"].(Array) {
				e := raw.(Object)
				if e["actor_kind"] != "model" || e["blind"] != false || e["decision_ref"] == legacy["decision_ref"] || e["cursor"] != at(10+i) {
					t.Fatal("wrong event after the unmarked verdict: ", string(Encode(e)))
				}
			}
			// A page that ends in the gap, then a page from inside the gap.
			if ends := readEvents(t, h, Object{"limit": 3, "cursor": at(5)}); len(ends["events"].(Array)) != 1 || ends["next_cursor"] != at(8) {
				t.Fatal("wrong page that ends in the gap: ", string(Encode(ends)))
			}
			if resumed := readEvents(t, h, Object{"limit": 2, "cursor": at(8)}); len(resumed["events"].(Array)) != 1 || resumed["next_cursor"] != at(10) {
				t.Fatal("wrong resume from inside the gap: ", string(Encode(resumed)))
			}
			var err error
			_, err = readAs(h, Object{"op": "decision_events", "limit": 1, "cursor": "bad"})
			if faultCode(t, err) != "invalid_cursor" {
				t.Fatal("source check before cursor")
			}
			for _, role := range []string{"reader", "reviewer"} {
				_, err = h.HandleAs("caller", []string{role}, Object{"op": "decision_events", "limit": 1})
				if faultCode(t, err) != "forbidden" {
					t.Fatal("decision_events accepted the role", role)
				}
			}
			changed := *h
			changed.Context = merge(h.Context, Object{"scope": "other"})
			_, err = readAs(&changed, Object{"op": "decision_events", "limit": 1, "cursor": end})
			if faultCode(t, err) != "invalid_cursor" {
				t.Fatal("cursor crossed scope")
			}
		})
	}
}

func TestDecisionStreamConcurrentStart(t *testing.T) {
	store := NewMemoryStore()
	h := eventHost(t, store)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- h.StartDecisionEvents() }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	history := must(store.History(must(store.Read(nil)).Cursor))
	if len(history) != 1 {
		t.Fatal("start committed more than once")
	}
	page := readEvents(t, h, Object{})
	if page["stream_start"] != history[0] || len(page["events"].(Array)) != 0 {
		t.Fatal("empty stream")
	}
}

// Both writers stage a verdict at the same revision. Only one publishes.
// The loser retries at the new root. Export must retain that durable order.
func TestDecisionEventsConcurrentWrites(t *testing.T) {
	store := NewMemoryStore()
	w, report := eventWorld(t, store)
	root := must(store.Read(nil)).Cursor
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	store.BeforePublish = func() error { ready <- struct{}{}; <-release; return nil }
	type answer struct {
		actor  string
		result any
		err    error
	}
	answers := make(chan answer, 2)
	request := func(actor, revision string) Object {
		return Object{"op": "decide", "report_ref": report, "reason": "concurrent review", "expected_revision": revision, "request_key": "concurrent-" + actor,
			"fact_reviews": Array{Object{"blind": false}, Object{"blind": true}, Object{"blind": false}}}
	}
	for _, actor := range []string{"human", "second"} {
		go func(actor string) {
			result, err := w.host(actor, "reviewer", w.runner).Handle(request(actor, root))
			answers <- answer{actor, result, err}
		}(actor)
	}
	<-ready
	<-ready
	close(release)
	a, b := <-answers, <-answers
	store.BeforePublish = nil
	if a.err != nil {
		a, b = b, a
	}
	if a.err != nil || faultCode(t, b.err) != "stale_revision" {
		t.Fatal("CAS did not choose one writer", a.err, b.err)
	}
	later, err := w.host(b.actor, "reviewer", w.runner).Handle(request(b.actor, must(store.Read(nil)).Cursor))
	if err != nil {
		t.Fatal(err)
	}
	page := readEvents(t, eventHost(t, store), Object{})
	events := page["events"].(Array)
	if len(events) != 6 {
		t.Fatal("CAS lost or repeated events")
	}
	for i, ref := range []any{a.result.(Object)["result"].(Object)["decision_ref"], later.(Object)["result"].(Object)["decision_ref"]} {
		for j := 0; j < 3; j++ {
			if events[i*3+j].(Object)["decision_ref"] != ref {
				t.Fatal("export differs from committed order")
			}
		}
	}
}

type eventReadHook struct {
	Store
	afterRead func()
}

func (s *eventReadHook) Read(cursor any) (*View, error) {
	v, err := s.Store.Read(cursor)
	if cursor == nil && s.afterRead != nil {
		f := s.afterRead
		s.afterRead = nil
		f()
	}
	return v, err
}

func TestDecisionEventsPinsReadPrefix(t *testing.T) {
	store := NewMemoryStore()
	w, report := eventWorld(t, store)
	marks := Array{Object{"blind": true}, Object{"blind": false, "corrected_value": nil}, Object{"blind": false, "corrected_value": ""}}
	w.must("human", "reviewer", "decide", Object{"report_ref": report, "reason": "review", "fact_reviews": marks})
	h := eventHost(t, store)
	first := readEvents(t, h, Object{})
	hook := &eventReadHook{Store: store, afterRead: func() { w.must("model", "reviewer", "decide", Object{"report_ref": report, "reason": "model review"}) }}
	h.Store = hook
	pinned := readEvents(t, h, Object{})
	if !equal(pinned, first) {
		t.Fatal("append changed an in-flight page")
	}
	next := readEvents(t, h, Object{"cursor": pinned["next_cursor"]})
	if len(next["events"].(Array)) != 3 {
		t.Fatal("next page did not see append")
	}
	for _, raw := range next["events"].(Array) {
		e := raw.(Object)
		if e["actor_kind"] != "model" || e["blind"] != false {
			t.Fatal("model kind was inferred from current actor")
		}
	}
}

// A fact-free decision between two decisions with facts uses no position.
// A page that starts before it and ends after it reads across it.
func TestDecisionEventsFactFreeDecisionInPage(t *testing.T) {
	eachStore(t, func(t *testing.T, store Store) {
		w, report := eventWorld(t, store)
		h := w.host("reviewer", "reviewer", w.runner)
		marks := Array{Object{"blind": false}, Object{"blind": true}, Object{"blind": false}}
		first := w.must("human", "reviewer", "decide", Object{"report_ref": report, "reason": "with facts", "fact_reviews": marks})
		source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "free-source", "reason": "fact-free", "supersedes": Array{}}))["source_ref"]
		plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"case": "one"}}, "origin": "independent"})["plan_ref"]
		proposal := w.must("author", "author", "propose", with(window, Object{"source_ref": source, "base_snapshot": get(must(store.Read(nil)), report, "qualification")["snapshot_ref"], "bundle": bundle("free"), "reason": "no evidence"}))
		w.runner.(*fixtureRunner).verdicts[proposal["bundle_ref"].(string)] = "passed"
		free := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan})["report_ref"]
		freeDecision := w.must("human", "reviewer", "decide", Object{"report_ref": free, "reason": "no marks"})
		if _, has := get(must(store.Read(nil)), freeDecision["decision_ref"].(string), "decision")["fact_reviews"]; has {
			t.Fatal("the fact-free decision has fact_reviews")
		}
		next := w.must("author", "author", "propose", with(window, Object{"source_ref": source, "base_snapshot": proposal["snapshot_ref"], "bundle": bundle("next"), "evidence": eventEvidence(), "reason": "with evidence"}))
		w.runner.(*fixtureRunner).verdicts[next["bundle_ref"].(string)] = "passed"
		nextReport := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": next["snapshot_ref"], "plan_ref": plan})["report_ref"]
		last := w.must("policy", "reviewer", "decide", Object{"report_ref": nextReport, "reason": "policy approval"})
		start := readEvents(t, h, Object{"limit": 2})
		page := readEvents(t, h, Object{"cursor": start["next_cursor"], "limit": 2})
		events := page["events"].(Array)
		if len(events) != 2 || events[0].(Object)["decision_ref"] != first["decision_ref"] || events[1].(Object)["decision_ref"] != last["decision_ref"] {
			t.Fatal("the page did not read across the fact-free decision:", Encode(page))
		}
		if all := readEvents(t, h, Object{}); len(all["events"].(Array)) != 6 {
			t.Fatal("the full read lost events")
		}
	})
}

// The event_reader role permits decision_events and no other operation.
func TestEventReaderRoleReadsOnlyEvents(t *testing.T) {
	ops := []string{}
	count := 0
	for _, table := range []map[string]struct {
		role   string
		fields []string
	}{schema, hostOnlyOps} {
		for op, rule := range table {
			count++
			if op != "decision_events" {
				ops = append(ops, op)
			}
			if rule.role == "event_reader" && op != "decision_events" {
				t.Fatal("another operation uses the event_reader role:", op)
			}
		}
	}
	if len(ops) != count-1 || schema["decision_events"].role != "event_reader" {
		t.Fatal("decision_events does not use the event_reader role")
	}
	h := eventHost(t, NewMemoryStore())
	for _, op := range ops {
		_, err := readAs(h, Object{"op": op})
		if faultCode(t, err) != "forbidden" {
			t.Fatal("the event_reader role reached", op, err)
		}
	}
}
