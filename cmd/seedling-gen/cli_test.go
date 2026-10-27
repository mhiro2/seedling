package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
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

func TestCliVersion_BuildInfoFallback(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		buildInfo *debug.BuildInfo
		want      string
	}{
		{name: "no build info", version: "dev", buildInfo: nil, want: "dev"},
		{name: "empty module version", version: "dev", buildInfo: &debug.BuildInfo{}, want: "dev"},
		{name: "devel module version", version: "dev", buildInfo: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, want: "dev"},
		{name: "module version", version: "dev", buildInfo: &debug.BuildInfo{Main: debug.Module{Version: "v1.4.0"}}, want: "v1.4.0"},
		{name: "ldflags version wins", version: "v2.0.0", buildInfo: &debug.BuildInfo{Main: debug.Module{Version: "v1.4.0"}}, want: "v2.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			prevVersion, prevReadBuildInfo := version, readBuildInfo
			t.Cleanup(func() { version, readBuildInfo = prevVersion, prevReadBuildInfo })
			version = tt.version
			readBuildInfo = func() (*debug.BuildInfo, bool) { return tt.buildInfo, tt.buildInfo != nil }

			// Act
			got := cliVersion()

			// Assert
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
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
	exitCode := run([]string{"sql", "-dialect", "postgres", "-pkg", "fixtures", schemaPath}, &stdout, &stderr)

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
	exitCode := run([]string{"sql", "-pkg", "fixtures", "-out", outputPath, schemaPath}, &stdout, &stderr)

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

func TestRun_CheckComparesGeneratedFile(t *testing.T) {
	const schema = `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`
	const changedSchema = `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL
);
`

	tests := []struct {
		name        string
		existing    bool
		checkSchema string
		wantExit    int
		wantStderr  string
	}{
		{name: "up to date", existing: true, checkSchema: schema, wantExit: 0},
		{name: "out of date", existing: true, checkSchema: changedSchema, wantExit: 1, wantStderr: "is out of date"},
		{name: "missing", checkSchema: schema, wantExit: 1, wantStderr: "does not exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			outputPath := filepath.Join(t.TempDir(), "blueprints_gen.go")
			if tt.existing {
				var stdout, stderr bytes.Buffer
				if code := run([]string{"sql", "-pkg", "fixtures", "-out", outputPath, writeSchemaFile(t, schema)}, &stdout, &stderr); code != 0 {
					t.Fatalf("initial generation exit code = %d, stderr = %s", code, stderr.String())
				}
			}
			before, beforeErr := os.ReadFile(outputPath)

			// Act
			var stdout, stderr bytes.Buffer
			exitCode := run([]string{"sql", "-pkg", "fixtures", "-out", outputPath, "-check", writeSchemaFile(t, tt.checkSchema)}, &stdout, &stderr)

			// Assert
			if exitCode != tt.wantExit {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", exitCode, tt.wantExit, stderr.String())
			}
			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			after, afterErr := os.ReadFile(outputPath)
			if !bytes.Equal(before, after) || (beforeErr == nil) != (afterErr == nil) {
				t.Fatal("--check modified the output file")
			}
		})
	}
}

func TestRun_CheckQuickstartExampleIsUpToDate(t *testing.T) {
	// Arrange
	exampleDir := filepath.Join("..", "..", "examples", "quickstart")
	args := []string{
		"sql", "-pkg", "quickstart",
		"-out", filepath.Join(exampleDir, "blueprints_gen.go"),
		"-check", filepath.Join(exampleDir, "schema.sql"),
	}

	// Act
	var stdout, stderr bytes.Buffer
	exitCode := run(args, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("exit code = %d; run go generate ./examples/quickstart\n%s", exitCode, stderr.String())
	}
}

func TestRun_CheckRejectsUnusableFlagCombinations(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "without out", args: []string{"-check"}, wantStderr: "--check requires --out"},
		{name: "with explain", args: []string{"-check", "-out", "blueprints_gen.go", "-explain"}, wantStderr: "--check cannot be combined with --explain or --json"},
		{name: "with json", args: []string{"-check", "-out", "blueprints_gen.go", "-json"}, wantStderr: "--check cannot be combined with --explain or --json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			args := append([]string{"sql"}, tt.args...)
			args = append(args, writeSchemaFile(t, "CREATE TABLE users (id SERIAL PRIMARY KEY);"))

			// Act
			var stdout, stderr bytes.Buffer
			exitCode := run(args, &stdout, &stderr)

			// Assert
			if exitCode != 1 {
				t.Fatalf("exit code = %d, want 1", exitCode)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRun_RequiresCommand(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run(nil, &stdout, &stderr)

	// Assert
	if got, want := exitCode, 1; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s := stdout.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}
	if !strings.Contains(stderr.String(), "Usage:\n  seedling-gen <command> [flags]") {
		t.Errorf("stderr does not contain root usage")
	}
	if !strings.Contains(stderr.String(), "Error: command is required") {
		t.Errorf("stderr does not contain %q", "Error: command is required")
	}
}

func TestRun_SQLRequiresSchemaPath(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"sql", "-pkg", "fixtures"}, &stdout, &stderr)

	// Assert
	if got, want := exitCode, 1; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s := stdout.String(); s != "" {
		t.Errorf("got %v, want empty string", s)
	}
	if !strings.Contains(stderr.String(), "Usage: seedling-gen sql [flags] <schema.sql>") {
		t.Errorf("stderr does not contain sql usage")
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
	exitCode := run([]string{"sql", "-dialect", "oracle", schemaPath}, &stdout, &stderr)

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

func TestRun_RejectsSchemaWithoutTables(t *testing.T) {
	tests := []struct {
		name    string
		command string
		file    string
		content string
		wantErr string
	}{
		{
			name:    "sql",
			command: "sql",
			file:    "schema.sql",
			content: "-- no tables yet\nCREATE INDEX idx_users_email ON users (email);\n",
			wantErr: "no CREATE TABLE statements found in",
		},
		{
			name:    "atlas",
			command: "atlas",
			file:    "schema.hcl",
			content: "schema \"public\" {}\n",
			wantErr: "no tables found in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeFile(t, dir, tt.file, tt.content)
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			// Act
			exitCode := run([]string{tt.command, filepath.Join(dir, tt.file)}, &stdout, &stderr)

			// Assert
			if got, want := exitCode, 1; got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
			if s := stdout.String(); s != "" {
				t.Errorf("got %v, want empty string", s)
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantErr)
			}
		})
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
				") ENGINE=InnoDB;\n" +
				"CREATE TABLE `users` (\n" +
				"    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,\n" +
				"    `company_id` BIGINT NOT NULL,\n" +
				"    CONSTRAINT `users_company_id_foreign` FOREIGN KEY (`company_id`) REFERENCES `companies`(`id`)\n" +
				") ENGINE=InnoDB;\n",
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
