package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

// This runtime describes fixture bytes; it never qualifies a proposal.
type evidenceRuntime struct{}

func (evidenceRuntime) Build() (string, error) { return "fixture", nil }
func (evidenceRuntime) Execute(_ map[string][]byte, command string, _ []byte, _ any) (any, error) {
	if command != "describe" {
		panic("unexpected fixture command")
	}
	return lifecycle.Object{"function_id": "record.visible", "bundle_ref": "fixture-bundle", "implementation_ref": "fixture"}, nil
}

func TestJSONLEvidence(t *testing.T) {
	raw, err := os.ReadFile("../../../protocol/approval/vectors/approval-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	original := doc.(lifecycle.Object)["steps"].(lifecycle.Array)[3].(lifecycle.Object)["request"].(lifecycle.Object)["body"].(lifecycle.Object)["evidence"]
	for _, name := range []string{"valid", "too-deep", "too-large", "missing-facts", "missing-fact-data", "wrong-producer-type", "wrong-span-type"} {
		t.Run(name, func(t *testing.T) {
			evidence, err := lifecycle.Decode(lifecycle.Encode(original))
			if err != nil {
				t.Fatal(err)
			}
			e := evidence.(lifecycle.Object)
			switch name {
			case "too-deep":
				var nested any = "leaf"
				for i := 0; i < 32; i++ {
					nested = lifecycle.Array{nested}
				}
				e["producer"].(lifecycle.Object)["producer_data"] = lifecycle.Object{"digest": "fixture", "value": lifecycle.Object{"nested": nested}}
			case "too-large":
				e["facts"].(lifecycle.Array)[0].(lifecycle.Object)["data"].(lifecycle.Object)["value"] = strings.Repeat("x", 256*1024)
			case "missing-facts":
				delete(e, "facts")
			case "missing-fact-data":
				delete(e["facts"].(lifecycle.Array)[0].(lifecycle.Object), "data")
			case "wrong-producer-type":
				e["producer"] = "producer"
			case "wrong-span-type":
				e["facts"].(lifecycle.Array)[0].(lifecycle.Object)["spans"].(lifecycle.Array)[0].(lifecycle.Object)["start"] = true
			}
			store := lifecycle.NewMemoryStore()
			host, err := lifecycle.NewHost(store, evidenceRuntime{}, lifecycle.Object{"actor": "author", "roles": lifecycle.Array{"author", "reviewer"}, "scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": lifecycle.Array{"record.visible"}})
			if err != nil {
				t.Fatal(err)
			}
			source, err := host.Handle(lifecycle.Object{"op": "source", "function": "record.visible", "from": "2026-10-01", "until": "2026-12-01", "text": "invented source", "reason": "fixture", "expected_revision": "", "request_key": "source"})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Read(nil)
			if err != nil {
				t.Fatal(err)
			}
			q := lifecycle.Object{"op": "propose", "function": "record.visible", "from": "2026-10-01", "until": "2026-12-01", "base_snapshot": "", "source_ref": source.(lifecycle.Object)["result"].(lifecycle.Object)["source_ref"], "bundle": lifecycle.Object{"manifest": "{}", "environment": "{}", "source": "fixture", "fixed": "{}"}, "reason": "fixture", "evidence": e, "expected_revision": before.Cursor, "request_key": "propose"}
			var out bytes.Buffer
			if exit := serve(host, bytes.NewReader(lifecycle.Encode(q)), &out); exit != 0 {
				t.Fatalf("serve = %d: %s", exit, out.Bytes())
			}
			response, err := lifecycle.Decode(out.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			result := response.(lifecycle.Object)
			if name == "valid" {
				if result["status"] != "ok" {
					t.Fatalf("%s", out.Bytes())
				}
				ref := result["data"].(lifecycle.Object)["result"].(lifecycle.Object)["snapshot_ref"]
				out.Reset()
				if serve(host, bytes.NewReader(lifecycle.Encode(lifecycle.Object{"op": "proposal", "snapshot_ref": ref})), &out) != 0 {
					t.Fatal("read failed")
				}
				response, err = lifecycle.Decode(out.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				retained := response.(lifecycle.Object)["data"].(lifecycle.Object)["evidence"]
				if !bytes.Equal(lifecycle.Encode(original), lifecycle.Encode(retained)) {
					t.Fatal("evidence changed")
				}
			} else {
				if result["status"] != "invalid_request" {
					t.Fatalf("%s", out.Bytes())
				}
				after, err := store.Read(nil)
				if err != nil || after.Cursor != before.Cursor {
					t.Fatalf("refusal changed store: %v", err)
				}
				// The same key and revision remain usable after refusal.
				q["evidence"] = original
				out.Reset()
				if serve(host, bytes.NewReader(lifecycle.Encode(q)), &out) != 0 {
					t.Fatal("retry failed")
				}
				response, err = lifecycle.Decode(out.Bytes())
				if err != nil || response.(lifecycle.Object)["status"] != "ok" {
					t.Fatalf("valid retry: %s (%v)", out.Bytes(), err)
				}
			}
		})
	}
}

func TestJSONLDecisionEvents(t *testing.T) {
	store := lifecycle.NewMemoryStore()
	host, err := lifecycle.NewHost(store, nil, lifecycle.Object{"actor": "events", "roles": lifecycle.Array{"event_reader"}, "scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": lifecycle.Array{"record.visible"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.StartDecisionEvents(); err != nil {
		t.Fatal(err)
	}
	request := lifecycle.Object{"op": "decision_events", "limit": 1}
	expected, err := host.Handle(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if exit := serve(host, bytes.NewReader(lifecycle.Encode(request)), &out); exit != 0 {
		t.Fatal(exit)
	}
	response, err := lifecycle.Decode(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lifecycle.Encode(response.(lifecycle.Object)["data"]), lifecycle.Encode(expected)) {
		t.Fatal("JSONL changed the Host page")
	}
}
