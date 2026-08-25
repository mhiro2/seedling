package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupSqlcProject writes go.mod, the sqlc.yaml content, and the given schema
// files into a temp dir and returns the dir.
func setupSqlcProject(t *testing.T, module, config string, schemaFiles ...string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "sqlc.yaml", config)
	writeFile(t, dir, "go.mod", "module "+module+"\n\ngo 1.26\n")
	for _, f := range schemaFiles {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, f, "CREATE TABLE users (id INT);")
	}
	return dir
}

func TestParseSqlcConfig_V2(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "github.com/example/myapp", `version: "2"
sql:
  - schema: "schema.sql"
    queries: "query.sql"
    engine: "postgresql"
    gen:
      go:
        package: "db"
        out: "internal/db"
        emit_json_tags: true
`, "schema.sql")

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
	if cfg.SqlcDir != filepath.Join(dir, "internal", "db") {
		t.Fatalf("unexpected sqlc dir %q", cfg.SqlcDir)
	}
	if cfg.SqlcImportPath != "github.com/example/myapp/internal/db" {
		t.Fatalf("expected import path %q, got %q", "github.com/example/myapp/internal/db", cfg.SqlcImportPath)
	}
}

func TestParseSqlcConfig_V2UnquotedVersion(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "example.com/app", `version: 2
sql:
  - schema: schema.sql
    gen:
      go:
        package: db
        out: db
`, "schema.sql")

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SqlcImportPath != "example.com/app/db" {
		t.Fatalf("unexpected import path %q", cfg.SqlcImportPath)
	}
}

func TestParseSqlcConfig_V1(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "github.com/example/v1app", `version: "1"
packages:
  - schema: "schema.sql"
    queries: "query.sql"
    name: "db"
    path: "internal/db"
`, "schema.sql")

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

func TestParseSqlcConfig_SchemaFlowList(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "example.com/app", `version: "2"
sql:
  - schema: [a.sql, "b.sql"]
    gen:
      go:
        package: "db"
        out: "db"
`, "a.sql", "b.sql")

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaBases(t, cfg.SchemaFiles, "a.sql", "b.sql")
}

func TestParseSqlcConfig_SchemaBlockList(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "example.com/app", `version: "2"
sql:
  - schema:
      - migrations/001_users.sql
      - migrations/002_posts.sql
    queries: query.sql
    gen:
      go:
        package: db
        out: db
`, "migrations/001_users.sql", "migrations/002_posts.sql")

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaBases(t, cfg.SchemaFiles, "001_users.sql", "002_posts.sql")
}

func TestParseSqlcConfig_SchemaDirectory(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "example.com/app", `version: "2"
sql:
  - schema: migrations
    gen:
      go:
        package: db
        out: db
`, "migrations/002_posts.sql", "migrations/001_users.sql", "migrations/notes.txt", "migrations/001_users.down.sql", "migrations/.hidden.sql")

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaBases(t, cfg.SchemaFiles, "001_users.sql", "002_posts.sql")
}

func TestParseSqlcConfig_SchemaGlob(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "example.com/app", `version: "2"
sql:
  - schema: "migrations/*"
    gen:
      go:
        package: db
        out: db
`, "migrations/002_posts.sql", "migrations/001_users.sql", "migrations/notes.txt", "migrations/001_users.down.sql", "migrations/.hidden.sql")

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaBases(t, cfg.SchemaFiles, "001_users.sql", "002_posts.sql")
}

func TestParseSqlcConfig_NonGoEntryIgnored(t *testing.T) {
	// Arrange
	dir := setupSqlcProject(t, "example.com/app", `version: "2"
sql:
  - schema: schema.sql
    gen:
      kotlin:
        package: com.example
        out: kt
  - schema: schema.sql
    gen:
      go:
        package: db
        out: db
`, "schema.sql")

	// Act
	cfg, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SqlcPkg != "db" {
		t.Fatalf("expected Go entry to be selected, got package %q", cfg.SqlcPkg)
	}
}

func TestParseSqlcConfig_Errors(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "multiple go packages",
			config: `version: "2"
sql:
  - schema: schema.sql
    gen:
      go:
        package: db
        out: db
  - schema: schema.sql
    gen:
      go:
        package: other
        out: other
`,
			wantErr: "2 sql entries generate Go code",
		},
		{
			name: "v1 multiple packages",
			config: `version: "1"
packages:
  - name: db
    path: db
    schema: schema.sql
  - name: other
    path: other
    schema: schema.sql
`,
			wantErr: "2 packages entries found",
		},
		{
			name: "missing out",
			config: `version: "2"
sql:
  - schema: schema.sql
    gen:
      go:
        package: db
`,
			wantErr: "no Go output directory",
		},
		{
			name: "missing gen",
			config: `version: "2"
sql:
  - schema: schema.sql
`,
			wantErr: "no sql entry has a gen.go block",
		},
		{
			name: "missing schema",
			config: `version: "2"
sql:
  - gen:
      go:
        package: db
        out: db
`,
			wantErr: "no schema path",
		},
		{
			name: "schema file does not exist",
			config: `version: "2"
sql:
  - schema: missing.sql
    gen:
      go:
        package: db
        out: db
`,
			wantErr: "schema path",
		},
		{
			name:    "no sql entries",
			config:  "version: \"2\"\nsql: []\n",
			wantErr: "no sql entries found",
		},
		{
			name:    "missing version",
			config:  "sql:\n  - schema: schema.sql\n",
			wantErr: "version is required",
		},
		{
			name:    "unsupported version",
			config:  "version: \"3\"\nsql: []\n",
			wantErr: `unsupported version "3"`,
		},
		{
			name: "schema wrong type",
			config: `version: "2"
sql:
  - schema: {a: b}
    gen:
      go:
        package: db
        out: db
`,
			wantErr: "schema must be a string or a list of strings",
		},
		{
			name:    "malformed yaml",
			config:  "version: \"2\"\nsql:\n  - schema: [a.sql\n",
			wantErr: "parse sqlc config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := setupSqlcProject(t, "example.com/app", tt.config, "schema.sql")

			// Act
			_, err := ParseSqlcConfig(filepath.Join(dir, "sqlc.yaml"))
			// Assert
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseSqlcConfig_MissingFile(t *testing.T) {
	// Act & Assert
	_, err := ParseSqlcConfig("/nonexistent/sqlc.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
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

	// Act & Assert
	_, err := ParseSqlcConfig(filepath.Join(nested, "sqlc.yaml"))
	if err == nil {
		t.Fatal("expected error for missing go.mod")
	}
}

func assertSchemaBases(t *testing.T, files []string, want ...string) {
	t.Helper()
	if len(files) != len(want) {
		t.Fatalf("expected %d schema files, got %d: %v", len(want), len(files), files)
	}
	for i, f := range files {
		if filepath.Base(f) != want[i] {
			t.Fatalf("schema file %d: expected %q, got %q", i, want[i], f)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
