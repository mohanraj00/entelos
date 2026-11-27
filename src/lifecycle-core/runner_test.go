package lifecycle

// Tests of the qualification runner contract. They use the in-memory store,
// a Go fake of the CEL runtime for describe and run, and a fixture runner.
// The fixture runner reads its verdicts from a table. It never runs a rule.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fixtureRunner gives the verdict that its table states for a bundle.
type fixtureRunner struct {
	name     string
	verdicts map[string]string
}

// fixtureIdentity is the identity of a fixture runner. The tests compute it
// here, not from the runner.
func fixtureIdentity(name string) string {
	return ContentRef(Envelope(Media("fixture-runner"), []byte(name)))
}

func (r *fixtureRunner) Identity() (string, error) { return fixtureIdentity(r.name), nil }

func (r *fixtureRunner) Expected(Candidate) string { return fixtureIdentity(r.name) }

func (r *fixtureRunner) Qualify(in QualificationInput) (Verdict, error) {
	status, ok := r.verdicts[in.Candidate.BundleRef]
	if !ok {
		return Verdict{}, NewFault("runner_unavailable", "the fixture has no verdict for this bundle", "runner")
	}
	return Verdict{Outcome: Object{"status": status, "bundle_ref": in.Candidate.BundleRef,
		"implementation_ref": fixtureIdentity(r.name), "cases": Int{Text: "1"}}}, nil
}

// fakeEngine is a Go fake of the CEL runtime. It describes and runs a bundle,
// but it refuses check, so that only the fixture runner can qualify.
type fakeEngine struct{ build string }

func (e *fakeEngine) Build() (string, error) { return e.build, nil }

func (e *fakeEngine) Execute(files map[string][]byte, command string, inputs []byte, cases any) (any, error) {
	bundle := ContentRef(Envelope(Mimes["manifest"], files["manifest"]))
	switch command {
	case "describe":
		return Object{"function_id": "record.visible", "bundle_ref": bundle, "implementation_ref": e.build}, nil
	case "run":
		return Object{"status": "ok", "bundle_ref": bundle, "implementation_ref": e.build, "result": true}, nil
	}
	return nil, NewFault("unexpected_command", "the fake engine does not run "+command, "runtime")
}

var engineBuild = "sha256:" + strings.Repeat("e", 64)

// world is one store with one Host for each actor.
type world struct {
	t      *testing.T
	store  Store
	runner Runner
	keys   int
	signer *Signer
	// publisher is the publisher of each Host. Nil publishes nothing.
	publisher Publisher
	// publishTimeout is the time limit of a publisher call. Zero is the
	// default limit.
	publishTimeout time.Duration
	// scope is the Host scope. An empty value is "demo".
	scope string
	// functions are the installed subjects. Nil is record.visible.
	functions Array
	// now is the clock value of each Host. Nil is the system clock.
	now *string
	// context has more Host context fields, such as actor_kinds.
	context Object
	// engine is the runtime of each Host. Nil is the fake engine.
	engine Runtime
}

func newWorld(t *testing.T, runner Runner) *world {
	return &world{t: t, store: NewMemoryStore(), runner: runner}
}

func (w *world) host(actor, role string, runner Runner) *Host {
	w.t.Helper()
	scope := w.scope
	if scope == "" {
		scope = "demo"
	}
	functions := w.functions
	if functions == nil {
		functions = Array{"record.visible"}
	}
	context := Object{"actor": actor, "roles": Array{role}, "scope": scope, "runner_actor": "fixture-runner",
		"worker": "worker-1", "functions": functions}
	for k, x := range w.context {
		context[k] = x
	}
	var engine Runtime = &fakeEngine{build: engineBuild}
	if w.engine != nil {
		engine = w.engine
	}
	h, err := NewHostWithRunner(w.store, engine, runner, context)
	if err != nil {
		w.t.Fatal(err)
	}
	h.SetSigner(w.signer)
	if w.now != nil {
		h.Clock = fixedClock{w.now}
	}
	h.SetPublisher(w.publisher)
	h.SetPublishTimeout(w.publishTimeout)
	return h
}

