package main

import (
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"strconv"
	"strings"
	"text/template"
)

type normalizedField struct {
	GoName       string
	GoType       string
	IsPK         bool
	IsRelationFK bool
	IsOptional   bool
}

type normalizedRelation struct {
	Name         string
	LocalField   string
	LocalFields  []string
	RefBlueprint string
	Optional     bool
}

type normalizedMutationHook struct {
	Body string
}

type normalizedModel struct {
	StructName    string
	TypeExpr      string
	ZeroValueExpr string
	BlueprintID   string
	TableName     string
	PKFields      []string
	Fields        []normalizedField
	Relations     []normalizedRelation
	InsertHook    *normalizedMutationHook
	DeleteHook    *normalizedMutationHook
}

const normalizedStructTemplate = `
{{- range .}}
type {{.StructName}} struct {
{{- range .Fields}}
	{{.GoName}} {{.GoType}}
{{- end}}
}
{{ end }}
`

const normalizedBlueprintTemplate = `
func NewRegistry() *seedling.Registry {
	reg := seedling.NewRegistry()
	RegisterBlueprints(reg)
	return reg
}

func RegisterBlueprints(reg *seedling.Registry) {
{{- range $i, $model := .}}
{{- if $i}}
{{ end }}
	seedling.MustRegisterTo(reg, seedling.Blueprint[{{$model.TypeExpr}}]{
		Name:  {{quote $model.BlueprintID}},
		Table: {{quote $model.TableName}},
{{- if isCompositePK $model}}
		PKFields: []string{ {{- range $i, $field := $model.PKFields}}{{if $i}}, {{end}}{{quote $field}}{{end}} },
{{- else}}
		PKField: {{quote (pkField $model)}},
{{- end}}
		Defaults: func() {{$model.TypeExpr}} {
			return {{ defaultLiteral $model }}
		},
{{- if $model.Relations}}
		Relations: []seedling.Relation{
{{- range $model.Relations}}
			{Name: {{quote .Name}}, Kind: seedling.BelongsTo, {{- if isCompositeRelation .}} LocalFields: []string{ {{- range $i, $field := .LocalFields}}{{if $i}}, {{end}}{{quote $field}}{{end}} }, {{- else}} LocalField: {{quote .LocalField}}, {{- end}} RefBlueprint: {{quote .RefBlueprint}}{{- if .Optional}}, Optional: true{{- end}}},
{{- end}}
		},
{{- end}}
{{- if $model.InsertHook}}
		Insert: func(ctx context.Context, dbtx seedling.DBTX, v {{$model.TypeExpr}}) ({{$model.TypeExpr}}, error) {
{{ indent 3 $model.InsertHook.Body }}
		},
{{- end}}
{{- if $model.DeleteHook}}
		Delete: func(ctx context.Context, dbtx seedling.DBTX, v {{$model.TypeExpr}}) error {
{{ indent 3 $model.DeleteHook.Body }}
		},
{{- end}}
	})
{{- end}}
}
`

func generateNormalizedCode(w io.Writer, kind, pkg string, imports []string, models []normalizedModel, emitStructs bool) error {
	if err := validateNormalizedModels(models, emitStructs); err != nil {
		return fmt.Errorf("validate %s generated code: %w", kind, err)
	}

	var buf strings.Builder

	buf.WriteString("package ")
	buf.WriteString(pkg)
	buf.WriteString("\n\n")

	renderImports(&buf, imports)

	if emitStructs {
		structs, err := renderNormalizedTemplate("structs", normalizedStructTemplate, models, nil)
		if err != nil {
			return fmt.Errorf("render %s structs: %w", kind, err)
		}
		buf.WriteString(structs)
		buf.WriteString("\n")
	}

	blueprints, err := renderNormalizedTemplate("blueprints", normalizedBlueprintTemplate, models, template.FuncMap{
		"pkField": func(model normalizedModel) string {
			return normalizedPKField(model.PKFields)
		},
		"isCompositePK": func(model normalizedModel) bool {
			return len(model.PKFields) > 1
		},
		"isCompositeRelation": func(rel normalizedRelation) bool {
			return len(rel.LocalFields) > 1
		},
		"defaultLiteral": buildDefaultLiteral,
		"indent":         indentBlock,
		"quote":          strconv.Quote,
	})
	if err != nil {
		return fmt.Errorf("render %s blueprints: %w", kind, err)
	}
	buf.WriteString(blueprints)

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return fmt.Errorf("format %s generated code: %w", kind, err)
	}

	if _, err := w.Write(formatted); err != nil {
		return fmt.Errorf("write %s generated code: %w", kind, err)
	}

	return nil
}

