package lifecycle

// Tests of the finding import contract of FINDING.md. The example is
// invented: an agent answers questions about the due dates of a sample
// library catalog. The records in this file are written by hand from the
// record format. No test in this file runs the tool that writes such records.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

const (
	mossQuestion   = "When is Field Notes on Moss due?"
	gardenQuestion = "When is The Night Garden due?"
	// The old catalog gives a wrong due date for The Night Garden. The
	// corrected catalog gives the date that the answer key has.
	oldCatalog       = `{"due": {"Field Notes on Moss": "2026-12-04", "The Night Garden": "2026-12-16"}}`
	correctedCatalog = `{"due": {"Field Notes on Moss": "2026-12-04", "The Night Garden": "2026-12-09"}}`
	libraryFindingID = "20261124-093000-c4d1/answer_key_mismatch/2"
)

func hexDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func digestOf(letter string) string { return strings.Repeat(letter, 64) }

// reseal writes the seal of a record again from its test and its files.
func reseal(record Object) {
	seal := record["seal"].(Object)
	text := string(Encode(Object{"copy": true, "files": seal["files"], "test": record["test"], "v": Int{Text: "1"}}))
	seal["text"], seal["sha256"] = text, hexDigest(text)
}

// libraryRecord is a finding record of a mismatch with the answer key: the
// agent gave 2026-12-16 for The Night Garden, and the key gives 2026-12-09.
func libraryRecord() Object {
	record := Object{
		"record": "nookku.finding", "version": "1.0", "id": libraryFindingID, "kind": "answer_key_mismatch",
		"test":   "20261124-093000-c4d1",
		"script": Object{"messages": Array{mossQuestion, gardenQuestion}, "sha256": digestOf("f")},
		"turn": Object{"number": Int{Text: "2"}, "input": gardenQuestion,
			"reply": `{"due": "2026-12-16", "reply": "The Night Garden is due on 2026-12-16."}`},
		"expected": Object{"value": "2026-12-09", "match": "exact", "tolerance": nil, "field": "due"},
		"observed": `"2026-12-16"`, "issue": nil,
		"adjudication": Object{"outcome": "agent_wrong", "reason": "The loan list gives 2026-12-09.", "author": "Asha",
			"author_type": "human", "ts": 1795512600.5},
		"subject": ByteDigest([]byte(oldCatalog)), "gate": nil,
		"seal": Object{"files": Object{"manifest.json": digestOf("a"), "tap.jsonl": digestOf("b"), "findings.json": digestOf("c"),
			"answer_key.json": digestOf("d"), "adjudications.jsonl": digestOf("e"), "script.jsonl": digestOf("f")}},
		"nookku": "0.6.1",
	}
	reseal(record)
	return record
}

// issueRecord is a finding record of an issue row of a person. It has no
// expected value.
func issueRecord() Object {
	record := libraryRecord()
	record["id"], record["kind"] = "20261124-093000-c4d1/issue_row/1", "issue_row"
	record["expected"], record["observed"] = nil, nil
	record["issue"] = Object{"class": "unsupported_reply", "evidence": "trace.jsonl:7", "text": "The reply gives a date that no tool result has."}
	record["adjudication"].(Object)["ts"] = nil
	record["seal"].(Object)["files"] = Object{"manifest.json": digestOf("a"), "tap.jsonl": digestOf("b"), "evaluation.json": digestOf("c"),
		"script.jsonl": digestOf("f")}
	reseal(record)
	return record
}

func findingTextOf(record Object, change func(Object)) string {
	if change != nil {
		change(record)
	}
	return string(Encode(record))
}

