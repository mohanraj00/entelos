package protocol

// Synthetic pagination vectors run without a CEL executable. Only fixture
// construction bypasses Host writes; every read uses the real Host.
import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

func pageFunction(i int) string { return fmt.Sprintf("page.%03d", i) }

func seedPage(v *lifecycle.View, i int, submitted string) string {
	files := lifecycle.Object{}
	for key, mime := range bundleMimes {
		files[key] = v.PutRaw(mime, []byte("fixture"))
	}
	source := v.Put("source", lifecycle.Object{"scope": "demo", "function": pageFunction(i), "text": "invented requirement"})
	ref := v.Put("snapshot", lifecycle.Object{"scope": "demo", "function": pageFunction(i), "from": "2026-11-01", "until": "2027-01-01", "producer": "producer-a", "reason": "fixture", "bundle_ref": source, "source_ref": source, "files": files, "submitted_at": submitted})
	key := string(lifecycle.Encode(lifecycle.Array{pageFunction(i), "demo"}))
	// Encode ends in a newline; head keys use the JSON text without it.
	key = strings.TrimSpace(key)
	v.State["heads"].(lifecycle.Object)[key] = ref
	return ref
}

func TestApprovalPageVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(protocolDir, "approval", "vectors", "page-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(lifecycle.Object)
	fixture := doc["fixture"].(lifecycle.Object)
	totalProposals := count(fixture["count"])
	for _, transport := range append([]string{"host"}, transports...) {
		t.Run(transport, func(t *testing.T) {
			inTempDir(t)
			dir, _ := os.Getwd()
			store := lifecycle.NewMemoryStore()
			refs := map[int]string{}
			start, _ := time.Parse(lifecycle.TimeLayout, fixture["submitted_start"].(string))
			_, err := store.Transact("", "fixture", lifecycle.Object{}, func(v *lifecycle.View) (any, error) {
				v.State["reviews"] = lifecycle.Object{}
				for i := 0; i < totalProposals; i++ {
					refs[i] = seedPage(v, i, start.Add(time.Duration(i)*time.Second).Format(lifecycle.TimeLayout))
				}
				return lifecycle.Object{}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			functions := lifecycle.Array{}
			for i := 0; i <= totalProposals; i++ {
				functions = append(functions, pageFunction(i))
			}
			clock := filepath.Join(dir, "clock")
			writePrivate(t, clock, []byte(fixture["clock"].(string)))
			host, err := lifecycle.NewHost(store, nil, lifecycle.Object{"scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": functions})
			if err != nil {
				t.Fatal(err)
			}
			host.Clock = lifecycle.FileClock{Path: clock}
			schemas := loadTestSchemas(t)
			principals := lifecycle.Object{"reviewer": lifecycle.Object{"actor": "reviewer", "roles": lifecycle.Array{"reviewer"}}}
			var send func(string, []byte) lifecycle.Object
			if transport != "host" {
				send = pageTransport(t, transport, host, schemas, principals, filepath.Join(dir, "transport"))
			}
			next, root := "", ""
			seen := map[string]bool{}
			for _, raw := range doc["steps"].(lifecycle.Array) {
				step := raw.(lifecycle.Object)
				t.Run(step["name"].(string), func(t *testing.T) {
					if change, ok := step["change"].(lifecycle.Object); ok {
						view, _ := store.Read(nil)
						_, err := store.Transact(view.Cursor, "changed-list", lifecycle.Object{}, func(v *lifecycle.View) (any, error) {
							closed := refs[count(change["close"])]
							decision := v.Put("decision", lifecycle.Object{"scope": "demo", "disposition": "reject"})
							v.State["reviews"].(lifecycle.Object)[closed] = lifecycle.Object{"reports": lifecycle.Array{}, "decisions": lifecycle.Array{decision}}
							seedPage(v, count(change["open"]), "2026-11-19T00:00:00Z")
							return lifecycle.Object{}, nil
						})
						if err != nil {
							t.Fatal(err)
						}
						// The Host clock can advance without changing continuation values.
						writePrivate(t, clock, []byte("2026-11-22T00:00:00Z"))
					}
					cursor := step["cursor"].(string)
					if cursor == "previous" {
						cursor = next
					}
					body := lifecycle.Object{"limit": step["limit"], "cursor": cursor}
					request := lifecycle.Object{"protocol": "1.2", "request_id": "page", "operation": "proposals", "body": body}
					var response lifecycle.Object
					if transport == "host" {
						result, err := host.HandleAs("reviewer", []string{"reviewer"}, lifecycle.Object{"op": "proposals", "limit": body["limit"], "cursor": cursor})
						response = lifecycle.Object{"protocol": "1.2", "request_id": "page", "status": "ok", "result": result}
						if err != nil {
							f, ok := err.(*lifecycle.Fault)
							if !ok {
								t.Fatal(err)
							}
							response = failure("page", f.Code, f.Stage, f.Message)
							response["protocol"] = "1.2"
						}
					} else {
						response = send("reviewer", lifecycle.Encode(request))
					}
					for _, e := range shape(schemas, response, "proposals") {
						t.Error(e)
					}
					expect := step["expect"].(lifecycle.Object)
					if status, ok := expect["status"]; ok {
						if response["status"] != status {
							t.Fatalf("%s", lifecycle.Encode(response))
						}
						return
					}
					if response["status"] != "ok" {
						t.Fatalf("%s", lifecycle.Encode(response))
					}
					out := response["result"].(lifecycle.Object)
					list := out["proposals"].(lifecycle.Array)
					if len(list) != count(expect["count"]) {
						t.Fatalf("count %d", len(list))
					}
					for j, x := range list {
						item := x.(lifecycle.Object)
						want := pageFunction(count(expect["first"]) + j)
						if item["function"] != want || item["state"] != "awaiting_qualification" {
							t.Fatalf("item %d: %s", j, lifecycle.Encode(item))
						}
						if step["name"] != "fresh-list" {
							if seen[want] {
								t.Fatalf("duplicate %s", want)
							}
							seen[want] = true
						}
					}
					next = out["next_cursor"].(string)
					if (next != "") != expect["more"].(bool) {
						t.Fatal("wrong next cursor")
					}
					if root == "" {
						root = out["cursor"].(string)
					}
					if step["name"] != "fresh-list" && (out["cursor"] != root || out["as_of"] != fixture["clock"]) {
						t.Fatal("continuation moved root or clock")
					}
				})
			}
			if len(seen) != 250 {
				t.Fatalf("read %d distinct proposals", len(seen))
			}
		})
	}
}

// Exercise the production framing/authentication and HTTP handler without
// opening a listener. The existing session vectors cover real listeners.
func pageTransport(t *testing.T, name string, host *lifecycle.Host, schemas *Schemas, principals lifecycle.Object, prefix string) func(string, []byte) lifecycle.Object {
	t.Helper()
	credentials := prefix + "-credentials.json"
	tokens := writeCredentials(t, credentials, principals)
	adapter := NewAdapter(host, schemas)
	if name == "socket" {
		server, err := NewCredentialServer(adapter, credentials)
		if err != nil {
			t.Fatal(err)
		}
		return func(principal string, payload []byte) lifecycle.Object {
			client, conn := net.Pipe()
			done := make(chan error, 1)
			go func() { defer conn.Close(); done <- adapter.serve(conn, conn, server.authorize(conn)) }()
			response := socketReply(t, client, authenticatedFrame(tokens[principal], payload))
			client.Close()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return response
		}
	}
	config := HTTPConfig{CredentialsFile: credentials, LocalTokens: true, Loopback: true}
	credential := func(principal string) string { return tokens[principal] }
	if name == "http-claim" {
		key := newKey(t, "page-key")
		issuer := prefix + "-issuer.json"
		entries := lifecycle.Array{}
		for _, name := range sortedNames(principals) {
			p := principals[name].(lifecycle.Object)
			entries = append(entries, lifecycle.Object{"subject": p["actor"], "roles": p["roles"]})
		}
		writeIssuer(t, issuer, []testKey{key}, entries)
		config = HTTPConfig{IssuerFile: issuer, Tenant: "demo", Loopback: true}
		credential = func(principal string) string {
			body := claimBody(principals[principal].(lifecycle.Object)["actor"].(string), time.Now())
			body["tenant"] = "demo"
			return mint(key, body)
		}
	}
	handler, err := NewHTTPHandler(adapter, config)
	if err != nil {
		t.Fatal(err)
	}
	return func(principal string, payload []byte) lifecycle.Object {
		request := httptest.NewRequest("POST", "http://127.0.0.1/v1/request", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+credential(principal))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		value, err := lifecycle.Decode(recorder.Body.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		response := value.(lifecycle.Object)
		if recorder.Code != httpStatus(response["status"].(string)) {
			t.Fatal("wrong HTTP status")
		}
		return response
	}
}

func TestApprovalPageCursorContextAndLegacyRequests(t *testing.T) {
	store := lifecycle.NewMemoryStore()
	_, err := store.Transact("", "fixture", lifecycle.Object{}, func(v *lifecycle.View) (any, error) {
		v.State["reviews"] = lifecycle.Object{}
		seedPage(v, 0, "2026-11-20T00:00:00Z")
		seedPage(v, 1, "2026-11-20T00:00:01Z")
		return lifecycle.Object{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	context := lifecycle.Object{"scope": "demo", "runner_actor": "runner", "worker": "worker", "functions": lifecycle.Array{pageFunction(0), pageFunction(1)}}
	host, err := lifecycle.NewHost(store, nil, context)
	if err != nil {
		t.Fatal(err)
	}
	read := func(h *lifecycle.Host, q lifecycle.Object) (any, error) {
		return h.HandleAs("reviewer", []string{"reviewer"}, q)
	}
	result, err := read(host, lifecycle.Object{"op": "proposals", "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	page := result.(lifecycle.Object)["next_cursor"].(string)
	// Legacy requests keep exactly the original three fields, including a
	// store cursor, and work through the old wire version.
	schemas := loadTestSchemas(t)
	a := NewAdapter(host, schemas)
	for _, version := range []string{"1.1", "1.2"} {
		response := a.MessageAs(&Principal{Actor: "reviewer", Roles: []string{"reviewer"}}, lifecycle.Object{"protocol": version, "request_id": "old", "operation": "proposals", "body": lifecycle.Object{}})
		out, ok := response["result"].(lifecycle.Object)
		if !ok || len(out) != 3 || len(out["proposals"].(lifecycle.Array)) != 2 {
			t.Fatalf("legacy result %s", lifecycle.Encode(response))
		}
	}
	for _, body := range []lifecycle.Object{{"limit": 1}, {"cursor": page}} {
		response := a.MessageAs(&Principal{Actor: "reviewer", Roles: []string{"reviewer"}}, lifecycle.Object{"protocol": "1.1", "request_id": "old", "operation": "proposals", "body": body})
		if response["status"] != "invalid_request" {
			t.Fatalf("old request accepted: %s", lifecycle.Encode(response))
		}
	}
	for _, bad := range []any{"1", true, lifecycle.Int{Text: "9007199254740991"}, -1} {
		if _, err := read(host, lifecycle.Object{"op": "proposals", "limit": bad}); err == nil {
			t.Fatalf("accepted limit %v", bad)
		}
	}
	if _, err := host.HandleAs("caller", []string{"caller"}, lifecycle.Object{"op": "proposals", "limit": 1, "cursor": page}); err == nil {
		t.Fatal("page token bypassed role check")
	}
	for _, field := range []string{"scope", "functions"} {
		other := *host
		other.Context = lifecycle.Object{}
		for k, v := range context {
			other.Context[k] = v
		}
		if field == "scope" {
			other.Context[field] = "other"
		} else {
			other.Context[field] = lifecycle.Array{pageFunction(0)}
		}
		if _, err := read(&other, lifecycle.Object{"op": "proposals", "limit": 1, "cursor": page}); err == nil {
			t.Fatal("cursor accepted for another " + field)
		}
	}
	// Tokens remain query positions: reject malformed fields and a root
	// unavailable in the accepted history, without trusting their content.
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(page, "approval-page/1."))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	token := value.(lifecycle.Object)
	for field, bad := range map[string]any{"offset": lifecycle.Int{Text: "999"}, "root": "unknown", "as_of": "not-a-time", "context": "wrong"} {
		changed := lifecycle.Object{}
		for k, v := range token {
			changed[k] = v
		}
		changed[field] = bad
		cursor := "approval-page/1." + base64.RawURLEncoding.EncodeToString(lifecycle.Encode(changed))
		_, err := read(host, lifecycle.Object{"op": "proposals", "limit": 1, "cursor": cursor})
		fault, ok := err.(*lifecycle.Fault)
		if !ok || fault.Code != "invalid_cursor" {
			t.Fatalf("%s: %v", field, err)
		}
	}
	// The same continuation works when the caller changes the page size.
	result, err = read(host, lifecycle.Object{"op": "proposals", "limit": 100, "cursor": page})
	if err != nil || result.(lifecycle.Object)["next_cursor"] != "" {
		t.Fatalf("resized page: %v", err)
	}
	empty, err := lifecycle.NewHost(lifecycle.NewMemoryStore(), nil, context)
	if err != nil {
		t.Fatal(err)
	}
	result, err = read(empty, lifecycle.Object{"op": "proposals", "limit": 50})
	if err != nil || len(result.(lifecycle.Object)["proposals"].(lifecycle.Array)) != 0 || result.(lifecycle.Object)["next_cursor"] != "" {
		t.Fatal("empty list changed")
	}
}

// The evidence vector uses no qualification. A describe-only fixture runtime
// lets the attachment/read contract run offline; it does not evaluate CEL.
type attachmentRuntime struct{}

func (attachmentRuntime) Build() (string, error) {
	return lifecycle.ContentRef([]byte("attachment-fixture")), nil
}
func (r attachmentRuntime) Execute(files map[string][]byte, command string, _ []byte, _ any) (any, error) {
	if command != "describe" {
		return nil, fmt.Errorf("unexpected fixture command: %s", command)
	}
	build, _ := r.Build()
	return lifecycle.Object{"function_id": "record.visible", "bundle_ref": contentRef(bundleMimes["manifest"], files["manifest"]), "implementation_ref": build}, nil
}

func TestApprovalEvidenceVectorsOffline(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(protocolDir, "approval", "vectors", "approval-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := lifecycle.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	doc := value.(lifecycle.Object)
	for _, transport := range transports {
		t.Run(transport, func(t *testing.T) {
			inTempDir(t)
			dir, _ := os.Getwd()
			// approvalSubstitute reads these fixture components for $record_bundle.
			for _, name := range bundleFiles {
				text := "fixture"
				if name == "manifest.json" {
					text = "{}"
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			host, err := lifecycle.NewHost(lifecycle.NewMemoryStore(), attachmentRuntime{}, doc["context"])
			if err != nil {
				t.Fatal(err)
			}
			clock := filepath.Join(dir, "clock")
			writePrivate(t, clock, []byte(doc["clock"].(string)))
			host.Clock = lifecycle.FileClock{Path: clock}
			schemas := loadTestSchemas(t)
			send := pageTransport(t, transport, host, schemas, doc["principals"].(lifecycle.Object), filepath.Join(dir, "transport"))
			bound := map[string]string{}
			for _, raw := range doc["steps"].(lifecycle.Array) {
				step := raw.(lifecycle.Object)
				request := approvalSubstitute(t, step["request"], bound, dir).(lifecycle.Object)
				actual := normal(t, send(step["principal"].(string), lifecycle.Encode(request)))
				for _, e := range approvalMatch(step["expect"].(lifecycle.Object)["response"], actual, bound, step["name"].(string)) {
					t.Error(e)
				}
				for _, e := range shape(schemas, actual, request["operation"]) {
					t.Error(e)
				}
			}
		})
	}
}
