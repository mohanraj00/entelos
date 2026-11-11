package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/common/types/traits"
)

// This file implements the typed profile cel-typed/v1 of
// src/cel-lifecycle/typed-profile-spec.md.

const (
	typedProfile  = "cel-typed/v1"
	maxSafeInt    = 1<<53 - 1
	maxListItems  = 64
	maxRecordSize = 16
)

// typeError gives the JSON pointer of the first bad value.
type typeError struct{ at, message string }

func (e *typeError) Error() string { return e.message }
func badValue(at, format string, args ...any) *typeError {
	return &typeError{at: at, message: fmt.Sprintf(format, args...)}
}

func isScalar(name string) bool {
	return name == "bool" || name == "string" || name == "int" || name == "date"
}

// checkDecl checks one type declaration at the pointer at, in the order of the
// declaration-check table of the specification. A field declaration cannot be
// a record.
func checkDecl(d Variable, at string, field bool) (*cel.Type, *Diagnostic) {
	switch d.Type {
	case "bool", "string", "int", "date", "list", "record":
	default:
		return nil, diag("unsupported_profile", "environment", at+"/type", "the type is not in cel-typed/v1")
	}
	if d.Items != nil && d.Type != "list" {
		return nil, diag("invalid_environment", "environment", at+"/items", "items is only for a list")
	}
	if d.Fields != nil && d.Type != "record" {
		return nil, diag("invalid_environment", "environment", at+"/fields", "fields is only for a record")
	}
	switch d.Type {
	case "bool":
		return cel.BoolType, nil
	case "string":
		return cel.StringType, nil
	case "int":
		return cel.IntType, nil
	case "date":
		return cel.TimestampType, nil
	case "list":
		if d.Items == nil {
			return nil, diag("invalid_environment", "environment", at+"/items", "a list needs items")
		}
		if !isScalar(*d.Items) {
			return nil, diag("unsupported_profile", "environment", at+"/items", "the items of a list must have a scalar type")
		}
		item, _ := checkDecl(Variable{Type: *d.Items}, at, true)
		return cel.ListType(item), nil
	}
	if field {
		return nil, diag("unsupported_profile", "environment", at+"/type", "a record in a record is not in cel-typed/v1")
	}
	if d.Fields == nil || len(*d.Fields) == 0 || len(*d.Fields) > maxRecordSize {
		return nil, diag("invalid_environment", "environment", at+"/fields", "a record needs 1 to 16 fields")
	}
	seen := map[string]bool{}
	for j, f := range *d.Fields {
		fat := fmt.Sprintf("%s/fields/%d", at, j)
		if !validName.MatchString(f.Name) || seen[f.Name] {
			return nil, diag("invalid_environment", "environment", fat, "a field needs a unique simple identifier")
		}
		seen[f.Name] = true
		if _, d := checkDecl(f, fat, true); d != nil {
			return nil, d
		}
	}
	return cel.MapType(cel.StringType, cel.DynType), nil
}

// assignable is the CEL checker relation: dyn on each side is assignable.
func assignable(want, got *cel.Type) bool {
	if want.Kind() == types.DynKind || got.Kind() == types.DynKind {
		return true
	}
	if want.Kind() != got.Kind() || want.TypeName() != got.TypeName() || len(want.Parameters()) != len(got.Parameters()) {
		return false
	}
	for i, p := range want.Parameters() {
		if !assignable(p, got.Parameters()[i]) {
			return false
		}
	}
	return true
}

// fromWire checks a decoded JSON value against a declaration and gives the
// CEL value. Numbers must be json.Number tokens, so the token text is known.
func fromWire(v any, d Variable, l Limits, at string) (any, *typeError) {
	switch d.Type {
	case "bool":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, badValue(at, "expected bool, null and coercion are unsupported")
	case "string":
		s, ok := v.(string)
		if !ok {
			return nil, badValue(at, "expected string, null and coercion are unsupported")
		}
		if len(s) > l.MaxStringBytes {
			return nil, badValue(at, "string exceeds UTF8 byte limit")
		}
		return s, nil
	case "int":
		n, ok := v.(json.Number)
		if !ok || strings.ContainsAny(string(n), ".eE") {
			return nil, badValue(at, "expected a JSON integer token, null and coercion are unsupported")
		}
		i, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil || i > maxSafeInt || i < -maxSafeInt {
			return nil, badValue(at, "integer is outside -(2^53-1) to 2^53-1")
		}
		return i, nil
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, badValue(at, "expected a YYYY-MM-DD date string")
		}
		if err := checkDate(s); err != nil {
			return nil, badValue(at, "expected a YYYY-MM-DD date in the calendar")
		}
		t, _ := time.Parse("2006-01-02", s)
		return t, nil
	case "list":
		items, ok := v.([]any)
		if !ok {
			return nil, badValue(at, "expected a JSON array")
		}
		if len(items) > l.MaxListItems {
			return nil, badValue(at, "list exceeds max_list_items")
		}
		out := make([]any, len(items))
		for i, item := range items {
			x, err := fromWire(item, Variable{Type: *d.Items}, l, fmt.Sprintf("%s/%d", at, i))
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	case "record":
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, badValue(at, "expected a JSON object")
		}
		declared := map[string]bool{}
		for _, f := range *d.Fields {
			declared[f.Name] = true
		}
		for _, k := range sortedKeys(obj) {
			if !declared[k] {
				return nil, badValue(at+pointer(k), "field is not declared")
			}
		}
		out := map[string]any{}
		for _, f := range *d.Fields {
			raw, found := obj[f.Name]
			if !found {
				return nil, badValue(at+pointer(f.Name), "declared field is absent")
			}
			x, err := fromWire(raw, f, l, at+pointer(f.Name))
			if err != nil {
				return nil, err
			}
			out[f.Name] = x
		}
		return out, nil
	}
	return nil, badValue(at, "unsupported type %s", d.Type)
}

