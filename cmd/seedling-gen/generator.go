package main

import (
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
	models := normalizeSqlcModels(tables, sqlcInfo)
	imports := []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
		sqlcInfo.Package + ` "` + sqlcImportPath + `"`,
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
