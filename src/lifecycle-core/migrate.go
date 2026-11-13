package lifecycle

// The migration of a v0.3.0 store to the review index of approval API 1.1.
// See "Migration from v0.3.0" in src/protocol/approval/APPROVAL.md.

import (
	"bytes"
	"sort"
	"strconv"
	"strings"
)

// migrationKey is the request key of the commit that adds the review index.
const migrationKey = "migrate-review-index"

// MigrateReviewIndex builds the review index of approval API 1.1 from the
// accepted history of a store. It adds only state.reviews and the request
// entry of the key "migrate-review-index", in one new commit. It changes no record.
//
// The result field "action" has one of these values:
//   - "empty": the store has no history. The Host makes the index on the first write.
//   - "current": the store has the index, and the index agrees with the history.
//   - "dry_run": the store has no index. The result gives the index and the writes. The command wrote nothing.
//   - "migrated": the command wrote the index.
//
// The migration refuses with a Fault, and writes nothing, if the history or
// the index is not consistent.
func MigrateReviewIndex(store Store, dryRun bool) (result Object, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, migrationError(r)
		}
	}()
	view := must(store.Read(nil))
	if view.Cursor == "" {
		return Object{"action": "empty", "cursor": "", "writes": Array{}}, nil
	}
	index, counts := rebuildReviewIndex(store, view)
	if existing, has := view.State["reviews"]; has {
		if !equal(existing, index) {
			fail("index_inconsistent", "state.reviews differs from the index that the accepted history gives", "store")
		}
		return Object{"action": "current", "cursor": view.Cursor, "writes": Array{}}, nil
	}
	if _, used := object(view.State["requests"])[migrationKey]; used {
		fail("index_inconsistent", "the migration request key is in the history, but state.reviews is missing", "store")
	}
	summary := Object{"snapshots": Int{strconv.Itoa(len(index))}, "reports": Int{strconv.Itoa(counts[0])},
		"decisions": Int{strconv.Itoa(counts[1])}}
	payload := Object{"migration": "review-index/1", "from_cursor": view.Cursor}
	record := Object{"migration": "review-index/1", "from_cursor": view.Cursor, "summary": summary}
	writes := Array{Object{"field": "state.reviews", "value": index},
		Object{"field": "state.requests." + migrationKey, "value": Object{"payload_ref": requestDigest(payload), "result": record}}}
	if dryRun {
		return Object{"action": "dry_run", "cursor": view.Cursor, "summary": summary, "writes": writes}, nil
	}
	out, err := store.Transact(view.Cursor, migrationKey, payload, func(v *View) (any, error) {
		if _, has := v.State["reviews"]; has {
			return nil, NewFault("index_inconsistent", "state.reviews appeared during the migration", "store")
		}
		v.State["reviews"] = index
		return record, nil
	})
	if err != nil {
		return nil, err
	}
	if out["reused"] == true {
		fail("index_inconsistent", "the migration request key was already used", "store")
	}
	return Object{"action": "migrated", "cursor": out["cursor"], "from_cursor": view.Cursor, "summary": summary, "writes": writes}, nil
}

// migrationError maps a malformed state or record to invalid_record.
func migrationError(r any) error {
	if e, ok := r.(*requestError); ok {
		return NewFault("invalid_record", "malformed state or record: "+e.message, "store")
	}
	return recovered(r)
}

// rebuildReviewIndex gives the index that a 1.1 Host would have written
// for each accepted qualify and decide request, in commit order. counts
// gives the number of reports and of decisions.
func rebuildReviewIndex(store Store, view *View) (Object, [2]int) {
	history := must(store.History(view.Cursor))
	index := Object{}
	named := map[string]bool{}
	var counts [2]int
	previous := Object{}
	for i := len(history) - 1; i >= 0; i-- {
		at := must(store.Read(history[i]))
		requests := object(at.State["requests"])
		var added []string
		for key := range requests {
			if _, had := previous[key]; !had {
				added = append(added, key)
			}
		}
		for key := range previous {
			if _, has := requests[key]; !has {
				fail("unexpected_history", "commit "+at.Cursor+" removes request key "+strconv.Quote(key), "store")
			}
		}
		if len(added) != 1 {
			sort.Strings(added)
			fail("unexpected_history", "commit "+at.Cursor+" adds "+strconv.Itoa(len(added))+" request keys, not 1: "+strings.Join(added, ", "), "store")
		}
		previous = requests
		res, _ := object(requests[added[0]])["result"].(Object)
		switch {
		case hasOnly(res, "report_ref", "report"):
			ref := text(res["report_ref"])
			report := get(at, ref, "qualification")
			appendEntry(reviewEntry(index, text(item(report, "snapshot_ref")), true), "reports", ref)
			named[ref] = true
			counts[0]++
		case hasOnly(res, "decision_ref") || hasOnly(res, "decision_ref", "decision"):
			ref := text(res["decision_ref"])
			decision := get(at, ref, "decision")
			report := get(at, item(decision, "report_ref"), "qualification")
			snapshotRef := text(item(report, "snapshot_ref"))
			if !equal(item(decision, "snapshot_ref"), snapshotRef) {
				fail("invalid_record", "decision "+ref+" names a snapshot that differs from its report", "store")
			}
			appendEntry(reviewEntry(index, snapshotRef, true), "decisions", ref)
			named[ref] = true
			counts[1]++
		}
	}
	// Each report and each decision in the store must come from an accepted
	// request. Else the index would be incomplete.
	for name, handle := range view.Entries {
		if !strings.HasPrefix(name, "o-") {
			continue
		}
		body := must(view.source.object(handle))
		media, _, _ := bytes.Cut(body, []byte{0})
		ref := "sha256:" + name[2:]
		if (string(media) == Media("qualification") || string(media) == Media("decision")) && !named[ref] {
			fail("index_incomplete", "no accepted qualify or decide request names record "+ref, "store")
		}
	}
	return index, counts
}

// hasOnly is true if m has exactly the keys.
func hasOnly(m Object, keys ...string) bool {
	if m == nil || len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
