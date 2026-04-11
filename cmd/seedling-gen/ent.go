package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EntSchema represents a parsed ent schema.
type EntSchema struct {
	Name   string
	Fields []EntField
	Edges  []EntEdge
}

// EntField represents a field from ent's Fields() method.
type EntField struct {
	Name     string
	Type     string // ent type method name: "String", "Int", "Time", etc.
	GoType   string
	Optional bool
}

// EntEdge represents an edge from ent's Edges() method.
type EntEdge struct {
	Name      string
	Type      string // target schema name
	Direction string // "To" or "From"
	Ref       string // for edge.From: the inverse edge name
	Unique    bool
	Required  bool
}

// ParseEntSchemaDir parses ent schema Go files in the given directory.
func ParseEntSchemaDir(dir string) ([]EntSchema, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read ent schema dir: %w", err)
	}

	fset := token.NewFileSet()
	var schemas []EntSchema

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}

		// Find schema types: types that have Fields() and/or Edges() methods.
		typeNames := extractTypeNames(f)
		methods := extractMethods(f)

		for _, typeName := range typeNames {
			schema := EntSchema{Name: typeName}

			if fieldsMethod, ok := methods[typeName+"_Fields"]; ok {
				schema.Fields = parseEntFields(fieldsMethod)
			}
			if edgesMethod, ok := methods[typeName+"_Edges"]; ok {
				schema.Edges = parseEntEdges(edgesMethod)
			}

			if len(schema.Fields) > 0 || len(schema.Edges) > 0 {
				schemas = append(schemas, schema)
			}
		}
	}

	if len(schemas) == 0 {
		return nil, fmt.Errorf("no ent schemas found in %s", dir)
	}

	return schemas, nil
}

func extractTypeNames(f *ast.File) []string {
	var names []string
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts := spec.(*ast.TypeSpec)
			if _, ok := ts.Type.(*ast.StructType); ok {
				names = append(names, ts.Name.Name)
			}
		}
	}
	return names
}

func extractMethods(f *ast.File) map[string]*ast.FuncDecl {
	methods := make(map[string]*ast.FuncDecl)
	for _, decl := range f.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil {
			continue
		}
		recvName := receiverTypeName(funcDecl.Recv)
		if recvName == "" {
			continue
		}
		key := recvName + "_" + funcDecl.Name.Name
		methods[key] = funcDecl
	}
	return methods
}

// parseEntFields extracts fields from a Fields() method body like:
//
//	return []ent.Field{
//	    field.String("name"),
//	    field.Int("age").Optional(),
//	}
func parseEntFields(fn *ast.FuncDecl) []EntField {
	if fn.Body == nil {
		return nil
	}

	var fields []EntField
	for _, stmt := range fn.Body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			continue
		}
		comp, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, elt := range comp.Elts {
			if f := parseEntFieldExpr(elt); f != nil {
				fields = append(fields, *f)
			}
		}
	}
	return fields
}

func parseEntFieldExpr(expr ast.Expr) *EntField {
	// Walk the chain to find the root call: field.String("name"), field.Int("age"), etc.
	chain := flattenCallChain(expr)
	if len(chain) == 0 {
		return nil
	}

	// First call should be field.Type("name").
	root := chain[0]
	sel, ok := root.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	// Check it's a call on "field" package.
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "field" {
		return nil
	}

	typeName := sel.Sel.Name // "String", "Int", "Time", etc.
	if len(root.Args) == 0 {
		return nil
	}
	nameArg, ok := root.Args[0].(*ast.BasicLit)
	if !ok || nameArg.Kind != token.STRING {
		return nil
	}
	fieldName := strings.Trim(nameArg.Value, `"`)

	f := &EntField{
		Name:   fieldName,
		Type:   typeName,
		GoType: entTypeToGoType(typeName),
	}

	// Check for chained methods.
	for _, call := range chain[1:] {
		if methodSel, ok := call.Fun.(*ast.SelectorExpr); ok {
			switch methodSel.Sel.Name {
			case "Optional", "Nillable":
				f.Optional = true
			}
		}
	}

	return f
}

