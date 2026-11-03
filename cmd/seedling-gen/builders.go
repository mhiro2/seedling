package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// builderModel describes the typed builder generated for one model.
type builderModel struct {
	Name     string // Fixtures method and model name, e.g. "User"
	Builder  string // builder type, e.g. "UserBuilder"
	TypeExpr string // blueprint type, e.g. "User" or "*ent.User"
	ElemExpr string // type the With callback points to, e.g. "User" or "ent.User"

	Setters   []builderSetter
	BelongsTo []builderBelongsTo
	HasMany   []builderHasMany
}

type builderSetter struct {
	Method string
	Field  string
	Type   string
}

type builderBelongsTo struct {
	Relation      string
	Method        string // configures the created parent, e.g. "Company"
	UseMethod     string // binds an existing parent, e.g. "UseCompany"
	LinkMethod    string // binds a scenario record, e.g. "LinkCompany"
	TargetBuilder string
	TargetType    string
}

type builderHasMany struct {
	Relation      string
	Method        string
	TargetBuilder string
}

// builderMethods are the methods every generated builder declares, which
// field and relation methods must not shadow.
var builderMethods = []string{
	"With", "WithOptions", "Trait", "Apply", "AfterInsert",
	"Insert", "InsertE", "InsertMany", "Build", "Add",
}

// builderTypeImports maps the package qualifiers a setter parameter type may
// use to the import they need. Types from any other package are left to the
// typed With callback, since their import path cannot be known here.
var builderTypeImports = map[string]string{
	"time": `"time"`,
	"sql":  `"database/sql"`,
	"json": `"encoding/json"`,
}

// buildBuilderModels derives the typed builders for models, whose relations
// must already include the reverse has-many relations. It returns the imports
// the setter parameter types need. qualifiedTypes allows field types from the
// packages in builderTypeImports; it must be false when the model source is
// hand-written, since its package qualifiers may be aliases of other imports.
func buildBuilderModels(models []normalizedModel, qualifiedTypes bool) ([]builderModel, []string) {
	byID := make(map[string]normalizedModel, len(models))
	for _, model := range models {
		byID[model.BlueprintID] = model
	}

	var imports []string
	builders := make([]builderModel, 0, len(models))
	for _, model := range models {
		b := builderModel{
			Name:     model.FieldName,
			Builder:  builderTypeName(model),
			TypeExpr: model.TypeExpr,
			ElemExpr: strings.TrimPrefix(model.TypeExpr, "*"),
		}
		taken := make(map[string]struct{}, len(builderMethods)+len(model.Fields)+3*len(model.Relations))
		for _, method := range builderMethods {
			taken[method] = struct{}{}
		}
		claim := func(name string) bool {
			if !token.IsIdentifier(name) || !token.IsExported(name) {
				return false
			}
			if _, exists := taken[name]; exists {
				return false
			}
			taken[name] = struct{}{}
			return true
		}

		// Relations claim their names first, so a field never hides the
		// typed way to configure a related record.
		for _, rel := range model.Relations {
			target, ok := byID[rel.RefBlueprint]
			if !ok {
				continue
			}
			method := exportedName(rel.Name)
			if rel.HasMany {
				if claim(method) {
					b.HasMany = append(b.HasMany, builderHasMany{Relation: rel.Name, Method: method, TargetBuilder: builderTypeName(target)})
				}
				continue
			}
			if !claim(method) || !claim("Use"+method) || !claim("Link"+method) {
				continue
			}
			b.BelongsTo = append(b.BelongsTo, builderBelongsTo{
				Relation:      rel.Name,
				Method:        method,
				UseMethod:     "Use" + method,
				LinkMethod:    "Link" + method,
				TargetBuilder: builderTypeName(target),
				TargetType:    target.TypeExpr,
			})
		}

		// A field promoted from an embedded struct is shadowed by a direct
		// field of the same name, which is the one Set resolves to.
		direct := make(map[string]struct{}, len(model.Fields))
		for _, field := range model.Fields {
			if field.Embedded == nil {
				direct[field.GoName] = struct{}{}
			}
		}
		for _, field := range model.Fields {
			if field.IsRelationFK {
				continue
			}
			if _, shadowed := direct[field.GoName]; shadowed && field.Embedded != nil {
				continue
			}
			typeImports, ok := setterTypeImports(field.GoType)
			if ok && len(typeImports) > 0 && !qualifiedTypes {
				ok = false
			}
			if !ok || !claim(field.GoName) {
				continue
			}
			imports = append(imports, typeImports...)
			b.Setters = append(b.Setters, builderSetter{Method: field.GoName, Field: field.GoName, Type: field.GoType})
		}
		builders = append(builders, b)
	}
	return builders, uniqueStrings(imports)
}