func TestFindingReaderRules(t *testing.T) {
	part := func(name string) func(Object) Object {
		return func(r Object) Object { return r[name].(Object) }
	}
	script, turn, expected, adjudication, seal := part("script"), part("turn"), part("expected"), part("adjudication"), part("seal")
	gate := Object{"id": "gate-20261124-092900-9f8e7d6c", "name": "due dates", "result_sha256": digestOf("9")}
	cases := []struct {
		name   string
		issue  bool
		change func(Object)
		want   string
	}{
		{"mismatch", false, nil, ""},
		{"issue row of a person", true, nil, ""},
		{"issue row with no turn", true, func(r Object) { r["turn"] = nil }, ""},
		{"model issue row", true, func(r Object) { a := adjudication(r); a["author"], a["author_type"] = nil, "model" }, ""},
		{"gate record", false, func(r Object) { r["gate"] = gate }, ""},
		{"script with no copy", false, func(r Object) { script(r)["sha256"] = nil }, ""},
		{"number match", false, func(r Object) { e := expected(r); e["match"], e["value"], e["tolerance"] = "number", "112", "0.5" }, ""},
		// The reader ignores an unknown field, in the record and in an object of the record.
		{"unknown field", false, func(r Object) { r["labels"], expected(r)["unit"] = Array{"loans"}, "days" }, ""},
		// The reader accepts a newer minor version.
		{"newer minor version", false, func(r Object) { r["version"], r["shelf"] = "1.7", "B4" }, ""},
		// The reader refuses an unknown major version.
		{"major version 2", false, func(r Object) { r["version"] = "2.0" }, ruleVersionMajor},
		{"major version 0", false, func(r Object) { r["version"] = "0.9" }, ruleVersionMajor},
		{"no version", false, func(r Object) { delete(r, "version") }, ruleVersionForm},
		{"version with one part", false, func(r Object) { r["version"] = "1" }, ruleVersionForm},
		{"version with a leading zero", false, func(r Object) { r["version"] = "01.0" }, ruleVersionForm},
		{"version as a number", false, func(r Object) { r["version"] = 1.0 }, ruleVersionForm},
		{"another record name", false, func(r Object) { r["record"] = "other.finding" }, ruleNotFinding},
		{"missing field", false, func(r Object) { delete(r, "subject") }, ruleMissingField},
		{"missing key", false, func(r Object) { delete(script(r), "sha256") }, ruleMissingKey},
		{"missing key with a null value elsewhere", false, func(r Object) { delete(turn(r), "reply") }, ruleMissingKey},
		{"unknown kind", false, func(r Object) { r["kind"] = "note" }, ruleFieldValue},
		{"blank id", false, func(r Object) { r["id"] = " \t" }, ruleFieldValue},
		{"empty script", false, func(r Object) { script(r)["messages"] = Array{} }, ruleFieldValue},
		{"message that is not a string", false, func(r Object) { script(r)["messages"] = Array{mossQuestion, Int{Text: "2"}} }, ruleFieldValue},
		{"mismatch with no turn", false, func(r Object) { r["turn"] = nil }, ruleFieldValue},
		{"turn 0", false, func(r Object) { turn(r)["number"] = Int{Text: "0"} }, ruleFieldValue},
		{"turn as a float", false, func(r Object) { turn(r)["number"] = 2.0 }, ruleFieldValue},
		{"turn after the script", false, func(r Object) { turn(r)["number"] = Int{Text: "3"} }, ruleTurn},
		{"turn input of another message", false, func(r Object) { turn(r)["input"] = mossQuestion }, ruleTurn},
		{"reply that is not a string", false, func(r Object) { turn(r)["reply"] = Array{} }, ruleFieldValue},
		{"unknown match", false, func(r Object) { expected(r)["match"] = "prefix" }, ruleFieldValue},
		{"exact match with a tolerance", false, func(r Object) { expected(r)["tolerance"] = "0.5" }, ruleFieldValue},
		{"number match with a word", false, func(r Object) { e := expected(r); e["match"], e["tolerance"] = "number", "0.5" }, ruleFieldValue},
		{"issue row with an expected value", true, func(r Object) { r["expected"] = Object{} }, ruleFieldValue},
		{"mismatch with an issue", false, func(r Object) { r["issue"] = issueRecord()["issue"] }, ruleFieldValue},
		{"issue with a blank text", true, func(r Object) { r["issue"].(Object)["text"] = "" }, ruleFieldValue},
		{"observed that is not a string", false, func(r Object) { r["observed"] = Int{Text: "16"} }, ruleFieldValue},
		{"another outcome", false, func(r Object) { adjudication(r)["outcome"] = "key_wrong" }, ruleFieldValue},
		// Only a person adjudicates a mismatch.
		{"mismatch with a model author", false, func(r Object) { a := adjudication(r); a["author"], a["author_type"] = nil, "model" }, ruleFieldValue},
		{"model row with an author", true, func(r Object) { adjudication(r)["author_type"] = "model" }, ruleFieldValue},
		{"person with no name", false, func(r Object) { adjudication(r)["author"] = nil }, ruleFieldValue},
		{"mismatch with no time", false, func(r Object) { adjudication(r)["ts"] = nil }, ruleFieldValue},
		{"negative time", false, func(r Object) { adjudication(r)["ts"] = Int{Text: "-1"} }, ruleFieldValue},
		{"empty subject", false, func(r Object) { r["subject"] = "" }, ruleFieldValue},
		{"subject with a control character", false, func(r Object) { r["subject"] = "sha256:\x7f" }, ruleFieldValue},
		{"subject of 513 characters", false, func(r Object) { r["subject"] = strings.Repeat("é", 513) }, ruleFieldValue},
		{"gate with no result digest", false, func(r Object) { r["gate"] = Object{"id": "g", "name": "n", "result_sha256": "9"} }, ruleFieldValue},
		{"seal digest of another text", false, func(r Object) { seal(r)["sha256"] = digestOf("0") }, ruleSealDigest},
		{"seal text that is not a seal", false, func(r Object) { s := seal(r); s["text"], s["sha256"] = "[]", hexDigest("[]") }, ruleSealText},
		{"seal of another test", false, func(r Object) { r["test"] = "20261124-093000-0000" }, ruleSealText},
		{"file that the seal text does not cover", false, func(r Object) { seal(r)["files"].(Object)["findings.json"] = digestOf("0") }, ruleSealCoverage},
		{"no source file of the kind", false, func(r Object) { delete(seal(r)["files"].(Object), "answer_key.json"); reseal(r) }, ruleSealSources},
		{"no script copy with the script digest", false, func(r Object) { script(r)["sha256"] = digestOf("0") }, ruleSealSources},
	}
	for _, c := range cases {
		record := libraryRecord()
		if c.issue {
			record = issueRecord()
		}
		if _, got := readFinding([]byte(findingTextOf(record, c.change))); got != c.want {
			t.Errorf("%s: got the rule %q, want %q", c.name, got, c.want)
		}
	}
	valid := findingTextOf(libraryRecord(), nil)
	texts := map[string]string{
		"not JSON":            valid[:len(valid)/2],
		"JSON array":          "[" + valid + "]",
		"duplicate key":       `{"record": "nookku.finding", "record": "nookku.finding"}`,
		"NaN":                 strings.Replace(valid, "1795512600.5", "NaN", 1),
		"two records":         valid + "\n" + valid,
		"over the size bound": valid + strings.Repeat(" ", maxFindingBytes),
	}
	for name, text := range texts {
		want := ruleNotJSON
		if name == "over the size bound" {
			want = ruleTooLarge
		}
		if _, got := readFinding([]byte(text)); got != want {
			t.Errorf("%s: got the rule %q, want %q", name, got, want)
		}
	}
}

