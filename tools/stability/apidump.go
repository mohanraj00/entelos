// Command apidump prints the exported Go API of the packages that
// docs/STABILITY.md names as stable. It reads source only. It uses only the
// standard library, so "go run tools/stability/apidump.go" needs no module.
//
// Usage: go run tools/stability/apidump.go <root> <import path>=<dir> ...
//
// The output has one line for each exported declaration. Comments,
// unexported struct fields, unexported interface methods, function bodies
// and the values of variables are not in the output.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: apidump <root> <import path>=<dir> ...")
		os.Exit(2)
	}
	root := os.Args[1]
	var lines []string
	for _, arg := range os.Args[2:] {
		path, dir, ok := strings.Cut(arg, "=")
		if !ok {
			fmt.Fprintln(os.Stderr, "apidump: argument must be <import path>=<dir>:", arg)
			os.Exit(2)
		}
		got, err := dump(path, filepath.Join(root, dir))
		if err != nil {
			fmt.Fprintln(os.Stderr, "apidump:", err)
			os.Exit(1)
		}
		lines = append(lines, got...)
	}
	sort.Strings(lines)
	for _, line := range lines {
		fmt.Println(line)
	}
}

func dump(path, dir string) ([]string, error) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var lines []string
	seen := map[string]bool{}
	add := func(kind, text string) {
		line := path + " " + kind + " " + text
		if !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				recv := ""
				if d.Recv != nil && len(d.Recv.List) == 1 {
					if !ast.IsExported(baseName(d.Recv.List[0].Type)) {
						continue
					}
					recv = "(" + show(fset, d.Recv.List[0].Type) + ") "
				}
				add("func", recv+d.Name.Name+strings.TrimPrefix(show(fset, d.Type), "func"))
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if !s.Name.IsExported() {
							continue
						}
						params := ""
						if s.TypeParams != nil {
							var parts []string
							for _, f := range s.TypeParams.List {
								for _, n := range f.Names {
									parts = append(parts, n.Name+" "+show(fset, f.Type))
								}
							}
							params = "[" + strings.Join(parts, ", ") + "]"
						}
						assign := " "
						if s.Assign.IsValid() {
							assign = " = "
						}
						add("type", s.Name.Name+params+assign+show(fset, exported(s.Type)))
					case *ast.ValueSpec:
						for i, id := range s.Names {
							if !id.IsExported() {
								continue
							}
							text := id.Name
							if s.Type != nil {
								text += " " + show(fset, exported(s.Type))
							}
							// A constant value is part of the API. A variable value is not.
							if d.Tok == token.CONST && i < len(s.Values) {
								text += " = " + show(fset, s.Values[i])
							}
							add(d.Tok.String(), text)
						}
					}
				}
			}
		}
	}
	return lines, nil
}

func baseName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return baseName(t.X)
	case *ast.IndexExpr:
		return baseName(t.X)
	case *ast.IndexListExpr:
		return baseName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// exported removes unexported fields and methods from struct and interface
// types, at each level.
func exported(e ast.Expr) ast.Expr {
	switch t := e.(type) {
	case *ast.StructType:
		return &ast.StructType{Fields: filter(t.Fields)}
	case *ast.InterfaceType:
		return &ast.InterfaceType{Methods: filter(t.Methods)}
	case *ast.StarExpr:
		return &ast.StarExpr{X: exported(t.X)}
	case *ast.ArrayType:
		return &ast.ArrayType{Len: t.Len, Elt: exported(t.Elt)}
	case *ast.MapType:
		return &ast.MapType{Key: exported(t.Key), Value: exported(t.Value)}
	}
	return e
}

func filter(list *ast.FieldList) *ast.FieldList {
	out := &ast.FieldList{}
	for _, f := range list.List {
		var names []*ast.Ident
		for _, n := range f.Names {
			if n.IsExported() {
				names = append(names, n)
			}
		}
		embedded := len(f.Names) == 0
		if embedded && !ast.IsExported(baseName(f.Type)) {
			// An embedded type without a name is a type constraint or an
			// unexported embedding. Keep the constraint, drop the embedding.
			if _, ok := f.Type.(*ast.Ident); ok {
				continue
			}
		}
		if !embedded && len(names) == 0 {
			continue
		}
		out.List = append(out.List, &ast.Field{Names: names, Type: exported(f.Type), Tag: f.Tag})
	}
	return out
}

func show(fset *token.FileSet, node any) string {
	var buf bytes.Buffer
	if err := (&printer.Config{Mode: printer.RawFormat}).Fprint(&buf, fset, node); err != nil {
		panic(err)
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}
