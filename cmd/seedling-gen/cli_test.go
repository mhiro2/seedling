package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCliVersion_ExplicitVersion(t *testing.T) {
	// Arrange
	prev := version
	version = "v2.0.0"
	t.Cleanup(func() { version = prev })

	// Act
	got := cliVersion()

	// Assert
	if got != "v2.0.0" {
		t.Fatalf("got %q, want %q", got, "v2.0.0")
	}
}

func TestCliVersion_DevFallback(t *testing.T) {
	// Arrange
	prev := version
	version = "dev"
	t.Cleanup(func() { version = prev })

	// Act
	got := cliVersion()

	// Assert
	// In test context, ReadBuildInfo may return (devel), so we expect "dev" fallback
	if got == "" {
		t.Fatal("cliVersion should not return empty string")
	}
}

func TestRun_PrintsVersion(t *testing.T) {
	// Arrange
	previousVersion := version
	version = "v1.2.3"
	t.Cleanup(func() {
		version = previousVersion
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"--version"}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if got, want := stdout.String(), "v1.2.3\n"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s := stderr.String(); s != "" {
		t.Fatalf("got %v, want empty string", s)
	}
}

func TestRun_GeneratesCodeToStdout(t *testing.T) {
	// Arrange
	schemaPath := writeSchemaFile(t, `
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"-dialect", "postgres", "-pkg", "fixtures", schemaPath}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if !strings.Contains(stdout.String(), "package fixtures") {
		t.Errorf("stdout does not contain %q", "package fixtures")
	}
	if !strings.Contains(stdout.String(), "type User struct") {
		t.Errorf("stdout does not contain %q", "type User struct")
	}
	if !strings.Contains(stdout.String(), `Name:    "user"`) {
		t.Errorf("stdout does not contain %q", `Name:    "user"`)
	}
	if s := stderr.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}
}

func TestRun_WritesGeneratedCodeToFile(t *testing.T) {
	// Arrange
	schemaPath := writeSchemaFile(t, `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`)
	outputPath := filepath.Join(t.TempDir(), "blueprints_gen.go")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"-pkg", "fixtures", "-out", outputPath, schemaPath}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if s := stdout.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}
	if s := stderr.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "package fixtures") {
		t.Errorf("output does not contain %q", "package fixtures")
	}
	if !strings.Contains(string(data), "type Company struct") {
		t.Errorf("output does not contain %q", "type Company struct")
	}
}

func TestRun_RequiresSchemaPath(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"-pkg", "fixtures"}, &stdout, &stderr)

	// Assert
	if got, want := exitCode, 1; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s := stdout.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}
	if !strings.Contains(stderr.String(), "Usage: seedling-gen [flags] <schema.sql>") {
		t.Errorf("stderr does not contain %q", "Usage: seedling-gen [flags] <schema.sql>")
	}
}

func TestRun_RejectsUnsupportedDialect(t *testing.T) {
	// Arrange
	schemaPath := writeSchemaFile(t, `
CREATE TABLE users (
    id SERIAL PRIMARY KEY
);
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"-dialect", "oracle", schemaPath}, &stdout, &stderr)

	// Assert
	if got, want := exitCode, 1; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s := stdout.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}
	if !strings.Contains(stderr.String(), `unsupported dialect "oracle"`) {
		t.Errorf("stderr does not contain %q", `unsupported dialect "oracle"`)
	}
}