// validateNormalizedModels guards against schema-derived identifiers and type
// expressions injecting arbitrary tokens into the generated source. Identifier
// positions must be valid Go identifiers, and type/expression positions must
// parse as a single Go expression so a crafted value cannot break out and
// append statements or fields. String-literal positions are escaped with
// strconv.Quote at render time and therefore need no validation here. This
// turns malformed input into a hard error instead of relying on format.Source
// to reject whatever the templates happen to emit.
func validateNormalizedModels(models []normalizedModel, emitStructs bool) error {
	for _, model := range models {
		name := model.StructName
		if name == "" {
			name = model.TypeExpr
		}

		if err := validateGoExpr("type expression", model.TypeExpr); err != nil {
			return fmt.Errorf("model %q: %w", name, err)
		}
		if model.ZeroValueExpr != "" {
			if err := validateGoExpr("zero-value expression", model.ZeroValueExpr); err != nil {
				return fmt.Errorf("model %q: %w", name, err)
			}
		}

		if emitStructs && !token.IsIdentifier(model.StructName) {
			return fmt.Errorf("model %q: invalid struct name %q", name, model.StructName)
		}

		for _, field := range model.Fields {
			if !token.IsIdentifier(field.GoName) {
				return fmt.Errorf("model %q: invalid field name %q", name, field.GoName)
			}
			// GoType is only emitted as a type in the struct template.
			if emitStructs {
				if err := validateGoExpr("field type", field.GoType); err != nil {
					return fmt.Errorf("model %q field %q: %w", name, field.GoName, err)
				}
			}
		}
	}
	return nil
}

// validateGoExpr reports whether expr parses as a single, complete Go
// expression. It is a breakout guard, not a type check: parser.ParseExpr
// rejects trailing tokens, so a crafted value cannot append statements or
// declarations after the expression, while the Go compiler still validates
// whether the expression is usable as a type when the generated code is built.
func validateGoExpr(label, expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("empty %s", label)
	}
	if _, err := parser.ParseExpr(expr); err != nil {
		return fmt.Errorf("invalid %s %q: %w", label, expr, err)
	}
	return nil
}

// importSpec builds one import line with the path escaped via strconv.Quote and
// the alias (when present) validated as a Go identifier. Import paths and
// aliases come from CLI flags / tool config rather than the schema, but they
// reach the generated source through the same raw-concatenation path, so they
// are hardened the same way instead of trusting the caller-supplied string.
func importSpec(alias, path string) (string, error) {
	if alias != "" && !token.IsIdentifier(alias) {
		return "", fmt.Errorf("invalid import alias %q", alias)
	}
	if alias == "" {
		return strconv.Quote(path), nil
	}
	return alias + " " + strconv.Quote(path), nil
}

func renderImports(buf *strings.Builder, imports []string) {
	buf.WriteString("import (\n")
	for _, imp := range uniqueStrings(imports) {
		buf.WriteString("\t")
		buf.WriteString(imp)
		buf.WriteString("\n")
	}
	buf.WriteString(")\n\n")
}

func renderNormalizedTemplate(name, text string, data any, funcs template.FuncMap) (string, error) {
	tmpl := template.New(name)
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}

	parsed, err := tmpl.Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse %s template: %w", name, err)
	}

	var buf strings.Builder
	if err := parsed.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute %s template: %w", name, err)
	}

	return buf.String(), nil
}

