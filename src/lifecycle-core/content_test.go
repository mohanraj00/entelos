package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contentVectors(t *testing.T, name string) Object {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "content", "vectors", name))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return doc.(Object)
}

// expandMarkers replaces each text marker of CONTENT.md with its text.
func expandMarkers(value any) any {
	switch x := value.(type) {
	case Object:
		if pad, ok := x["$pad"].(Object); ok && len(x) == 1 {
			text, count := pad["text"].(string), intOf(pad["count"])
			return text[:len(text)-1] + strings.Repeat(" ", count) + text[len(text)-1:]
		}
		if items, ok := x["$items"].(Object); ok && len(x) == 1 {
			copies := make([]string, intOf(items["count"]))
			for i := range copies {
				copies[i] = strings.ReplaceAll(items["item"].(string), "{i}", fmt.Sprintf("%04d", i))
			}
			return strings.ReplaceAll(items["text"].(string), "$ITEMS", strings.Join(copies, ","))
		}
		out := Object{}
		for k, v := range x {
			out[k] = expandMarkers(v)
		}
		return out
	case Array:
		out := Array{}
		for _, v := range x {
			out = append(out, expandMarkers(v))
		}
		return out
	}
	return value
}

func intOf(value any) int {
	var n int
	if _, err := fmt.Sscan(value.(Int).Text, &n); err != nil {
		panic(err)
	}
	return n
}

// The Host rules give the result that a person wrote for each case of the
// content vectors.
func TestContentCandidateVectors(t *testing.T) {
	doc := contentVectors(t, "content-cases.json")
	for _, x := range doc["cases"].(Array) {
		c := x.(Object)
		got := judgeCandidate(expandMarkers(c["candidate"])).Object()
		if !equal(got, c["expect"]) {
			t.Errorf("%s: got %s, want %s", c["name"], Encode(got), Encode(c["expect"]))
		}
	}
}

// contentRunner is a fixture runner of an application. It checks that the
// Host gave it the exact manifest bytes of the candidate, then gives the
// verdict of its table. A verdict can name another candidate, as a runner
// that qualified other bytes does.
type contentRunner struct {
	status map[string]string
	names  map[string]string
}

func (r *contentRunner) Identity() (string, error) { return fixtureIdentity("content-fixture"), nil }

func (r *contentRunner) Expected(Candidate) string { return fixtureIdentity("content-fixture") }

func (r *contentRunner) Qualify(in QualificationInput) (Verdict, error) {
	text, ok := in.Files[contentFile]
	sum := sha256.Sum256(text)
	if !ok || len(in.Files) != 1 || "sha256:"+hex.EncodeToString(sum[:]) != in.Candidate.BundleRef {
		return Verdict{}, NewFault("runner_unavailable", "the files are not the manifest of the candidate", "runner")
	}
	ref := in.Candidate.BundleRef
	return Verdict{Outcome: Object{"status": r.status[ref], "bundle_ref": r.names[ref],
		"implementation_ref": fixtureIdentity("content-fixture")}}, nil
}

type contentRun struct {
	t         *testing.T
	w         *world
	runner    *contentRunner
	subject   string
	manifests Object
	source    string
	plan      string
	snapshots map[string]string
	reports   map[string]string
	decisions map[string]string
	events    map[string]string
	cursors   map[string]string
}

func newContentRun(t *testing.T, doc Object) *contentRun {
	runner := &contentRunner{status: map[string]string{}, names: map[string]string{}}
	w := newWorld(t, runner)
	w.functions = doc["subjects"].(Array)
	w.signer, _ = newTestSigner(t)
	r := &contentRun{t: t, w: w, runner: runner, subject: w.functions[0].(string), manifests: doc["manifests"].(Object),
		snapshots: map[string]string{}, reports: map[string]string{}, decisions: map[string]string{}, events: map[string]string{},
		cursors: map[string]string{}}
	wide := Object{"function": r.subject, "from": "2025-01-01", "until": "2029-01-01"}
	r.source = w.must("reviewer", "reviewer", "source", with(wide, Object{"text": "the content rules", "reason": "first", "supersedes": Array{}}))["source_ref"].(string)
	r.plan = w.must("reviewer", "reviewer", "plan", Object{"source_ref": r.source, "cases": Array{Object{"name": "limits"}}, "origin": "independent",
		"supersedes": Array{}})["plan_ref"].(string)
	return r
}

