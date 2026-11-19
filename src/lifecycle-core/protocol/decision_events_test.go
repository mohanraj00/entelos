package protocol

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

func TestDecisionEventsBindings(t *testing.T) {
	for _, transport := range transports {
		t.Run(transport, func(t *testing.T) {
			inTempDir(t)
			dir, _ := os.Getwd()
			store := lifecycle.NewMemoryStore()
			host, err := lifecycle.NewHost(store, nil, lifecycle.Object{"scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": lifecycle.Array{"record.visible"}, "actor_kinds": lifecycle.Object{"reviewer": "human"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := host.StartDecisionEvents(); err != nil {
				t.Fatal(err)
			}
			principals := lifecycle.Object{"reviewer": lifecycle.Object{"actor": "reviewer", "roles": lifecycle.Array{"reviewer"}}, "reader": lifecycle.Object{"actor": "reader", "roles": lifecycle.Array{"reader"}},
				"events": lifecycle.Object{"actor": "events", "roles": lifecycle.Array{"event_reader"}}}
			schemas := loadTestSchemas(t)
			send := pageTransport(t, transport, host, schemas, principals, filepath.Join(dir, "events"))
			// A failed report is enough for a reject. This tests the real wire
			// decide path, including provenance capture, without a CEL runtime.
			root, _ := store.Read(nil)
			var report string
			_, err = store.Transact(root.Cursor, "fixture", lifecycle.Object{}, func(v *lifecycle.View) (any, error) {
				snapshot := v.Put("snapshot", lifecycle.Object{"scope": "demo", "function": "record.visible", "from": "2026-11-01", "until": "2026-12-01", "producer": "author", "bundle_ref": tokenA,
					"evidence": lifecycle.Object{"facts": lifecycle.Array{lifecycle.Object{"decision_digest": "sha256:" + tokenA, "receipt_digest": "sha256:" + tokenB}}}})
				report = v.Put("qualification", lifecycle.Object{"scope": "demo", "function": "record.visible", "snapshot_ref": snapshot, "runner": "runner"})
				v.State["reviews"] = lifecycle.Object{}
				return lifecycle.Object{}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			request := func(version, principal, op string, body lifecycle.Object) lifecycle.Object {
				q := lifecycle.Object{"protocol": version, "request_id": "events", "operation": op, "body": body}
				return send(principal, lifecycle.Encode(q))
			}
			for _, version := range []string{"1.0", "1.1"} {
				if got := request(version, "events", "decision_events", lifecycle.Object{"limit": 1}); got["status"] != "invalid_request" {
					t.Fatal("old version accepted read", got)
				}
				if got := request(version, "reviewer", "decide", lifecycle.Object{"report_ref": "sha256:" + tokenA, "reason": "review", "disposition": "reject", "fact_reviews": lifecycle.Array{}, "expected_revision": "", "request_key": "old"}); got["status"] != "invalid_request" || at(got, "diagnostic", "stage") != "protocol" {
					t.Fatal("old version accepted marks", got)
				}
			}
			root, _ = store.Read(nil)
			body := lifecycle.Object{"report_ref": report, "reason": "reject synthetic facts", "disposition": "reject", "fact_reviews": lifecycle.Array{lifecycle.Object{"blind": true}}, "expected_revision": root.Cursor, "request_key": "decide"}
			// Malformed and semantically invalid marks never commit.
			for i, marks := range []lifecycle.Array{nil, {}, {lifecycle.Object{"blind": "yes"}}, {lifecycle.Object{"blind": true, "corrected_value": "green"}}, {lifecycle.Object{"blind": true, "metadata": "extra"}}} {
				bad := lifecycle.Object{}
				for k, v := range body {
					bad[k] = v
				}
				bad["fact_reviews"] = marks
				bad["request_key"] = fmt.Sprint(i)
				if got := request("1.2", "reviewer", "decide", bad); got["status"] != "invalid_request" {
					t.Fatal("bad marks accepted", got)
				}
				after, _ := store.Read(nil)
				if after.Cursor != root.Cursor {
					t.Fatal("refusal wrote a revision")
				}
			}
			if got := request("1.2", "reviewer", "decide", body); got["status"] != "ok" {
				t.Fatal(got)
			}
			root, _ = store.Read(nil)
			page := request("1.2", "events", "decision_events", lifecycle.Object{"limit": 1})
			if page["status"] != "ok" {
				t.Fatal(page)
			}
			out := page["result"].(lifecycle.Object)
			events := out["events"].(lifecycle.Array)
			if len(events) != 1 || at(events[0], "actor_kind") != "human" || at(events[0], "blind") != true || at(events[0], "disposition") != "reject" {
				t.Fatal(page)
			}
			if errs := shape(schemas, normal(t, page), "decision_events"); len(errs) > 0 {
				t.Fatal(errs)
			}
			direct, err := host.HandleAs("events", []string{"event_reader"}, lifecycle.Object{"op": "decision_events", "limit": 1})
			if err != nil || !sameValue(direct, out) {
				t.Fatal("transport differs from Host", err)
			}
			for _, principal := range []string{"events", "reader", "reviewer"} {
				got := request("1.2", principal, "decision_events", lifecycle.Object{"cursor": out["next_cursor"], "limit": 1})
				if principal != "events" {
					if got["status"] != "forbidden" {
						t.Fatal("resume skipped authorization")
					}
				} else if got["status"] != "ok" || len(at(got, "result", "events").(lifecycle.Array)) != 0 {
					t.Fatal("bad resume", got)
				}
			}
			after, _ := store.Read(nil)
			if after.Cursor != root.Cursor {
				t.Fatal("read changed state")
			}
		})
	}
}

// Issue #197. A reviewer records a verdict with one cited fact and no blind
// marks through protocol 1.1, after the stream start. The v0.5.0 read then
// gave unexportable_decision for each page with this verdict. The read now
// leaves the unmarked verdict out and continues from the saved cursor.
func TestDecisionEventsReadPastUnmarkedVerdict(t *testing.T) {
	for _, transport := range transports {
		t.Run(transport, func(t *testing.T) {
			inTempDir(t)
			dir, _ := os.Getwd()
			store := lifecycle.NewMemoryStore()
			host, err := lifecycle.NewHost(store, nil, lifecycle.Object{"scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": lifecycle.Array{"record.visible"},
				"actor_kinds": lifecycle.Object{"reviewer": "human", "legacy": "human", "model": "model"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := host.StartDecisionEvents(); err != nil {
				t.Fatal(err)
			}
			principals := lifecycle.Object{"events": lifecycle.Object{"actor": "events", "roles": lifecycle.Array{"event_reader"}}}
			for _, actor := range []string{"reviewer", "legacy", "nokind", "model"} {
				principals[actor] = lifecycle.Object{"actor": actor, "roles": lifecycle.Array{"reviewer"}}
			}
			schemas := loadTestSchemas(t)
			send := pageTransport(t, transport, host, schemas, principals, filepath.Join(dir, "events"))
			root, _ := store.Read(nil)
			var report string
			_, err = store.Transact(root.Cursor, "fixture", lifecycle.Object{}, func(v *lifecycle.View) (any, error) {
				snapshot := v.Put("snapshot", lifecycle.Object{"scope": "demo", "function": "record.visible", "from": "2026-11-01", "until": "2026-12-01", "producer": "author", "bundle_ref": tokenA,
					"evidence": lifecycle.Object{"facts": lifecycle.Array{lifecycle.Object{"decision_digest": "sha256:" + tokenA, "receipt_digest": "sha256:" + tokenB}}}})
				report = v.Put("qualification", lifecycle.Object{"scope": "demo", "function": "record.visible", "snapshot_ref": snapshot, "runner": "runner"})
				v.State["reviews"] = lifecycle.Object{}
				return lifecycle.Object{}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			request := func(version, principal, op string, body lifecycle.Object) lifecycle.Object {
				return send(principal, lifecycle.Encode(lifecycle.Object{"protocol": version, "request_id": "events", "operation": op, "body": body}))
			}
			// decide records a rejection and gives the decision record.
			decide := func(version, principal string, marks lifecycle.Array) lifecycle.Object {
				t.Helper()
				root, _ := store.Read(nil)
				body := lifecycle.Object{"report_ref": report, "reason": "reject synthetic facts", "disposition": "reject", "expected_revision": root.Cursor, "request_key": principal}
				if marks != nil {
					body["fact_reviews"] = marks
				}
				got := request(version, principal, "decide", body)
				if got["status"] != "ok" {
					t.Fatal(principal, got)
				}
				return at(got, "result", "result", "decision").(lifecycle.Object)
			}
			read := func(body lifecycle.Object) lifecycle.Object {
				t.Helper()
				page := request("1.2", "events", "decision_events", body)
				if page["status"] != "ok" {
					t.Fatal(page)
				}
				if errs := shape(schemas, normal(t, page), "decision_events"); len(errs) > 0 {
					t.Fatal(errs)
				}
				out := page["result"].(lifecycle.Object)
				direct, err := host.HandleAs("events", []string{"event_reader"}, merged(lifecycle.Object{"op": "decision_events"}, body))
				if err != nil || !sameValue(direct, out) {
					t.Fatal("transport differs from Host", err)
				}
				return out
			}

			decide("1.2", "reviewer", lifecycle.Array{lifecycle.Object{"blind": true}})
			first := read(lifecycle.Object{"limit": 1})
			saved := first["next_cursor"]
			if len(first["events"].(lifecycle.Array)) != 1 {
				t.Fatal(first)
			}

			// The two unmarked verdicts, each through protocol 1.1: a human
			// actor, and an actor with no recorded kind. Protocol 1.0 has no
			// rejection, and this fixture report permits only a rejection.
			for _, principal := range []string{"legacy", "nokind"} {
				record := decide("1.1", principal, nil)
				if _, marked := record["fact_reviews"]; marked {
					t.Fatal("the Host supplied marks for", principal)
				}
				if kind, has := record["kind"]; principal == "nokind" && has {
					t.Fatal("the Host supplied a kind:", kind)
				}
			}
			// The reader that stopped here on v0.5.0 sends its saved cursor.
			stopped := read(lifecycle.Object{"cursor": saved, "limit": 256})
			if len(stopped["events"].(lifecycle.Array)) != 0 || stopped["next_cursor"] != saved || stopped["stream_start"] != first["stream_start"] {
				t.Fatal("an unmarked verdict gave events, consumed positions or changed the stream", stopped)
			}

			if marks, ok := decide("1.1", "model", nil)["fact_reviews"].(lifecycle.Array); !ok || len(marks) != 1 {
				t.Fatal("the model verdict has no marks")
			}
			past := read(lifecycle.Object{"cursor": saved, "limit": 256})
			events := past["events"].(lifecycle.Array)
			if len(events) != 1 || at(events[0], "actor_kind") != "model" || at(events[0], "blind") != false || at(events[0], "disposition") != "reject" {
				t.Fatal("the read did not continue past the unmarked verdicts", past)
			}
			namespace := saved.(string)[:len(saved.(string))-1]
			if saved != namespace+"1" || at(events[0], "cursor") != namespace+"2" || past["next_cursor"] != namespace+"2" {
				t.Fatal("the positions have a gap or a duplicate", saved, past)
			}
			// A whole read from the start gives the same two events.
			all := read(lifecycle.Object{"limit": 256})
			if len(all["events"].(lifecycle.Array)) != 2 || !sameValue(all["events"].(lifecycle.Array)[1], events[0]) || !sameValue(all["events"].(lifecycle.Array)[0], first["events"].(lifecycle.Array)[0]) {
				t.Fatal("the first read and the resumed read differ", all)
			}
			if got := request("1.2", "events", "decision_events", lifecycle.Object{"cursor": namespace + "3", "limit": 1}); got["status"] != "invalid_cursor" {
				t.Fatal("an unmarked verdict consumed a position", got)
			}
		})
	}
}

func merged(a, b lifecycle.Object) lifecycle.Object {
	out := lifecycle.Object{}
	for _, m := range []lifecycle.Object{a, b} {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
