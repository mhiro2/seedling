package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

type diagnosticFormat string

const (
	diagnosticFormatText diagnosticFormat = "text"
	diagnosticFormatJSON diagnosticFormat = "json"
)

type diagnosticReport struct {
	Adapter    string                `json:"adapter"`
	Dialect    string                `json:"dialect,omitempty"`
	Tables     []diagnosticTable     `json:"tables,omitempty"`
	GormModels []diagnosticGormModel `json:"gormModels,omitempty"`
	EntSchemas []diagnosticEntSchema `json:"entSchemas,omitempty"`
	SQLC       *diagnosticSQLC       `json:"sqlc,omitempty"`
	Blueprints []diagnosticBlueprint `json:"blueprints"`
}

type diagnosticTable struct {
	Name        string                 `json:"name"`
	GoName      string                 `json:"goName"`
	BlueprintID string                 `json:"blueprintID"`
	Columns     []diagnosticColumn     `json:"columns"`
	ForeignKeys []diagnosticForeignKey `json:"foreignKeys,omitempty"`
}

type diagnosticColumn struct {
	Name       string `json:"name"`
	SQLType    string `json:"sqlType"`
	GoName     string `json:"goName"`
	GoType     string `json:"goType"`
	PrimaryKey bool   `json:"primaryKey"`
	ForeignKey bool   `json:"foreignKey"`
	RefTable   string `json:"refTable,omitempty"`
	NotNull    bool   `json:"notNull"`
}

type diagnosticForeignKey struct {
	Columns      []string `json:"columns"`
	LocalFields  []string `json:"localFields,omitempty"`
	RefTable     string   `json:"refTable"`
	RelationName string   `json:"relationName"`
	RefBlueprint string   `json:"refBlueprint"`
	Optional     bool     `json:"optional"`
}

type diagnosticGormModel struct {
	Name   string                `json:"name"`
	Table  string                `json:"table"`
	Fields []diagnosticGormField `json:"fields"`
}

type diagnosticGormField struct {
	Name       string                  `json:"name"`
	Type       string                  `json:"type"`
	ColumnName string                  `json:"columnName"`
	PrimaryKey bool                    `json:"primaryKey"`
	NotNull    bool                    `json:"notNull"`
	Relation   *diagnosticGormRelation `json:"relation,omitempty"`
}

type diagnosticGormRelation struct {
	Kind       string `json:"kind"`
	ForeignKey string `json:"foreignKey,omitempty"`
	JoinTable  string `json:"joinTable,omitempty"`
	RefModel   string `json:"refModel"`
}

type diagnosticEntSchema struct {
	Name   string               `json:"name"`
	Fields []diagnosticEntField `json:"fields"`
	Edges  []diagnosticEntEdge  `json:"edges,omitempty"`
}

type diagnosticEntField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	GoType   string `json:"goType"`
	Optional bool   `json:"optional"`
}

type diagnosticEntEdge struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Direction string `json:"direction"`
	Ref       string `json:"ref,omitempty"`
	Unique    bool   `json:"unique"`
	Required  bool   `json:"required"`
}

type diagnosticSQLC struct {
	Package       string                      `json:"package"`
	ImportPath    string                      `json:"importPath"`
	Models        []diagnosticSQLCModel       `json:"models,omitempty"`
	Queries       []diagnosticSQLCQuery       `json:"queries,omitempty"`
	DeleteQueries []diagnosticSQLCDeleteQuery `json:"deleteQueries,omitempty"`
}

type diagnosticSQLCModel struct {
	Name   string                `json:"name"`
	Fields []diagnosticFieldType `json:"fields"`
}

type diagnosticSQLCQuery struct {
	Name        string                `json:"name"`
	ReturnType  string                `json:"returnType"`
	ParamType   string                `json:"paramType,omitempty"`
	ParamFields []diagnosticFieldType `json:"paramFields,omitempty"`
}

type diagnosticSQLCDeleteQuery struct {
	Name      string `json:"name"`
	ParamType string `json:"paramType,omitempty"`
	ArgName   string `json:"argName,omitempty"`
	ArgType   string `json:"argType,omitempty"`
}

type diagnosticBlueprint struct {
	Name         string                        `json:"name"`
	Table        string                        `json:"table"`
	Type         string                        `json:"type"`
	PKFields     []string                      `json:"pkFields"`
	Fields       []diagnosticFieldType         `json:"fields,omitempty"`
	Relations    []diagnosticBlueprintRelation `json:"relations,omitempty"`
	InsertSource string                        `json:"insertSource,omitempty"`
	DeleteSource string                        `json:"deleteSource,omitempty"`
}

