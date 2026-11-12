package lifecycle

// The typed scenario: the conformance vector
// src/cel-lifecycle/conformance/typed/admission-record.json through the
// Host, from the first proposal to the pinned replay. The rule has an int,
// a date and a list input and a record output (profile cel-typed/v1).
//
// The expected statuses, values and fields come from the vector file. The
// qualification cases come from the step "typed-cases-pass" of
// case-checks.json. The canonical input bytes come from
// conformance/storage/typed-values.json. No expected value comes from a run.
//
// The test needs LIFECYCLE_CEL_BINARY, as the lifecycle scenario does. After
// a failure, the test keeps the store and logs its path.

import (
	"os"
	"path/filepath"
	"testing"
)

const typedConformance = "../cel-lifecycle/conformance"

// The canonical bytes of the input of the step "eligible", and their
// reference, as typed-values.json states them.
const (
	eligibleInputText = "{\"observed_on\":\"2026-03-14\",\"score\":812,\"sources\":[\"registry\",\"survey\"]}\n"
	eligibleInputRef  = "sha256:f4360512c7c1f92180bdda77194323eebd38515318386e335567f2c778fda322"
)

func readVector(t *testing.T, name string) Object {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(typedConformance, name))
	if err != nil {
		t.Fatal(err)
	}
	value, err := Decode(raw)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return value.(Object)
}

// typedBundle makes the four bundle files of a typed vector. The manifest
// names the build under test and the content references of the other files.
func typedBundle(t *testing.T, vector Object, build string) Object {
	t.Helper()
	b := vector["bundle"].(Object)
	data := map[string][]byte{"environment": Encode(b["environment"]), "source": []byte(b["source"].(string)), "fixed": Encode(b["fixed"])}
	manifest := Object{"function_id": b["function_id"], "profile": b["profile"], "caller_variables": b["caller_variables"],
		"limits": b["limits"], "implementation_ref": build}
	for key, field := range map[string]string{"environment": "environment_ref", "source": "source_ref", "fixed": "fixed_ref"} {
		manifest[field] = ContentRef(Envelope(Mimes[key], data[key]))
	}
	data["manifest"] = Encode(manifest)
	files := Object{}
	for key, raw := range data {
		files[key] = string(raw)
	}
	return files
}

// typedPlanCases gives the cases of the step "typed-cases-pass" of
// case-checks.json. Its expected report is "passed".
func typedPlanCases(t *testing.T) Array {
	t.Helper()
	for _, step := range readVector(t, "typed/case-checks.json")["steps"].(Array) {
		s := step.(Object)
		if s["name"] == "typed-cases-pass" && at(s, "expect", "status") == "passed" {
			return s["cases"].(Array)
		}
	}
	t.Fatal("case-checks.json has no passing step typed-cases-pass")
	return nil
}