// toWire checks a CEL result against a declaration and gives the wire value.
func toWire(v ref.Val, d Variable, l Limits, at string) (any, *typeError) {
	switch d.Type {
	case "bool":
		if b, ok := v.(types.Bool); ok {
			return bool(b), nil
		}
	case "string":
		if s, ok := v.(types.String); ok {
			if len(s) > l.MaxStringBytes {
				return nil, badValue(at, "string exceeds UTF8 byte limit")
			}
			return string(s), nil
		}
	case "int":
		if i, ok := v.(types.Int); ok {
			if i > maxSafeInt || i < -maxSafeInt {
				return nil, badValue(at, "integer is outside -(2^53-1) to 2^53-1")
			}
			return json.Number(strconv.FormatInt(int64(i), 10)), nil
		}
	case "date":
		if ts, ok := v.(types.Timestamp); ok {
			t := ts.Time.UTC()
			if t.Year() < 1 || t.Year() > 9999 || !t.Equal(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)) {
				return nil, badValue(at, "a date must be a timestamp at 00:00:00 UTC from year 0001 to 9999")
			}
			return t.Format("2006-01-02"), nil
		}
	case "list":
		list, ok := v.(traits.Lister)
		if !ok {
			break
		}
		n, ok := list.Size().(types.Int)
		if !ok || n > types.Int(l.MaxListItems) {
			return nil, badValue(at, "list exceeds max_list_items")
		}
		out := make([]any, int(n))
		for i := range out {
			x, err := toWire(list.Get(types.Int(i)), Variable{Type: *d.Items}, l, fmt.Sprintf("%s/%d", at, i))
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	case "record":
		m, ok := v.(traits.Mapper)
		if !ok {
			break
		}
		declared := map[string]bool{}
		for _, f := range *d.Fields {
			declared[f.Name] = true
		}
		var unknown []string
		for it := m.Iterator(); it.HasNext() == types.True; {
			k, ok := it.Next().(types.String)
			if !ok {
				return nil, badValue(at, "a record key must be a string")
			}
			if !declared[string(k)] {
				unknown = append(unknown, string(k))
			}
		}
		sort.Strings(unknown)
		if len(unknown) > 0 {
			return nil, badValue(at+pointer(unknown[0]), "field is not declared")
		}
		out := map[string]any{}
		for _, f := range *d.Fields {
			x, found := m.Find(types.String(f.Name))
			if !found {
				return nil, badValue(at+pointer(f.Name), "declared field is absent")
			}
			w, err := toWire(x, f, l, at+pointer(f.Name))
			if err != nil {
				return nil, err
			}
			out[f.Name] = w
		}
		return out, nil
	}
	return nil, badValue(at, "expected %s, got %s", d.Type, v.Type().TypeName())
}

// canonical gives the canonical JSON text of a checked wire value, without
// the final newline: sorted keys, raw UTF-8 and integer text without -0.
func canonical(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case bool:
		fmt.Fprint(b, x)
	case string:
		writeString(b, x)
	case json.Number:
		i, _ := strconv.ParseInt(string(x), 10, 64)
		b.WriteString(strconv.FormatInt(i, 10))
	case []any:
		b.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			canonical(b, item)
		}
		b.WriteByte(']')
	case map[string]any:
		b.WriteByte('{')
		for i, k := range sortedKeys(x) {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k)
			b.WriteByte(':')
			canonical(b, x[k])
		}
		b.WriteByte('}')
	}
}

// schema gives the JSON Schema of a declaration for the descriptor.
func schema(d Variable, l Limits) map[string]any {
	var s map[string]any
	switch d.Type {
	case "bool":
		s = map[string]any{"type": "boolean"}
	case "string":
		s = map[string]any{"type": "string", "x-max-utf8-bytes": l.MaxStringBytes}
	case "int":
		s = map[string]any{"type": "integer", "minimum": -maxSafeInt, "maximum": maxSafeInt}
	case "date":
		s = map[string]any{"type": "string", "format": "date", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}
	case "list":
		s = map[string]any{"type": "array", "items": schema(Variable{Type: *d.Items}, l), "maxItems": l.MaxListItems}
	case "record":
		properties, required := map[string]any{}, []string{}
		for _, f := range *d.Fields {
			properties[f.Name] = schema(f, l)
			required = append(required, f.Name)
		}
		s = map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	if d.Description != "" {
		s["description"] = d.Description
	}
	return s
}
