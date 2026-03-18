package main

import (
	"io"
	"strings"
)

func Generate(w io.Writer, pkg string, tables []Table) error {
	needsTime := false
	for _, table := range tables {
		for _, column := range table.Columns {
			if column.GoType == "time.Time" {
				needsTime = true
			}
		}
	}

	imports := []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
	}
	if needsTime {
		imports = append(imports, `"time"`)
	}

	return generateNormalizedCode(w, "sql", pkg, imports, normalizeTableModels(tables), true)
}

// GenerateSqlc generates blueprint code that imports and uses sqlc-generated types.
func GenerateSqlc(w io.Writer, pkg, sqlcImportPath string, tables []Table, sqlcInfo *SqlcInfo) error {
	return generateNormalizedCode(w, "sqlc", pkg, []string{
		`"context"`,
		`"github.com/mhiro2/seedling"`,
		sqlcInfo.Package + ` "` + sqlcImportPath + `"`,
	}, normalizeSqlcModels(tables, sqlcInfo), false)
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
