package protocol

// The content workflow of protocol 1.3 (#213): one content proposal goes
// from propose to activate through the Unix socket and through the HTTP
// binding. An application installs its own qualification runner for
// content, so this test installs a stub runner. The expected values come
// from CONTENT.md and PROTOCOL.md: the candidate ref is the SHA-256 digest of
// the exact manifest text, computed here and not read from the Host.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

const wireSubject = "content:loan-rules/region-a"

var wireManifest = `{"format":"entelos.content/1","collection":"loan-rules","partition":"region-a","base":"",` +
	`"items":[{"item_id":"rules-de","digest":"sha256:` + fmt.Sprintf("%064d", 0) + `","size":200,"applicability":{"region":["eu","de"]}}]}`

// wireRunner passes a candidate only if the Host gives it the exact
// manifest text, and nothing else.
type wireRunner struct{ ref string }

func (r wireRunner) identity() string {
	sum := sha256.Sum256([]byte("wire-content-runner"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (r wireRunner) Identity() (string, error) { return r.identity(), nil }

func (r wireRunner) Expected(lifecycle.Candidate) string { return r.identity() }

func (r wireRunner) Qualify(in lifecycle.QualificationInput) (lifecycle.Verdict, error) {
	if len(in.Files) != 1 || string(in.Files["content"]) != wireManifest || in.Candidate.BundleRef != r.ref {
		return lifecycle.Verdict{}, lifecycle.NewFault("runner_unavailable", "the files are not the manifest of the candidate", "runner")
	}
	return lifecycle.Verdict{Outcome: lifecycle.Object{"status": "passed", "bundle_ref": r.ref, "implementation_ref": r.identity()}}, nil
}

func TestContentWorkflowOnTheWire(t *testing.T) {
	sum := sha256.Sum256([]byte(wireManifest))
	ref := "sha256:" + hex.EncodeToString(sum[:])
	s := loadTestSchemas(t)
	principals := lifecycle.Object{
		"author":   lifecycle.Object{"actor": "author-a", "roles": lifecycle.Array{"author"}},
		"reviewer": lifecycle.Object{"actor": "reviewer-b", "roles": lifecycle.Array{"reviewer", "reader"}},
		"runner":   lifecycle.Object{"actor": "fixture-runner", "roles": lifecycle.Array{"runner"}},
	}
	for _, transport := range []string{"socket", "http-token", "http-claim"} {
		t.Run(transport, func(t *testing.T) {
			inTempDir(t)
			dir, _ := os.Getwd()
			store, err := lifecycle.OpenGitStore(filepath.Join(dir, "store.git"))
			if err != nil {
				t.Fatal(err)
			}
			host, err := lifecycle.NewHost(store, noRuntime{}, lifecycle.Object{"actor": "author-a", "roles": lifecycle.Array{"author"},
				"scope": "demo", "functions": lifecycle.Array{wireSubject}, "runner_actor": "fixture-runner", "worker": "local-worker"})
			if err != nil {
				t.Fatal(err)
			}
			host.Runner = wireRunner{ref}
			send := serveTransport(t, transport, host, s, principals, "demo", "content.sock", filepath.Join(dir, transport))
			cursor := ""
			n := 0
			call := func(principal, version, op string, body lifecycle.Object) lifecycle.Object {
				t.Helper()
				n++
				if !lifecycle.Reads[op] {
					body["expected_revision"], body["request_key"] = cursor, fmt.Sprintf("k-%d", n)
				}
				request := lifecycle.Object{"protocol": version, "request_id": fmt.Sprintf("w-%d", n), "operation": op, "body": body}
				payload, err := Canonical(request)
				if err != nil {
					t.Fatal(err)
				}
				response := normal(t, send(principal, payload)).(lifecycle.Object)
				if response["protocol"] != version {
					t.Fatalf("%s: response version %v, want %s", op, response["protocol"], version)
				}
				for _, e := range shape(s, response, op) {
					t.Errorf("%s: %s", op, e)
				}
				if response["status"] == "ok" && !lifecycle.Reads[op] {
					cursor = response["result"].(lifecycle.Object)["cursor"].(string)
				}
				return response
			}
			result := func(response lifecycle.Object) lifecycle.Object {
				t.Helper()
				if response["status"] != "ok" {
					t.Fatalf("refused: %s", lifecycle.Encode(response))
				}
				out := response["result"].(lifecycle.Object)
				if inner, ok := out["result"].(lifecycle.Object); ok {
					return inner
				}
				return out
			}
			refusal := func(response lifecycle.Object, code, stage string) {
				t.Helper()
				d, _ := response["diagnostic"].(lifecycle.Object)
				if response["status"] != code || d["code"] != code || d["stage"] != stage {
					t.Fatalf("got %s, want %s at stage %s", lifecycle.Encode(response), code, stage)
				}
			}
			window := lifecycle.Object{"from": "2026-01-01", "until": "2099-01-01"}
			with := func(extra lifecycle.Object) lifecycle.Object {
				out := lifecycle.Object{}
				for _, m := range []lifecycle.Object{window, extra} {
					for k, v := range m {
						out[k] = v
					}
				}
				return out
			}
			source := result(call("reviewer", "1.3", "source", with(lifecycle.Object{"function": wireSubject, "text": "The content rules.",
				"reason": "first", "supersedes": lifecycle.Array{}})))["source_ref"]
			plan := result(call("reviewer", "1.3", "plan", lifecycle.Object{"source_ref": source, "cases": lifecycle.Array{lifecycle.Object{"name": "limits"}},
				"origin": "independent", "supersedes": lifecycle.Array{}}))["plan_ref"]
			candidate := lifecycle.Object{"kind": "content", "manifest": wireManifest}
			proposed := result(call("author", "1.3", "propose", with(lifecycle.Object{"source_ref": source, "reason": "first content", "candidate": candidate})))
			if !sameValue(proposed["candidate"], lifecycle.Object{"kind": "content", "ref": ref}) ||
				!sameValue(proposed["head"], lifecycle.Object{"kind": "content", "collection": "loan-rules", "partition": "region-a"}) {
				t.Fatalf("propose result %s", lifecycle.Encode(proposed))
			}
			snapshot := proposed["snapshot_ref"]
			// Protocol 1.2 does not have the candidate field.
			refusal(call("author", "1.2", "propose", with(lifecycle.Object{"source_ref": source, "reason": "again", "candidate": candidate})), "invalid_request", "protocol")
			if got := result(call("reviewer", "1.3", "evidence", lifecycle.Object{"snapshot_ref": snapshot})); got["admission"] != "absent" {
				t.Fatalf("evidence %s", lifecycle.Encode(got))
			}
			proposal := result(call("reviewer", "1.3", "proposal", lifecycle.Object{"snapshot_ref": snapshot}))
			if !sameValue(proposal["bundle"], lifecycle.Object{"content": wireManifest}) {
				t.Fatalf("proposal bundle %s", lifecycle.Encode(proposal["bundle"]))
			}
			report := result(call("runner", "1.3", "qualify", lifecycle.Object{"snapshot_ref": snapshot, "plan_ref": plan}))["report_ref"]
			if got := result(call("reviewer", "1.3", "promotion", lifecycle.Object{"report_ref": report})); got["promotion"] != "invalid" || got["rule"] != "not_promotion" {
				t.Fatalf("promotion %s", lifecycle.Encode(got))
			}
			asked := lifecycle.Object{"function": wireSubject, "candidate": lifecycle.Object{"kind": "content", "ref": ref}}
			if got := result(call("reviewer", "1.3", "approval", asked)); got["approved"] != false || got["state"] != "unknown" {
				t.Fatalf("approval before decide %s", lifecycle.Encode(got))
			}
			decision := result(call("reviewer", "1.3", "decide", lifecycle.Object{"report_ref": report, "reason": "the cases pass", "disposition": "accept"}))["decision_ref"]
			if got := result(call("reviewer", "1.3", "approval", asked)); got["approved"] != true || got["state"] != "current" {
				t.Fatalf("approval after decide %s", lifecycle.Encode(got))
			}
			values := lifecycle.Object{"rules-de": lifecycle.Object{"region": lifecycle.Array{"eu", "de"}}}
			// Protocol 1.2 cannot confirm the values, and the Host needs them.
			refusal(call("reviewer", "1.2", "activate", lifecycle.Object{"decision_ref": decision, "targets": lifecycle.Array{},
				"applicability": values}), "invalid_request", "protocol")
			refusal(call("reviewer", "1.2", "activate", lifecycle.Object{"decision_ref": decision, "targets": lifecycle.Array{}}), "applicability_mismatch", "admission")
			refusal(call("reviewer", "1.3", "activate", lifecycle.Object{"decision_ref": decision, "targets": lifecycle.Array{},
				"applicability": lifecycle.Object{"rules-de": lifecycle.Object{"region": lifecycle.Array{"eu"}}}}), "applicability_mismatch", "admission")
			event := result(call("reviewer", "1.3", "activate", lifecycle.Object{"decision_ref": decision, "targets": lifecycle.Array{},
				"applicability": values}))["event_ref"]
			selected := result(call("reviewer", "1.3", "resolve", lifecycle.Object{"function": wireSubject, "date": "2026-11-27"}))
			if selected["event_ref"] != event || selected["bundle_ref"] != ref || !sameValue(selected["applicability"], values) ||
				!sameValue(selected["candidate"], lifecycle.Object{"kind": "content", "ref": ref}) {
				t.Fatalf("resolve %s", lifecycle.Encode(selected))
			}
		})
	}
}