type diagnosticFieldType struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type diagnosticBlueprintRelation struct {
	Name         string   `json:"name"`
	LocalFields  []string `json:"localFields"`
	RefBlueprint string   `json:"refBlueprint"`
	Optional     bool     `json:"optional"`
}

func diagnosticModeEnabled(explain, json bool) bool {
	return explain || json
}

func resolveDiagnosticFormat(json bool) diagnosticFormat {
	if json {
		return diagnosticFormatJSON
	}
	return diagnosticFormatText
}

func writeDiagnosticOutput(stdout, stderr io.Writer, out string, report diagnosticReport, format diagnosticFormat) int {
	return writeOutput(stdout, stderr, out, func(w io.Writer) error {
		return renderDiagnosticReport(w, report, format)
	})
}

func renderDiagnosticReport(w io.Writer, report diagnosticReport, format diagnosticFormat) error {
	switch format {
	case diagnosticFormatJSON:
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal diagnostic report: %w", err)
		}
		if _, err := w.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write diagnostic report: %w", err)
		}
		return nil
	case diagnosticFormatText:
		var buf strings.Builder
		buf.WriteString("Adapter: ")
		buf.WriteString(report.Adapter)
		buf.WriteString("\n")
		if report.Dialect != "" {
			buf.WriteString("Dialect: ")
			buf.WriteString(report.Dialect)
			buf.WriteString("\n")
		}

		writeTableSection(&buf, report.Tables)
		writeSQLCSection(&buf, report.SQLC)
		writeGormSection(&buf, report.GormModels)
		writeEntSection(&buf, report.EntSchemas)
		writeBlueprintSection(&buf, report.Blueprints)

		if _, err := io.WriteString(w, buf.String()); err != nil {
			return fmt.Errorf("write diagnostic report: %w", err)
		}
		return nil
	}

	return fmt.Errorf("render diagnostic report: unsupported format %q", format)
}

func writeTableSection(buf *strings.Builder, tables []diagnosticTable) {
	if len(tables) == 0 {
		return
	}

	buf.WriteString("\nParsed tables:\n")
	for _, table := range tables {
		buf.WriteString("- ")
		buf.WriteString(table.Name)
		buf.WriteString(" (go: ")
		buf.WriteString(table.GoName)
		buf.WriteString(", blueprint: ")
		buf.WriteString(table.BlueprintID)
		buf.WriteString(")\n")

		buf.WriteString("  Columns:\n")
		for _, column := range table.Columns {
			buf.WriteString("  - ")
			buf.WriteString(column.Name)
			buf.WriteString(": sql=")
			buf.WriteString(column.SQLType)
			buf.WriteString(" go=")
			buf.WriteString(column.GoName)
			buf.WriteString("(")
			buf.WriteString(column.GoType)
			buf.WriteString(")")
			buf.WriteString(" pk=")
			buf.WriteString(boolString(column.PrimaryKey))
			buf.WriteString(" notNull=")
			buf.WriteString(boolString(column.NotNull))
			if column.ForeignKey {
				buf.WriteString(" fk=")
				buf.WriteString(column.RefTable)
			}
			buf.WriteString("\n")
		}

		if len(table.ForeignKeys) == 0 {
			continue
		}
		buf.WriteString("  Foreign keys:\n")
		for _, foreignKey := range table.ForeignKeys {
			buf.WriteString("  - columns=")
			buf.WriteString(strings.Join(foreignKey.Columns, ","))
			if len(foreignKey.LocalFields) > 0 {
				buf.WriteString(" localFields=")
				buf.WriteString(strings.Join(foreignKey.LocalFields, ","))
			}
			buf.WriteString(" refTable=")
			buf.WriteString(foreignKey.RefTable)
			buf.WriteString(" relation=")
			buf.WriteString(foreignKey.RelationName)
			buf.WriteString(" refBlueprint=")
			buf.WriteString(foreignKey.RefBlueprint)
			buf.WriteString(" optional=")
			buf.WriteString(boolString(foreignKey.Optional))
			buf.WriteString("\n")
		}
	}
}