func builderTypeName(model normalizedModel) string {
	return model.FieldName + "Builder"
}

// exportedName converts a relation name such as "assignee_tasks" or
// "ownedPets" into an exported method name.
func exportedName(name string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(name, "_") {
		if part == "" {
			continue
		}
		switch upper := strings.ToUpper(part); upper {
		case "ID", "URL", "API", "HTTP", "SQL":
			b.WriteString(upper)
		default:
			b.WriteString(upper[:1] + part[1:])
		}
	}
	return b.String()
}

// setterTypeImports reports whether a field type can be named in the generated
// file, and which imports naming it needs. Predeclared types and pointers,
// slices, arrays, and maps of them are accepted, as are types from the few
// standard packages in builderTypeImports. any is rejected: adapters fall back
// to it for types they cannot render, so it does not mean the field is untyped.
func setterTypeImports(goType string) ([]string, bool) {
	expr, err := parser.ParseExpr(goType)
	if err != nil {
		return nil, false
	}
	var imports []string
	ok := true
	ast.Inspect(expr, func(node ast.Node) bool {
		switch n := node.(type) {
		case nil:
			return false
		case *ast.Ident:
			if !predeclaredTypes[n.Name] {
				ok = false
			}
		case *ast.SelectorExpr:
			pkg, isIdent := n.X.(*ast.Ident)
			spec, known := "", false
			if isIdent {
				spec, known = builderTypeImports[pkg.Name]
			}
			if !known {
				ok = false
			} else {
				imports = append(imports, spec)
			}
			return false
		case *ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.BasicLit:
		default:
			ok = false
		}
		return ok
	})
	return imports, ok
}

var predeclaredTypes = map[string]bool{
	"bool": true, "string": true, "byte": true, "rune": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
}

// validateBuilderNames rejects a builder type that collides with another
// declaration of the generated file.
func validateBuilderNames(models []normalizedModel, emitStructs bool) error {
	declared := make(map[string]string, 2*len(models))
	for name := range reservedGeneratedNames {
		declared[name] = "a generated declaration"
	}
	if emitStructs {
		for _, model := range models {
			declared[model.StructName] = "model struct " + strconv.Quote(model.StructName)
		}
	}
	for _, model := range models {
		name := builderTypeName(model)
		if other, exists := declared[name]; exists {
			return fmt.Errorf("model %q: builder type %q collides with %s", model.FieldName, name, other)
		}
		declared[name] = "builder type " + strconv.Quote(name)
	}
	return nil
}

