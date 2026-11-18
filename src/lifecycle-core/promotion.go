package lifecycle

import (
	"math/big"
	"regexp"
)

// The profile promotion rules of src/protocol/promotion/PROMOTION.md.

// promotionItems are the item ids of a promotion manifest, in item order.
var promotionItems = []string{"measure", "profile"}

var (
	promotionVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	// measureText is a string of 1 to max code points.
	measureText = func(max int) *evRule {
		return &evRule{kind: "string", pattern: regexp.MustCompile(`(?s)^.+$`), maxLen: max}
	}
	// measureSummary is schema/summary.schema.json of PROMOTION.md.
	measureSummary = evObject([]string{"format", "profile_digest", "measure_digest", "gold_digest", "labels_digest", "routes"},
		map[string]*evRule{
			"format":         {kind: "string", enum: []string{"entelos.measure/1"}},
			"profile_digest": evDigest, "measure_digest": evDigest, "gold_digest": evDigest, "labels_digest": evDigest,
			"routes": evList(evObject([]string{"field", "route", "bound_bp"}, map[string]*evRule{
				"field": measureText(256), "route": measureText(64),
				"bound_bp": {kind: "integer", low: big.NewRat(0, 1), high: big.NewRat(10000, 1)}}), 1, 2000)})
)

// checkPromotionContext checks the optional Host context field
// promotion_collections: a list of nonempty collection names.
func checkPromotionContext(ctx Object) error {
	raw, has := ctx["promotion_collections"]
	if !has {
		return nil
	}
	list, valid := raw.(Array)
	for _, x := range list {
		if s, ok := x.(string); !ok || s == "" {
			valid = false
		}
	}
	if !valid {
		return NewFault("configuration_error", "host promotion_collections must be a list of nonempty strings", "")
	}
	return nil
}

// judgePromotion applies the rules of PROMOTION.md in their order. The
// candidate has the request shape of CONTENT.md. A nil report is no report.
// The first rule that fails gives its code, and then there are no inputs.
func judgePromotion(collections Array, candidate any, report Object) (inputs Object, rule string) {
	c, _ := candidate.(Object)
	if kind, _ := c["kind"].(string); kind != contentKind {
		return nil, "not_promotion"
	}
	result := judgeCandidate(c)
	if !result.Valid {
		return nil, "content"
	}
	if !within(result.Collection, collections) {
		return nil, "not_promotion"
	}
	items := result.Manifest["items"].(Array)
	byID := map[string]Object{}
	for _, x := range items {
		byID[x.(Object)["item_id"].(string)] = x.(Object)
	}
	if len(items) != len(promotionItems) {
		return nil, "items"
	}
	for _, id := range promotionItems {
		if x, has := byID[id]; !has || x["media_type"] != "application/json" {
			return nil, "items"
		}
	}
	metadata, _ := result.Manifest["metadata"].(Object)
	id, _ := metadata["profile_id"].(string)
	version, _ := metadata["profile_version"].(string)
	if id != result.Partition || !promotionVersion.MatchString(version) {
		return nil, "metadata"
	}
	summary := report["measure"]
	if summary == nil {
		return nil, "no_summary"
	}
	if !measureSummary.agrees(summary) {
		return nil, "summary_schema"
	}
	s := summary.(Object)
	if s["profile_digest"] != byID["profile"]["digest"] || s["measure_digest"] != byID["measure"]["digest"] {
		return nil, "digest_binding"
	}
	routes := s["routes"].(Array)
	seen := map[[2]string]bool{}
	for _, x := range routes {
		r := x.(Object)
		key := [2]string{r["field"].(string), r["route"].(string)}
		if seen[key] {
			return nil, "duplicate_route"
		}
		seen[key] = true
	}
	var largest *big.Rat
	for _, x := range routes {
		if n, _ := number(x.(Object)["bound_bp"]); largest == nil || n.Cmp(largest) > 0 {
			largest = n
		}
	}
	return Object{"measure_gold_digest": s["gold_digest"], "measure_max_bound_bp": Int{Text: largest.Num().String()}}, ""
}

// promotionRead is the promotion read of PROMOTION.md. It applies the rules
// to the candidate and the report that the store retains, and gives the
// result and the measure inputs. It writes nothing and changes no record.
func (h *Host) promotionRead(v *View, q Object) Object {
	ref := text(item(q, "report_ref"))
	report := h.scoped(v, ref, "qualification")
	snapshot := h.scoped(v, report["snapshot_ref"], "snapshot")
	candidate := Object{"kind": snapshotKind(snapshot)}
	if snapshotKind(snapshot) == contentKind {
		candidate["manifest"] = string(getRaw(v, item(snapshot, "manifest_ref"), contentMime))
	}
	collections, _ := h.Context["promotion_collections"].(Array)
	out := Object{"cursor": v.Cursor, "report_ref": ref, "snapshot_ref": report["snapshot_ref"]}
	inputs, rule := judgePromotion(collections, candidate, report)
	if rule != "" {
		out["promotion"], out["rule"] = "invalid", rule
		return out
	}
	out["promotion"], out["inputs"] = "valid", inputs
	return out
}