func indentBlock(level int, body string) string {
	if body == "" {
		return ""
	}

	prefix := strings.Repeat("\t", level)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}

	return strings.Join(lines, "\n") + "\n"
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeTableModels(tables []Table) []normalizedModel {
	models := make([]normalizedModel, 0, len(tables))
	for _, table := range tables {
		fields := make([]normalizedField, 0, len(table.Columns))
		for _, column := range table.Columns {
			fields = append(fields, normalizedField{
				GoName:       column.GoName,
				GoType:       column.GoType,
				IsPK:         column.IsPK,
				IsRelationFK: column.IsFK,
				IsOptional:   !column.NotNull,
			})
		}

		models = append(models, normalizedModel{
			StructName:    table.GoName,
			TypeExpr:      table.GoName,
			ZeroValueExpr: table.GoName + "{}",
			BlueprintID:   table.BlueprintID,
			TableName:     table.Name,
			PKFields:      normalizedPKFields(table.Columns),
			Fields:        fields,
			Relations:     normalizeTableRelations(table),
			InsertHook: &normalizedMutationHook{
				Body: "// TODO: implement\nreturn v, nil",
			},
		})
	}
	return models
}

func normalizeSqlcModels(tables []Table, sqlcInfo *SqlcInfo) []normalizedModel {
	models := make([]normalizedModel, 0, len(tables))
	for _, table := range tables {
		model := normalizedModel{
			TypeExpr:      sqlcInfo.Package + "." + table.GoName,
			ZeroValueExpr: sqlcInfo.Package + "." + table.GoName + "{}",
			BlueprintID:   table.BlueprintID,
			TableName:     table.Name,
			PKFields:      normalizedPKFields(table.Columns),
			Fields:        normalizedTableFields(table),
			Relations:     normalizeTableRelations(table),
		}

		if query := sqlcInfo.FindQueryForTable(table); query != nil && query.ParamType != "" {
			model.InsertHook = &normalizedMutationHook{
				Body: buildSqlcInsertHook(sqlcInfo.Package, *query),
			}
		} else {
			model.InsertHook = &normalizedMutationHook{
				Body: "// TODO: implement\nreturn v, nil",
			}
		}

		if deleteQuery := sqlcInfo.FindDeleteQueryForTable(table); deleteQuery != nil {
			model.DeleteHook = &normalizedMutationHook{
				Body: buildSqlcDeleteHook(sqlcInfo.Package, *deleteQuery, model.PKFields),
			}
		}

		models = append(models, model)
	}
	return models
}

func normalizeGormModels(models []GormModel, alias string) []normalizedModel {
	normalized := make([]normalizedModel, 0, len(models))
	for _, model := range models {
		pkFields := make([]string, 0, len(model.Fields))
		for _, field := range model.Fields {
			if field.IsPK {
				pkFields = append(pkFields, field.Name)
			}
		}
		if len(pkFields) == 0 {
			pkFields = []string{"ID"}
		}

		relations := make([]normalizedRelation, 0, len(model.Fields))
		relationLocalFields := make(map[string]struct{})
		for _, field := range model.Fields {
			if field.Relation == nil || field.Relation.Kind != "BelongsTo" {
				continue
			}

			localField := field.Relation.ForeignKey
			if localField == "" {
				localField = field.Name + "ID"
			}

			relations = append(relations, normalizedRelation{
				Name:         strings.ToLower(field.Name[:1]) + field.Name[1:],
				LocalField:   localField,
				LocalFields:  []string{localField},
				RefBlueprint: singularize(strings.ToLower(field.Relation.RefModel)),
				Optional:     !field.NotNull,
			})
			relationLocalFields[localField] = struct{}{}
		}

		fields := make([]normalizedField, 0, len(model.Fields))
		for _, field := range model.Fields {
			_, isRelationFK := relationLocalFields[field.Name]
			fields = append(fields, normalizedField{
				GoName:       field.Name,
				GoType:       field.Type,
				IsPK:         field.IsPK,
				IsRelationFK: isRelationFK,
				IsOptional:   !field.NotNull,
			})
		}

		normalized = append(normalized, normalizedModel{
			TypeExpr:      alias + "." + model.Name,
			ZeroValueExpr: alias + "." + model.Name + "{}",
			BlueprintID:   singularize(strings.ToLower(model.Name)),
			TableName:     model.Table,
			PKFields:      pkFields,
			Fields:        fields,
			Relations:     relations,
			InsertHook: &normalizedMutationHook{
				Body: "if err := dbtx.(*gorm.DB).WithContext(ctx).Create(&v).Error; err != nil {\n\treturn v, err\n}\nreturn v, nil",
			},
			DeleteHook: &normalizedMutationHook{
				Body: "return dbtx.(*gorm.DB).WithContext(ctx).Delete(&v).Error",
			},
		})
	}
	return normalized
}

