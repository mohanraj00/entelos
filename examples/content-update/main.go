package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"

	lifecycle "github.com/mohanraj00/entelos/src/lifecycle-core"
)

//go:embed manifest.json expected.json items/* refused/*
var files embed.FS

const subject = "content:product-faq/en"

func run(out io.Writer) error {
	items, err := fs.Sub(files, "items")
	if err != nil {
		return err
	}
	runner := &contentRunner{files: items}
	store := lifecycle.NewMemoryStore()
	context := lifecycle.Object{"scope": "faq-demo", "runner_actor": "file-checker",
		"worker": "unused", "functions": lifecycle.Array{subject}}
	host, err := lifecycle.NewHostWithRunner(store, nil, runner, context)
	if err != nil {
		return err
	}
	key := 0
	write := func(actor, role, op string, fields lifecycle.Object) (lifecycle.Object, error) {
		root, err := store.Read(nil)
		if err != nil {
			return nil, err
		}
		key++
		fields["op"], fields["expected_revision"], fields["request_key"] = op, root.Cursor, fmt.Sprint(key)
		result, err := host.HandleAs(actor, []string{role}, fields)
		if err != nil {
			return nil, err
		}
		return result.(lifecycle.Object)["result"].(lifecycle.Object), nil
	}
	source, err := write("reviewer", "reviewer", "source", lifecycle.Object{
		"function": subject, "from": "2026-11-01", "until": "2026-12-01",
		"text":   "Publish the synthetic product FAQ after file checks and human approval.",
		"reason": "FAQ update", "supersedes": lifecycle.Array{}})
	if err != nil {
		return err
	}
	expected, err := files.ReadFile("expected.json")
	if err != nil {
		return err
	}
	cases, err := lifecycle.Decode(expected)
	if err != nil {
		return err
	}
	plan, err := write("reviewer", "reviewer", "plan", lifecycle.Object{
		"source_ref": source["source_ref"], "cases": cases,
		"origin": "independent", "supersedes": lifecycle.Array{}})
	if err != nil {
		return err
	}
	raw, err := files.ReadFile("manifest.json")
	if err != nil {
		return err
	}
	proposal, err := write("author", "author", "propose", lifecycle.Object{
		"candidate":  lifecycle.Object{"kind": "content", "manifest": string(raw)},
		"source_ref": source["source_ref"], "from": "2026-11-01", "until": "2026-12-01", "reason": "Publish two FAQ items"})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "1. Propose: product-faq/en, two items.")
	// The refused folder has a changed charging file. It has the same size.
	runner.files, err = fs.Sub(files, "refused")
	if err != nil {
		return err
	}
	qualify := func(want string) (lifecycle.Object, error) {
		report, err := write("file-checker", "runner", "qualify", lifecycle.Object{
			"snapshot_ref": proposal["snapshot_ref"], "plan_ref": plan["plan_ref"]})
		if err == nil && report["report"].(lifecycle.Object)["status"] != want {
			err = fmt.Errorf("qualification: want %s, got %v", want, report["report"])
		}
		return report, err
	}
	failed, err := qualify("failed")
	if err != nil {
		return err
	}
	_, err = write("reviewer", "reviewer", "decide", lifecycle.Object{"report_ref": failed["report_ref"], "reason": "Check refusal"})
	if fault, ok := err.(*lifecycle.Fault); !ok || fault.Code != "qualification_failed" {
		return fmt.Errorf("failed qualification must block approval: %v", err)
	}
	fmt.Fprintln(out, "2. Qualify: failed, charging digest differs. Approval refused: qualification_failed.")
	runner.files = items
	report, err := qualify("passed")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "3. Qualify: passed, files agree with the plan and manifest.")
	retained, err := host.HandleAs("reviewer", []string{"reviewer"}, lifecycle.Object{
		"op": "record", "kind": "qualification", "ref": failed["report_ref"]})
	if err != nil {
		return err
	}
	if !bytes.Equal(lifecycle.Encode(retained), lifecycle.Encode(failed["report"])) {
		return fmt.Errorf("the retained failed report differs")
	}
	fmt.Fprintf(out, "   Retained report: %s, status failed.\n", failed["report_ref"])
	decision, err := write("reviewer", "reviewer", "decide", lifecycle.Object{
		"report_ref": report["report_ref"], "disposition": "accept", "reason": "Files checked; approve this FAQ"})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "4. Decide: reviewer accepts the qualified manifest.")
	_, err = write("reviewer", "reviewer", "activate", lifecycle.Object{
		"decision_ref": decision["decision_ref"], "targets": lifecycle.Array{}})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "5. Activate: from 2026-11-01 until 2026-12-01 (end excluded).")
	selection, err := host.HandleAs("reader", []string{"reader"}, lifecycle.Object{"op": "resolve", "function": subject, "date": "2026-11-15"})
	if err != nil {
		return err
	}
	if selection.(lifecycle.Object)["candidate"].(lifecycle.Object)["ref"] != digest(raw) {
		return fmt.Errorf("read selected another manifest: %v", selection)
	}
	fmt.Fprintf(out, "6. Read at 2026-11-15: %s\n", digest(raw))
	// Resolve identifies the manifest. The application reads its item files.
	return printItems(out, items, raw)
}

func printItems(out io.Writer, items fs.FS, raw []byte) error {
	var active manifest
	if err := json.Unmarshal(raw, &active); err != nil {
		return err
	}
	for _, item := range active.Items {
		body, err := fs.ReadFile(items, item.ID+".md")
		if err != nil {
			return err
		}
		if digest(body) != item.Digest || len(body) != item.Size {
			return fmt.Errorf("read refused: %s digest or size differs from the active manifest", item.ID)
		}
		fmt.Fprintf(out, "   %s: %s", item.ID, body)
	}
	fmt.Fprintln(out, "   Read checks passed: item digests and sizes agree with the active manifest.")
	return nil
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