func writeSQLCSection(buf *strings.Builder, sqlc *diagnosticSQLC) {
	if sqlc == nil {
		return
	}

	buf.WriteString("\nParsed sqlc metadata:\n")
	buf.WriteString("- package=")
	buf.WriteString(sqlc.Package)
	buf.WriteString(" importPath=")
	buf.WriteString(sqlc.ImportPath)
	buf.WriteString("\n")

	if len(sqlc.Models) > 0 {
		buf.WriteString("- models:\n")
		for _, model := range sqlc.Models {
			buf.WriteString("  - ")
			buf.WriteString(model.Name)
			buf.WriteString(": ")
			buf.WriteString(joinFieldTypes(model.Fields))
			buf.WriteString("\n")
		}
	}
	if len(sqlc.Queries) > 0 {
		buf.WriteString("- insert queries:\n")
		for _, query := range sqlc.Queries {
			buf.WriteString("  - ")
			buf.WriteString(query.Name)
			buf.WriteString(" -> ")
			buf.WriteString(query.ReturnType)
			if query.ParamType != "" {
				buf.WriteString(" using ")
				buf.WriteString(query.ParamType)
			}
			if len(query.ParamFields) > 0 {
				buf.WriteString(" {")
				buf.WriteString(joinFieldTypes(query.ParamFields))
				buf.WriteString("}")
			}
			buf.WriteString("\n")
		}
	}
	if len(sqlc.DeleteQueries) > 0 {
		buf.WriteString("- delete queries:\n")
		for _, query := range sqlc.DeleteQueries {
			buf.WriteString("  - ")
			buf.WriteString(query.Name)
			if query.ParamType != "" {
				buf.WriteString(" using ")
				buf.WriteString(query.ParamType)
			} else if query.ArgName != "" {
				buf.WriteString(" arg ")
				buf.WriteString(query.ArgName)
				if query.ArgType != "" {
					buf.WriteString("(")
					buf.WriteString(query.ArgType)
					buf.WriteString(")")
				}
			}
			buf.WriteString("\n")
		}
	}
}

func writeGormSection(buf *strings.Builder, models []diagnosticGormModel) {
	if len(models) == 0 {
		return
	}

	buf.WriteString("\nParsed GORM models:\n")
	for _, model := range models {
		buf.WriteString("- ")
		buf.WriteString(model.Name)
		buf.WriteString(" (table: ")
		buf.WriteString(model.Table)
		buf.WriteString(")\n")
		for _, field := range model.Fields {
			buf.WriteString("  - ")
			buf.WriteString(field.Name)
			buf.WriteString(": type=")
			buf.WriteString(field.Type)
			buf.WriteString(" column=")
			buf.WriteString(field.ColumnName)
			buf.WriteString(" pk=")
			buf.WriteString(boolString(field.PrimaryKey))
			buf.WriteString(" notNull=")
			buf.WriteString(boolString(field.NotNull))
			if field.Relation != nil {
				buf.WriteString(" relation=")
				buf.WriteString(field.Relation.Kind)
				buf.WriteString(" ref=")
				buf.WriteString(field.Relation.RefModel)
				if field.Relation.ForeignKey != "" {
					buf.WriteString(" foreignKey=")
					buf.WriteString(field.Relation.ForeignKey)
				}
				if field.Relation.JoinTable != "" {
					buf.WriteString(" joinTable=")
					buf.WriteString(field.Relation.JoinTable)
				}
			}
			buf.WriteString("\n")
		}
	}
}

func writeEntSection(buf *strings.Builder, schemas []diagnosticEntSchema) {
	if len(schemas) == 0 {
		return
	}

	buf.WriteString("\nParsed ent schemas:\n")
	for _, schema := range schemas {
		buf.WriteString("- ")
		buf.WriteString(schema.Name)
		buf.WriteString("\n")
		if len(schema.Fields) > 0 {
			buf.WriteString("  Fields:\n")
			for _, field := range schema.Fields {
				buf.WriteString("  - ")
				buf.WriteString(field.Name)
				buf.WriteString(": ent=")
				buf.WriteString(field.Type)
				buf.WriteString(" go=")
				buf.WriteString(field.GoType)
				buf.WriteString(" optional=")
				buf.WriteString(boolString(field.Optional))
				buf.WriteString("\n")
			}
		}
		if len(schema.Edges) > 0 {
			buf.WriteString("  Edges:\n")
			for _, edge := range schema.Edges {
				buf.WriteString("  - ")
				buf.WriteString(edge.Name)
				buf.WriteString(": direction=")
				buf.WriteString(edge.Direction)
				buf.WriteString(" type=")
				buf.WriteString(edge.Type)
				if edge.Ref != "" {
					buf.WriteString(" ref=")
					buf.WriteString(edge.Ref)
				}
				buf.WriteString(" unique=")
				buf.WriteString(boolString(edge.Unique))
				buf.WriteString(" required=")
				buf.WriteString(boolString(edge.Required))
				buf.WriteString("\n")
			}
		}
	}
}