const builderTemplate = `
// Fixtures builds records through the blueprints registered in a registry,
// with a typed builder per model. Setters exist for fields whose type the
// generator can name; use With for the others.
type Fixtures struct {
	reg *seedling.Registry
}

// NewFixtures returns typed builders backed by reg, which must hold the
// registered Blueprints.
func NewFixtures(reg *seedling.Registry) Fixtures {
	return Fixtures{reg: reg}
}
{{range $b := .}}
// {{$b.Builder}} configures a {{$b.Name}} record. Builders are values: every
// method returns a new builder, so a shared base can be extended safely.
type {{$b.Builder}} struct {
	reg  *seedling.Registry
	opts []seedling.Option
}

// {{$b.Name}} starts a {{$b.Name}} record.
func (f Fixtures) {{$b.Name}}() {{$b.Builder}} {
	return {{$b.Builder}}{reg: f.reg}
}

func (b {{$b.Builder}}) with(opts ...seedling.Option) {{$b.Builder}} {
	b.opts = append(b.opts[:len(b.opts):len(b.opts)], opts...)
	return b
}
{{range $b.Setters}}
// {{.Method}} sets {{.Field}}.
func (b {{$b.Builder}}) {{.Method}}(v {{.Type}}) {{$b.Builder}} {
	return b.with(seedling.Set({{quote .Field}}, v))
}
{{end}}
{{- range $b.BelongsTo}}
// {{.Method}} configures the {{quote .Relation}} record created for this record.
func (b {{$b.Builder}}) {{.Method}}(parent {{.TargetBuilder}}) {{$b.Builder}} {
	return b.with(seedling.Ref({{quote .Relation}}, parent.opts...))
}

// {{.UseMethod}} binds {{quote .Relation}} to an existing record instead of creating one.
func (b {{$b.Builder}}) {{.UseMethod}}(parent {{.TargetType}}) {{$b.Builder}} {
	return b.with(seedling.Use({{quote .Relation}}, parent))
}

// {{.LinkMethod}} binds {{quote .Relation}} to a record added earlier to the same scenario.
func (b {{$b.Builder}}) {{.LinkMethod}}(parent seedling.Node[{{.TargetType}}]) {{$b.Builder}} {
	return b.with(seedling.Link({{quote .Relation}}, parent))
}
{{end}}
{{- range $b.HasMany}}
// {{.Method}} creates one {{quote .Relation}} record per builder; with none, it creates none.
func (b {{$b.Builder}}) {{.Method}}(children ...{{.TargetBuilder}}) {{$b.Builder}} {
	opts := make([]seedling.Option, 0, len(children)+1)
	opts = append(opts, seedling.Count(len(children)))
	for i, child := range children {
		opts = append(opts, seedling.At(i, child.opts...))
	}
	return b.with(seedling.Ref({{quote .Relation}}, opts...))
}
{{end}}
// With mutates the record with a typed function after Set options apply.
func (b {{$b.Builder}}) With(fn func(*{{$b.ElemExpr}})) {{$b.Builder}} {
	return b.with(seedling.With(fn))
}

// WithOptions appends untyped options, such as Seq or Omit.
func (b {{$b.Builder}}) WithOptions(opts ...seedling.Option) {{$b.Builder}} {
	return b.with(opts...)
}

// Trait applies a trait registered on the blueprint.
func (b {{$b.Builder}}) Trait(name string) {{$b.Builder}} {
	return b.with(seedling.BlueprintTrait(name))
}

// Apply applies typed presets in order.
func (b {{$b.Builder}}) Apply(presets ...func({{$b.Builder}}) {{$b.Builder}}) {{$b.Builder}} {
	for _, preset := range presets {
		b = preset(b)
	}
	return b
}

// AfterInsert registers a callback that runs after the record is inserted.
// It applies only to the record a terminal method inserts, so a builder that
// carries it cannot be passed to another builder's relation method.
func (b {{$b.Builder}}) AfterInsert(fn func({{$b.TypeExpr}}, seedling.DBTX) error) {{$b.Builder}} {
	return b.with(seedling.AfterInsertE(fn))
}

// Insert inserts the record with its dependencies. Fails the test on error.
func (b {{$b.Builder}}) Insert(tb testing.TB, db seedling.DBTX) seedling.Result[{{$b.TypeExpr}}] {
	tb.Helper()
	return seedling.NewSession[{{$b.TypeExpr}}](b.reg).InsertOne(tb, db, b.opts...)
}

// InsertE inserts the record with its dependencies, returning an error on failure.
func (b {{$b.Builder}}) InsertE(ctx context.Context, db seedling.DBTX) (seedling.Result[{{$b.TypeExpr}}], error) {
	return seedling.NewSession[{{$b.TypeExpr}}](b.reg).InsertOneE(ctx, db, b.opts...)
}

// InsertMany inserts n records configured alike. Fails the test on error.
func (b {{$b.Builder}}) InsertMany(tb testing.TB, db seedling.DBTX, n int) []{{$b.TypeExpr}} {
	tb.Helper()
	return seedling.NewSession[{{$b.TypeExpr}}](b.reg).InsertMany(tb, db, n, b.opts...)
}

// Build plans the record without inserting it. Fails the test on error.
func (b {{$b.Builder}}) Build(tb testing.TB) *seedling.Plan[{{$b.TypeExpr}}] {
	tb.Helper()
	return seedling.NewSession[{{$b.TypeExpr}}](b.reg).Build(tb, b.opts...)
}

// Add adds the record to a scenario and returns its handle.
func (b {{$b.Builder}}) Add(s *seedling.Scenario) seedling.Node[{{$b.TypeExpr}}] {
	return seedling.Add[{{$b.TypeExpr}}](s, b.opts...)
}
{{end}}`
