package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSqlcConfig_V2(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "sqlc.yaml", `version: "2"
sql:
  - schema: "schema.sql"
    queries: "query.sql"
    gen:
      go:
        package: "db"
        out: "internal/db"
`)
	writeFile(t, dir, "go.mod", "module github.com/example/myapp\n\ngo 1.26\n")
	writeFile(t, dir, "schema.sql", "CREATE TABLE users (id INT);")
	if err := os.MkdirAll(filepath.Join(dir, "internal", "db"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SchemaFiles) != 1 {
		t.Fatalf("expected 1 schema file, got %d", len(cfg.SchemaFiles))
	}
	if filepath.Base(cfg.SchemaFiles[0]) != "schema.sql" {
		t.Fatalf("expected schema.sql, got %q", cfg.SchemaFiles[0])
	}
	if cfg.SqlcPkg != "db" {
		t.Fatalf("expected package %q, got %q", "db", cfg.SqlcPkg)
	}
	if cfg.SqlcImportPath != "github.com/example/myapp/internal/db" {
		t.Fatalf("expected import path %q, got %q", "github.com/example/myapp/internal/db", cfg.SqlcImportPath)
	}
}

func TestParseSqlcConfig_V1(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "sqlc.yaml", `version: "1"
packages:
  - schema: "schema.sql"
    queries: "query.sql"
    name: "db"
    path: "internal/db"
`)
	writeFile(t, dir, "go.mod", "module github.com/example/v1app\n\ngo 1.26\n")
	writeFile(t, dir, "schema.sql", "CREATE TABLE users (id INT);")
	if err := os.MkdirAll(filepath.Join(dir, "internal", "db"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SqlcPkg != "db" {
		t.Fatalf("expected package %q, got %q", "db", cfg.SqlcPkg)
	}
	if cfg.SqlcImportPath != "github.com/example/v1app/internal/db" {
		t.Fatalf("expected import path %q, got %q", "github.com/example/v1app/internal/db", cfg.SqlcImportPath)
	}
}

func TestParseSqlcConfig_MultipleSchemas(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "sqlc.yaml", `version: "2"
sql:
  - schema: [a.sql, b.sql]
    gen:
      go:
        package: "db"
        out: "db"
`)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.26\n")
	if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SchemaFiles) != 2 {
		t.Fatalf("expected 2 schema files, got %d", len(cfg.SchemaFiles))
	}
}

func TestParseSqlcConfig_MissingFile(t *testing.T) {
	// Act & Assert
	_, err := ParseSqlcConfig("/nonexistent/sqlc.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseSqlcConfig_MissingSchema(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "sqlc.yaml", `version: "2"
sql:
  - gen:
      go:
        package: "db"
        out: "db"
`)

	// Act & Assert
	_, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	if err == nil {
		t.Fatal("expected error for missing schema")
	}
}

func TestParseSqlcConfig_MissingOutput(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "sqlc.yaml", `version: "2"
sql:
  - schema: "schema.sql"
`)

	// Act & Assert
	_, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	if err == nil {
		t.Fatal("expected error for missing output directory")
	}
}

func TestParseSqlcConfig_MissingGoMod(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	// Create a nested dir so the walk-up doesn't find any go.mod
	nested := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nested, "sqlc.yaml", `version: "2"
sql:
  - schema: "schema.sql"
    gen:
      go:
        package: "db"
        out: "db"
`)
	writeFile(t, nested, "schema.sql", "CREATE TABLE x (id INT);")
	if err := os.MkdirAll(filepath.Join(nested, "db"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Act & Assert
	_, err := ParseSqlcConfig(filepath.Join(nested, "sqlc.yaml"))
	if err == nil {
		t.Fatal("expected error for missing go.mod")
	}
}

func TestDetectSqlcConfigVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "v2 quoted", input: `version: "2"`, want: 2},
		{name: "v2 unquoted", input: "version: 2", want: 2},
		{name: "v1 quoted", input: `version: "1"`, want: 1},
		{name: "v1 unquoted", input: "version: 1", want: 1},
		{name: "no version", input: "packages:", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := detectSqlcConfigVersion([]string{tt.input})

			// Assert
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveSchemaFiles(t *testing.T) {
	tests := []struct {
		name  string
		val   string
		count int
	}{
		{name: "single file", val: "schema.sql", count: 1},
		{name: "list syntax", val: "[a.sql, b.sql, c.sql]", count: 3},
		{name: "empty", val: "", count: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			files := resolveSchemaFiles(tt.val, "/base")

			// Assert
			if len(files) != tt.count {
				t.Fatalf("got %d files, want %d", len(files), tt.count)
			}
		})
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
