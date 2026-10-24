package main

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func readSeedlingGenFixture(t *testing.T, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", relativePath))
	if err != nil {
		t.Fatalf("read fixture %q: %v", relativePath, err)
	}

	return string(data)
}

func findTableByName(t *testing.T, tables []Table, name string) Table {
	t.Helper()

	for _, table := range tables {
		if table.Name == name {
			return table
		}
	}

	t.Fatalf("find table %q: not found", name)
	return Table{}
}

func findColumnByName(t *testing.T, table Table, name string) Column {
	t.Helper()

	for _, column := range table.Columns {
		if column.Name == name {
			return column
		}
	}

	t.Fatalf("find column %q on table %q: not found", name, table.Name)
	return Column{}
}

func findForeignKeyByColumns(t *testing.T, table Table, columns ...string) ForeignKey {
	t.Helper()

	for _, foreignKey := range table.ForeignKeys {
		if strings.Join(foreignKey.Columns, ",") == strings.Join(columns, ",") {
			return foreignKey
		}
	}

	t.Fatalf("find foreign key on table %q for columns %v: not found", table.Name, columns)
	return ForeignKey{}
}

// generatedHook parses generated blueprint code and returns the function
// literal assigned to the hook field (Insert or Delete) of the named blueprint,
// together with the file set needed to render its nodes.
func generatedHook(t *testing.T, src, blueprint, hook string) (*token.FileSet, *ast.FuncLit) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "blueprints.go", src, 0)
	if err != nil {
		t.Fatalf("parse generated code: %v\n%s", err, src)
	}

	var found *ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || compositeLitString(lit, "Name") != strconv.Quote(blueprint) {
			return found == nil
		}
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok && isIdent(kv.Key, hook) {
				found, _ = kv.Value.(*ast.FuncLit)
			}
		}
		return false
	})
	if found == nil {
		t.Fatalf("%s hook of blueprint %q not found in generated code:\n%s", hook, blueprint, src)
	}
	return fset, found
}

// compositeLitString returns the literal source of a basic-literal field in a
// keyed composite literal, or "" when the field is absent.
func compositeLitString(lit *ast.CompositeLit, name string) string {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || !isIdent(kv.Key, name) {
			continue
		}
		if value, ok := kv.Value.(*ast.BasicLit); ok {
			return value.Value
		}
	}
	return ""
}

func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// renderNode prints an AST node back to Go source for comparison.
func renderNode(t *testing.T, fset *token.FileSet, node ast.Node) string {
	t.Helper()

	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		t.Fatalf("render node: %v", err)
	}
	return buf.String()
}

// requireGuardedCall asserts that the hook calls the named method exactly once,
// as the only statement of an if statement whose condition is guard, and that
// the call renders as wantCall.
func requireGuardedCall(t *testing.T, fset *token.FileSet, hook *ast.FuncLit, method, guard, wantCall string) {
	t.Helper()

	var calls []string
	var guarded *ast.IfStmt
	ast.Inspect(hook.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
				calls = append(calls, renderNode(t, fset, n))
			}
		case *ast.IfStmt:
			if renderNode(t, fset, n.Cond) == guard {
				guarded = n
			}
		}
		return true
	})

	if len(calls) != 1 || calls[0] != wantCall {
		t.Fatalf("%s calls = %q, want exactly [%s]", method, calls, wantCall)
	}
	if guarded == nil {
		t.Fatalf("no if statement guarded by %q in hook:\n%s", guard, renderNode(t, fset, hook))
	}
	if len(guarded.Body.List) != 1 || guarded.Else != nil ||
		renderNode(t, fset, guarded.Body.List[0]) != wantCall {
		t.Fatalf("if %s should contain only %s, got:\n%s", guard, wantCall, renderNode(t, fset, guarded))
	}
}
