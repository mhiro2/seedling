package main

import (
	"fmt"
	"io"
	"strings"
)

func Generate(w io.Writer, pkg string, tables []Table) error {
	models := normalizeTableModels(tables)

	imports := []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
	}
	if normalizedModelsNeedTimeImport(models) {
		imports = append(imports, `"time"`)
	}

	return generateNormalizedCode(w, "sql", pkg, imports, models, true)
}

// GenerateSqlc generates blueprint code that imports and uses sqlc-generated types.
func GenerateSqlc(w io.Writer, pkg, sqlcImportPath string, tables []Table, sqlcInfo *SqlcInfo) error {
	models, err := normalizeSqlcModels(tables, sqlcInfo)
	if err != nil {
		return fmt.Errorf("normalize sqlc models: %w", err)
	}
	spec, err := importSpec(sqlcInfo.Package, sqlcImportPath)
	if err != nil {
		return fmt.Errorf("sqlc import: %w", err)
	}
	imports := []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
		spec,
	}
	if normalizedModelsNeedTimeImport(models) {
		imports = append(imports, `"time"`)
	}
	return generateNormalizedCode(w, "sqlc", pkg, imports, models, false)
}

func relationNameForColumn(columnName, refTable string) string {
	if name, ok := strings.CutSuffix(columnName, "_id"); ok {
		return name
	}
	return singularize(refTable)
}

// relationNamesForTable names every foreign key on a table, indexed alongside
// table.ForeignKeys.
//
// A composite key whose columns carry an explicit qualifier keeps that
// qualifier, because that is the name the schema author chose (owner_tenant_id,
// owner_id -> "owner"). Everything else is named after the referenced table, the
// same as a single-column key would be, so that recording the referenced columns
// does not rename relations that already worked.
//
// Column-derived names are used only to break an actual collision. Deriving them
// unconditionally would rename the common (tenant_id, user_id) shape from "user"
// to "tenant_id_user_id" and silently invalidate every Use()/Ref() call site.
func relationNamesForTable(table Table) []string {
	names := make([]string, len(table.ForeignKeys))
	counts := make(map[string]int, len(table.ForeignKeys))
	for i, foreignKey := range table.ForeignKeys {
		names[i] = preferredRelationName(foreignKey)
		counts[names[i]]++
	}
	for i, foreignKey := range table.ForeignKeys {
		if counts[names[i]] > 1 {
			names[i] = columnDerivedRelationName(foreignKey)
		}
	}
	return names
}

func preferredRelationName(foreignKey ForeignKey) string {
	if len(foreignKey.Columns) == 1 {
		return relationNameForColumn(foreignKey.Columns[0], foreignKey.RefTable)
	}
	if qualifier := compositeColumnQualifier(foreignKey); qualifier != "" {
		return qualifier
	}
	return singularize(foreignKey.RefTable)
}

// columnDerivedRelationName builds a name from the FK columns themselves, which
// is distinct for any two keys that do not share a column set.
func columnDerivedRelationName(foreignKey ForeignKey) string {
	if len(foreignKey.Columns) == 0 {
		return singularize(foreignKey.RefTable)
	}
	if len(foreignKey.Columns) == 1 {
		if name, ok := strings.CutSuffix(foreignKey.Columns[0], "_id"); ok {
			return name
		}
		return foreignKey.Columns[0]
	}
	if qualifier := compositeColumnQualifier(foreignKey); qualifier != "" {
		return qualifier
	}
	if prefix := commonColumnPrefix(foreignKey.Columns); prefix != "" {
		return prefix
	}
	return strings.Join(foreignKey.Columns, "_")
}

// compositeColumnQualifier returns the prefix shared by every FK column once its
// referenced column name is stripped, which is how a schema distinguishes two
// composite keys pointing at the same table.
func compositeColumnQualifier(foreignKey ForeignKey) string {
	if len(foreignKey.RefColumns) != len(foreignKey.Columns) {
		return ""
	}
	var qualifier string
	for i, column := range foreignKey.Columns {
		candidate, ok := strings.CutSuffix(column, "_"+foreignKey.RefColumns[i])
		if !ok || candidate == "" || qualifier != "" && qualifier != candidate {
			return ""
		}
		qualifier = candidate
	}
	return qualifier
}

func commonColumnPrefix(columns []string) string {
	if len(columns) == 0 {
		return ""
	}
	prefix := strings.Split(columns[0], "_")
	for _, column := range columns[1:] {
		parts := strings.Split(column, "_")
		i := 0
		for i < len(prefix) && i < len(parts) && prefix[i] == parts[i] {
			i++
		}
		prefix = prefix[:i]
		if len(prefix) == 0 {
			return ""
		}
	}
	return strings.Join(prefix, "_")
}
