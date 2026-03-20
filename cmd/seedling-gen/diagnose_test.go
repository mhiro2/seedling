package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_SQLExplain_PrintsParsedTablesAndRelations(t *testing.T) {
	// Arrange
	schemaPath := writeSchemaFile(t, `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name TEXT NOT NULL
);
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"sql", "-dialect", "postgres", "-explain", schemaPath}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if stderr.String() != "" {
		t.Fatalf("got %q, want empty stderr", stderr.String())
	}

	output := stdout.String()
	checks := []string{
		"Adapter: sql",
		"Dialect: postgres",
		"Parsed tables:",
		"- users (go: User, blueprint: user)",
		"fk=companies",
		"relation=company",
		"Inferred blueprints:",
		"- user (table: users, type: User, pk: ID, insert: stub)",
		"Relations:",
		"localFields=CompanyID",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q\n\nGot:\n%s", check, output)
		}
	}
}

func TestRun_SQLJSON_PrintsDiagnosticReport(t *testing.T) {
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
	exitCode := run([]string{"sql", "-json", schemaPath}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if stderr.String() != "" {
		t.Fatalf("got %q, want empty stderr", stderr.String())
	}

	var report diagnosticReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report.Adapter != "sql" {
		t.Fatalf("got adapter %q, want %q", report.Adapter, "sql")
	}
	if report.Dialect != "auto" {
		t.Fatalf("got dialect %q, want %q", report.Dialect, "auto")
	}
	if len(report.Tables) != 1 {
		t.Fatalf("got %d tables, want %d", len(report.Tables), 1)
	}
	if report.Tables[0].Name != "users" {
		t.Fatalf("got table %q, want %q", report.Tables[0].Name, "users")
	}
	if len(report.Blueprints) != 1 {
		t.Fatalf("got %d blueprints, want %d", len(report.Blueprints), 1)
	}
	if report.Blueprints[0].InsertSource != "stub" {
		t.Fatalf("got insert source %q, want %q", report.Blueprints[0].InsertSource, "stub")
	}
}

func TestRun_SQLCExplain_PrintsSqlcQueryMappings(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeSqlcFiles(t, dir, map[string]string{
		"models.go": `package db

type Company struct {
	ID   int64
	Name string
}

type User struct {
	ID        int64
	Name      string
	CompanyID int64
}
`,
		"query.sql.go": `package db

import "context"

type DBTX interface{}

type Queries struct{}

func New(DBTX) *Queries { return &Queries{} }

type InsertUserParams struct {
	Name      string
	CompanyID int64
}

func (*Queries) InsertUser(context.Context, InsertUserParams) (User, error) {
	return User{}, nil
}

func (*Queries) DeleteUser(context.Context, int64) error {
	return nil
}
`,
	})
	schemaPath := writeSchemaFile(t, `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name TEXT NOT NULL
);
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"sqlc", "--dir", dir, "--import-path", "github.com/example/db", "-explain", schemaPath}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if stderr.String() != "" {
		t.Fatalf("got %q, want empty stderr", stderr.String())
	}

	output := stdout.String()
	checks := []string{
		"Adapter: sqlc",
		"Parsed sqlc metadata:",
		"package=db importPath=github.com/example/db",
		"InsertUser -> User using InsertUserParams",
		"DeleteUser using int64",
		"- user (table: users, type: db.User, pk: ID, insert: InsertUser, delete: DeleteUser)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q\n\nGot:\n%s", check, output)
		}
	}
}

func TestRun_GormExplain_PrintsParsedModels(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type Company struct {
	ID   uint   `+"`"+`gorm:"primaryKey"`+"`"+`
	Name string
}

type User struct {
	ID        uint   `+"`"+`gorm:"primaryKey"`+"`"+`
	CompanyID uint
	Company   Company `+"`"+`gorm:"foreignKey:CompanyID"`+"`"+`
}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"gorm", "--dir", dir, "--import-path", "github.com/example/models", "-explain"}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if stderr.String() != "" {
		t.Fatalf("got %q, want empty stderr", stderr.String())
	}

	output := stdout.String()
	checks := []string{
		"Adapter: gorm",
		"Parsed GORM models:",
		"- User (table: users)",
		"relation=BelongsTo ref=Company foreignKey=CompanyID",
		"- user (table: users, type: models.User, pk: ID, insert: gorm.Create, delete: gorm.Delete)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q\n\nGot:\n%s", check, output)
		}
	}
}

func TestRun_EntExplain_PrintsParsedSchemas(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "user.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).Ref("users").Unique().Required(),
	}
}
`)
	writeFile(t, dir, "company.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

type Company struct {
	ent.Schema
}

func (Company) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
	}
}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"ent", "--dir", dir, "--import-path", "github.com/example/ent", "-explain"}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if stderr.String() != "" {
		t.Fatalf("got %q, want empty stderr", stderr.String())
	}

	output := stdout.String()
	checks := []string{
		"Adapter: ent",
		"Parsed ent schemas:",
		"- User",
		"direction=From type=Company ref=users unique=true required=true",
		"- user (table: users, type: *ent.User, pk: ID, insert: ent.Create, delete: ent.DeleteOneID)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q\n\nGot:\n%s", check, output)
		}
	}
}

func TestRun_AtlasExplain_PrintsParsedTables(t *testing.T) {
	// Arrange
	atlasPath := filepath.Join("testdata", "atlas", "pass", "service_schema.hcl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"atlas", "-explain", atlasPath}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("got %v, want %v", exitCode, 0)
	}
	if stderr.String() != "" {
		t.Fatalf("got %q, want empty stderr", stderr.String())
	}

	output := stdout.String()
	checks := []string{
		"Adapter: atlas",
		"Parsed tables:",
		"- users (go: User, blueprint: user)",
		"relation=manager",
		"- user (table: users, type: User, pk: ID, insert: stub)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q\n\nGot:\n%s", check, output)
		}
	}
}