// do sends one write request and gives the result or the error.
func (w *world) do(actor, role string, runner Runner, op string, fields Object) (Object, error) {
	w.t.Helper()
	root, err := w.store.Read(nil)
	if err != nil {
		w.t.Fatal(err)
	}
	w.keys++
	q := Object{"op": op, "expected_revision": root.Cursor, "request_key": op + "-" + string(rune('a'+w.keys))}
	for k, v := range fields {
		q[k] = v
	}
	out, err := w.host(actor, role, runner).Handle(q)
	if err != nil {
		return nil, err
	}
	return out.(Object)["result"].(Object), nil
}

func (w *world) must(actor, role, op string, fields Object) Object {
	w.t.Helper()
	out, err := w.do(actor, role, w.runner, op, fields)
	if err != nil {
		w.t.Fatalf("%s: %v", op, err)
	}
	return out
}

var window = Object{"function": "record.visible", "from": "2026-11-01", "until": "2027-01-01"}

func with(base Object, extra Object) Object {
	return merge(copyObject(base), extra)
}

// manifest holds the contract fields that a replacement must keep.
func manifest(rule string) string {
	return `{"caller_variables":[],"environment_ref":"env","limits":{},"profile":"p","rule":"` + rule + `"}`
}

func bundle(rule string) Object {
	return Object{"manifest": manifest(rule), "environment": "{}", "source": rule, "fixed": "{}"}
}

// candidate makes a source, a plan and a snapshot. It gives the snapshot
// reference, the plan reference and the bundle reference.
func (w *world) candidate(rule string) (string, string, string) {
	source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))["source_ref"]
	plan := w.must("reviewer", "reviewer", "plan", Object{"source_ref": source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"]
	proposal := w.must("author", "author", "propose", with(window, Object{"base_snapshot": "", "source_ref": source, "bundle": bundle(rule), "reason": "first"}))
	return proposal["snapshot_ref"].(string), plan.(string), proposal["bundle_ref"].(string)
}

func faultCode(t *testing.T, err error) string {
	t.Helper()
	f, ok := err.(*Fault)
	if !ok {
		t.Fatalf("got %v, want a fault", err)
	}
	return f.Code
}

// The manifest bytes give the bundle identity, so the test states it.
func bundleRefOf(rule string) string {
	return ContentRef(Envelope(Mimes["manifest"], []byte(manifest(rule))))
}

// A fixture runner qualifies a candidate through the same lifecycle path as
// CEL: source, plan, propose, qualify, decide, activate, load and invoke.
func TestFixtureRunnerCompletesTheLifecyclePath(t *testing.T) {
	passing := bundleRefOf("pass")
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed"}})
	snapshot, plan, bundleRef := w.candidate("pass")
	if bundleRef != passing {
		t.Fatalf("bundle %s, want %s", bundleRef, passing)
	}
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	report := qualified["report"].(Object)
	if report["status"] != "passed" || report["engine_build"] != fixtureIdentity("fixture-a") {
		t.Fatalf("report %v", report)
	}
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": qualified["report_ref"], "reason": "fixture verdict"})["decision_ref"]
	event := w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})["event_ref"]
	loaded := w.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
	if loaded["event_ref"] != event || loaded["snapshot_ref"] != snapshot {
		t.Fatalf("load %v", loaded)
	}
	answer := w.must("caller", "caller", "invoke", Object{"function": "record.visible", "date": "2026-11-15",
		"expected_bundle_ref": passing, "inputs": Object{"state": "ready"}})
	receipt := answer["receipt"].(Object)
	if receipt["actual_bundle"] != passing || receipt["engine_build"] != engineBuild {
		t.Fatalf("receipt %v", receipt)
	}
	if receipt["selection"].(Object)["report_ref"] != qualified["report_ref"] {
		t.Fatalf("receipt does not name the fixture report: %v", receipt)
	}
}

