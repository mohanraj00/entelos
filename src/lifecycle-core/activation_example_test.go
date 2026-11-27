package lifecycle

import (
	"crypto/rand"
	"fmt"
)

// printPublisher is a consumer that verifies each signed activation record
// and each signed suspension record offline, and prints some of its fields.
type printPublisher struct{ keys *KeySet }

func (p printPublisher) Publish(record, signed []byte) error {
	// The record kind selects the rules. Each verifier refuses the other
	// kind with unexpected_record_kind.
	result, err := VerifySuspension(signed, p.keys)
	if err == nil {
		r := result["record"].(Object)
		fmt.Println("suspended:", r["function"], r["from"], r["until"], "targets:", len(r["targets"].(Array)), "actor:", r["actor"])
		return nil
	}
	if fault, ok := err.(*Fault); !ok || fault.Code != "unexpected_record_kind" {
		return err
	}
	result, err = VerifyActivation(signed, p.keys)
	if err != nil {
		return err
	}
	r := result["record"].(Object)
	fmt.Println("signer:", result["signer"], result["key_status"])
	fmt.Println("candidate:", item(r["candidate"], "kind"), r["function"], r["from"], r["until"])
	fmt.Println("approver:", r["approver"], "runner:", r["runner"], "request:", r["request_key"])
	return nil
}

// The Host activates a synthetic update and publishes its signed activation
// record before the commit. Then the Host suspends the update and
// publishes the signed suspension record. The fixture runner and the fake
// engine are test parts. An embedding host installs its own.
func ExampleHost_SetPublisher() {
	raw, _ := NewSigningKey(rand.Reader)
	signer, _ := NewSigner(raw)
	keys, _ := ParseKeySet(Encode(Object{"format": KeySetFormat, "signer": "example-host", "keys": Array{signer.PublicKey()}}))
	store := NewMemoryStore()
	runner := &fixtureRunner{name: "fixture-a", verdicts: map[string]string{bundleRefOf("pass"): "passed"}}
	host, _ := NewHostWithRunner(store, &fakeEngine{build: engineBuild}, runner, Object{"scope": "demo",
		"runner_actor": "fixture-runner", "worker": "worker-1", "functions": Array{"record.visible"}})
	host.SetSigner(signer)
	host.SetPublisher(printPublisher{keys})
	write := func(actor, role, op string, fields Object) Object {
		root, _ := store.Read(nil)
		q := merge(Object{"op": op, "expected_revision": root.Cursor, "request_key": op + "-1"}, fields)
		out, err := host.HandleAs(actor, []string{role}, q)
		if err != nil {
			panic(err)
		}
		return out.(Object)["result"].(Object)
	}
	source := write("reviewer-a", "reviewer", "source", with(window, Object{"text": "visible rule", "reason": "first", "supersedes": Array{}}))
	plan := write("reviewer-a", "reviewer", "plan", Object{"source_ref": source["source_ref"], "cases": Array{Object{"name": "one"}},
		"origin": "independent", "supersedes": Array{}})
	proposal := write("author-a", "author", "propose", with(window, Object{"base_snapshot": "", "source_ref": source["source_ref"],
		"bundle": bundle("pass"), "reason": "first"}))
	report := write("fixture-runner", "runner", "qualify", Object{"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan["plan_ref"]})
	decision := write("reviewer-a", "reviewer", "decide", Object{"report_ref": report["report_ref"], "reason": "fixture verdict"})
	event := write("reviewer-b", "reviewer", "activate", Object{"decision_ref": decision["decision_ref"], "targets": Array{}})
	write("reviewer-b", "reviewer", "suspend", Object{"function": "record.visible", "from": "2026-12-01", "until": "2027-01-01",
		"targets": Array{event["event_ref"]}, "reason": "stop"})
	// Output:
	// signer: example-host active
	// candidate: cel-bundle record.visible 2026-11-01 2027-01-01
	// approver: reviewer-a runner: fixture-runner request: activate-1
	// suspended: record.visible 2026-12-01 2027-01-01 targets: 1 actor: reviewer-b
}
