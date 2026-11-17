package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// verdictRun runs the steps of one verdict vector on a fresh memory store.
// The clock moves one minute for each step, so submitted_at gives the
// proposal order.
type verdictRun struct {
	t         *testing.T
	w         *world
	verdicts  map[string]string
	source    string
	plan      string
	head      string
	clock     time.Time
	now       string
	snapshots map[string]string
	reports   map[string]any
	decisions map[string]string
	cursors   map[string]string
}

func newVerdictRun(t *testing.T) *verdictRun {
	r := &verdictRun{t: t, verdicts: map[string]string{}, clock: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC),
		snapshots: map[string]string{}, reports: map[string]any{}, decisions: map[string]string{}, cursors: map[string]string{}}
	r.now = r.clock.Format(time.RFC3339)
	r.w = newWorld(t, &fixtureRunner{name: "fixture-a", verdicts: r.verdicts})
	r.w.now = &r.now
	r.source = r.w.must("reviewer", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))["source_ref"].(string)
	r.plan = r.w.must("reviewer", "reviewer", "plan", Object{"source_ref": r.source, "cases": Array{Object{"name": "one"}}, "origin": "independent", "supersedes": Array{}})["plan_ref"].(string)
	return r
}

func (r *verdictRun) candidate(content string) Object {
	return Object{"kind": "cel-bundle", "ref": bundleRefOf(content)}
}

// step runs one step. It gives the observed value in the form of the
// expect field.
func (r *verdictRun) step(step Object) Object {
	r.clock = r.clock.Add(time.Minute)
	r.now = r.clock.Format(time.RFC3339)
	w := r.w
	var err error
	switch {
	case step["propose"] != nil:
		content := step["content"].(string)
		r.verdicts[bundleRefOf(content)] = "passed"
		var out Object
		out, err = w.do("author", "author", w.runner, "propose", with(window, Object{"base_snapshot": r.head,
			"source_ref": r.source, "bundle": bundle(content), "reason": "propose " + step["propose"].(string)}))
		if err == nil {
			r.head = out["snapshot_ref"].(string)
			r.snapshots[step["propose"].(string)] = r.head
		}
	case step["qualify"] != nil:
		var out Object
		out, err = w.do("fixture-runner", "runner", w.runner, "qualify", Object{"snapshot_ref": r.snapshots[step["qualify"].(string)], "plan_ref": r.plan})
		if err == nil {
			r.reports[step["qualify"].(string)] = out["report_ref"]
		}
	case step["decide"] != nil:
		actor := step["actor"].(string)
		var out Object
		out, err = w.do(actor, "reviewer", w.runner, "decide", Object{"report_ref": r.reports[step["proposal"].(string)],
			"reason": "verdict " + step["decide"].(string), "disposition": lookup(step, "disposition", "accept")})
		if err == nil {
			r.decisions[step["decide"].(string)] = out["decision_ref"].(string)
		}
	case step["activate"] != nil:
		_, err = w.do("reviewer", "reviewer", w.runner, "activate", Object{"decision_ref": r.decisions[step["activate"].(string)], "targets": Array{}})
	case step["mark"] != nil:
		root, readErr := w.store.Read(nil)
		if readErr != nil {
			r.t.Fatal(readErr)
		}
		r.cursors[step["mark"].(string)] = root.Cursor
	case step["approval"] != nil:
		q := Object{"op": "approval", "function": lookup(step, "function", "record.visible"),
			"candidate": lookup(step, "candidate", r.candidate(step["approval"].(string)))}
		if name, has := step["cursor"]; has {
			q["cursor"] = r.cursors[name.(string)]
		}
		var out any
		out, err = w.host("reader", "reader", w.runner).Handle(q)
		if err == nil {
			return r.answer(out.(Object))
		}
	default:
		r.t.Fatalf("unknown step %v", step)
	}
	if err != nil {
		if fault, ok := err.(*Fault); ok {
			return Object{"error": fault.Code}
		}
		return Object{"error": err.Error()}
	}
	return nil
}

// answer gives an approval result in the form of the vectors: each verdict
// as its decision name and state.
func (r *verdictRun) answer(out Object) Object {
	verdicts := Array{}
	for _, entry := range elements(out["verdicts"]) {
		e := entry.(Object)
		name := "unknown decision"
		for n, ref := range r.decisions {
			if ref == e["decision_ref"] {
				name = n
			}
		}
		// The decision record has no candidate field (VERDICT.md decision 1).
		if decision := e["decision"].(Object); decision["format"] != "entelos.record/1" || decision["candidate"] != nil {
			r.t.Fatalf("decision record %v", decision)
		}
		verdicts = append(verdicts, Array{name, e["state"]})
	}
	return Object{"approved": out["approved"], "state": out["state"], "verdicts": verdicts}
}

// normal gives a JSON value in the form that encoding/json reads.
func normal(t *testing.T, value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The verdict vectors state each expected value from VERDICT.md.
func TestVerdictVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "verdict", "vectors", "verdict-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name  string   `json:"name"`
			Steps []Object `json:"steps"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Cases) == 0 {
		t.Fatal("no verdict cases")
	}
	for _, c := range vectors.Cases {
		t.Run(c.Name, func(t *testing.T) {
			r := newVerdictRun(t)
			for i, step := range c.Steps {
				got := normal(t, r.step(step))
				want := normal(t, step["expect"])
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("step %d %v: got %v, want %v", i+1, step, got, want)
				}
			}
		})
	}
}

// After an edit, the history keeps each stale verdict.
func TestStaleVerdictStaysInTheHistory(t *testing.T) {
	r := newVerdictRun(t)
	for _, step := range []Object{
		{"propose": "p1", "content": "a"}, {"qualify": "p1"}, {"decide": "d1", "proposal": "p1", "actor": "reviewer-a"},
		{"propose": "p2", "content": "b"},
	} {
		if got := r.step(step); got != nil {
			t.Fatalf("%v: %v", step, got)
		}
	}
	out, err := r.w.host("reviewer", "reviewer", r.w.runner).Handle(Object{"op": "proposal", "snapshot_ref": r.snapshots["p1"]})
	if err != nil {
		t.Fatal(err)
	}
	decisions := elements(out.(Object)["decisions"])
	if len(decisions) != 1 || item(decisions[0], "decision_ref") != r.decisions["d1"] {
		t.Fatalf("decisions %v, want the stale verdict %s", decisions, r.decisions["d1"])
	}
}
