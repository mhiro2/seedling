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

// pkFieldForDeleteArg maps a delete function's arg name (e.g., "id") to the model's PK field name (e.g., "ID").
func pkFieldForDeleteArg(argName string, pks []string) string {
	goName := toGoFieldName(argName)
	for _, pk := range pks {
		if pk == goName {
			return pk
		}
	}
	if len(pks) > 0 {
		return pks[0]
	}
	return "ID"
}

func relationNameForColumn(columnName, refTable string) string {
	if name, ok := strings.CutSuffix(columnName, "_id"); ok {
		return name
	}
	return singularize(refTable)
}

func relationNameForForeignKey(foreignKey ForeignKey) string {
	if len(foreignKey.Columns) == 1 {
		return relationNameForColumn(foreignKey.Columns[0], foreignKey.RefTable)
	}

	if len(foreignKey.RefColumns) == len(foreignKey.Columns) {
		var qualifier string
		for i, column := range foreignKey.Columns {
			candidate, ok := strings.CutSuffix(column, "_"+foreignKey.RefColumns[i])
			if !ok || candidate == "" || qualifier != "" && qualifier != candidate {
				qualifier = ""
				break
			}
			qualifier = candidate
		}
		if qualifier != "" {
			return qualifier
		}
	}

	prefix := commonColumnPrefix(foreignKey.Columns)
	if prefix != "" {
		return prefix
	}
	if len(foreignKey.Columns) > 0 {
		return strings.Join(foreignKey.Columns, "_")
	}
	return singularize(foreignKey.RefTable)
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