// catalogRunner is a fixture runner. It is the invented agent of the
// example: it answers the last message of each case from the fixed data of
// the candidate, and compares the answer with the expected value of the case.
type catalogRunner struct{}

func (catalogRunner) Identity() (string, error) { return fixtureIdentity("catalog-agent"), nil }

func (catalogRunner) Expected(Candidate) string { return fixtureIdentity("catalog-agent") }

func catalogFault(message string) *Fault { return NewFault("runner_unavailable", message, "runner") }

func (catalogRunner) Qualify(in QualificationInput) (Verdict, error) {
	catalog, err := Decode(in.Files["fixed"])
	due, ok := lookupAny(catalog, "due").(Object)
	if err != nil || !ok {
		return Verdict{}, catalogFault("the fixed data is not a catalog")
	}
	titles := sortedKeys(due)
	status, results := "passed", Array{}
	for _, x := range in.Cases.(Array) {
		c, _ := x.(Object)
		expected, _ := c["expected"].(Object)
		messages, _ := c["messages"].(Array)
		// The runner gives no verdict on a case that it cannot read.
		if c["format"] != findingCaseFormat || expected["match"] != "exact" || expected["field"] != "due" || len(messages) == 0 {
			return Verdict{}, catalogFault("the runner does not know this case")
		}
		var answer any
		for _, title := range titles {
			if strings.Contains(messages[len(messages)-1].(string), title) {
				answer = due[title]
			}
		}
		passed := answer == expected["value"]
		if !passed {
			status = "failed"
		}
		results = append(results, Object{"name": c["name"], "passed": passed, "observed": answer})
	}
	return Verdict{Outcome: Object{"status": status, "bundle_ref": in.Candidate.BundleRef,
		"implementation_ref": fixtureIdentity("catalog-agent"), "results": results}}, nil
}

