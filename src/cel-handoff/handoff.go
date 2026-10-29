// Package handoff implements a host-selected primitive CEL callable.
package handoff

import (
	"cel.dev/cel-go/interpreter"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"cel.dev/cel-go/cel"
)

type Files struct{ Manifest, Environment, Source, Fixed []byte }
type Limits struct {
	MaxRequestBytes int    `json:"max_request_bytes"`
	MaxStringBytes  int    `json:"max_string_utf8_bytes"`
	MaxSourceBytes  int    `json:"max_source_utf8_bytes"`
	ParserRecursion int    `json:"parser_recursion_limit"`
	EvaluationCost  uint64 `json:"evaluation_cost_limit"`
}
type Manifest struct {
	FunctionID        string   `json:"function_id"`
	Profile           string   `json:"profile"`
	CallerVariables   []string `json:"caller_variables"`
	ImplementationRef string   `json:"implementation_ref"`
	EnvironmentRef    string   `json:"environment_ref"`
	SourceRef         string   `json:"source_ref"`
	FixedRef          string   `json:"fixed_ref"`
	Limits            Limits   `json:"limits"`
}
type Variable struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}
type Environment struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Variables   []Variable `json:"variables"`
}
type Location struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}
type Diagnostic struct {
	Locations []Location `json:"locations,omitempty"`
	Code      string     `json:"code"`
	Stage     string     `json:"stage"`
	Field     string     `json:"field,omitempty"`
	Message   string     `json:"message"`
}

func (d *Diagnostic) Error() string { return d.Stage + ": " + d.Code + ": " + d.Message }
func diag(code, stage, field, message string) *Diagnostic {
	return &Diagnostic{Code: code, Stage: stage, Field: field, Message: message}
}

type Descriptor struct {
	FunctionID        string         `json:"function_id"`
	BundleRef         string         `json:"bundle_ref"`
	ImplementationRef string         `json:"implementation_ref"`
	Profile           string         `json:"profile"`
	Inputs            map[string]any `json:"inputs"`
	Output            map[string]any `json:"output"`
	Limits            Limits         `json:"limits"`
}
type Response struct {
	Status            string      `json:"status"`
	Value             *bool       `json:"value,omitempty"`
	BundleRef         string      `json:"bundle_ref,omitempty"`
	ImplementationRef string      `json:"implementation_ref,omitempty"`
	Diagnostic        *Diagnostic `json:"diagnostic,omitempty"`
	Cost              *uint64     `json:"cost,omitempty"`
}
type Bundle struct {
	manifest  Manifest
	variables map[string]Variable
	fixed     map[string]any
	ref       string
	program   cel.Program
}

func Digest(media string, body []byte) string {
	sum := sha256.Sum256(append([]byte(media+"\x00"), body...))
	return fmt.Sprintf("sha256:%x", sum)
}