func (r *contentRun) ref(name any) string { return r.manifests[name.(string)].(Object)["ref"].(string) }

// nameOf gives the manifest name of a candidate reference.
func (r *contentRun) nameOf(ref any) string {
	for name, m := range r.manifests {
		if m.(Object)["ref"] == ref {
			return name
		}
	}
	return "unknown candidate"
}

// failure gives the vector shape of an error. A rule refusal names its rule
// and, for rule unknown_field, its field.
func failure(err error) Object {
	fault, ok := err.(*Fault)
	if !ok {
		return Object{"error": err.Error()}
	}
	out := Object{"error": fault.Code}
	if rest, ok := strings.CutPrefix(fault.Message, "rule "); ok {
		rule, field, has := strings.Cut(rest, ": unknown field ")
		out["rule"] = rule
		if has {
			out["field"] = field
		}
	}
	return out
}

func (r *contentRun) step(step Object) any {
	w, at := r.w, Object{"function": r.subject, "date": step["date"]}
	switch {
	case step["propose"] != nil:
		name := step["propose"].(string)
		manifest := r.manifests[name].(Object)
		out, err := w.do("author", "author", w.runner, "propose", Object{"candidate": Object{"kind": "content", "manifest": manifest["text"]},
			"from": step["from"], "until": step["until"], "source_ref": r.source, "reason": "update " + name})
		if err != nil {
			if _, rule := step["expect"].(Object)["rule"]; !rule {
				return Object{"error": failure(err)["error"]}
			}
			return failure(err)
		}
		if !equal(out["candidate"], Object{"kind": "content", "ref": manifest["ref"]}) {
			r.t.Errorf("propose %s: candidate %s", name, Encode(out["candidate"]))
		}
		r.snapshots[name] = out["snapshot_ref"].(string)
		return name
	case step["qualify"] != nil:
		name := step["qualify"].(string)
		ref := r.ref(name)
		r.runner.status[ref], r.runner.names[ref] = step["verdict"].(string), r.ref(lookup(step, "runner_names", name))
		out, err := w.do("fixture-runner", "runner", w.runner, "qualify", Object{"snapshot_ref": r.snapshots[name], "plan_ref": r.plan})
		if err != nil {
			return failure(err)
		}
		report := out["report"].(Object)
		if !equal(report["candidate"], Object{"kind": "content", "ref": ref}) {
			r.t.Errorf("qualify %s: the report names %s", name, Encode(report["candidate"]))
		}
		r.reports[name] = out["report_ref"].(string)
		return report["status"]
	case step["decide"] != nil:
		name := step["decide"].(string)
		out, err := w.do("reviewer", "reviewer", w.runner, "decide", Object{"report_ref": r.reports[name], "reason": "accept " + name})
		if err != nil {
			return failure(err)
		}
		r.decisions[name] = out["decision_ref"].(string)
		return "accept"
	case step["activate"] != nil:
		name := step["activate"].(string)
		out, err := w.do("reviewer", "reviewer", w.runner, "activate", Object{"decision_ref": r.decisions[name], "targets": Array{}})
		if err != nil {
			return failure(err)
		}
		r.events[name] = out["event_ref"].(string)
		return name
	case step["mark"] != nil:
		root, err := w.store.Read(nil)
		if err != nil {
			r.t.Fatal(err)
		}
		r.cursors[step["mark"].(string)] = root.Cursor
		return nil
	case step["state"] != nil:
		out, err := w.read(Object{"op": "proposal", "snapshot_ref": r.snapshots[step["state"].(string)]})
		if err != nil {
			return failure(err)
		}
		return out["proposal"].(Object)["state"]
	case step["resolve"] != nil:
		q := Object{"op": "resolve", "function": r.subject, "date": step["resolve"]}
		if mark, has := step["cursor"]; has {
			q["cursor"] = r.cursors[mark.(string)]
		}
		out, err := w.host("reader", "reader", w.runner).Handle(q)
		if err != nil {
			return failure(err)
		}
		selection := out.(Object)
		name := r.nameOf(lookupAny(selection["candidate"], "ref"))
		if selection["bundle_ref"] != r.ref(name) || selection["snapshot_ref"] != r.snapshots[name] || selection["event_ref"] != r.events[name] ||
			selection["collection"] != "loan-rules" || selection["partition"] != "region-a" {
			r.t.Errorf("resolve %s: selection %s", step["resolve"], Encode(selection))
		}
		return name
	case step["load"] != nil:
		at["date"] = step["load"]
		_, err := w.do("worker", "worker", w.runner, "load", at)
		return failure(err)
	case step["invoke"] != nil:
		at["date"] = step["invoke"]
		_, err := w.do("caller", "caller", w.runner, "invoke", with(at, Object{"expected_bundle_ref": r.ref("first"), "inputs": Object{}}))
		return failure(err)
	case step["describe"] != nil:
		_, err := w.host("caller", "caller", w.runner).Handle(Object{"op": "describe", "function": r.subject, "date": step["describe"]})
		return failure(err)
	}
	r.t.Fatalf("unknown step %s", Encode(step))
	return nil
}

