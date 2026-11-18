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
			principals := lifecycle.Object{"reviewer": lifecycle.Object{"actor": "reviewer", "roles": lifecycle.Array{"reviewer"}}, "reader": lifecycle.Object{"actor": "reader", "roles": lifecycle.Array{"reader"}}}
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
				if got := request(version, "reviewer", "decision_events", lifecycle.Object{"limit": 1}); got["status"] != "invalid_request" {
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
			page := request("1.2", "reviewer", "decision_events", lifecycle.Object{"limit": 1})
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
			direct, err := host.HandleAs("reviewer", []string{"reviewer"}, lifecycle.Object{"op": "decision_events", "limit": 1})
			if err != nil || !sameValue(direct, out) {
				t.Fatal("transport differs from Host", err)
			}
			for _, principal := range []string{"reviewer", "reader"} {
				got := request("1.2", principal, "decision_events", lifecycle.Object{"cursor": out["next_cursor"], "limit": 1})
				if principal == "reader" {
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