// parseEntEdges extracts edges from an Edges() method body like:
//
//	return []ent.Edge{
//	    edge.To("cars", Car.Type),
//	    edge.From("owner", User.Type).Ref("cars").Unique(),
//	}
func parseEntEdges(fn *ast.FuncDecl) []EntEdge {
	if fn.Body == nil {
		return nil
	}

	var edges []EntEdge
	for _, stmt := range fn.Body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			continue
		}
		comp, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, elt := range comp.Elts {
			if e := parseEntEdgeExpr(elt); e != nil {
				edges = append(edges, *e)
			}
		}
	}
	return edges
}

func parseEntEdgeExpr(expr ast.Expr) *EntEdge {
	chain := flattenCallChain(expr)
	if len(chain) == 0 {
		return nil
	}

	root := chain[0]
	sel, ok := root.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "edge" {
		return nil
	}

	direction := sel.Sel.Name // "To" or "From"
	if direction != "To" && direction != "From" {
		return nil
	}

	if len(root.Args) < 2 {
		return nil
	}
	nameArg, ok := root.Args[0].(*ast.BasicLit)
	if !ok || nameArg.Kind != token.STRING {
		return nil
	}
	edgeName := strings.Trim(nameArg.Value, `"`)

	// Extract target type: Car.Type -> "Car".
	targetType := extractEntEdgeType(root.Args[1])

	e := &EntEdge{
		Name:      edgeName,
		Type:      targetType,
		Direction: direction,
	}

	// Process chained methods.
	for _, call := range chain[1:] {
		if methodSel, ok := call.Fun.(*ast.SelectorExpr); ok {
			switch methodSel.Sel.Name {
			case "Unique":
				e.Unique = true
			case "Required":
				e.Required = true
			case "Ref":
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						e.Ref = strings.Trim(lit.Value, `"`)
					}
				}
			}
		}
	}

	return e
}

func extractEntEdgeType(expr ast.Expr) string {
	// Car.Type -> SelectorExpr{X: Ident{Car}, Sel: Ident{Type}}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// flattenCallChain walks a chain of method calls like a().B().C() and returns
// them in order [a(), B(), C()].
func flattenCallChain(expr ast.Expr) []*ast.CallExpr {
	var chain []*ast.CallExpr
	for {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			break
		}
		chain = append(chain, call)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}
		expr = sel.X
	}
	// Reverse so root is first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func entTypeToGoType(entType string) string {
	switch entType {
	case "String", "Text":
		return "string"
	case "Int":
		return "int"
	case "Int8":
		return "int8"
	case "Int16":
		return "int16"
	case "Int32":
		return "int32"
	case "Int64":
		return "int64"
	case "Uint":
		return "uint"
	case "Uint8":
		return "uint8"
	case "Uint16":
		return "uint16"
	case "Uint32":
		return "uint32"
	case "Uint64":
		return "uint64"
	case "Float", "Float32":
		return "float32"
	case "Float64":
		return "float64"
	case "Bool":
		return "bool"
	case "Time":
		return "time.Time"
	case "UUID":
		return "uuid.UUID"
	case "Bytes":
		return "[]byte"
	case "JSON":
		return "json.RawMessage"
	case "Enum":
		return "string"
	default:
		return "string"
	}
}

// GenerateEnt generates Blueprint registration code for ent schemas.
func GenerateEnt(w io.Writer, pkg, entImportPath string, schemas []EntSchema) error {
	models := normalizeEntModels(schemas)
	imports := []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
		`ent "` + entImportPath + `"`,
	}
	if normalizedModelsNeedTimeImport(models) {
		imports = append(imports, `"time"`)
	}
	return generateNormalizedCode(w, "ent", pkg, imports, models, false)
}
