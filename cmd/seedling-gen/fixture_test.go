package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSeedlingGenFixture(t *testing.T, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", relativePath))
	if err != nil {
		t.Fatalf("read fixture %q: %v", relativePath, err)
	}

	return string(data)
}

func findTableByName(t *testing.T, tables []Table, name string) Table {
	t.Helper()

	for _, table := range tables {
		if table.Name == name {
			return table
		}
	}

	t.Fatalf("find table %q: not found", name)
	return Table{}
}

func findColumnByName(t *testing.T, table Table, name string) Column {
	t.Helper()

	for _, column := range table.Columns {
		if column.Name == name {
			return column
		}
	}

	t.Fatalf("find column %q on table %q: not found", name, table.Name)
	return Column{}
}

func findForeignKeyByColumns(t *testing.T, table Table, columns ...string) ForeignKey {
	t.Helper()

	for _, foreignKey := range table.ForeignKeys {
		if strings.Join(foreignKey.Columns, ",") == strings.Join(columns, ",") {
			return foreignKey
		}
	}

	t.Fatalf("find foreign key on table %q for columns %v: not found", table.Name, columns)
	return ForeignKey{}
}
