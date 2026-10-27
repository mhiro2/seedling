package main

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func mustParseSchema(t *testing.T, sql string) []Table {
	t.Helper()
	tables, err := ParseSchema(sql)
	if err != nil {
		t.Fatalf("ParseSchema error: %v", err)
	}
	return tables
}

const testSchema = `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    company_id INTEGER NOT NULL REFERENCES companies(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
`

func TestParseSchema_BasicTableMetadata(t *testing.T) {
	// Arrange
	tables := mustParseSchema(t, testSchema)
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}

	tests := []struct {
		name            string
		index           int
		wantName        string
		wantGoName      string
		wantBlueprintID string
	}{
		{name: "companies", index: 0, wantName: "companies", wantGoName: "Company", wantBlueprintID: "company"},
		{name: "users", index: 1, wantName: "users", wantGoName: "User", wantBlueprintID: "user"},
	}

	// Assert
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := tables[tt.index]
			if table.Name != tt.wantName {
				t.Fatalf("expected table name %q, got %q", tt.wantName, table.Name)
			}
			if table.GoName != tt.wantGoName {
				t.Fatalf("expected GoName %q, got %q", tt.wantGoName, table.GoName)
			}
			if table.BlueprintID != tt.wantBlueprintID {
				t.Fatalf("expected BlueprintID %q, got %q", tt.wantBlueprintID, table.BlueprintID)
			}
		})
	}
}

