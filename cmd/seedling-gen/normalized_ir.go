package main

import (
	"fmt"
	"go/format"
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
func RegisterBlueprints() {
{{- range $i, $model := .}}
{{- if $i}}
{{ end }}
	seedling.MustRegister(seedling.Blueprint[{{$model.TypeExpr}}]{
		Name:  "{{$model.BlueprintID}}",
		Table: "{{$model.TableName}}",
{{- if isCompositePK $model}}
		PKFields: []string{ {{- range $i, $field := $model.PKFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} },
{{- else}}
		PKField: "{{pkField $model}}",
{{- end}}
		Defaults: func() {{$model.TypeExpr}} {
			return {{ defaultLiteral $model }}
		},
{{- if $model.Relations}}
		Relations: []seedling.Relation{
{{- range $model.Relations}}
			{Name: "{{.Name}}", Kind: seedling.BelongsTo, {{- if isCompositeRelation .}} LocalFields: []string{ {{- range $i, $field := .LocalFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} }, {{- else}} LocalField: "{{.LocalField}}", {{- end}} RefBlueprint: "{{.RefBlueprint}}"{{- if .Optional}}, Optional: true{{- end}}},
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