// catalogBundle is a bundle with the catalog as its fixed data. The manifest
// names the byte digest of the catalog, so each catalog has its own bundle.
func catalogBundle(catalog string) Object {
	return Object{"manifest": `{"caller_variables":[],"environment_ref":"env","fixed_ref":"` + ByteDigest([]byte(catalog)) +
		`","limits":{},"profile":"p","rule":"catalog"}`, "environment": "{}", "source": "catalog", "fixed": catalog}
}

// mossCase is the case that the reviewer wrote by hand for the first plan.
var mossCase = Object{"format": findingCaseFormat, "name": "moss", "messages": Array{mossQuestion},
	"expected": Object{"value": "2026-12-04", "match": "exact", "tolerance": nil, "field": "due"}, "issue": nil}

// catalogWorld is a store with a source, a plan with the hand-written case
// and the old catalog as the head snapshot.
type catalogWorld struct {
	*world
	source, plan, old string
}

func newCatalogWorld(t *testing.T) *catalogWorld {
	w := &catalogWorld{world: newWorld(t, catalogRunner{})}
	w.source = w.must("reviewer", "reviewer", "source", with(window, Object{"text": "the loan list", "reason": "first", "supersedes": Array{}}))["source_ref"].(string)
	w.plan = w.must("reviewer", "reviewer", "plan", Object{"source_ref": w.source, "cases": Array{mossCase}, "origin": "independent",
		"supersedes": Array{}})["plan_ref"].(string)
	w.old = w.propose(oldCatalog, "")
	return w
}

func (w *catalogWorld) propose(catalog, base string) string {
	return w.must("author", "author", "propose", with(window, Object{"base_snapshot": base, "source_ref": w.source,
		"bundle": catalogBundle(catalog), "reason": "catalog"}))["snapshot_ref"].(string)
}

func (w *catalogWorld) qualify(snapshot, plan string) (string, Object) {
	out := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	return out["report_ref"].(string), out["report"].(Object)
}

func (w *catalogWorld) importFinding(text string) (Object, error) {
	return w.do("importer", "author", w.runner, "import_finding", Object{"source_ref": w.source, "finding": text})
}

func (w *catalogWorld) decideCase(candidate any, disposition string) (Object, error) {
	return w.do("reviewer", "reviewer", w.runner, "decide_case", Object{"candidate_ref": candidate, "disposition": disposition, "reason": "read"})
}

func (w *catalogWorld) planWith(imports ...any) (Object, error) {
	return w.do("reviewer", "reviewer", w.runner, "plan", Object{"source_ref": w.source, "cases": Array{mossCase}, "origin": "independent",
		"supersedes": Array{w.plan}, "imports": Array(imports)})
}

func (w *catalogWorld) view() *View {
	v, err := w.store.Read(nil)
	if err != nil {
		w.t.Fatal(err)
	}
	return v
}

// lifecycleState is the state that a qualification, a verdict or an
// activation changes.
func (w *catalogWorld) lifecycleState() string {
	state := w.view().State
	return string(Encode(Object{"heads": state["heads"], "events": state["events"], "reviews": state["reviews"],
		"plans": state["plans"], "sources": state["sources"]}))
}

func wantFault(t *testing.T, what string, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: got no fault, want %s", what, code)
	}
	if got := faultCode(t, err); got != code {
		t.Fatalf("%s: got the fault %s, want %s", what, got, code)
	}
}