func TestGitVersionChangeKeepsTheQualifiedBuild(t *testing.T) {
	setGitVersion := func(version string) {
		t.Helper()
		dir := t.TempDir()
		git := `#!/bin/sh
[ "$1" = --version ] || exit 1
printf 'git version %s\n' '` + version + `'
`
		if err := os.WriteFile(filepath.Join(dir, "git"), []byte(git), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
	}
	setGitVersion("2.54.0")

	bundleRef := bundleRefOf("pass")
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRef: "passed"}})
	snapshot, plan, _ := w.candidate("pass")
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	reportRef := qualified["report_ref"]
	build := qualified["report"].(Object)["lifecycle_build"]
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": reportRef, "reason": "fixture verdict"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})
	target := Object{"function": "record.visible", "date": "2026-11-15"}
	w.must("worker", "worker", "load", target)
	inputs := with(target, Object{"expected_bundle_ref": bundleRef, "inputs": Object{"state": "ready"}})

	caller := w.host("caller", "caller", w.runner)
	before := w.must("caller", "caller", "invoke", inputs)
	setGitVersion("2.55.0")
	root, err := w.store.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	sameHost, err := caller.Handle(with(inputs, Object{"op": "invoke", "expected_revision": root.Cursor, "request_key": "invoke-same-host"}))
	if err != nil {
		t.Fatal(err)
	}
	after := w.must("caller", "caller", "invoke", inputs)
	w.must("worker", "worker", "load", target)
	replay := w.must("reader", "reader", "replay", Object{"receipt_ref": before["receipt_ref"], "reason": "Git upgrade"})
	if replay["receipt"].(Object)["matches_original"] != true {
		t.Fatalf("replay: %v", replay)
	}

	for _, check := range []struct {
		result  Object
		version string
	}{
		{before, "2.54.0"},
		{sameHost.(Object)["result"].(Object), "2.54.0"},
		{after, "2.55.0"},
		{replay, "2.55.0"},
	} {
		receipt := check.result["receipt"].(Object)
		wantTools := Object{"go": runtime.Version(), "git": "git version " + check.version}
		if receipt["lifecycle_build"] != build || receipt["selection"].(Object)["report_ref"] != reportRef ||
			!equal(receipt["host_tools"], wantTools) {
			t.Fatalf("receipt for Git %s: %v", check.version, receipt)
		}
	}
}

// The fixture states a failed verdict for one bundle and no verdict for
// another. The Host records failed and unresolved, and admission refuses both.
func TestFixtureRunnerFailsAsItsFixtureSays(t *testing.T) {
	cases := []struct {
		rule, verdict, status, diagnostic string
	}{
		{"fail", "failed", "failed", ""},
		{"absent", "", "unresolved", "runner_unavailable"},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			verdicts := map[string]string{}
			if c.verdict != "" {
				verdicts[bundleRefOf(c.rule)] = c.verdict
			}
			w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: verdicts})
			snapshot, plan, _ := w.candidate(c.rule)
			qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
			report := qualified["report"].(Object)
			if report["status"] != c.status {
				t.Fatalf("status %v, want %s", report["status"], c.status)
			}
			if c.diagnostic != "" && lookupAny(lookupAny(report["diagnostic"], "diagnostic"), "code") != c.diagnostic {
				t.Fatalf("diagnostic %v, want %s", report["diagnostic"], c.diagnostic)
			}
			_, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": qualified["report_ref"], "reason": "must refuse"})
			if got := faultCode(t, err); got != "qualification_failed" {
				t.Fatalf("decide gave %s", got)
			}
		})
	}
}

// A verdict that names another bundle or another runner does not pass, also
// if its status is "passed". The Host sets the status, not the runner.
type lyingRunner struct{ fixtureRunner }

func (r *lyingRunner) Qualify(in QualificationInput) (Verdict, error) {
	return Verdict{Outcome: Object{"status": "passed", "bundle_ref": bundleRefOf("other"), "implementation_ref": fixtureIdentity(r.name)}}, nil
}

func TestHostDoesNotTrustTheRunnerStatus(t *testing.T) {
	w := newWorld(t, &lyingRunner{fixtureRunner{name: "fixture-a"}})
	snapshot, plan, _ := w.candidate("pass")
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report"].(Object)
	if report["status"] != "failed" {
		t.Fatalf("status %v, want failed", report["status"])
	}
}