func normalizeEntModels(schemas []EntSchema) []normalizedModel {
	models := make([]normalizedModel, 0, len(schemas))
	for _, schema := range schemas {
		model := normalizedModel{
			TypeExpr:      "*ent." + schema.Name,
			ZeroValueExpr: "&ent." + schema.Name + "{}",
			BlueprintID:   singularize(strings.ToLower(schema.Name)),
			TableName:     singularize(strings.ToLower(schema.Name)) + "s",
			PKFields:      []string{"ID"},
			Fields:        normalizeEntFields(schema.Fields),
		}

		for _, edge := range schema.Edges {
			if edge.Direction != "From" {
				continue
			}

			localField := toGoFieldName(edge.Name) + "ID"
			model.Relations = append(model.Relations, normalizedRelation{
				Name:         edge.Name,
				LocalField:   localField,
				LocalFields:  []string{localField},
				RefBlueprint: singularize(strings.ToLower(edge.Type)),
				Optional:     !edge.Required,
			})
		}

		model.InsertHook = &normalizedMutationHook{
			Body: buildEntInsertHook(schema),
		}
		model.DeleteHook = &normalizedMutationHook{
			Body: "return dbtx.(*ent.Client)." + schema.Name + ".DeleteOneID(v.ID).Exec(ctx)",
		}

		models = append(models, model)
	}
	return models
}

func normalizedPKFields(columns []Column) []string {
	fields := make([]string, 0, len(columns))
	for _, column := range columns {
		if column.IsPK {
			fields = append(fields, column.GoName)
		}
	}
	if len(fields) == 0 {
		return []string{"ID"}
	}
	return fields
}

func normalizedPKField(fields []string) string {
	if len(fields) == 0 {
		return "ID"
	}
	return fields[0]
}

func normalizedTableFields(table Table) []normalizedField {
	fields := make([]normalizedField, 0, len(table.Columns))
	for _, column := range table.Columns {
		fields = append(fields, normalizedField{
			GoName:       column.GoName,
			GoType:       column.GoType,
			IsPK:         column.IsPK,
			IsRelationFK: column.IsFK,
			IsOptional:   !column.NotNull,
		})
	}
	return fields
}

func normalizeEntFields(fields []EntField) []normalizedField {
	normalized := make([]normalizedField, 0, len(fields))
	for _, field := range fields {
		normalized = append(normalized, normalizedField{
			GoName:     toGoFieldName(field.Name),
			GoType:     field.GoType,
			IsOptional: field.Optional,
		})
	}
	return normalized
}