// findingFlow runs the example of FINDING.md with one finding record of the
// mismatch on the old catalog. The seam test runs it with a record of the
// writer of the format.
func findingFlow(t *testing.T, finding string) {
	w := newCatalogWorld(t)
	before, report := w.qualify(w.old, w.plan)
	if report["status"] != "passed" {
		t.Fatalf("the old catalog with the first plan: %v", report)
	}
	state := w.lifecycleState()

	imported, err := w.importFinding(finding)
	if err != nil {
		t.Fatal(err)
	}
	candidate := imported["candidate_ref"]
	record := imported["candidate"].(Object)
	if got := record["input_digest"]; got != "sha256:"+hexDigest(finding) {
		t.Fatalf("input digest %v", got)
	}
	// The finding names the tested thing. Here it is the old catalog.
	if got := record["origin"].(Object)["subject"]; got != ByteDigest([]byte(oldCatalog)) {
		t.Fatalf("subject %v", got)
	}
	// The import writes no report, no verdict and no event.
	if got := w.lifecycleState(); got != state {
		t.Fatalf("the import changed the lifecycle state:\n%s\n%s", state, got)
	}

	// An unapproved candidate has no effect on qualification. The same
	// qualify gives the same report record, and a plan refuses the candidate.
	if again, report := w.qualify(w.old, w.plan); again != before || report["status"] != "passed" {
		t.Fatalf("the report changed after the import: %s, %v", again, report)
	}
	// The second qualify added its report to the review index.
	state = w.lifecycleState()
	cursor := w.view().Cursor
	_, err = w.planWith(candidate)
	wantFault(t, "plan with an unapproved candidate", err, "case_not_approved")
	if got := w.view().Cursor; got != cursor {
		t.Fatal("the refused plan published state")
	}

	if _, err := w.decideCase(candidate, "accept"); err != nil {
		t.Fatal(err)
	}
	// The approval alone gives no plan, no report, no verdict and no event.
	if got := w.lifecycleState(); got != state {
		t.Fatalf("the case decision changed the lifecycle state:\n%s\n%s", state, got)
	}
	planned, err := w.planWith(candidate)
	if err != nil {
		t.Fatal(err)
	}
	plan := planned["plan_ref"].(string)

	// The case fails on the old catalog and passes on the corrected catalog.
	failed, report := w.qualify(w.old, plan)
	results := report["outcome"].(Object)["results"].(Array)
	if report["status"] != "failed" || !equal(results, Array{
		Object{"name": "moss", "passed": true, "observed": "2026-12-04"},
		Object{"name": record["case"].(Object)["name"], "passed": false, "observed": "2026-12-16"}}) {
		t.Fatalf("the old catalog with the imported case: %v", report)
	}
	_, err = w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": failed, "reason": "try"})
	wantFault(t, "decide on the failed report", err, "qualification_failed")
	corrected := w.propose(correctedCatalog, w.old)
	passed, report := w.qualify(corrected, plan)
	if report["status"] != "passed" {
		t.Fatalf("the corrected catalog with the imported case: %v", report)
	}
	if _, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": passed, "reason": "corrected"}); err != nil {
		t.Fatal(err)
	}
}

func TestFindingCaseFailsOnTheOldSnapshotAndPassesOnTheCorrectedSnapshot(t *testing.T) {
	findingFlow(t, findingTextOf(libraryRecord(), nil))
}