// The Host gives the stated result of each step of the content lifecycle
// vectors, from propose to activate and to a dated read at an earlier root.
// Each activation record is also a signed record.
func TestContentLifecycleVectors(t *testing.T) {
	doc := contentVectors(t, "lifecycle-cases.json")
	for _, x := range doc["cases"].(Array) {
		c := x.(Object)
		t.Run(c["name"].(string), func(t *testing.T) {
			r := newContentRun(t, doc)
			for i, y := range c["steps"].(Array) {
				step := y.(Object)
				got := r.step(step)
				if want, has := step["expect"]; has && !equal(got, want) {
					t.Errorf("step %d %s: got %s", i, Encode(step), Encode(got))
				}
			}
			index := r.w.signatures()
			for name, ref := range r.events {
				if _, signed := index[ref]; !signed {
					t.Errorf("the event of %s has no signed record", name)
				}
			}
		})
	}
}

const loanSubject = "content:loan-rules/region-a"

func contentWorld(t *testing.T, runner Runner) (*world, string) {
	w := newWorld(t, runner)
	w.functions = Array{loanSubject, "record.visible"}
	source := w.must("reviewer", "reviewer", "source", Object{"function": loanSubject, "from": "2026-01-01", "until": "2027-01-01",
		"text": "the content rules", "reason": "first", "supersedes": Array{}})["source_ref"].(string)
	return w, source
}

// propose sends each candidate of the content vectors to the Host. A refusal
// has the code and the rule of the vector. A valid content candidate of the
// installed head with an empty base gives the stated reference.
func TestHostProposeAppliesTheContentVectors(t *testing.T) {
	doc := contentVectors(t, "content-cases.json")
	for _, x := range doc["cases"].(Array) {
		c := x.(Object)
		expect := c["expect"].(Object)
		candidate := expandMarkers(c["candidate"])
		if expect["valid"] == true && (expect["kind"] != "content" || strings.Contains(candidate.(Object)["manifest"].(string), `"base":"sha256:`)) {
			continue
		}
		t.Run(c["name"].(string), func(t *testing.T) {
			w, source := contentWorld(t, &contentRunner{})
			out, err := w.do("author", "author", w.runner, "propose", Object{"candidate": candidate, "from": "2026-01-01", "until": "2027-01-01",
				"source_ref": source, "reason": "vector"})
			if expect["valid"] == true {
				if err != nil || !equal(out["candidate"], Object{"kind": "content", "ref": expect["ref"]}) || !equal(out["head"], expect["head"]) {
					t.Fatalf("got %v %v", out, err)
				}
				return
			}
			want := Object{"error": expect["code"]}
			for _, k := range []string{"rule", "field"} {
				if v, has := expect[k]; has {
					want[k] = v
				}
			}
			if got := failure(err); !equal(got, want) {
				t.Fatalf("got %s, want %s", Encode(got), Encode(want))
			}
			root, _ := w.store.Read(nil)
			if snapshots := lookup(root.State, "heads", Object{}).(Object); len(snapshots) != 0 {
				t.Fatalf("a refusal changed the heads: %s", Encode(snapshots))
			}
		})
	}
}

