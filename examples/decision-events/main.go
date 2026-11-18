package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

//go:embed main.go manifest.json expected.json
var files embed.FS

// This runner checks manifest identity against the installed plan. It makes
// no claim about the item's content or the producer's facts.
type manifestRunner struct{}

func (manifestRunner) Identity() (string, error) {
	source, _ := files.ReadFile("main.go")
	return lifecycle.ContentRef(append(source, []byte(runtime.Version())...)), nil
}
func (r manifestRunner) Expected(lifecycle.Candidate) string { ref, _ := r.Identity(); return ref }
func (r manifestRunner) Qualify(in lifecycle.QualificationInput) (lifecycle.Verdict, error) {
	sum := sha256.Sum256(in.Files["content"])
	ref := "sha256:" + hex.EncodeToString(sum[:])
	status := "failed"
	if ref == in.Candidate.BundleRef && ref == in.Cases.(lifecycle.Array)[0].(lifecycle.Object)["manifest_ref"] {
		status = "passed"
	}
	return lifecycle.Verdict{Outcome: lifecycle.Object{"status": status, "bundle_ref": ref, "implementation_ref": r.Expected(in.Candidate)}}, nil
}

func evidence() lifecycle.Object {
	facts := lifecycle.Array{}
	for i := 1; i <= 3; i++ {
		facts = append(facts, lifecycle.Object{"fact_id": fmt.Sprint(i), "decision_digest": "sha256:" + strings.Repeat(fmt.Sprint(i), 64),
			"receipt_digest": "sha256:" + strings.Repeat("9", 64), "source_kind": "document", "source_digest": nil, "text_digest": nil,
			"spans": lifecycle.Array{}, "outcome": "admitted", "codes": lifecycle.Array{}, "data": lifecycle.Object{"field": "color", "value": "red"}})
	}
	return lifecycle.Object{"format": "entelos.evidence/1", "producer": lifecycle.Object{"id": "sample-ledger", "version": "1", "digest_scheme": "sample/1"}, "facts": facts}
}

func setup(host *lifecycle.Host) error {
	const subject = "content:sample-ledger/demo"
	write := func(actor, role, op string, body lifecycle.Object) (lifecycle.Object, error) {
		root, err := host.Store.Read(nil)
		if err != nil {
			return nil, err
		}
		body["op"], body["expected_revision"], body["request_key"] = op, root.Cursor, "example-"+op
		result, err := host.HandleAs(actor, []string{role}, body)
		if err != nil {
			return nil, err
		}
		return result.(lifecycle.Object)["result"].(lifecycle.Object), nil
	}
	source, err := write("reviewer", "reviewer", "source", lifecycle.Object{"function": subject, "from": "2026-11-01", "until": "2026-12-01", "text": "Review three invented facts.", "reason": "example"})
	if err != nil {
		return err
	}
	expected, _ := files.ReadFile("expected.json")
	cases, err := lifecycle.Decode(expected)
	if err != nil {
		return err
	}
	plan, err := write("reviewer", "reviewer", "plan", lifecycle.Object{"source_ref": source["source_ref"], "cases": lifecycle.Array{cases}, "origin": "reviewer fixture"})
	if err != nil {
		return err
	}
	manifest, _ := files.ReadFile("manifest.json")
	proposal, err := write("producer", "author", "propose", lifecycle.Object{"candidate": lifecycle.Object{"kind": "content", "manifest": string(manifest)}, "source_ref": source["source_ref"], "from": "2026-11-01", "until": "2026-12-01", "reason": "three facts", "evidence": evidence()})
	if err != nil {
		return err
	}
	report, err := write("runner", "runner", "qualify", lifecycle.Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan["plan_ref"]})
	if err != nil {
		return err
	}
	_, err = write("reviewer", "reviewer", "decide", lifecycle.Object{"report_ref": report["report_ref"], "reason": "Correct the second fact to green.", "fact_reviews": lifecycle.Array{lifecycle.Object{"blind": false}, lifecycle.Object{"blind": true, "corrected_value": "green"}, lifecycle.Object{"blind": true}}})
	return err
}

// A single file keeps the effects and the cursor together. On refusal, this
// function returns before it replaces that file.
type checkpoint struct {
	NextCursor string             `json:"next_cursor"`
	Values     map[string]*string `json:"values"`
}

func consume(host *lifecycle.Host, path string, out io.Writer) (int, error) {
	state := checkpoint{Values: map[string]*string{}}
	for i := 1; i <= 3; i++ {
		value := "red"
		state.Values["sha256:"+strings.Repeat(fmt.Sprint(i), 64)] = &value
	}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &state); err != nil {
			return 0, err
		}
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	request := lifecycle.Object{"op": "decision_events", "limit": 2}
	if state.NextCursor != "" {
		request["cursor"] = state.NextCursor
	}
	result, err := host.HandleAs("producer-reader", []string{"reviewer"}, request)
	if err != nil {
		return 0, err
	}
	page := result.(lifecycle.Object)
	events := page["events"].(lifecycle.Array)
	for _, raw := range events {
		event := raw.(lifecycle.Object)
		if lifecycle.CheckDecisionEvent(event)["valid"] != true {
			return 0, fmt.Errorf("invalid event")
		}
		if _, known := state.Values[event["fact_digest"].(string)]; !known || event["evidence_receipt_digest"] != "sha256:"+strings.Repeat("9", 64) {
			return 0, fmt.Errorf("unknown fact or receipt")
		}
		// The producer owns the learning policy. This example applies only
		// human blind corrections to its synthetic fact values.
		if event["actor_kind"] == "human" && event["blind"] == true && event["disposition"] == "correct" {
			var value *string
			if s, ok := event["corrected_value"].(string); ok {
				value = &s
			}
			state.Values[event["fact_digest"].(string)] = value
		}
		fmt.Fprintln(out, "Event:", event["disposition"])
	}
	state.NextCursor = page["next_cursor"].(string)
	raw, err := json.Marshal(state)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path+".next", raw, 0o600); err != nil {
		return 0, err
	}
	if err := os.Rename(path+".next", path); err != nil {
		return 0, err
	}
	fmt.Fprintf(out, "Saved next_cursor after %d events.\n", len(events))
	return len(events), nil
}

func run(work string, out io.Writer) error {
	if work == "" {
		return fmt.Errorf("--work must name a local work directory")
	}
	if err := os.MkdirAll(work, 0o700); err != nil {
		return err
	}
	store, err := lifecycle.OpenGitStore(filepath.Join(work, "store.git"))
	if err != nil {
		return err
	}
	host, err := lifecycle.NewHostWithRunner(store, nil, manifestRunner{}, lifecycle.Object{"scope": "sample-ledger", "runner_actor": "runner", "worker": "unused", "functions": lifecycle.Array{"content:sample-ledger/demo"}, "actor_kinds": lifecycle.Object{"reviewer": "human"}})
	if err != nil {
		return err
	}
	if err := setup(host); err != nil {
		return err
	}
	path := filepath.Join(work, "checkpoint.json")
	if _, err := consume(host, path, out); err != nil {
		return err
	}
	fmt.Fprintln(out, "Resume from the saved file.")
	if _, err := consume(host, path, out); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var saved checkpoint
	if err := json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	value := saved.Values["sha256:"+strings.Repeat("2", 64)]
	if value == nil || *value != "green" {
		return fmt.Errorf("the second fact must be green")
	}
	fmt.Fprintln(out, "Second fact: green.")
	return nil
}
func main() {
	work := flag.String("work", "", "local store and checkpoint directory")
	flag.Parse()
	if err := run(*work, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