func TestTypedPolicyFromProposalToReplay(t *testing.T) {
	binary := os.Getenv("LIFECYCLE_CEL_BINARY")
	if binary == "" {
		if os.Getenv("LIFECYCLE_REQUIRE_SCENARIO") != "" {
			t.Fatal("LIFECYCLE_CEL_BINARY is not set")
		}
		t.Skip("LIFECYCLE_CEL_BINARY is not set")
	}
	vector := readVector(t, "typed/admission-record.json")
	function := vector["bundle"].(Object)["function_id"]
	parent := ""
	if out := os.Getenv("LIFECYCLE_SCENARIO_RESULTS"); out != "" {
		parent = filepath.Dir(out)
	}
	dir, err := os.MkdirTemp(parent, "lifecycle-typed-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() {
			t.Logf("typed scenario: kept the store at %s", dir)
		} else {
			os.RemoveAll(dir)
		}
	}()
	d, err := newScenarioDriver(filepath.Join(dir, "typed"), binary, Object{"scope": "demo", "function": function})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := NewCELRuntime(binary, filepath.Join(d.work, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	build, err := rt.Build()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			failure, ok := r.(scenarioFailure)
			if !ok {
				panic(r)
			}
			t.Fatal(failure.message)
		}
	}()

	// Propose, qualify, approve, activate and load.
	from, until, date := "2026-01-01", "2027-01-01", "2026-06-01"
	source := result(d.call("reviewer", "source", "typed-source", "ok", Object{"function": function, "from": from, "until": until,
		"text":       "Invented approved admission table: admit a score of 700 or more, observed on 2026-01-01 or later, with 2 sources or more.",
		"supersedes": Array{}, "reason": "typed requirement"}))["source_ref"].(string)
	plan := result(d.call("reviewer", "plan", "typed-plan", "ok", Object{"source_ref": source, "cases": typedPlanCases(t),
		"origin": "The step typed-cases-pass of conformance/typed/case-checks.json", "supersedes": Array{}}))["plan_ref"].(string)
	proposal := result(d.call("author", "propose", "typed-proposal", "ok", Object{"function": function, "from": from, "until": until,
		"base_snapshot": "", "source_ref": source, "bundle": typedBundle(t, vector, build), "reason": "typed proposal"}))
	q, decision := d.qualifyDecide(proposal["snapshot_ref"].(string), plan, "typed")
	d.check("typed-qualification", "Do the stated typed cases qualify the typed bundle?", at(q, "report", "status") == "passed", q)
	d.call("reviewer", "activate", "typed-activate", "ok", Object{"decision_ref": decision, "targets": Array{}})
	d.call("worker", "load", "typed-load", "ok", Object{"function": function, "date": date})

	// Invoke each run step of the vector. A refused input is an answer with
	// a receipt, not a Host fault.
	type call struct {
		name    string
		inputs  any
		receipt Object
	}
	var calls []call
	for _, step := range vector["steps"].(Array) {
		s := step.(Object)
		if s["op"] != "run" {
			continue
		}
		name := s["name"].(string)
		inputs, err := Decode([]byte(s["inputs_text"].(string)))
		if err != nil {
			t.Fatalf("%s: inputs_text: %v", name, err)
		}
		got := result(d.call("caller", "invoke", "typed-"+name, "ok", Object{"function": function, "date": date,
			"expected_bundle_ref": proposal["bundle_ref"], "inputs": inputs}))
		want := s["expect"].(Object)
		actual := got["actual"].(Object)
		_, hasValue := actual["value"]
		var passed bool
		if want["status"] == "ok" {
			passed = actual["status"] == "ok" && equal(actual["value"], want["value"]) && actual["diagnostic"] == nil
		} else {
			passed = actual["status"] == want["status"] && !hasValue && at(actual, "diagnostic", "field") == want["field"]
		}
		d.check("typed-answer-"+name, "Does the Host give the answer that the vector states?", passed, actual)
		receipt := got["receipt"].(Object)
		retained := d.call("reader", "receipt", "", "ok", Object{"receipt_ref": got["receipt_ref"]})
		d.check("typed-receipt-"+name, "Does the receipt keep the canonical input bytes and the response?",
			receipt["inputs_ref"] == InputDigest(inputs) && equal(retained, receipt) &&
				receipt["response_ref"] == ContentRef(Envelope("application/json", Encode(actual))), receipt)
		calls = append(calls, call{name, inputs, Object{"ref": got["receipt_ref"], "record": receipt}})
	}
	if len(calls) != 29 {
		t.Fatalf("the vector has %d run steps, want 29", len(calls))
	}
	byName := map[string]Object{}
	for _, c := range calls {
		byName[c.name] = c.receipt["record"].(Object)
	}
	d.check("typed-canonical-inputs", "Do the stated canonical bytes identify the eligible input in each key order?",
		ContentRef(Envelope("application/json", []byte(eligibleInputText))) == eligibleInputRef &&
			byName["eligible"]["inputs_ref"] == eligibleInputRef && byName["eligible-key-order"]["inputs_ref"] == eligibleInputRef,
		Object{"eligible": byName["eligible"]["inputs_ref"], "key_order": byName["eligible-key-order"]["inputs_ref"]})

	// Replay each receipt. The replay runs the pinned selection on the
	// retained input bytes and gives the same response bytes.
	for _, c := range calls {
		old := c.receipt["record"].(Object)
		replay := result(d.call("reader", "replay", "typed-replay-"+c.name, "ok", Object{"receipt_ref": c.receipt["ref"],
			"reason": "Reproduce the typed answer without resolving latest."}))
		r := replay["receipt"].(Object)
		d.check("typed-replay-"+c.name, "Does the replay give the same response bytes on the same input bytes and selection?",
			r["matches_original"] == true && r["response_ref"] == old["response_ref"] && r["inputs_ref"] == old["inputs_ref"] &&
				equal(r["selection"], old["selection"]) && r["previous_receipt"] == c.receipt["ref"], r)
	}
	// A replay refuses input text that differs from the retained bytes. The
	// vector text of int-with-fraction is not the canonical text.
	refused := calls[0]
	for _, c := range calls {
		if c.name == "int-with-fraction" {
			refused = c
		}
	}
	d.call("reader", "replay", "typed-replay-raw-text", "input_mismatch", Object{"receipt_ref": refused.receipt["ref"],
		"inputs_json": `{"score": 812.0, "observed_on": "2026-03-14", "sources": ["registry", "survey"]}`, "reason": "must keep the retained bytes"})
	if refused.name != "int-with-fraction" {
		t.Fatal("the vector has no step int-with-fraction")
	}
}