func firstManifest(t *testing.T) string {
	return contentVectors(t, "lifecycle-cases.json")["manifests"].(Object)["first"].(Object)["text"].(string)
}

func TestContentProposeRefusesMixedBodies(t *testing.T) {
	w, source := contentWorld(t, &contentRunner{})
	content := Object{"kind": "content", "manifest": firstManifest(t)}
	body := Object{"candidate": content, "from": "2026-01-01", "until": "2027-01-01", "source_ref": source, "reason": "r"}
	for name, extra := range map[string]Object{
		"bundle":        {"bundle": bundle("x")},
		"function":      {"function": loanSubject},
		"base_snapshot": {"base_snapshot": ""},
	} {
		if _, err := w.do("author", "author", w.runner, "propose", with(body, extra)); faultCode(t, err) != "invalid_request" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	// A CEL bundle never takes a content subject.
	cel := Object{"function": loanSubject, "from": "2026-01-01", "until": "2027-01-01", "base_snapshot": "", "source_ref": source,
		"bundle": bundle("x"), "reason": "r"}
	if _, err := w.do("author", "author", w.runner, "propose", cel); faultCode(t, err) != "invalid_request" {
		t.Errorf("CEL bundle on a content subject: got %v", err)
	}
}

// A cel-bundle candidate gives the snapshot of the 1.0 bundle field.
func TestCELCandidateIsTheBundleField(t *testing.T) {
	var snapshots []Object
	for _, useCandidate := range []bool{false, true} {
		w := newWorld(t, &fixtureRunner{name: "fixture-a"})
		source := w.must("reviewer", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))["source_ref"]
		body := with(window, Object{"base_snapshot": "", "source_ref": source, "reason": "first"})
		if useCandidate {
			body["candidate"] = Object{"kind": "cel-bundle", "bundle": bundle("r")}
		} else {
			body["bundle"] = bundle("r")
		}
		ref := w.must("author", "author", "propose", body)["snapshot_ref"]
		snapshot, err := w.read(Object{"op": "record", "kind": "snapshot", "ref": ref})
		if err != nil {
			t.Fatal(err)
		}
		delete(snapshot, "submitted_at")
		snapshots = append(snapshots, snapshot)
	}
	if !equal(snapshots[0], snapshots[1]) {
		t.Fatalf("snapshots differ: %s %s", Encode(snapshots[0]), Encode(snapshots[1]))
	}
}

// The CEL runner does not qualify a content candidate. The report is
// unresolved, with a structured diagnostic.
func TestCELRunnerRefusesContent(t *testing.T) {
	_, err := (&CELRunner{Runtime: &fakeEngine{build: engineBuild}}).Qualify(QualificationInput{Files: map[string][]byte{contentFile: []byte("{}")}})
	if faultCode(t, err) != "unsupported_candidate_kind" {
		t.Fatalf("got %v", err)
	}
}

// A retry of a content proposal checks access to its head again.
func TestContentProposeRetryChecksTheHead(t *testing.T) {
	w, source := contentWorld(t, &contentRunner{})
	root, _ := w.store.Read(nil)
	q := Object{"op": "propose", "expected_revision": root.Cursor, "request_key": "content-once", "from": "2026-01-01", "until": "2027-01-01",
		"candidate": Object{"kind": "content", "manifest": firstManifest(t)}, "source_ref": source, "reason": "r"}
	if _, err := w.host("author", "author", w.runner).Handle(q); err != nil {
		t.Fatal(err)
	}
	out, err := w.host("author", "author", w.runner).Handle(q)
	if err != nil || out.(Object)["reused"] != true {
		t.Fatalf("retry: %v %v", out, err)
	}
	w.functions = Array{"record.visible"}
	if _, err := w.host("author", "author", w.runner).Handle(q); faultCode(t, err) != "out_of_scope" {
		t.Fatalf("retry without the head: got %v", err)
	}
}

