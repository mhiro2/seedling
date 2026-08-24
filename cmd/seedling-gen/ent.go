package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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
	Name         string
	GoName       string // field name from the generated Ent entity
	JSONName     string // JSON name from the schema's StructTag or Sensitive option
	Type         string // ent type method name: "String", "Int", "Time", etc.
	GoType       string
	DefaultType  string // safely assignable built-in type used for generated defaults
	Optional     bool
	Nillable     bool
	CustomGoType bool // whether the schema uses field.GoType
	FromMixin    bool // whether the field exists only in generated mixin output
	SetterName   string
	SetterDeref  bool // whether the setter accepts the entity field's pointed-to value
}

// EntEdge represents an edge from ent's Edges() method.
type EntEdge struct {
	Name      string
	Type      string // target schema name
	Direction string // "To" or "From"
	Ref       string // for edge.From: the inverse edge name
	Field     string // exposed foreign-key field configured with edge.Field
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

		// Find schema types from their embedded base or schema methods. Mixin-only
		// schemas have no direct fields for the AST parser, but the generated
		// package resolver can recover their concrete fields.
		typeNames := extractTypeNames(f)
		methods := extractMethods(f)

		for _, typeName := range typeNames {
			_, hasFields := methods[typeName+"_Fields"]
			_, hasEdges := methods[typeName+"_Edges"]
			_, hasMixin := methods[typeName+"_Mixin"]
			if !hasFields && !hasEdges && !hasMixin && !typeEmbedsEntSchema(f, typeName) {
				continue
			}
			schema := EntSchema{Name: typeName}

			if fieldsMethod, ok := methods[typeName+"_Fields"]; ok {
				schema.Fields = parseEntFields(fieldsMethod)
			}
			if edgesMethod, ok := methods[typeName+"_Edges"]; ok {
				schema.Edges = parseEntEdges(edgesMethod)
			}

			schemas = append(schemas, schema)
		}
	}

	if len(schemas) == 0 {
		return nil, fmt.Errorf("no ent schemas found in %s", dir)
	}
	if err := validateEntEdgeExposure(schemas); err != nil {
		return nil, fmt.Errorf("validate ent schemas: %w", err)
	}

	return schemas, nil
}

func typeEmbedsEntSchema(f *ast.File, typeName string) bool {
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name != typeName {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return false
			}
			for _, field := range structType.Fields.List {
				if len(field.Names) != 0 {
					continue
				}
				selector, ok := field.Type.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Schema" {
					continue
				}
				packageName, ok := selector.X.(*ast.Ident)
				if ok && packageName.Name == "ent" {
					return true
				}
			}
			return false
		}
	}
	return false
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
		Name:     fieldName,
		JSONName: fieldName,
		Type:     typeName,
		GoType:   entTypeToGoType(typeName),
	}

	// Check for chained methods.
	sensitive := false
	for _, call := range chain[1:] {
		if methodSel, ok := call.Fun.(*ast.SelectorExpr); ok {
			switch methodSel.Sel.Name {
			case "Optional":
				f.Optional = true
			case "Nillable":
				f.Nillable = true
			case "Sensitive":
				sensitive = true
			case "StructTag":
				if jsonName, ok := parseEntJSONStructTag(call); ok {
					f.JSONName = jsonName
				}
			case "GoType":
				f.CustomGoType = true
			}
		}
	}
	if sensitive {
		f.JSONName = "-"
	}
	if f.Nillable {
		f.GoType = "*" + f.GoType
	}

	return f
}

func parseEntJSONStructTag(call *ast.CallExpr) (string, bool) {
	if len(call.Args) != 1 {
		return "", false
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	tag, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	jsonTag, ok := reflect.StructTag(tag).Lookup("json")
	if !ok {
		return "", false
	}
	return strings.Split(jsonTag, ",")[0], true
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
			case "Field":
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						e.Field = strings.Trim(lit.Value, `"`)
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
	if err := validateEntSchemas(schemas); err != nil {
		return fmt.Errorf("validate ent schemas: %w", err)
	}
	if err := validateResolvedEntSchemas(schemas); err != nil {
		return fmt.Errorf("validate generated ent signatures: %w", err)
	}
	models := normalizeEntModels(schemas)
	spec, err := importSpec("ent", entImportPath)
	if err != nil {
		return fmt.Errorf("ent import: %w", err)
	}
	imports := []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
		spec,
	}
	if normalizedModelsNeedTimeImport(models) {
		imports = append(imports, `"time"`)
	}
	if entSchemasNeedReflect(schemas) {
		imports = append(imports, `"reflect"`)
	}
	return generateNormalizedCode(w, "ent", pkg, imports, models, false)
}

