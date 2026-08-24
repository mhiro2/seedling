package main

import (
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"slices"
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
	RefField     string
	RefFields    []string
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
			{Name: {{quote .Name}}, Kind: seedling.BelongsTo, {{- if isCompositeRelation .}} LocalFields: []string{ {{- range $i, $field := .LocalFields}}{{if $i}}, {{end}}{{quote $field}}{{end}} }, {{- else}} LocalField: {{quote .LocalField}}, {{- end}}{{- if .RefFields}}{{- if isCompositeReference .}} RefFields: []string{ {{- range $i, $field := .RefFields}}{{if $i}}, {{end}}{{quote $field}}{{end}} }, {{- else}} RefField: {{quote .RefField}}, {{- end}}{{- end}} RefBlueprint: {{quote .RefBlueprint}}{{- if .Optional}}, Optional: true{{- end}}},
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
		"isCompositeReference": func(rel normalizedRelation) bool {
			return len(rel.RefFields) > 1
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
		relationNames := make(map[string]struct{}, len(model.Relations))
		for _, relation := range model.Relations {
			if relation.Name == "" {
				return fmt.Errorf("model %q: relation name must not be empty", name)
			}
			if _, exists := relationNames[relation.Name]; exists {
				return fmt.Errorf("model %q: duplicate relation name %q", name, relation.Name)
			}
			relationNames[relation.Name] = struct{}{}
			if len(relation.RefFields) > 0 && len(relation.RefFields) != len(relation.LocalFields) {
				return fmt.Errorf("model %q relation %q: %d local fields do not match %d referenced fields", name, relation.Name, len(relation.LocalFields), len(relation.RefFields))
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

func normalizeSqlcModels(tables []Table, sqlcInfo *SqlcInfo) ([]normalizedModel, error) {
	fieldMappings := make(map[string]map[string]SqlcField, len(tables))
	modelsByTable := make(map[string]*SqlcModel, len(tables))
	for _, table := range tables {
		sqlcModel := sqlcInfo.FindModelForTable(table)
		if sqlcModel == nil {
			return nil, fmt.Errorf("table %q: sqlc model %q not found", table.Name, table.GoName)
		}

		fieldsByColumn, err := mapSqlcModelFields(table, sqlcModel)
		if err != nil {
			return nil, fmt.Errorf("table %q model %q: %w", table.Name, sqlcModel.Name, err)
		}
		fieldMappings[table.Name] = fieldsByColumn
		modelsByTable[table.Name] = sqlcModel
	}

	models := make([]normalizedModel, 0, len(tables))
	for _, table := range tables {
		fieldsByColumn := fieldMappings[table.Name]
		sqlcModel := modelsByTable[table.Name]
		pkFields, err := normalizedSqlcPKFields(table, fieldsByColumn)
		if err != nil {
			return nil, fmt.Errorf("table %q model %q: %w", table.Name, sqlcModel.Name, err)
		}
		relations, err := normalizeSqlcRelations(table, fieldMappings)
		if err != nil {
			return nil, fmt.Errorf("table %q model %q: %w", table.Name, sqlcModel.Name, err)
		}

		model := normalizedModel{
			TypeExpr:      sqlcInfo.Package + "." + sqlcModel.Name,
			ZeroValueExpr: sqlcInfo.Package + "." + sqlcModel.Name + "{}",
			BlueprintID:   table.BlueprintID,
			TableName:     table.Name,
			PKFields:      pkFields,
			Fields:        normalizedSqlcFields(table, fieldsByColumn),
			Relations:     relations,
		}

		if query := sqlcInfo.FindQueryForTable(table); query != nil {
			scalarField, err := resolveSqlcScalarInsertField(sqlcModel, *query, pkFields)
			if err != nil {
				return nil, fmt.Errorf("table %q query %q: %w", table.Name, query.Name, err)
			}
			model.InsertHook = &normalizedMutationHook{
				Body: buildSqlcInsertHook(sqlcInfo.Package, *query, scalarField),
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
	return models, nil
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
		fieldsByName := make(map[string]GormField, len(model.Fields))
		for _, field := range model.Fields {
			fieldsByName[field.Name] = field
		}
		for _, field := range model.Fields {
			if field.Relation == nil || field.Relation.Kind != "BelongsTo" {
				continue
			}

			localFields := splitCommaSeparatedFields(field.Relation.ForeignKey)
			if len(localFields) == 0 {
				localFields = []string{field.Name + "ID"}
			}
			refFields := splitCommaSeparatedFields(field.Relation.References)

			required := true
			for _, localField := range localFields {
				fkField, ok := fieldsByName[localField]
				if !ok || !fkField.NotNull {
					required = false
					break
				}
			}

			relation := normalizedRelation{
				Name:         strings.ToLower(field.Name[:1]) + field.Name[1:],
				LocalFields:  localFields,
				RefFields:    refFields,
				RefBlueprint: singularize(strings.ToLower(field.Relation.RefModel)),
				Optional:     !required,
			}
			if len(localFields) == 1 {
				relation.LocalField = localFields[0]
			}
			if len(refFields) == 1 {
				relation.RefField = refFields[0]
			}
			relations = append(relations, relation)
			for _, localField := range localFields {
				relationLocalFields[localField] = struct{}{}
			}
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

func splitCommaSeparatedFields(value string) []string {
	fields := make([]string, 0, strings.Count(value, ",")+1)
	for field := range strings.SplitSeq(value, ",") {
		if field = strings.TrimSpace(field); field != "" {
			fields = append(fields, field)
		}
	}
	return fields
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
			if edge.Field == "" {
				continue
			}

			localField := entGoName(edge.Field)
			for i := range model.Fields {
				if model.Fields[i].GoName == localField {
					model.Fields[i].IsRelationFK = true
					break
				}
			}
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

func mapSqlcModelFields(table Table, model *SqlcModel) (map[string]SqlcField, error) {
	if len(model.Fields) != len(table.Columns) {
		return nil, fmt.Errorf("has %d fields but schema table has %d columns", len(model.Fields), len(table.Columns))
	}

	fieldsByColumn := make(map[string]SqlcField, len(table.Columns))
	usedFields := make([]bool, len(model.Fields))
	for _, column := range table.Columns {
		matchIndex := -1
		for i, field := range model.Fields {
			if usedFields[i] || normalizeSQLCIdentifier(field.Name) != normalizeSQLCIdentifier(column.Name) {
				continue
			}
			if matchIndex != -1 {
				return nil, fmt.Errorf("column %q matches multiple Go fields", column.Name)
			}
			matchIndex = i
		}
		if matchIndex == -1 {
			continue
		}
		fieldsByColumn[column.Name] = model.Fields[matchIndex]
		usedFields[matchIndex] = true
	}

	// sqlc preserves table column order in generated model structs. Use that
	// ordering only for fields whose configured rename no longer resembles the
	// SQL column, after all unambiguous name matches have been consumed.
	for i, column := range table.Columns {
		if _, ok := fieldsByColumn[column.Name]; ok {
			continue
		}
		if usedFields[i] {
			return nil, fmt.Errorf("cannot map column %q to a Go field without ambiguity", column.Name)
		}
		fieldsByColumn[column.Name] = model.Fields[i]
		usedFields[i] = true
	}

	return fieldsByColumn, nil
}

func normalizedSqlcFields(table Table, fieldsByColumn map[string]SqlcField) []normalizedField {
	fields := make([]normalizedField, 0, len(table.Columns))
	for _, column := range table.Columns {
		field := fieldsByColumn[column.Name]
		fields = append(fields, normalizedField{
			GoName:       field.Name,
			GoType:       field.Type,
			IsPK:         column.IsPK,
			IsRelationFK: column.IsFK,
			IsOptional:   !column.NotNull,
		})
	}
	return fields
}

func normalizedSqlcPKFields(table Table, fieldsByColumn map[string]SqlcField) ([]string, error) {
	fields := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		if column.IsPK {
			fields = append(fields, fieldsByColumn[column.Name].Name)
		}
	}
	if len(fields) > 0 {
		return fields, nil
	}

	for _, column := range table.Columns {
		if normalizeSQLCIdentifier(column.Name) == "id" {
			return []string{fieldsByColumn[column.Name].Name}, nil
		}
	}
	return nil, fmt.Errorf("has no primary key and no conventional id field")
}

func normalizeSqlcRelations(table Table, fieldMappings map[string]map[string]SqlcField) ([]normalizedRelation, error) {
	relations := make([]normalizedRelation, 0, len(table.ForeignKeys))
	for _, foreignKey := range table.ForeignKeys {
		if len(foreignKey.Columns) == 0 {
			continue
		}

		localFields := make([]string, 0, len(foreignKey.Columns))
		for _, columnName := range foreignKey.Columns {
			field, ok := fieldMappings[table.Name][columnName]
			if !ok {
				return nil, fmt.Errorf("relation %q: local column %q not found", relationNameForForeignKey(foreignKey), columnName)
			}
			localFields = append(localFields, field.Name)
		}

		refFields := make([]string, 0, len(foreignKey.RefColumns))
		if len(foreignKey.RefColumns) > 0 {
			refMapping, ok := fieldMappings[foreignKey.RefTable]
			if !ok {
				return nil, fmt.Errorf("relation %q: referenced table %q is not present in sqlc models", relationNameForForeignKey(foreignKey), foreignKey.RefTable)
			}
			for _, columnName := range foreignKey.RefColumns {
				field, ok := refMapping[columnName]
				if !ok {
					return nil, fmt.Errorf("relation %q: referenced column %q.%s not found", relationNameForForeignKey(foreignKey), foreignKey.RefTable, columnName)
				}
				refFields = append(refFields, field.Name)
			}
		}

		relation := normalizedRelation{
			Name:         relationNameForForeignKey(foreignKey),
			LocalFields:  localFields,
			RefFields:    refFields,
			RefBlueprint: singularize(foreignKey.RefTable),
			Optional:     !foreignKey.NotNull,
		}
		if len(localFields) == 1 {
			relation.LocalField = localFields[0]
		}
		if len(refFields) == 1 {
			relation.RefField = refFields[0]
		}
		relations = append(relations, relation)
	}
	return relations, nil
}

func normalizeEntFields(fields []EntField) []normalizedField {
	normalized := make([]normalizedField, 0, len(fields))
	for _, field := range fields {
		normalized = append(normalized, normalizedField{
			GoName:     entGoName(field.Name),
			GoType:     field.GoType,
			IsOptional: field.Optional || field.Nillable,
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

		refFields := make([]string, 0, len(foreignKey.RefColumns))
		for _, columnName := range foreignKey.RefColumns {
			refFields = append(refFields, toGoFieldName(columnName))
		}

		name := relationNameForForeignKey(foreignKey)

		relation := normalizedRelation{
			Name:         name,
			LocalFields:  localFields,
			RefFields:    refFields,
			RefBlueprint: singularize(foreignKey.RefTable),
			Optional:     !foreignKey.NotNull,
		}
		if len(localFields) == 1 {
			relation.LocalField = localFields[0]
		}
		if len(refFields) == 1 {
			relation.RefField = refFields[0]
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

func buildSqlcInsertHook(alias string, query SqlcQuery, scalarField string) string {
	var body strings.Builder
	body.WriteString("return ")
	body.WriteString(alias)
	body.WriteString(".New(dbtx.(")
	body.WriteString(alias)
	body.WriteString(".DBTX)).")
	body.WriteString(query.Name)
	body.WriteString("(ctx")
	switch {
	case query.ParamType != "":
		body.WriteString(", ")
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
		body.WriteString("}")
	case query.ArgType != "":
		body.WriteString(", v.")
		body.WriteString(scalarField)
	}
	body.WriteString(")")
	return body.String()
}

func resolveSqlcScalarInsertField(model *SqlcModel, query SqlcQuery, pkFields []string) (string, error) {
	if query.ParamType != "" || query.ArgType == "" {
		return "", nil
	}

	type candidate struct {
		name       string
		typeName   string
		primaryKey bool
	}
	isPrimaryKey := func(name string) bool {
		return slices.Contains(pkFields, name)
	}

	candidates := make([]candidate, 0, len(model.Fields))
	for _, field := range model.Fields {
		candidates = append(candidates, candidate{
			name:       field.Name,
			typeName:   field.Type,
			primaryKey: isPrimaryKey(field.Name),
		})
	}

	if query.ArgName != "" && query.ArgName != "_" {
		normalizedArgument := normalizeSQLCIdentifier(query.ArgName)
		for _, candidate := range candidates {
			if normalizeSQLCIdentifier(candidate.name) == normalizedArgument {
				return candidate.name, nil
			}
		}
	}

	typeMatches := make([]candidate, 0, len(candidates))
	nonPKMatches := make([]candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.typeName != query.ArgType {
			continue
		}
		typeMatches = append(typeMatches, candidate)
		if !candidate.primaryKey {
			nonPKMatches = append(nonPKMatches, candidate)
		}
	}
	if len(nonPKMatches) == 1 {
		return nonPKMatches[0].name, nil
	}
	if len(typeMatches) == 1 {
		return typeMatches[0].name, nil
	}

	return "", fmt.Errorf("cannot map scalar argument %q (%s) to exactly one model field", query.ArgName, query.ArgType)
}

func normalizeSQLCIdentifier(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "_", ""))
}

func buildSqlcDeleteHook(alias string, deleteQuery SqlcDeleteQuery, pkFields []string) string {
	var body strings.Builder
	body.WriteString("return ")
	body.WriteString(alias)
	body.WriteString(".New(dbtx.(")
	body.WriteString(alias)
	body.WriteString(".DBTX)).")
	body.WriteString(deleteQuery.Name)
	body.WriteString("(ctx")
	if deleteQuery.ParamType != "" {
		body.WriteString(", ")
		body.WriteString(alias)
		body.WriteString(".")
		body.WriteString(deleteQuery.ParamType)
		body.WriteString("{\n")
		for _, field := range deleteQuery.ParamFields {
			body.WriteString("\t")
			body.WriteString(field.Name)
			body.WriteString(": v.")
			body.WriteString(field.Name)
			body.WriteString(",\n")
		}
		body.WriteString("}")
	} else if deleteQuery.ArgType != "" {
		body.WriteString(", v.")
		body.WriteString(pkFieldForDeleteArg(deleteQuery.ArgName, pkFields))
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
		if field.Nillable {
			body.WriteString("Nillable")
		}
		body.WriteString(entGoName(field.Name))
		body.WriteString("(v.")
		body.WriteString(entGoName(field.Name))
		body.WriteString(")\n")
	}
	body.WriteString("return builder.Save(ctx)")
	return body.String()
}