// The candidate has the values that a person wrote from the record.
func TestImportFindingGivesTheCandidate(t *testing.T) {
	w := newCatalogWorld(t)
	finding := findingTextOf(libraryRecord(), func(r Object) {
		r["gate"] = Object{"id": "gate-1", "name": "due dates", "result_sha256": digestOf("9")}
	})
	imported, err := w.importFinding(finding)
	if err != nil {
		t.Fatal(err)
	}
	want := Object{
		"format": RecordFormat, "function": "record.visible", "scope": "demo", "from": "2026-11-01", "until": "2027-01-01",
		"source_ref": w.source, "importer": "importer", "input_digest": "sha256:" + hexDigest(finding),
		"origin": Object{"record": "nookku.finding", "version": "1.0", "id": libraryFindingID, "kind": "answer_key_mismatch",
			"subject":     "sha256:" + hexDigest(oldCatalog),
			"seal_digest": "sha256:" + libraryRecord()["seal"].(Object)["sha256"].(string),
			"adjudication": Object{"outcome": "agent_wrong", "reason": "The loan list gives 2026-12-09.", "author": "Asha",
				"author_type": "human"},
			"gate":             Object{"id": "gate-1", "name": "due dates", "result_digest": "sha256:" + digestOf("9")},
			"producer_version": "0.6.1"},
		"case": Object{"format": "entelos.finding-case/1", "name": libraryFindingID, "messages": Array{mossQuestion, gardenQuestion},
			"expected": Object{"value": "2026-12-09", "match": "exact", "tolerance": nil, "field": "due"}, "issue": nil},
	}
	if !equal(imported["candidate"], want) {
		t.Fatalf("candidate:\n%s\nwant:\n%s", Encode(imported["candidate"]), Encode(want))
	}
	stored, err := w.view().Get(imported["candidate_ref"], kindCandidate)
	if err != nil || !equal(stored, want) {
		t.Fatalf("stored candidate: %v, %s", err, Encode(stored))
	}
	// A turn before the last message gives only the messages to that turn.
	first, err := w.importFinding(findingTextOf(libraryRecord(), func(r Object) {
		r["turn"] = Object{"number": Int{Text: "1"}, "input": mossQuestion, "reply": nil}
	}))
	if err != nil || !equal(first["candidate"].(Object)["case"].(Object)["messages"], Array{mossQuestion}) {
		t.Fatalf("turn 1: %v, %v", err, first)
	}
	// The same importer, source and text give the same candidate.
	again, err := w.importFinding(finding)
	if err != nil || again["candidate_ref"] != imported["candidate_ref"] {
		t.Fatalf("second import: %v, %v", err, again)
	}
}

func TestImportFindingRefusals(t *testing.T) {
	w := newCatalogWorld(t)
	cursor := w.view().Cursor
	_, err := w.importFinding(findingTextOf(libraryRecord(), func(r Object) { r["version"] = "2.0" }))
	wantFault(t, "major version 2", err, "unsupported_finding_version")
	_, err = w.importFinding(findingTextOf(libraryRecord(), func(r Object) { r["seal"].(Object)["sha256"] = digestOf("0") }))
	wantFault(t, "seal digest", err, "invalid_finding")
	_, err = w.importFinding("")
	wantFault(t, "empty text", err, "invalid_finding")
	_, err = w.do("importer", "author", w.runner, "import_finding", Object{"source_ref": w.source, "finding": libraryRecord()})
	wantFault(t, "finding as an object", err, "invalid_request")
	_, err = w.do("importer", "author", w.runner, "import_finding", Object{"source_ref": w.plan, "finding": findingTextOf(libraryRecord(), nil)})
	if err == nil {
		t.Fatal("a plan reference as the source: got no fault")
	}
	_, err = w.do("caller", "caller", w.runner, "import_finding", Object{"source_ref": w.source, "finding": findingTextOf(libraryRecord(), nil)})
	wantFault(t, "caller role", err, "forbidden")
	if got := w.view().Cursor; got != cursor {
		t.Fatal("a refused import published state")
	}
}