func entSchemasNeedReflect(schemas []EntSchema) bool {
	for _, schema := range schemas {
		optionalRelationFields := entOptionalRelationFields(schema)
		for _, field := range schema.Fields {
			if entFieldUsesReflectGuard(field, optionalRelationFields) {
				return true
			}
		}
	}
	return false
}

// entOptionalRelationFields returns the Go names of fields backing an optional
// edge's foreign key, whose zero value must not reach the database.
func entOptionalRelationFields(schema EntSchema) map[string]struct{} {
	fields := make(map[string]struct{}, len(schema.Edges))
	for _, edge := range schema.Edges {
		if edge.Field != "" && !edge.Required {
			if field, ok := lookupEntField(schema.Fields, edge.Field); ok {
				fields[field.GoName] = struct{}{}
			}
		}
	}
	return fields
}

// entFieldUsesReflectGuard reports whether the generated insert hook wraps the
// field's setter in a reflect.ValueOf(...).IsZero() guard. A pointer-backed
// setter is guarded by a nil check instead and never needs the reflect import.
func entFieldUsesReflectGuard(field EntField, optionalRelationFields map[string]struct{}) bool {
	if field.Nillable || field.SetterDeref {
		return false
	}
	_, ok := optionalRelationFields[field.GoName]
	return ok
}

func validateResolvedEntSchemas(schemas []EntSchema) error {
	for _, schema := range schemas {
		for _, field := range schema.Fields {
			if field.GoName == "" {
				return fmt.Errorf("schema %q field %q has no generated Go field mapping", schema.Name, field.Name)
			}
		}
	}
	return nil
}

func validateEntSchemas(schemas []EntSchema) error {
	if err := validateEntEdgeExposure(schemas); err != nil {
		return err
	}
	for _, schema := range schemas {
		for _, edge := range schema.Edges {
			if edge.Field == "" {
				continue
			}
			if _, ok := lookupEntField(schema.Fields, edge.Field); !ok {
				return fmt.Errorf("schema %q edge %q references unknown field %q", schema.Name, edge.Name, edge.Field)
			}
		}
	}
	return nil
}

// validateEntEdgeExposure rejects singular edges whose foreign key column
// neither side exposes with .Field(...). Ent only permits .Field(...) on the
// schema that actually holds the column, so an edge declared from the other side
// is fine as long as its counterpart exposes it. Requiring it on both sides
// would reject the standard two-type one-to-one shape outright.
func validateEntEdgeExposure(schemas []EntSchema) error {
	byName := make(map[string]EntSchema, len(schemas))
	for _, schema := range schemas {
		byName[schema.Name] = schema
	}

	for _, schema := range schemas {
		for _, edge := range schema.Edges {
			if edge.Field != "" || !edge.Unique && !edge.Required {
				continue
			}
			if counterpart, ok := entCounterpartEdge(byName, schema.Name, edge); ok && counterpart.Field != "" {
				continue
			}
			return fmt.Errorf("schema %q edge %q requires .Field(...) on whichever schema holds its foreign key column", schema.Name, edge.Name)
		}
	}
	return nil
}

// entCounterpartEdge finds the edge on the referenced schema that describes the
// same relationship. An inverse edge names its owner through Ref, and an owning
// edge is the one an inverse edge's Ref points back to.
func entCounterpartEdge(byName map[string]EntSchema, schemaName string, edge EntEdge) (EntEdge, bool) {
	target, ok := byName[edge.Type]
	if !ok {
		return EntEdge{}, false
	}
	for _, candidate := range target.Edges {
		if candidate.Type != schemaName {
			continue
		}
		if edge.Ref != "" && candidate.Name == edge.Ref {
			return candidate, true
		}
		if candidate.Ref != "" && candidate.Ref == edge.Name {
			return candidate, true
		}
	}
	return EntEdge{}, false
}

func lookupEntField(fields []EntField, name string) (EntField, bool) {
	for _, field := range fields {
		if field.Name == name || normalizeSQLCIdentifier(field.GoName) == normalizeSQLCIdentifier(name) {
			return field, true
		}
	}
	return EntField{}, false
}