func TestParseSchema_UserColumnMetadata(t *testing.T) {
	// Arrange
	tables := mustParseSchema(t, testSchema)
	users := tables[1]

	expected := []struct {
		name     string
		goName   string
		goType   string
		isPK     bool
		isFK     bool
		refTable string
		notNull  bool
	}{
		{name: "id", goName: "ID", goType: "int", isPK: true, notNull: true},
		{name: "name", goName: "Name", goType: "string", notNull: true},
		{name: "email", goName: "Email", goType: "string", notNull: true},
		{name: "company_id", goName: "CompanyID", goType: "int", isFK: true, refTable: "companies", notNull: true},
		{name: "created_at", goName: "CreatedAt", goType: "time.Time", notNull: true},
	}

	// Assert
	if len(users.Columns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(users.Columns))
	}

	for i, want := range expected {
		t.Run(want.name, func(t *testing.T) {
			col := users.Columns[i]
			if col.Name != want.name {
				t.Fatalf("expected name %q, got %q", want.name, col.Name)
			}
			if col.GoName != want.goName {
				t.Fatalf("expected GoName %q, got %q", want.goName, col.GoName)
			}
			if col.GoType != want.goType {
				t.Fatalf("expected GoType %q, got %q", want.goType, col.GoType)
			}
			if col.IsPK != want.isPK {
				t.Fatalf("expected IsPK=%v, got %v", want.isPK, col.IsPK)
			}
			if col.IsFK != want.isFK {
				t.Fatalf("expected IsFK=%v, got %v", want.isFK, col.IsFK)
			}
			if col.FKRefTable != want.refTable {
				t.Fatalf("expected FKRefTable %q, got %q", want.refTable, col.FKRefTable)
			}
			if col.NotNull != want.notNull {
				t.Fatalf("expected NotNull=%v, got %v", want.notNull, col.NotNull)
			}
		})
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"users", "user"},
		{"companies", "company"},
		{"addresses", "address"},
		{"task", "task"},
		{"statuses", "status"},
	}
	for _, tt := range tests {
		// Act & Assert
		got := singularize(tt.input)
		if got != tt.want {
			t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToGoFieldName(t *testing.T) {
	// Arrange
	tests := []struct {
		input, want string
	}{
		{"id", "ID"},
		{"name", "Name"},
		{"company_id", "CompanyID"},
		{"created_at", "CreatedAt"},
		{"api_url", "APIURL"},
		{"type", "Type"},
		{"func", "Func"},
	}

	// Act & Assert
	for _, tt := range tests {
		got := toGoFieldName(tt.input)
		if got != tt.want {
			t.Errorf("toGoFieldName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToGoStructName(t *testing.T) {
	// Arrange
	tests := []struct {
		input string
		want  string
	}{
		{input: "users", want: "User"},
		{input: "types", want: "Type"},
		{input: "funcs", want: "Func"},
	}

	// Act & Assert
	for _, tt := range tests {
		got := toGoStructName(tt.input)
		if got != tt.want {
			t.Errorf("toGoStructName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSQLTypeToGoType(t *testing.T) {
	// Arrange
	tests := []struct {
		input, want string
	}{
		{"SERIAL", "int"},
		{"BIGSERIAL", "int64"},
		{"INTEGER", "int"},
		{"BIGINT", "int64"},
		{"INT", "int"},
		{"TEXT", "string"},
		{"VARCHAR", "string"},
		{"CHAR", "string"},
		{"UUID", "string"},
		{"BOOLEAN", "bool"},
		{"BOOL", "bool"},
		{"TIMESTAMP", "time.Time"},
		{"TIMESTAMPTZ", "time.Time"},
		{"NUMERIC", "float64"},
		{"DECIMAL", "float64"},
		{"REAL", "float64"},
		{"FLOAT", "float64"},
		{"DOUBLE", "float64"},
	}
	for _, tt := range tests {
		// Act & Assert
		got := sqlTypeToGoType(tt.input)
		if got != tt.want {
			t.Errorf("sqlTypeToGoType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerate_OutputIncludesExpectedSections(t *testing.T) {
	// Arrange
	tables := mustParseSchema(t, testSchema)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "blueprints", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	output := buf.String()

	// Assert
	tests := []struct {
		name    string
		substr  string
		message string
	}{
		{name: "generated header", substr: "// Code generated by seedling-gen. DO NOT EDIT.", message: "output should be marked as generated"},
		{name: "package", substr: "package blueprints", message: "output should contain package declaration"},
		{name: "time import", substr: `"time"`, message: "output should import time"},
		{name: "seedling import", substr: `"github.com/mhiro2/seedling"`, message: "output should import seedling"},
		{name: "blueprints struct", substr: "type Blueprints struct", message: "output should expose the blueprints as a struct"},
		{name: "blueprints field", substr: "User    seedling.Blueprint[User]", message: "output should hold each blueprint in a field"},
		{name: "blueprints constructor", substr: "func NewBlueprints() Blueprints", message: "output should contain the blueprints constructor"},
		{name: "register method", substr: "func (b Blueprints) Register(reg *seedling.Registry) error", message: "output should register into the provided registry"},
		{name: "must register method", substr: "func (b Blueprints) MustRegister(reg *seedling.Registry)", message: "output should offer a panicking registration helper"},
		{name: "register to registry", substr: "seedling.RegisterTo(reg, b.User)", message: "output should register every blueprint"},
		{name: "company struct", substr: "type Company struct", message: "output should contain Company struct"},
		{name: "user struct", substr: "type User struct", message: "output should contain User struct"},
		{name: "company blueprint", substr: `Name:    "company"`, message: "output should register company blueprint"},
		{name: "user blueprint", substr: `Name:    "user"`, message: "output should register user blueprint"},
		{name: "companies table", substr: `Table:   "companies"`, message: "output should contain companies table"},
		{name: "users table", substr: `Table:   "users"`, message: "output should contain users table"},
		{name: "belongs to relation", substr: `seedling.BelongsTo`, message: "output should contain BelongsTo relation"},
		{name: "local field", substr: `LocalField: "CompanyID"`, message: "output should reference CompanyID"},
		{name: "ref blueprint", substr: `RefBlueprint: "company"`, message: "output should reference company blueprint"},
		{name: "required relation (no Optional)", substr: `RefBlueprint: "company"}`, message: "required relation should omit Optional field"},
		{name: "insert left to hand-written code", substr: "// Insert is not generated: set it in hand-written code before Register.", message: "output should explain the missing Insert"},
	}

	for _, check := range tests {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(output, check.substr) {
				t.Fatal(check.message)
			}
		})
	}

	// A DDL-only schema says nothing about how rows are written, so no
	// blueprint may carry an Insert that silently succeeds.
	for _, absent := range []string{"Insert: func", `"context"`, "TODO"} {
		if strings.Contains(output, absent) {
			t.Errorf("output should not contain %q:\n%s", absent, output)
		}
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"unknown"}, &stdout, &stderr)

	// Assert
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), `Error: unknown command "unknown"`) {
		t.Fatalf("expected unknown command error, got: %s", stderr.String())
	}
}

func TestGenerate_NoTime(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE tags (
    id SERIAL PRIMARY KEY,
    label TEXT NOT NULL
);
`
	tables := mustParseSchema(t, schema)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "mypkg", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Assert
	output := buf.String()
	if strings.Contains(output, `"time"`) {
		t.Error("output should not import time when no TIMESTAMP columns exist")
	}
	if !strings.Contains(output, "package mypkg") {
		t.Error("output should use custom package name")
	}
}

func TestGenerate_PrimaryKeyFallback(t *testing.T) {
	// Arrange
	tables := mustParseSchema(t, `
CREATE TABLE users (
    user_key BIGINT PRIMARY KEY
);

CREATE TABLE legacy_users (
    id BIGINT NOT NULL,
    name TEXT NOT NULL
);

CREATE TABLE audit_logs (
    message TEXT NOT NULL
);

CREATE TABLE configs ();
`)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "blueprints", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Assert: each blueprint names only a key field its struct has.
	output := buf.String()
	tests := []struct {
		blueprint string
		want      string
	}{
		{blueprint: "user", want: `PKField: "UserKey"`},
		{blueprint: "legacy_user", want: `PKField: "ID"`},
		{blueprint: "audit_log", want: ""},
		{blueprint: "config", want: ""},
	}
	for _, tt := range tests {
		block := generatedBlueprintBlock(t, output, tt.blueprint)
		if tt.want == "" {
			if strings.Contains(block, "PKField") {
				t.Errorf("blueprint %q should declare no primary key, got:\n%s", tt.blueprint, block)
			}
			continue
		}
		if !strings.Contains(block, tt.want) {
			t.Errorf("blueprint %q should contain %q, got:\n%s", tt.blueprint, tt.want, block)
		}
	}
}

// generatedBlueprintBlock returns the blueprint literal for the named
// blueprint in generated output.
func generatedBlueprintBlock(t *testing.T, output, name string) string {
	t.Helper()
	nameField := regexp.MustCompile(`\bName:\s+` + regexp.QuoteMeta(strconv.Quote(name)) + `,`)
	for block := range strings.SplitSeq(output, "seedling.Blueprint[") {
		if nameField.MatchString(block) {
			return block
		}
	}
	t.Fatalf("blueprint %q not found in output:\n%s", name, output)
	return ""
}

func TestParseSchema_GoKeywordIdentifiersRemainValidGoNames(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE types (
    id SERIAL PRIMARY KEY,
    func TEXT NOT NULL
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].GoName != "Type" {
		t.Fatalf("expected GoName %q, got %q", "Type", tables[0].GoName)
	}
	if tables[0].Columns[1].GoName != "Func" {
		t.Fatalf("expected column GoName %q, got %q", "Func", tables[0].Columns[1].GoName)
	}
}

func TestParseSchema_VarcharWithLength(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    sku VARCHAR(50) NOT NULL,
    price NUMERIC(10,2) NOT NULL
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(tables[0].Columns))
	}
	if got := findColumnByName(t, tables[0], "sku").GoType; got != "string" {
		t.Errorf("VARCHAR(50) should map to string, got %q", got)
	}
	if got := findColumnByName(t, tables[0], "price").GoType; got != "float64" {
		t.Errorf("NUMERIC(10,2) should map to float64, got %q", got)
	}
}

func TestParseSchema_CaseInsensitive(t *testing.T) {
	// Arrange
	schema := `
create table items (
    id serial primary key,
    name text not null
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "items" {
		t.Errorf("expected table name 'items', got %q", tables[0].Name)
	}
	if !tables[0].Columns[0].IsPK {
		t.Error("expected id to be primary key (case-insensitive)")
	}
}

func TestParseSchema_TableLevelConstraints(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE memberships (
    id BIGSERIAL,
    company_id BIGINT NOT NULL,
    CONSTRAINT memberships_pkey PRIMARY KEY (id),
    CONSTRAINT memberships_company_id_fkey FOREIGN KEY (company_id) REFERENCES companies(id)
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	var idCol, companyIDCol Column
	for _, col := range tables[0].Columns {
		switch col.Name {
		case "id":
			idCol = col
		case "company_id":
			companyIDCol = col
		}
	}

	if !idCol.IsPK {
		t.Fatal("expected id to be marked as primary key from table constraint")
	}
	if !companyIDCol.IsFK {
		t.Fatal("expected company_id to be marked as foreign key from table constraint")
	}
	if companyIDCol.FKRefTable != "companies" {
		t.Fatalf("company_id FKRefTable = %q, want %q", companyIDCol.FKRefTable, "companies")
	}
}

func TestParseSchema_QuotedIdentifiers(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE "users" (
    "id" SERIAL PRIMARY KEY,
    "company_id" INTEGER REFERENCES "companies"("id")
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "users" {
		t.Fatalf("table name = %q, want %q", tables[0].Name, "users")
	}
	if tables[0].Columns[1].Name != "company_id" {
		t.Fatalf("column name = %q, want %q", tables[0].Columns[1].Name, "company_id")
	}
	if tables[0].Columns[1].FKRefTable != "companies" {
		t.Fatalf("FKRefTable = %q, want %q", tables[0].Columns[1].FKRefTable, "companies")
	}
}

func TestGenerate_RelationNamesFromColumnNames(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE reviews (
    id SERIAL PRIMARY KEY,
    author_id INTEGER NOT NULL REFERENCES users(id),
    reviewer_id INTEGER REFERENCES users(id)
);
`
	tables := mustParseSchema(t, schema)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "blueprints", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Assert
	output := buf.String()
	if !strings.Contains(output, `{Name: "author"`) {
		t.Fatalf("expected relation name derived from author_id: %s", output)
	}
	if !strings.Contains(output, `{Name: "reviewer"`) {
		t.Fatalf("expected relation name derived from reviewer_id: %s", output)
	}
}

func TestParseSchemaWithDialect_Unsupported(t *testing.T) {
	// Act & Assert
	if _, err := ParseSchemaWithDialect("CREATE TABLE x (id INT);", "oracle"); err == nil {
		t.Fatal("expected unsupported dialect error")
	}
}

func TestParseSchema_UnclosedParenthesis(t *testing.T) {
	// Arrange
	tests := []struct {
		name   string
		schema string
	}{
		{
			name:   "missing closing paren",
			schema: "CREATE TABLE users (id INT, name TEXT",
		},
		{
			name: "second table broken",
			schema: `CREATE TABLE companies (id INT PRIMARY KEY);
CREATE TABLE users (id INT, company_id INT`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := ParseSchema(tt.schema)

			// Assert
			if err == nil {
				t.Fatal("expected error for unclosed parenthesis")
			}
			if !strings.Contains(err.Error(), "unclosed parenthesis") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGenerate_CompositePKAndFK(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE regions (
    country_code TEXT NOT NULL,
    region_code TEXT NOT NULL,
    PRIMARY KEY (country_code, region_code)
);

CREATE TABLE deployments (
    id SERIAL PRIMARY KEY,
    region_country_code TEXT NOT NULL,
    region_code TEXT NOT NULL,
    CONSTRAINT deployments_region_fkey FOREIGN KEY (region_country_code, region_code) REFERENCES regions(country_code, region_code)
);
`
	tables := mustParseSchema(t, schema)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "blueprints", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Assert
	output := buf.String()
	if !strings.Contains(output, `PKFields: []string{"CountryCode", "RegionCode"}`) {
		t.Fatalf("expected composite PKFields output, got: %s", output)
	}
	if !strings.Contains(output, `LocalFields: []string{"RegionCountryCode", "RegionCode"}`) {
		t.Fatalf("expected composite LocalFields output, got: %s", output)
	}
}

func TestGenerate_OptionalRelation(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    company_id INTEGER REFERENCES companies(id)
);
`
	tables := mustParseSchema(t, schema)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "blueprints", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Assert
	output := buf.String()
	if !strings.Contains(output, "Optional: true") {
		t.Fatalf("expected Optional: true for nullable FK, got:\n%s", output)
	}
}

func TestGenerate_DefaultsAutofillSupportedFields(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    score DOUBLE NOT NULL,
    avatar BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL,
    company_id INTEGER NOT NULL REFERENCES companies(id)
);
`
	tables := mustParseSchema(t, schema)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "blueprints", tables); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	// Assert
	output := buf.String()
	tests := []struct {
		name    string
		substr  string
		missing bool
	}{
		{name: "string default", substr: `Name: "user-name"`},
		{name: "bool default", substr: `Active: true`},
		{name: "numeric default", substr: `Score: 1`},
		{name: "bytes default", substr: `Avatar: []byte("user-avatar")`},
		{name: "time default", substr: `CreatedAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)`},
		{name: "relation key skipped", substr: `CompanyID: 1`, missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contains := strings.Contains(output, tt.substr)
			if tt.missing && contains {
				t.Fatalf("expected output not to contain %q\n\nGot:\n%s", tt.substr, output)
			}
			if !tt.missing && !contains {
				t.Fatalf("expected output to contain %q\n\nGot:\n%s", tt.substr, output)
			}
		})
	}
}

func TestParseSchema_SelfReferencingFK(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE employees (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    manager_id INTEGER REFERENCES employees(id)
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	var managerCol Column
	for _, col := range tables[0].Columns {
		if col.Name == "manager_id" {
			managerCol = col
			break
		}
	}

	if !managerCol.IsFK {
		t.Fatal("expected manager_id to be a foreign key")
	}
	if managerCol.FKRefTable != "employees" {
		t.Fatalf("expected FKRefTable %q, got %q", "employees", managerCol.FKRefTable)
	}
	if managerCol.NotNull {
		t.Fatal("expected manager_id to be nullable (self-referencing optional FK)")
	}
}

func TestParseSchema_WithComments(t *testing.T) {
	// Arrange
	tests := []struct {
		name     string
		schema   string
		wantCols int
		colNames []string
	}{
		{
			name: "line comment after column",
			schema: `
CREATE TABLE items (
    id SERIAL PRIMARY KEY, -- auto-increment
    name TEXT NOT NULL -- required
);
`,
			wantCols: 2,
			colNames: []string{"id", "name"},
		},
		{
			name: "line comment between columns",
			schema: `
CREATE TABLE items (
    id SERIAL PRIMARY KEY,
    -- user-facing name
    name TEXT NOT NULL
);
`,
			wantCols: 2,
			colNames: []string{"id", "name"},
		},
		{
			name: "block comment wrapping column",
			schema: `
CREATE TABLE items (
    id SERIAL PRIMARY KEY,
    /* temporarily disabled
    legacy_code TEXT,
    */
    name TEXT NOT NULL
);
`,
			wantCols: 2,
			colNames: []string{"id", "name"},
		},
		{
			name: "comment-like content in string literal",
			schema: `
CREATE TABLE items (
    id SERIAL PRIMARY KEY,
    note TEXT NOT NULL DEFAULT '-- not a comment'
);
`,
			wantCols: 2,
			colNames: []string{"id", "note"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			tables := mustParseSchema(t, tt.schema)

			// Assert
			if len(tables) != 1 {
				t.Fatalf("expected 1 table, got %d", len(tables))
			}
			if len(tables[0].Columns) != tt.wantCols {
				t.Fatalf("expected %d columns, got %d", tt.wantCols, len(tables[0].Columns))
			}
			for i, want := range tt.colNames {
				if tables[0].Columns[i].Name != want {
					t.Fatalf("column[%d] = %q, want %q", i, tables[0].Columns[i].Name, want)
				}
			}
		})
	}
}

func TestParseSchema_SchemaQualifiedFKRef(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE public.companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE public.users (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL,
    CONSTRAINT users_company_fkey FOREIGN KEY (company_id) REFERENCES public.companies(id)
);
`
	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	if tables[0].Name != "companies" {
		t.Fatalf("expected table name %q, got %q", "companies", tables[0].Name)
	}
	if tables[1].Name != "users" {
		t.Fatalf("expected table name %q, got %q", "users", tables[1].Name)
	}
	if len(tables[1].ForeignKeys) != 1 {
		t.Fatalf("expected 1 FK, got %d", len(tables[1].ForeignKeys))
	}
	if tables[1].ForeignKeys[0].RefTable != "companies" {
		t.Fatalf("expected FK ref %q, got %q", "companies", tables[1].ForeignKeys[0].RefTable)
	}
}

func TestParseSchema_IfNotExists(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE IF NOT EXISTS items (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "items" {
		t.Fatalf("expected table name %q, got %q", "items", tables[0].Name)
	}
}

func TestParseSchema_CheckConstraint(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    price NUMERIC(10,2) NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 0,
    CHECK (price > 0),
    CONSTRAINT positive_qty CHECK (quantity >= 0)
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(tables[0].Columns))
	}
	if len(tables[0].ForeignKeys) != 0 {
		t.Fatalf("expected 0 FKs, got %d", len(tables[0].ForeignKeys))
	}
}

func TestParseSchema_ComplexDefaults(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE events (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	expected := []struct {
		name   string
		goType string
	}{
		{"id", "int"},
		{"created_at", "time.Time"},
		{"status", "string"},
		{"metadata", "string"},
	}

	if len(tables[0].Columns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(tables[0].Columns))
	}
	for i, want := range expected {
		col := tables[0].Columns[i]
		if col.Name != want.name {
			t.Fatalf("column[%d] name = %q, want %q", i, col.Name, want.name)
		}
		if col.GoType != want.goType {
			t.Fatalf("column[%d] GoType = %q, want %q", i, col.GoType, want.goType)
		}
	}
}

func TestParseSchema_MixedQuotedIdentifiers(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE "orders" (
    id SERIAL PRIMARY KEY,
    "user_id" INTEGER NOT NULL REFERENCES users(id),
    total NUMERIC(10,2) NOT NULL
);
`

	// Act
	tables := mustParseSchema(t, schema)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "orders" {
		t.Fatalf("expected table name %q, got %q", "orders", tables[0].Name)
	}

	colNames := []string{"id", "user_id", "total"}
	for i, want := range colNames {
		if tables[0].Columns[i].Name != want {
			t.Fatalf("column[%d] = %q, want %q", i, tables[0].Columns[i].Name, want)
		}
	}
	if !tables[0].Columns[1].IsFK {
		t.Fatal("expected user_id to be a foreign key")
	}
	if tables[0].Columns[1].FKRefTable != "users" {
		t.Fatalf("expected FKRefTable %q, got %q", "users", tables[0].Columns[1].FKRefTable)
	}
}