func TestParseSchemaWithDialect_DialectsHandleForeignKeys(t *testing.T) {
	tests := []struct {
		name              string
		dialect           string
		schema            string
		wantTableName     string
		wantColumnType    string
		wantRefTable      string
		wantBlueprintName string
	}{
		{
			name:    "postgres",
			dialect: "postgres",
			schema: `
CREATE TABLE IF NOT EXISTS public.companies (
    id BIGSERIAL PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS public.users (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES public.companies(id)
);
`,
			wantTableName:     "users",
			wantColumnType:    "int64",
			wantRefTable:      "companies",
			wantBlueprintName: "user",
		},
		{
			name:    "mysql",
			dialect: "mysql",
			schema: "" +
				"CREATE TABLE `companies` (\n" +
				"    `id` BIGINT PRIMARY KEY AUTO_INCREMENT\n" +
				");\n" +
				"CREATE TABLE `users` (\n" +
				"    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,\n" +
				"    `company_id` BIGINT NOT NULL,\n" +
				"    CONSTRAINT `users_company_id_foreign` FOREIGN KEY (`company_id`) REFERENCES `companies`(`id`)\n" +
				");\n",
			wantTableName:     "users",
			wantColumnType:    "int64",
			wantRefTable:      "companies",
			wantBlueprintName: "user",
		},
		{
			name:    "sqlite",
			dialect: "sqlite",
			schema: `
CREATE TABLE [companies] (
    [id] INTEGER PRIMARY KEY
);

CREATE TABLE [users] (
    [id] INTEGER PRIMARY KEY,
    [company_id] INTEGER NOT NULL REFERENCES [companies]([id])
);
`,
			wantTableName:     "users",
			wantColumnType:    "int",
			wantRefTable:      "companies",
			wantBlueprintName: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange

			// Act
			tables, err := ParseSchemaWithDialect(tt.schema, tt.dialect)
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if len(tables) != 2 {
				t.Fatalf("got len %d, want %d", len(tables), 2)
			}
			if got, want := tables[1].Name, tt.wantTableName; got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
			if got, want := tables[1].BlueprintID, tt.wantBlueprintName; got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
			if len(tables[1].ForeignKeys) != 1 {
				t.Fatalf("got len %d, want %d", len(tables[1].ForeignKeys), 1)
			}
			if got, want := tables[1].ForeignKeys[0].RefTable, tt.wantRefTable; got != want {
				t.Fatalf("got %v, want %v", got, want)
			}

			companyID := findColumn(t, tables[1], "company_id")
			if got, want := companyID.GoType, tt.wantColumnType; got != want {
				t.Errorf("got %v, want %v", got, want)
			}
			if !companyID.IsFK {
				t.Errorf("expected true")
			}
			if got, want := companyID.FKRefTable, tt.wantRefTable; got != want {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func TestParseSchemaWithDialect_SupportsEnumAndStandaloneIndexStatements(t *testing.T) {
	// Arrange
	schema := `
CREATE TYPE public.user_status AS ENUM ('active', 'inactive');

CREATE TABLE IF NOT EXISTS public.users (
    id BIGSERIAL PRIMARY KEY,
    status public.user_status NOT NULL
);

CREATE UNIQUE INDEX idx_users_status ON public.users (status);
`

	// Act
	tables, err := ParseSchemaWithDialect(schema, "postgres")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("got len %d, want %d", len(tables), 1)
	}
	if got, want := tables[0].Name, "users"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	status := findColumn(t, tables[0], "status")
	if got, want := status.SQLType, "USER_STATUS"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := status.GoType, "string"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if !status.NotNull {
		t.Errorf("expected true")
	}
}

func TestParseSchemaWithDialect_SupportsMySQLEnumAndInlineIndexDefinitions(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE posts (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    status ENUM('draft', 'published') NOT NULL,
    KEY idx_posts_status (status)
);
`

	// Act
	tables, err := ParseSchemaWithDialect(schema, "mysql")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("got len %d, want %d", len(tables), 1)
	}

	status := findColumn(t, tables[0], "status")
	if got, want := status.SQLType, "ENUM"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := status.GoType, "string"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if !status.NotNull {
		t.Errorf("expected true")
	}
}

func writeSchemaFile(t *testing.T, schema string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "schema.sql")
	err := os.WriteFile(path, []byte(schema), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

func findColumn(t *testing.T, table Table, name string) Column {
	t.Helper()

	for _, column := range table.Columns {
		if column.Name == name {
			return column
		}
	}

	t.Fatalf("column %q not found", name)
	return Column{}
}