// Load validates and prepares one explicit closure. It does not grant serving authority.
func Load(f Files) (*Bundle, *Diagnostic) {
	b := &Bundle{variables: map[string]Variable{}, fixed: map[string]any{}, ref: Digest("application/vnd.cel-handoff.bundle+json", f.Manifest)}
	for _, raw := range [][]byte{f.Manifest, f.Environment, f.Fixed} {
		if len(raw) > 65536 {
			return nil, diag("invalid_bundle", "bundle", "", "bundle metadata exceeds byte ceiling")
		}
	}
	if err := strictJSON(f.Manifest, &b.manifest); err != nil {
		return nil, diag("invalid_bundle", "bundle", "/manifest", err.Error())
	}
	m := b.manifest
	if m.ImplementationRef == "" || m.ImplementationRef != BuildID() {
		return nil, diag("implementation_unavailable", "bundle", "/implementation_ref", "the exact implementation is unavailable")
	}
	if m.EnvironmentRef != Digest("application/json", f.Environment) || m.SourceRef != Digest("text/plain", f.Source) || m.FixedRef != Digest("application/json", f.Fixed) {
		return nil, diag("artifact_mismatch", "bundle", "/manifest", "component bytes do not match manifest references")
	}
	if m.Profile != "cel-string-bool/v1" {
		return nil, diag("unsupported_profile", "profile", "/profile", "only cel-string-bool/v1 is supported")
	}
	l := m.Limits
	if m.FunctionID == "" || len(m.CallerVariables) == 0 || l.MaxRequestBytes <= 0 || l.MaxRequestBytes > 4096 || l.MaxStringBytes <= 0 || l.MaxStringBytes > 256 || l.MaxSourceBytes <= 0 || l.MaxSourceBytes > 4096 || l.ParserRecursion <= 0 || l.ParserRecursion > 64 || l.EvaluationCost == 0 || l.EvaluationCost > 1000 {
		return nil, diag("invalid_bundle", "profile", "/limits", "positive limits within the reference ceilings and a function/caller contract are required")
	}
	if len(f.Source) > l.MaxSourceBytes || !utf8.Valid(f.Source) {
		return nil, diag("invalid_bundle", "source", "/source", "source exceeds UTF8 byte bound or is invalid UTF8")
	}
	var env Environment
	if err := strictJSON(f.Environment, &env); err != nil {
		return nil, diag("unsupported_profile", "environment", "/environment", err.Error())
	}
	options := []cel.EnvOption{cel.ParserExpressionSizeLimit(l.MaxSourceBytes), cel.ParserRecursionLimit(l.ParserRecursion)}
	validName := regexp.MustCompile(`^[A-Za-z_][A-Za-z_0-9]*$`)
	if env.Name == "" || len(env.Variables) == 0 {
		return nil, diag("invalid_environment", "environment", "/environment", "name and declarations are required")
	}
	for i, v := range env.Variables {
		field := fmt.Sprintf("/variables/%d", i)
		if !validName.MatchString(v.Name) {
			return nil, diag("invalid_environment", "environment", field, "only simple identifiers are supported")
		}
		if _, exists := b.variables[v.Name]; exists {
			return nil, diag("invalid_environment", "environment", field, "duplicate variable")
		}
		typ := cel.StringType
		switch v.Type {
		case "string":
		case "bool":
			typ = cel.BoolType
		default:
			return nil, diag("unsupported_profile", "environment", field+"/type", "only string and bool declarations are supported")
		}
		b.variables[v.Name] = v
		options = append(options, cel.Variable(v.Name, typ))
	}
	if err := strictJSON(f.Fixed, &b.fixed); err != nil || b.fixed == nil {
		return nil, diag("invalid_bindings", "bindings", "/fixed", "fixed bindings must be a JSON object with unique keys")
	}
	caller := map[string]bool{}
	for _, name := range m.CallerVariables {
		if _, ok := b.variables[name]; !ok || caller[name] {
			return nil, diag("invalid_bindings", "bindings", pointer(name), "caller must name each declared caller variable exactly once")
		}
		caller[name] = true
	}
	for _, name := range sortedKeys(b.fixed) {
		v, ok := b.variables[name]
		if !ok || caller[name] {
			return nil, diag("invalid_bindings", "bindings", pointer(name), "fixed and caller bindings must be declared and disjoint")
		}
		if err := validateValue(b.fixed[name], v.Type, l.MaxStringBytes); err != nil {
			return nil, diag("invalid_bindings", "bindings", pointer(name), err.Error())
		}
	}
	for name := range b.variables {
		if !caller[name] {
			if _, ok := b.fixed[name]; !ok {
				return nil, diag("invalid_bindings", "bindings", pointer(name), "declaration has no caller or fixed binding")
			}
		}
	}
	e, err := cel.NewEnv(options...)
	if err != nil {
		return nil, diag("invalid_environment", "environment", "/environment", err.Error())
	}
	ast, issues := e.Compile(string(f.Source))
	if issues.Err() != nil {
		d := diag("compile_error", "compile", "/source", issues.String())
		for _, e := range issues.Errors() {
			d.Locations = append(d.Locations, Location{Line: e.Location.Line(), Column: e.Location.Column() + 1, Message: e.Message})
		}
		return nil, d
	}
	if ast.OutputType() != cel.BoolType {
		return nil, diag("output_type", "compile", "/source", "expected bool, got "+ast.OutputType().String())
	}
	b.program, err = e.Program(ast, cel.CostLimit(l.EvaluationCost), cel.InterruptCheckFrequency(1))
	if err != nil {
		return nil, diag("prepare_error", "prepare", "/source", err.Error())
	}
	return b, nil
}
func (b *Bundle) Describe() Descriptor {
	properties := map[string]any{}
	for _, name := range b.manifest.CallerVariables {
		v := b.variables[name]
		typ := "string"
		if v.Type == "bool" {
			typ = "boolean"
		}
		properties[name] = map[string]any{"type": typ, "description": v.Description}
		if v.Type == "string" {
			properties[name].(map[string]any)["x-max-utf8-bytes"] = b.manifest.Limits.MaxStringBytes
		}
	}
	return Descriptor{FunctionID: b.manifest.FunctionID, BundleRef: b.ref, ImplementationRef: BuildID(), Profile: b.manifest.Profile, Inputs: map[string]any{"type": "object", "properties": properties, "required": append([]string{}, b.manifest.CallerVariables...), "additionalProperties": false}, Output: map[string]any{"type": "boolean"}, Limits: b.manifest.Limits}
}
func (b *Bundle) Run(ctx context.Context, raw []byte) Response {
	failure := func(code, stage, field, message string) Response {
		return Response{Status: code, BundleRef: b.ref, ImplementationRef: BuildID(), Diagnostic: diag(code, stage, field, message)}
	}
	if len(raw) > b.manifest.Limits.MaxRequestBytes {
		return failure("invalid_input", "input", "", "request exceeds byte limit")
	}
	var req struct {
		BundleRef string         `json:"bundle_ref"`
		Inputs    map[string]any `json:"inputs"`
	}
	if err := strictJSON(raw, &req); err != nil {
		return failure("invalid_input", "input", "", err.Error())
	}
	if req.Inputs == nil {
		return failure("invalid_input", "input", "/inputs", "inputs must be a non-null JSON object")
	}
	if req.BundleRef != b.ref {
		return failure("artifact_mismatch", "bundle", "/bundle_ref", "an exact expected bundle reference is required")
	}
	caller := map[string]bool{}
	for _, name := range b.manifest.CallerVariables {
		caller[name] = true
	}
	for _, name := range sortedKeys(req.Inputs) {
		if !caller[name] {
			return failure("invalid_input", "input", pointer(name), "unknown or fixed input")
		}
	}
	vars := map[string]any{}
	for k, v := range b.fixed {
		vars[k] = v
	}
	for _, name := range b.manifest.CallerVariables {
		value, found := req.Inputs[name]
		if !found {
			return failure("needs_input", "input", pointer(name), "required input is absent")
		}
		if err := validateValue(value, b.variables[name].Type, b.manifest.Limits.MaxStringBytes); err != nil {
			return failure("invalid_input", "input", pointer(name), err.Error())
		}
		vars[name] = value
	}
	if ctx.Err() != nil {
		return failure("cancelled", "evaluate", "", ctx.Err().Error())
	}
	out, details, err := b.program.ContextEval(ctx, vars)
	if err != nil {
		var cancelled interpreter.EvalCancelledError
		if errors.As(err, &cancelled) {
			if cancelled.Cause == interpreter.CostLimitExceeded {
				return failure("resource_exhausted", "evaluate", "/source", err.Error())
			}
			return failure("cancelled", "evaluate", "/source", err.Error())
		}
		return failure("evaluation_error", "evaluate", "/source", err.Error())
	}
	value, ok := out.Value().(bool)
	if !ok {
		return failure("output_type", "output", "", fmt.Sprint(out))
	}
	return Response{Status: "ok", Value: &value, BundleRef: b.ref, ImplementationRef: BuildID(), Cost: details.ActualCost()}
}
func validateValue(value any, typ string, maxBytes int) error {
	switch typ {
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool, null and coercion are unsupported")
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, null and coercion are unsupported")
		}
		if len(s) > maxBytes {
			return fmt.Errorf("string exceeds UTF8 byte limit")
		}
	default:
		return fmt.Errorf("unsupported type %s", typ)
	}
	return nil
}
func pointer(name string) string {
	return "/" + strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
}
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type Case struct {
	Name   string         `json:"case"`
	Inputs map[string]any `json:"inputs"`
	Status string         `json:"status"`
	Value  *bool          `json:"value,omitempty"`
	Field  string         `json:"field,omitempty"`
}
type CaseResult struct {
	Name   string   `json:"case"`
	Passed bool     `json:"passed"`
	Actual Response `json:"actual"`
}
type Report struct {
	Status            string       `json:"status"`
	BundleRef         string       `json:"bundle_ref"`
	ImplementationRef string       `json:"implementation_ref"`
	Cases             []CaseResult `json:"cases"`
}