// The approval API lists a content proposal. The proposal read gives the
// candidate, the evidence reference and the exact manifest text.
func TestApprovalAPIListsAContentProposal(t *testing.T) {
	doc := contentVectors(t, "lifecycle-cases.json")
	manifests := doc["manifests"].(Object)
	w, source := contentWorld(t, &contentRunner{})
	first := manifests["first"].(Object)
	w.must("author", "author", "propose", Object{"candidate": Object{"kind": "content", "manifest": first["text"]}, "from": "2026-01-01",
		"until": "2027-01-01", "source_ref": source, "reason": "first"})
	second := manifests["second"].(Object)
	ref := w.must("author", "author", "propose", Object{"candidate": Object{"kind": "content", "manifest": second["text"]}, "from": "2026-01-01",
		"until": "2027-01-01", "source_ref": source, "reason": "second"})["snapshot_ref"]
	list, err := w.read(Object{"op": "proposals"})
	if err != nil {
		t.Fatal(err)
	}
	items := list["proposals"].(Array)
	if len(items) != 1 || item(items[0], "function") != loanSubject || item(items[0], "bundle_ref") != second["ref"] ||
		item(items[0], "snapshot_ref") != ref {
		t.Fatalf("proposals: %s", Encode(list))
	}
	one, err := w.read(Object{"op": "proposal", "snapshot_ref": ref})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := one["snapshot"].(Object)
	evidence := Object{"format": "entelos.evidence/1", "digest": "sha256:" + strings.Repeat("c", 64)}
	if !equal(snapshot["candidate"], Object{"kind": "content", "ref": second["ref"]}) || !equal(snapshot["evidence_reference"], evidence) ||
		snapshot["base"] != first["ref"] || !equal(one["bundle"], Object{"content": second["text"]}) {
		t.Fatalf("proposal: %s", Encode(one))
	}
}

// A content proposal keeps the evidence document of the request, as a CEL
// proposal does, and the proposal read gives it.
func TestContentProposalKeepsTheEvidenceDocument(t *testing.T) {
	raw, err := os.ReadFile("../protocol/approval/vectors/approval-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	doc := must(Decode(raw)).(Object)
	evidence := doc["steps"].(Array)[3].(Object)["request"].(Object)["body"].(Object)["evidence"]
	first := contentVectors(t, "lifecycle-cases.json")["manifests"].(Object)["first"].(Object)
	w, source := contentWorld(t, &contentRunner{})
	ref := w.must("author", "author", "propose", Object{"candidate": Object{"kind": "content", "manifest": first["text"]}, "from": "2026-01-01",
		"until": "2027-01-01", "source_ref": source, "reason": "first", "evidence": evidence})["snapshot_ref"]
	one, err := w.read(Object{"op": "proposal", "snapshot_ref": ref})
	if err != nil {
		t.Fatal(err)
	}
	if !equal(one["evidence"], evidence) || !equal(item(one["snapshot"], "evidence"), evidence) {
		t.Fatalf("proposal: %s", Encode(one))
	}
}

// A large integer does not stop the walk for unknown fields. The walk keeps
// each number as text.
func TestUnknownFieldAfterALargeInteger(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	text := `{"format":"entelos.content/1","collection":"c","partition":"p","base":"","items":[{"item_id":"a","digest":"` + digest +
		`","size":1` + strings.Repeat("0", 400) + `}],"owner":"x"}`
	got := judgeCandidate(Object{"kind": "content", "manifest": text}).Object()
	want := Object{"valid": false, "code": "invalid_content", "rule": "unknown_field", "field": "owner"}
	if !equal(got, want) {
		t.Fatalf("got %s", Encode(got))
	}
}
