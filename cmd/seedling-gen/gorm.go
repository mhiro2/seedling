package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
)

// GormModel represents a parsed GORM model struct.
type GormModel struct {
	Name   string
	Table  string
	Fields []GormField
}

// GormField represents a field in a GORM model.
type GormField struct {
	Name       string
	Type       string
	ColumnName string
	IsPK       bool
	NotNull    bool
	IsFK       bool
	FKRefModel string
	Relation   *GormRelation
}

// GormRelation represents a GORM relationship extracted from struct tags.
type GormRelation struct {
	Kind       string // "BelongsTo", "HasMany", "HasOne", "ManyToMany"
	ForeignKey string
	JoinTable  string
	RefModel   string
}

// ParseGormDir parses Go source files containing GORM model definitions.
func ParseGormDir(dir string) ([]GormModel, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read gorm dir: %w", err)
	}

	fset := token.NewFileSet()
	var files []*ast.File

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
		files = append(files, f)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files found in %s", dir)
	}

	// Collect all struct types and their table name methods.
	structTypes := make(map[string]*ast.StructType)
	tableNames := make(map[string]string) // struct name -> table name from TableName() method

	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts := spec.(*ast.TypeSpec)
					if st, ok := ts.Type.(*ast.StructType); ok {
						structTypes[ts.Name.Name] = st
					}
				}
			case *ast.FuncDecl:
				// Detect TableName() method.
				if d.Name.Name == "TableName" && d.Recv != nil && d.Type.Results != nil {
					recvName := receiverTypeName(d.Recv)
					if recvName != "" {
						if ret := extractStringReturn(d.Body); ret != "" {
							tableNames[recvName] = ret
						}
					}
				}
			}
		}
	}

	var models []GormModel
	for name, st := range structTypes {
		model := parseGormStruct(name, st, structTypes)
		if table, ok := tableNames[name]; ok {
			model.Table = table
		}
		if model.Table == "" {
			model.Table = toSnakeCase(name) + "s"
		}
		models = append(models, model)
	}

	return models, nil
}

func parseGormStruct(name string, st *ast.StructType, allStructs map[string]*ast.StructType) GormModel {
	model := GormModel{Name: name}

	for _, field := range st.Fields.List {
		// Handle embedded gorm.Model.
		if len(field.Names) == 0 {
			if isGormModelEmbed(field.Type) {
				model.Fields = append(model.Fields,
					GormField{Name: "ID", Type: "uint", ColumnName: "id", IsPK: true},
					GormField{Name: "CreatedAt", Type: "time.Time", ColumnName: "created_at"},
					GormField{Name: "UpdatedAt", Type: "time.Time", ColumnName: "updated_at"},
					GormField{Name: "DeletedAt", Type: "gorm.DeletedAt", ColumnName: "deleted_at"},
				)
				continue
			}
			continue
		}

		fieldName := field.Names[0].Name
		fieldType := exprToString(field.Type)
		gormTag := extractTag(field, "gorm")

		gf := GormField{
			Name:       fieldName,
			Type:       fieldType,
			ColumnName: toSnakeCase(fieldName),
		}

		// Parse gorm tag.
		tagParts := parseGormTag(gormTag)
		if v, ok := tagParts["column"]; ok {
			gf.ColumnName = v
		}
		if _, ok := tagParts["primaryKey"]; ok {
			gf.IsPK = true
		}
		if _, ok := tagParts["not null"]; ok {
			gf.NotNull = true
		}

		// Detect relationships.
		if rel := detectGormRelation(field, fieldType, tagParts, allStructs); rel != nil {
			gf.Relation = rel
			gf.IsFK = rel.Kind == "BelongsTo"
			gf.FKRefModel = rel.RefModel
		}

		model.Fields = append(model.Fields, gf)
	}

	return model
}

func detectGormRelation(field *ast.Field, fieldType string, tagParts map[string]string, allStructs map[string]*ast.StructType) *GormRelation {
	// many2many tag.
	if joinTable, ok := tagParts["many2many"]; ok {
		refModel := stripSlicePrefix(fieldType)
		return &GormRelation{
			Kind:       "ManyToMany",
			JoinTable:  joinTable,
			ForeignKey: tagParts["foreignKey"],
			RefModel:   refModel,
		}
	}

	// foreignKey tag on a struct or slice field.
	if fk, ok := tagParts["foreignKey"]; ok {
		refModel := stripSlicePrefix(fieldType)
		if _, isStruct := allStructs[refModel]; isStruct {
			if strings.HasPrefix(fieldType, "[]") {
				return &GormRelation{Kind: "HasMany", ForeignKey: fk, RefModel: refModel}
			}
			return &GormRelation{Kind: "BelongsTo", ForeignKey: fk, RefModel: refModel}
		}
	}

	// Detect BelongsTo by convention: field type is a known struct.
	bareType := stripPointer(fieldType)
	if _, isStruct := allStructs[bareType]; isStruct && !strings.HasPrefix(fieldType, "[]") {
		fkField := field.Names[0].Name + "ID"
		return &GormRelation{Kind: "BelongsTo", ForeignKey: fkField, RefModel: bareType}
	}

	return nil
}

func isGormModelEmbed(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "gorm" && sel.Sel.Name == "Model"
}

func extractTag(field *ast.Field, key string) string {
	if field.Tag == nil {
		return ""
	}
	raw := field.Tag.Value
	raw = strings.Trim(raw, "`")
	tag := reflect.StructTag(raw)
	val, _ := tag.Lookup(key)
	return val
}