// The runner identity is part of the report. The same candidate and plan
// with a different runner identity give a different qualification.
func TestRunnerIdentityIsPartOfTheQualification(t *testing.T) {
	passing := bundleRefOf("pass")
	a := &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed"}}
	b := &fixtureRunner{name: "fixture-b", verdicts: map[string]string{passing: "passed"}}
	w := newWorld(t, a)
	snapshot, plan, _ := w.candidate("pass")
	first, err := w.do("fixture-runner", "runner", a, "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.do("fixture-runner", "runner", b, "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	if err != nil {
		t.Fatal(err)
	}
	if first["report"].(Object)["engine_build"] != fixtureIdentity("fixture-a") || second["report"].(Object)["engine_build"] != fixtureIdentity("fixture-b") {
		t.Fatalf("identities %v %v", first["report"], second["report"])
	}
	if first["report_ref"] == second["report_ref"] {
		t.Fatal("two runner identities gave one qualification")
	}
	// Each report is admissible only on a host with its own runner.
	if _, err := w.do("reviewer", "reviewer", b, "decide", Object{"report_ref": first["report_ref"], "reason": "other runner"}); faultCode(t, err) != "implementation_unavailable" {
		t.Fatalf("decide gave %v", err)
	}
	if _, err := w.do("reviewer", "reviewer", b, "decide", Object{"report_ref": second["report_ref"], "reason": "same runner"}); err != nil {
		t.Fatal(err)
	}
}

// After qualification, the installed runner identity changes. Admission
// refuses the old report with the existing build fault, for decide and for
// activate.
func TestAdmissionRefusesAChangedRunnerIdentity(t *testing.T) {
	passing := bundleRefOf("pass")
	a := &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed"}}
	changed := &fixtureRunner{name: "fixture-a-changed", verdicts: map[string]string{passing: "passed"}}
	w := newWorld(t, a)
	snapshot, plan, _ := w.candidate("pass")
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report_ref"]
	if _, err := w.do("reviewer", "reviewer", changed, "decide", Object{"report_ref": report, "reason": "changed runner"}); faultCode(t, err) != "implementation_unavailable" {
		t.Fatalf("decide gave %v", err)
	}
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": report, "reason": "same runner"})["decision_ref"]
	if _, err := w.do("reviewer", "reviewer", changed, "activate", Object{"decision_ref": decision, "targets": Array{}}); faultCode(t, err) != "implementation_unavailable" {
		t.Fatalf("activate gave %v", err)
	}
}

// After qualification, the candidate bytes change: a new proposal stores
// other bytes. A report that carries the passed flag onto the new snapshot
// is refused with the existing qualification fault. The old report still
// admits only the old snapshot.
func TestAdmissionRefusesAPassedFlagOnChangedBytes(t *testing.T) {
	passing := bundleRefOf("pass")
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed"}})
	snapshot, plan, _ := w.candidate("pass")
	qualified := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})
	reportRef := qualified["report_ref"].(string)
	report := qualified["report"].(Object)
	source := report["source_ref"]
	changed := w.must("author", "author", "propose", with(window, Object{"base_snapshot": snapshot, "source_ref": source, "bundle": bundle("changed"), "reason": "new bytes"}))
	if changed["bundle_ref"] == passing {
		t.Fatal("the changed bytes kept the bundle identity")
	}
	// Deliberate trusted-store corruption: copy the passed report onto the
	// new snapshot. The store accepts the record; admission must not.
	moved := copyObject(report)
	moved["snapshot_ref"] = changed["snapshot_ref"]
	var movedRef string
	latest, _ := w.store.Read(nil)
	if _, err := w.store.Transact(latest.Cursor, "move-passed-flag", Object{"op": "test-corruption"}, func(v *View) (any, error) {
		movedRef = v.Put("qualification", moved)
		return Object{"report_ref": movedRef}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": movedRef, "reason": "moved flag"}); faultCode(t, err) != "qualification_failed" {
		t.Fatalf("decide gave %v", err)
	}
	// The head has other bytes, so a verdict on the old bytes is stale
	// (VERDICT.md). A proposal of the old bytes makes it possible again.
	if _, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": reportRef, "reason": "old bytes"}); faultCode(t, err) != "stale_verdict" {
		t.Fatalf("decide on the old bytes gave %v", err)
	}
	w.must("author", "author", "propose", with(window, Object{"base_snapshot": changed["snapshot_ref"], "source_ref": source, "bundle": bundle("pass"), "reason": "old bytes again"}))
	// The honest report still binds the old bytes only.
	decision := w.must("reviewer", "reviewer", "decide", Object{"report_ref": reportRef, "reason": "old bytes"})["decision_ref"]
	w.must("reviewer", "reviewer", "activate", Object{"decision_ref": decision, "targets": Array{}})
	loaded := w.must("worker", "worker", "load", Object{"function": "record.visible", "date": "2026-11-15"})
	if loaded["snapshot_ref"] != snapshot || loaded["bundle_ref"] != passing {
		t.Fatalf("load %v, want the qualified snapshot %s", loaded, snapshot)
	}
}