func normalizeTableRelations(table Table) []normalizedRelation {
	relations := make([]normalizedRelation, 0, len(table.ForeignKeys))
	for _, foreignKey := range table.ForeignKeys {
		if len(foreignKey.Columns) == 0 {
			continue
		}

		localFields := make([]string, 0, len(foreignKey.Columns))
		for _, columnName := range foreignKey.Columns {
			for _, column := range table.Columns {
				if column.Name != columnName {
					continue
				}
				localFields = append(localFields, column.GoName)
				break
			}
		}
		if len(localFields) == 0 {
			continue
		}

		name := singularize(foreignKey.RefTable)
		if len(foreignKey.Columns) == 1 {
			name = relationNameForColumn(foreignKey.Columns[0], foreignKey.RefTable)
		}

		relation := normalizedRelation{
			Name:         name,
			LocalFields:  localFields,
			RefBlueprint: singularize(foreignKey.RefTable),
			Optional:     !foreignKey.NotNull,
		}
		if len(localFields) == 1 {
			relation.LocalField = localFields[0]
		}

		relations = append(relations, relation)
	}
	return relations
}

func buildDefaultLiteral(model normalizedModel) string {
	assignments := make([]string, 0, len(model.Fields))
	for _, field := range model.Fields {
		expr := defaultFieldExpr(model.BlueprintID, field)
		if expr == "" {
			continue
		}
		assignments = append(assignments, field.GoName+": "+expr)
	}
	if len(assignments) == 0 {
		return model.ZeroValueExpr
	}
	prefix, ok := strings.CutSuffix(model.ZeroValueExpr, "{}")
	if !ok {
		return model.ZeroValueExpr
	}
	return prefix + "{" + strings.Join(assignments, ", ") + "}"
}

func defaultFieldExpr(blueprintID string, field normalizedField) string {
	if field.IsPK || field.IsRelationFK {
		return ""
	}

	label := blueprintID + "-" + toSnakeCase(field.GoName)

	switch field.GoType {
	case "string":
		return strconv.Quote(label)
	case "bool":
		return "true"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return "1"
	case "[]byte":
		return "[]byte(" + strconv.Quote(label) + ")"
	case "time.Time":
		return "time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)"
	default:
		return ""
	}
}

func normalizedModelsNeedTimeImport(models []normalizedModel) bool {
	for _, model := range models {
		for _, field := range model.Fields {
			if field.GoType == "time.Time" {
				return true
			}
		}
	}
	return false
}

func buildSqlcInsertHook(alias string, query SqlcQuery) string {
	var body strings.Builder
	body.WriteString("return ")
	body.WriteString(alias)
	body.WriteString(".New(dbtx.(")
	body.WriteString(alias)
	body.WriteString(".DBTX)).")
	body.WriteString(query.Name)
	body.WriteString("(ctx, ")
	body.WriteString(alias)
	body.WriteString(".")
	body.WriteString(query.ParamType)
	body.WriteString("{\n")
	for _, field := range query.ParamFields {
		body.WriteString("\t")
		body.WriteString(field.Name)
		body.WriteString(": v.")
		body.WriteString(field.Name)
		body.WriteString(",\n")
	}
	body.WriteString("})")
	return body.String()
}

func buildSqlcDeleteHook(alias string, deleteQuery SqlcDeleteQuery, pkFields []string) string {
	var body strings.Builder
	body.WriteString("return ")
	body.WriteString(alias)
	body.WriteString(".New(dbtx.(")
	body.WriteString(alias)
	body.WriteString(".DBTX)).")
	body.WriteString(deleteQuery.Name)
	body.WriteString("(ctx, ")
	if deleteQuery.ArgName != "" {
		body.WriteString("v.")
		body.WriteString(pkFieldForDeleteArg(deleteQuery.ArgName, pkFields))
	} else {
		body.WriteString("v")
	}
	body.WriteString(")")
	return body.String()
}

func buildEntInsertHook(schema EntSchema) string {
	var body strings.Builder
	body.WriteString("builder := dbtx.(*ent.Client).")
	body.WriteString(schema.Name)
	body.WriteString(".Create()\n")
	for _, field := range schema.Fields {
		body.WriteString("builder.Set")
		body.WriteString(toGoFieldName(field.Name))
		body.WriteString("(v.")
		body.WriteString(toGoFieldName(field.Name))
		body.WriteString(")\n")
	}
	body.WriteString("return builder.Save(ctx)")
	return body.String()
}