func parseGormTag(tag string) map[string]string {
	parts := make(map[string]string)
	if tag == "" {
		return parts
	}
	for segment := range strings.SplitSeq(tag, ";") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if k, v, ok := strings.Cut(segment, ":"); ok {
			parts[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			parts[segment] = ""
		}
	}
	return parts
}

func receiverTypeName(recv *ast.FieldList) string {
	if len(recv.List) != 1 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func extractStringReturn(body *ast.BlockStmt) string {
	if body == nil || len(body.List) == 0 {
		return ""
	}
	for _, stmt := range body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			continue
		}
		lit, ok := ret.Results[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		return strings.Trim(lit.Value, `"`)
	}
	return ""
}

func stripSlicePrefix(s string) string {
	s = strings.TrimPrefix(s, "[]")
	s = strings.TrimPrefix(s, "*")
	return s
}

func stripPointer(s string) string {
	return strings.TrimPrefix(s, "*")
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

const gormCodeTemplate = `package {{.Package}}

import (
	"context"

	"github.com/mhiro2/seedling"
	"gorm.io/gorm"
	{{.ModelPkgAlias}} "{{.ModelImportPath}}"
)

func RegisterBlueprints() {
{{- range $i, $entry := .Entries}}
{{- if $i}}
{{end}}
	seedling.MustRegister(seedling.Blueprint[{{$.ModelPkgAlias}}.{{$entry.GoName}}]{
		Name:  "{{$entry.BlueprintID}}",
		Table: "{{$entry.TableName}}",
{{- if $entry.CompositePK}}
		PKFields: []string{ {{- range $i, $field := $entry.PKFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} },
{{- else}}
		PKField: "{{$entry.PKField}}",
{{- end}}
		Defaults: func() {{$.ModelPkgAlias}}.{{$entry.GoName}} {
			return {{$.ModelPkgAlias}}.{{$entry.GoName}}{}
		},
{{- if $entry.HasRelations}}
		Relations: []seedling.Relation{
{{- range $entry.Relations}}
			{Name: "{{.Name}}", Kind: seedling.BelongsTo, {{- if .Composite}} LocalFields: []string{ {{- range $i, $field := .LocalFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} }, {{- else}} LocalField: "{{.LocalField}}", {{- end}} RefBlueprint: "{{.RefBlueprint}}"{{- if .Optional}}, Optional: true{{- end}}},
{{- end}}
		},
{{- end}}
		Insert: func(ctx context.Context, dbtx seedling.DBTX, v {{$.ModelPkgAlias}}.{{$entry.GoName}}) ({{$.ModelPkgAlias}}.{{$entry.GoName}}, error) {
			if err := dbtx.(*gorm.DB).WithContext(ctx).Create(&v).Error; err != nil {
				return v, err
			}
			return v, nil
		},
		Delete: func(ctx context.Context, dbtx seedling.DBTX, v {{$.ModelPkgAlias}}.{{$entry.GoName}}) error {
			return dbtx.(*gorm.DB).WithContext(ctx).Delete(&v).Error
		},
	})
{{- end}}
}
`

type gormEntry struct {
	GoName       string
	BlueprintID  string
	TableName    string
	PKField      string
	PKFields     []string
	CompositePK  bool
	HasRelations bool
	Relations    []relationInfo
}

type gormTemplateData struct {
	Package         string
	ModelPkgAlias   string
	ModelImportPath string
	Entries         []gormEntry
}

// GenerateGorm generates Blueprint registration code for GORM models.
func GenerateGorm(w io.Writer, pkg, modelImportPath string, models []GormModel) error {
	alias := filepath.Base(modelImportPath)

	entries := make([]gormEntry, 0, len(models))
	for _, m := range models {
		entry := gormEntry{
			GoName:      m.Name,
			BlueprintID: strings.ToLower(m.Name[:1]) + m.Name[1:],
			TableName:   m.Table,
		}

		// Extract PKs.
		var pks []string
		for _, f := range m.Fields {
			if f.IsPK {
				pks = append(pks, f.Name)
			}
		}
		if len(pks) == 0 {
			pks = []string{"ID"}
		}
		entry.PKField = pks[0]
		entry.PKFields = pks
		entry.CompositePK = len(pks) > 1

		// Build relations from detected relationships.
		var rels []relationInfo
		for _, f := range m.Fields {
			if f.Relation == nil {
				continue
			}
			rel := f.Relation
			if rel.Kind != "BelongsTo" {
				continue // Only BelongsTo for now.
			}
			fkField := rel.ForeignKey
			if fkField == "" {
				fkField = f.Name + "ID"
			}
			refBP := strings.ToLower(rel.RefModel[:1]) + rel.RefModel[1:]
			rels = append(rels, relationInfo{
				Name:         strings.ToLower(f.Name[:1]) + f.Name[1:],
				LocalField:   fkField,
				LocalFields:  []string{fkField},
				RefBlueprint: refBP,
				Optional:     !f.NotNull,
			})
		}
		entry.HasRelations = len(rels) > 0
		entry.Relations = rels

		entry.BlueprintID = singularize(strings.ToLower(m.Name))

		entries = append(entries, entry)
	}

	data := gormTemplateData{
		Package:         pkg,
		ModelPkgAlias:   alias,
		ModelImportPath: modelImportPath,
		Entries:         entries,
	}

	tmpl, err := template.New("gorm").Parse(gormCodeTemplate)
	if err != nil {
		return fmt.Errorf("parse gorm template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute gorm template: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return fmt.Errorf("format gorm generated code: %w", err)
	}

	_, err = w.Write(formatted)
	if err != nil {
		return fmt.Errorf("write gorm generated code: %w", err)
	}
	return nil
}