// A report record whose runner identity changed after qualification, while
// the installed runner stays the same, is refused with the build fault.
func TestAdmissionRefusesAChangedReportIdentity(t *testing.T) {
	passing := bundleRefOf("pass")
	w := newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: map[string]string{passing: "passed"}})
	snapshot, plan, _ := w.candidate("pass")
	report := w.must("fixture-runner", "runner", "qualify", Object{"snapshot_ref": snapshot, "plan_ref": plan})["report"].(Object)
	forged := copyObject(report)
	forged["engine_build"] = fixtureIdentity("fixture-b")
	var forgedRef string
	latest, _ := w.store.Read(nil)
	if _, err := w.store.Transact(latest.Cursor, "change-identity", Object{"op": "test-corruption"}, func(v *View) (any, error) {
		forgedRef = v.Put("qualification", forged)
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": forgedRef, "reason": "changed identity"}); faultCode(t, err) != "implementation_unavailable" {
		t.Fatalf("decide gave %v", err)
	}
}

// The CEL runner is the CEL executable through the contract. The script
// states the build identity and the check report, so they are the oracle.
func TestCELRunnerUsesTheCheckCommandAndTheBuild(t *testing.T) {
	ref := "sha256:" + strings.Repeat("c", 64)
	rt := fakeRuntime(t, `case "$*" in
  -build) echo '{"implementation_ref":"`+ref+`"}' ;;
  *check) echo '{"status":"failed","bundle_ref":"b","implementation_ref":"`+ref+`"}'; exit 1 ;;
  *) exit 9 ;;
esac`)
	runner := &CELRunner{Runtime: rt}
	if got, err := runner.Identity(); err != nil || got != ref {
		t.Fatalf("identity %v %v", got, err)
	}
	if got := runner.Expected(Candidate{EngineBuild: "sha256:snapshot"}); got != "sha256:snapshot" {
		t.Fatalf("expected %s", got)
	}
	verdict, err := runner.Qualify(QualificationInput{Files: emptyFiles, Cases: Array{}})
	if err != nil || lookupAny(verdict.Outcome, "status") != "failed" || lookupAny(verdict.Outcome, "bundle_ref") != "b" {
		t.Fatalf("verdict %v %v", verdict, err)
	}
}

func TestHostRequiresARunner(t *testing.T) {
	context := Object{"actor": "a", "roles": Array{"reader"}, "scope": "demo", "runner_actor": "r", "worker": "w", "functions": Array{"f"}}
	if _, err := NewHostWithRunner(NewMemoryStore(), &fakeEngine{build: engineBuild}, nil, context); faultCode(t, err) != "configuration_error" {
		t.Fatal(err)
	}
}

func TestHostRequiresGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	context := Object{"actor": "a", "roles": Array{"reader"}, "scope": "demo", "runner_actor": "r", "worker": "w", "functions": Array{"f"}}
	if _, err := NewHost(NewMemoryStore(), &fakeEngine{build: engineBuild}, context); faultCode(t, err) != "configuration_error" {
		t.Fatal(err)
	}
}