func writeBlueprintSection(buf *strings.Builder, blueprints []diagnosticBlueprint) {
	buf.WriteString("\nInferred blueprints:\n")
	for _, blueprint := range blueprints {
		buf.WriteString("- ")
		buf.WriteString(blueprint.Name)
		buf.WriteString(" (table: ")
		buf.WriteString(blueprint.Table)
		buf.WriteString(", type: ")
		buf.WriteString(blueprint.Type)
		buf.WriteString(", pk: ")
		buf.WriteString(strings.Join(blueprint.PKFields, ","))
		if blueprint.InsertSource != "" {
			buf.WriteString(", insert: ")
			buf.WriteString(blueprint.InsertSource)
		}
		if blueprint.DeleteSource != "" {
			buf.WriteString(", delete: ")
			buf.WriteString(blueprint.DeleteSource)
		}
		buf.WriteString(")\n")

		if len(blueprint.Fields) > 0 {
			buf.WriteString("  Fields:\n")
			for _, field := range blueprint.Fields {
				buf.WriteString("  - ")
				buf.WriteString(field.Name)
				buf.WriteString(" ")
				buf.WriteString(field.Type)
				buf.WriteString("\n")
			}
		}
		if len(blueprint.Relations) == 0 {
			continue
		}
		buf.WriteString("  Relations:\n")
		for _, relation := range blueprint.Relations {
			buf.WriteString("  - ")
			buf.WriteString(relation.Name)
			buf.WriteString(": ref=")
			buf.WriteString(relation.RefBlueprint)
			buf.WriteString(" localFields=")
			buf.WriteString(strings.Join(relation.LocalFields, ","))
			buf.WriteString(" optional=")
			buf.WriteString(boolString(relation.Optional))
			buf.WriteString("\n")
		}
	}
}

func buildSQLDiagnosticReport(dialect string, tables []Table) diagnosticReport {
	return diagnosticReport{
		Adapter:    "sql",
		Dialect:    normalizeDialectName(dialect),
		Tables:     buildTableDiagnostics(tables),
		Blueprints: buildBlueprintDiagnostics(normalizeTableModels(tables), tableInsertSources(tables, "stub"), nil),
	}
}

func buildSQLCDiagnosticReport(dialect, importPath string, tables []Table, sqlcInfo *SqlcInfo) diagnosticReport {
	insertSources := make(map[string]string, len(tables))
	deleteSources := make(map[string]string, len(tables))
	for _, table := range tables {
		insertSources[table.Name] = "stub"
		if query := sqlcInfo.FindQueryForTable(table); query != nil {
			insertSources[table.Name] = query.Name
		}
		if query := sqlcInfo.FindDeleteQueryForTable(table); query != nil {
			deleteSources[table.Name] = query.Name
		}
	}

	return diagnosticReport{
		Adapter:    "sqlc",
		Dialect:    normalizeDialectName(dialect),
		Tables:     buildTableDiagnostics(tables),
		SQLC:       buildSQLCDiagnostics(sqlcInfo, importPath),
		Blueprints: buildBlueprintDiagnostics(normalizeSqlcModels(tables, sqlcInfo), insertSources, deleteSources),
	}
}

func buildGormDiagnosticReport(importPath string, models []GormModel) diagnosticReport {
	insertSources := make(map[string]string, len(models))
	deleteSources := make(map[string]string, len(models))
	for _, model := range models {
		insertSources[model.Table] = "gorm.Create"
		deleteSources[model.Table] = "gorm.Delete"
	}

	return diagnosticReport{
		Adapter:    "gorm",
		GormModels: buildGormDiagnostics(models),
		Blueprints: buildBlueprintDiagnostics(normalizeGormModels(models, filepath.Base(importPath)), insertSources, deleteSources),
	}
}

