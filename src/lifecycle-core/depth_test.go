package lifecycle

import (
	"bytes"
	"testing"
)

func nestedArrays(depth int) any {
	var value any = Int{"0"}
	for i := 0; i < depth; i++ {
		value = Array{value}
	}
	return value
}

// SEC-01: the request fits the decoder limit, but storing its note in the
// task and the retained result adds enough nesting to make the root unreadable.
func TestCheckpointRejectsDeepNote(t *testing.T) {
	eachStore(t, func(t *testing.T, s Store) {
		seed, err := s.Transact("", "seed", Object{}, func(v *View) (any, error) {
			v.State["tasks"].(Object)["task"] = Object{"function": "record.visible", "scope": "demo",
				"owner": "maintainer", "token": "fence", "checkpoint": "1", "status": "working", "note": ""}
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		before := root(t, s)
		h, err := NewHost(s, nil, Object{"actor": "maintainer", "roles": Array{"maintainer"}, "scope": "demo",
			"runner_actor": "runner", "worker": "worker", "functions": Array{"record.visible"}})
		if err != nil {
			t.Fatal(err)
		}
		q := Object{"op": "checkpoint", "expected_revision": seed["cursor"], "request_key": "checkpoint",
			"task_id": "task", "checkpoint": "1", "token": "fence", "status": "working", "note": nestedArrays(997)}
		request, err := Decode(Encode(q))
		if err != nil {
			t.Fatalf("the request must fit the decoder limit: %v", err)
		}
		_, err = h.Handle(request)
		if code(err) != "invalid_request" {
			t.Errorf("checkpoint error = %v, want invalid_request", err)
		}
		after, err := s.Read(nil)
		if err != nil {
			t.Errorf("current root is unreadable after refusal: %v", err)
		} else if after.Cursor != before.Cursor || !equal(after.State, before.State) {
			t.Error("refused checkpoint changed the root or state")
		}
		// A refusal must leave both the fence and request key usable.
		q["note"] = "still working"
		if _, err := h.Handle(q); err != nil {
			t.Fatalf("valid checkpoint after refusal: %v", err)
		}
	})
}

func TestStoresRejectUnreadableState(t *testing.T) {
	eachStore(t, func(t *testing.T, s Store) {
		for _, where := range []string{"state", "result"} {
			t.Run(where, func(t *testing.T) {
				before := root(t, s)
				_, err := s.Transact(before.Cursor, where, Object{}, func(v *View) (any, error) {
					if where == "state" {
						v.State["too_deep"] = nestedArrays(999)
						return nil, nil
					}
					return nestedArrays(997), nil
				})
				if code(err) != "invalid_record" {
					t.Errorf("transaction error = %v, want invalid_record", err)
				}
				after := root(t, s)
				if after.Cursor != before.Cursor || !equal(after.State, before.State) {
					t.Fatal("refused state changed the root or state")
				}
				write(t, s, before.Cursor, where, "valid")
			})
		}
	})
}

func TestStoresAcceptReadableDepthBoundary(t *testing.T) {
	eachStore(t, func(t *testing.T, s Store) {
		_, err := s.Transact("", "boundary", Object{}, func(v *View) (any, error) {
			v.State["boundary"] = nestedArrays(998)
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(Encode(root(t, s).State["boundary"]), Encode(nestedArrays(998))) {
			t.Fatal("the deepest readable state value changed")
		}
	})
}

func TestHostRejectsStoredFieldTypes(t *testing.T) {
	eachStore(t, func(t *testing.T, s Store) {
		w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}})
		w.store = s
		snapshot, plan, bundleRef := w.candidate("pass")
		snapshotRecord, err := root(t, s).Get(snapshot, "snapshot")
		if err != nil {
			t.Fatal(err)
		}
		source := snapshotRecord["source_ref"]
		report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
		decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "checked"})["decision_ref"]
		event := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})["event_ref"]
		w.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
		receipt := w.must("caller", "caller", "invoke", Object{"function": "record.visible", "date": "2026-11-15",
			"expected_bundle_ref": bundleRef, "inputs": Object{"state": "ready"}})["receipt_ref"]
		task := w.must("observer", "observer", "observe", Object{"receipt_ref": receipt, "route": "source", "message": "check this"})["task_id"]
		claimed := w.must("maintainer", "maintainer", "claim", Object{"task_id": task, "checkpoint": "0"})
		checkpointFields := Object{"task_id": task, "checkpoint": claimed["checkpoint"], "token": claimed["token"],
			"status": "working", "note": "", "correction_snapshot": ""}
		before := root(t, s)
		cases := []struct {
			op, role string
			fields   Object
			names    []string
		}{
			{"source", "reviewer", with(window, Object{"text": "source", "reason": "checked", "supersedes": Array{}}), []string{"text", "reason", "supersedes"}},
			{"plan", "reviewer", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}}, []string{"origin", "supersedes"}},
			{"propose", "author", with(window, Object{"base_snapshot": snapshot, "source_ref": source, "bundle": bundle("pass"), "reason": "checked"}), []string{"reason"}},
			{"decide", "reviewer", Object{"report_ref": report, "reason": "checked"}, []string{"reason"}},
			{"activate", "reviewer", Object{"decision_ref": decision, "targets": Array{event}}, []string{"targets"}},
			{"suspend", "reviewer", with(window, Object{"targets": Array{event}, "reason": "checked"}), []string{"reason", "targets"}},
			{"replay", "reader", Object{"receipt_ref": receipt, "reason": "checked"}, []string{"reason"}},
			{"restate", "reader", Object{"receipt_ref": receipt, "cursor": before.Cursor, "reason": "checked"}, []string{"reason"}},
			{"observe", "observer", Object{"receipt_ref": receipt, "route": "source", "message": "check this"}, []string{"message"}},
			{"claim", "maintainer", Object{"task_id": task, "checkpoint": claimed["checkpoint"]}, []string{"task_id", "checkpoint"}},
			{"checkpoint", "maintainer", checkpointFields, []string{"task_id", "checkpoint", "token", "status", "note", "correction_snapshot"}},
			{"takeover", "reviewer", Object{"task_id": task, "checkpoint": claimed["checkpoint"], "owner": "other", "reason": "checked"}, []string{"task_id", "checkpoint", "owner", "reason"}},
		}
		for _, tc := range cases {
			for _, name := range tc.names {
				t.Run(tc.op+"/"+name, func(t *testing.T) {
					invalid := []any{nil, false, Int{"1"}, Array{}, Object{}, nestedArrays(997)}
					if name == "targets" || name == "supersedes" {
						invalid = []any{nil, false, Int{"1"}, "", Object{}, Array{Int{"1"}}, Array{Array{}}}
					}
					for _, value := range invalid {
						q := with(tc.fields, Object{"op": tc.op, "expected_revision": before.Cursor, "request_key": "bad-field", name: value})
						if _, err := w.host(tc.role, tc.role, w.runner).Handle(q); code(err) != "invalid_request" {
							t.Fatalf("%s type %T: got %v, want invalid_request", name, value, err)
						}
						after := root(t, s)
						if after.Cursor != before.Cursor || !equal(after.State, before.State) {
							t.Fatal("refused field changed the root or state")
						}
					}
				})
			}
		}
		// Empty optional text stays valid. The same task fence still works.
		checked := w.must("maintainer", "maintainer", "checkpoint", checkpointFields)
		w.must("reviewer", "reviewer", "takeover", Object{"task_id": task, "checkpoint": checked["checkpoint"], "owner": "other", "reason": "checked"})
	})
}