// Check compares actual calls with host-owned expectations. It never activates a bundle.
func (b *Bundle) Check(ctx context.Context, cases []Case) Report {
	report := Report{Status: "passed", BundleRef: b.ref, ImplementationRef: BuildID(), Cases: []CaseResult{}}
	if len(cases) == 0 {
		report.Status = "failed"
		return report
	}
	names := map[string]bool{}
	for _, c := range cases {
		raw, err := json.Marshal(map[string]any{"bundle_ref": b.ref, "inputs": c.Inputs})
		actual := b.Run(ctx, raw)
		passed := err == nil && c.Name != "" && !names[c.Name] && actual.Status == c.Status
		names[c.Name] = true
		if c.Status == "ok" {
			passed = passed && c.Value != nil && actual.Value != nil
			if c.Value != nil && actual.Value != nil {
				passed = passed && *actual.Value == *c.Value
			}
		} else {
			passed = passed && c.Value == nil && actual.Value == nil
		}
		if c.Field != "" {
			passed = passed && actual.Diagnostic != nil && actual.Diagnostic.Field == c.Field
		}
		report.Cases = append(report.Cases, CaseResult{Name: c.Name, Passed: passed, Actual: actual})
		if !passed {
			report.Status = "failed"
		}
	}
	return report
}