func buildEntDiagnosticReport(schemas []EntSchema) diagnosticReport {
	insertSources := make(map[string]string, len(schemas))
	deleteSources := make(map[string]string, len(schemas))
	for _, schema := range schemas {
		tableName := singularize(strings.ToLower(schema.Name)) + "s"
		insertSources[tableName] = "ent.Create"
		deleteSources[tableName] = "ent.DeleteOneID"
	}

	return diagnosticReport{
		Adapter:    "ent",
		EntSchemas: buildEntDiagnostics(schemas),
		Blueprints: buildBlueprintDiagnostics(normalizeEntModels(schemas), insertSources, deleteSources),
	}
}

func buildAtlasDiagnosticReport(tables []Table) diagnosticReport {
	return diagnosticReport{
		Adapter:    "atlas",
		Tables:     buildTableDiagnostics(tables),
		Blueprints: buildBlueprintDiagnostics(normalizeTableModels(tables), tableInsertSources(tables, "stub"), nil),
	}
}

func buildTableDiagnostics(tables []Table) []diagnosticTable {
	diagnostics := make([]diagnosticTable, 0, len(tables))
	for _, table := range tables {
		columns := make([]diagnosticColumn, 0, len(table.Columns))
		for _, column := range table.Columns {
			columns = append(columns, diagnosticColumn{
				Name:       column.Name,
				SQLType:    column.SQLType,
				GoName:     column.GoName,
				GoType:     column.GoType,
				PrimaryKey: column.IsPK,
				ForeignKey: column.IsFK,
				RefTable:   column.FKRefTable,
				NotNull:    column.NotNull,
			})
		}

		foreignKeys := make([]diagnosticForeignKey, 0, len(table.ForeignKeys))
		for _, foreignKey := range table.ForeignKeys {
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

			relationName := singularize(foreignKey.RefTable)
			if len(foreignKey.Columns) == 1 {
				relationName = relationNameForColumn(foreignKey.Columns[0], foreignKey.RefTable)
			}

			foreignKeys = append(foreignKeys, diagnosticForeignKey{
				Columns:      slices.Clone(foreignKey.Columns),
				LocalFields:  localFields,
				RefTable:     foreignKey.RefTable,
				RelationName: relationName,
				RefBlueprint: singularize(foreignKey.RefTable),
				Optional:     !foreignKey.NotNull,
			})
		}

		diagnostics = append(diagnostics, diagnosticTable{
			Name:        table.Name,
			GoName:      table.GoName,
			BlueprintID: table.BlueprintID,
			Columns:     columns,
			ForeignKeys: foreignKeys,
		})
	}

	slices.SortFunc(diagnostics, func(a, b diagnosticTable) int {
		return strings.Compare(a.Name, b.Name)
	})
	return diagnostics
}

func buildGormDiagnostics(models []GormModel) []diagnosticGormModel {
	diagnostics := make([]diagnosticGormModel, 0, len(models))
	for _, model := range models {
		fields := make([]diagnosticGormField, 0, len(model.Fields))
		for _, field := range model.Fields {
			var relation *diagnosticGormRelation
			if field.Relation != nil {
				relation = &diagnosticGormRelation{
					Kind:       field.Relation.Kind,
					ForeignKey: field.Relation.ForeignKey,
					JoinTable:  field.Relation.JoinTable,
					RefModel:   field.Relation.RefModel,
				}
			}

			fields = append(fields, diagnosticGormField{
				Name:       field.Name,
				Type:       field.Type,
				ColumnName: field.ColumnName,
				PrimaryKey: field.IsPK,
				NotNull:    field.NotNull,
				Relation:   relation,
			})
		}

		diagnostics = append(diagnostics, diagnosticGormModel{
			Name:   model.Name,
			Table:  model.Table,
			Fields: fields,
		})
	}

	slices.SortFunc(diagnostics, func(a, b diagnosticGormModel) int {
		return strings.Compare(a.Name, b.Name)
	})
	return diagnostics
}

func buildEntDiagnostics(schemas []EntSchema) []diagnosticEntSchema {
	diagnostics := make([]diagnosticEntSchema, 0, len(schemas))
	for _, schema := range schemas {
		fields := make([]diagnosticEntField, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			fields = append(fields, diagnosticEntField(field))
		}

		edges := make([]diagnosticEntEdge, 0, len(schema.Edges))
		for _, edge := range schema.Edges {
			edges = append(edges, diagnosticEntEdge(edge))
		}

		diagnostics = append(diagnostics, diagnosticEntSchema{
			Name:   schema.Name,
			Fields: fields,
			Edges:  edges,
		})
	}

	slices.SortFunc(diagnostics, func(a, b diagnosticEntSchema) int {
		return strings.Compare(a.Name, b.Name)
	})
	return diagnostics
}