func TestCaseDecisionRules(t *testing.T) {
	w := newCatalogWorld(t)
	imported, err := w.importFinding(findingTextOf(libraryRecord(), nil))
	if err != nil {
		t.Fatal(err)
	}
	candidate := imported["candidate_ref"]
	decide := func(actor, role string, fields Object) error {
		_, err := w.do(actor, role, w.runner, "decide_case", with(Object{"candidate_ref": candidate, "disposition": "accept", "reason": "read"}, fields))
		return err
	}
	wantFault(t, "author role", decide("author", "author", nil), "forbidden")
	wantFault(t, "the importer as the reviewer", decide("importer", "reviewer", nil), "separation_required")
	wantFault(t, "no disposition", decide("reviewer", "reviewer", Object{"disposition": "approve"}), "invalid_request")
	wantFault(t, "no reason", decide("reviewer", "reviewer", Object{"reason": ""}), "invalid_request")
	if err := decide("reviewer", "reviewer", Object{"candidate_ref": w.plan}); err == nil {
		t.Fatal("a plan reference as the candidate: got no fault")
	}
	now := "2026-11-25T10:00:00Z"
	w.now = &now
	decided, err := w.decideCase(candidate, "accept")
	if err != nil {
		t.Fatal(err)
	}
	want := Object{"format": RecordFormat, "function": "record.visible", "scope": "demo", "from": "2026-11-01", "until": "2027-01-01",
		"candidate_ref": candidate, "input_digest": imported["candidate"].(Object)["input_digest"], "actor": "reviewer", "reason": "read",
		"disposition": "accept", "decided_at": now}
	if !equal(decided["case_decision"], want) {
		t.Fatalf("case decision %s", Encode(decided["case_decision"]))
	}
	wantFault(t, "second decision", decide("reviewer-2", "reviewer", Object{"disposition": "reject"}), "duplicate_case_decision")

	// A candidate of an issue row has no expected value. A reviewer can
	// reject it, and cannot accept it.
	issue, err := w.importFinding(findingTextOf(issueRecord(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := issue["candidate"].(Object)["case"].(Object); got["expected"] != nil || got["issue"] == nil {
		t.Fatalf("issue case %v", got)
	}
	_, err = w.decideCase(issue["candidate_ref"], "accept")
	wantFault(t, "accept with no expected value", err, "case_without_expectation")
	if _, err := w.decideCase(issue["candidate_ref"], "reject"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanTakesOnlyAnAcceptedCandidate(t *testing.T) {
	w := newCatalogWorld(t)
	text := func(test string) string {
		return findingTextOf(libraryRecord(), func(r Object) { r["test"] = test; reseal(r) })
	}
	refOf := func(text string) any {
		imported, err := w.importFinding(text)
		if err != nil {
			t.Fatal(err)
		}
		return imported["candidate_ref"]
	}
	accepted, rejected, open := refOf(text("a")), refOf(text("b")), refOf(text("c"))
	decision, err := w.decideCase(accepted, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.decideCase(rejected, "reject"); err != nil {
		t.Fatal(err)
	}
	_, err = w.planWith(accepted, open)
	wantFault(t, "open candidate", err, "case_not_approved")
	_, err = w.planWith(rejected)
	wantFault(t, "rejected candidate", err, "case_not_approved")
	_, err = w.planWith(accepted, accepted)
	wantFault(t, "one candidate two times", err, "invalid_plan")
	if _, err = w.planWith(w.plan); err == nil {
		t.Fatal("a plan reference as an import: got no fault")
	}
	_, err = w.do("reviewer", "reviewer", w.runner, "plan", Object{"source_ref": w.source, "cases": Array{}, "origin": "independent", "imports": "x"})
	wantFault(t, "imports as a string", err, "invalid_request")
	// A candidate of another source is not a candidate of this plan.
	other := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "the loan list, second copy", "reason": "copy", "supersedes": Array{}}))["source_ref"]
	_, err = w.do("reviewer", "reviewer", w.runner, "plan", Object{"source_ref": other, "cases": Array{}, "origin": "independent", "imports": Array{accepted}})
	wantFault(t, "candidate of another source", err, "case_source_mismatch")

	// A plan can have only imported cases. The record names each import.
	planned, err := w.do("reviewer", "reviewer", w.runner, "plan", Object{"source_ref": w.source, "cases": Array{}, "origin": "finding import",
		"imports": Array{accepted}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.view().Get(planned["plan_ref"], "plan")
	if err != nil {
		t.Fatal(err)
	}
	if !equal(plan["imports"], Array{Object{"candidate_ref": accepted, "decision_ref": decision["case_decision_ref"]}}) ||
		len(plan["cases"].(Array)) != 1 || plan["cases"].(Array)[0].(Object)["name"] != libraryFindingID {
		t.Fatalf("plan %s", Encode(plan))
	}
	// A plan with no import has no imports field.
	first, err := w.view().Get(w.plan, "plan")
	if _, has := first["imports"]; err != nil || has {
		t.Fatalf("first plan %v, %s", err, Encode(first))
	}
	_, err = w.do("reviewer", "reviewer", w.runner, "plan", Object{"source_ref": w.source, "cases": Array{}, "origin": "independent", "imports": Array{}})
	wantFault(t, "no case", err, "invalid_plan")
}