func buildSQLCDiagnostics(info *SqlcInfo, importPath string) *diagnosticSQLC {
	if info == nil {
		return nil
	}

	models := make([]diagnosticSQLCModel, 0, len(info.Models))
	for _, model := range info.Models {
		fields := make([]diagnosticFieldType, 0, len(model.Fields))
		for _, field := range model.Fields {
			fields = append(fields, diagnosticFieldType(field))
		}
		models = append(models, diagnosticSQLCModel{
			Name:   model.Name,
			Fields: fields,
		})
	}
	slices.SortFunc(models, func(a, b diagnosticSQLCModel) int {
		return strings.Compare(a.Name, b.Name)
	})

	queries := make([]diagnosticSQLCQuery, 0, len(info.Queries))
	for _, query := range info.Queries {
		fields := make([]diagnosticFieldType, 0, len(query.ParamFields))
		for _, field := range query.ParamFields {
			fields = append(fields, diagnosticFieldType(field))
		}
		queries = append(queries, diagnosticSQLCQuery{
			Name:        query.Name,
			ReturnType:  query.ReturnType,
			ParamType:   query.ParamType,
			ParamFields: fields,
		})
	}
	slices.SortFunc(queries, func(a, b diagnosticSQLCQuery) int {
		return strings.Compare(a.Name, b.Name)
	})

	deleteQueries := make([]diagnosticSQLCDeleteQuery, 0, len(info.DeleteQueries))
	for _, query := range info.DeleteQueries {
		deleteQueries = append(deleteQueries, diagnosticSQLCDeleteQuery(query))
	}
	slices.SortFunc(deleteQueries, func(a, b diagnosticSQLCDeleteQuery) int {
		return strings.Compare(a.Name, b.Name)
	})

	return &diagnosticSQLC{
		Package:       info.Package,
		ImportPath:    importPath,
		Models:        models,
		Queries:       queries,
		DeleteQueries: deleteQueries,
	}
}

func buildBlueprintDiagnostics(models []normalizedModel, insertSources, deleteSources map[string]string) []diagnosticBlueprint {
	diagnostics := make([]diagnosticBlueprint, 0, len(models))
	for _, model := range models {
		fields := make([]diagnosticFieldType, 0, len(model.Fields))
		for _, field := range model.Fields {
			fields = append(fields, diagnosticFieldType{
				Name: field.GoName,
				Type: field.GoType,
			})
		}

		relations := make([]diagnosticBlueprintRelation, 0, len(model.Relations))
		for _, relation := range model.Relations {
			localFields := relation.LocalFields
			if len(localFields) == 0 && relation.LocalField != "" {
				localFields = []string{relation.LocalField}
			}
			relations = append(relations, diagnosticBlueprintRelation{
				Name:         relation.Name,
				LocalFields:  slices.Clone(localFields),
				RefBlueprint: relation.RefBlueprint,
				Optional:     relation.Optional,
			})
		}

		diagnostics = append(diagnostics, diagnosticBlueprint{
			Name:         model.BlueprintID,
			Table:        model.TableName,
			Type:         model.TypeExpr,
			PKFields:     slices.Clone(model.PKFields),
			Fields:       fields,
			Relations:    relations,
			InsertSource: lookupDiagnosticSource(insertSources, model.TableName),
			DeleteSource: lookupDiagnosticSource(deleteSources, model.TableName),
		})
	}

	slices.SortFunc(diagnostics, func(a, b diagnosticBlueprint) int {
		return strings.Compare(a.Name, b.Name)
	})
	return diagnostics
}

func tableInsertSources(tables []Table, source string) map[string]string {
	sources := make(map[string]string, len(tables))
	for _, table := range tables {
		sources[table.Name] = source
	}
	return sources
}

func lookupDiagnosticSource(sources map[string]string, key string) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[key]
}

func normalizeDialectName(dialect string) string {
	dialect = strings.ToLower(strings.TrimSpace(dialect))
	if dialect == "" {
		return "auto"
	}
	return dialect
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func joinFieldTypes(fields []diagnosticFieldType) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, field.Name+"("+field.Type+")")
	}
	return strings.Join(parts, ", ")
}
