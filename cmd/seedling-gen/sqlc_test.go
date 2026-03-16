package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSqlcFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseSqlcDir_ModelsAndQueries(t *testing.T) {
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
	Email     string
	CompanyID int64
}
`,
		"query.sql.go": `package db

import "context"

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (any, error)
}

type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}

type InsertCompanyParams struct {
	Name string
}

func (q *Queries) InsertCompany(ctx context.Context, arg InsertCompanyParams) (Company, error) {
	return Company{}, nil
}

type InsertUserParams struct {
	Name      string
	Email     string
	CompanyID int64
}

func (q *Queries) InsertUser(ctx context.Context, arg InsertUserParams) (User, error) {
	return User{}, nil
}

func (q *Queries) DeleteUser(ctx context.Context, id int64) error {
	return nil
}
`,
	})

	info, err := ParseSqlcDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if info.Package != "db" {
		t.Fatalf("expected package %q, got %q", "db", info.Package)
	}

	if len(info.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(info.Models))
	}

	// Find Company model.
	var companyModel *SqlcModel
	for i, m := range info.Models {
		if m.Name == "Company" {
			companyModel = &info.Models[i]
			break
		}
	}
	if companyModel == nil {
		t.Fatal("Company model not found")
		return
	}
	if len(companyModel.Fields) != 2 {
		t.Fatalf("expected 2 fields on Company, got %d", len(companyModel.Fields))
	}

	// Find User model.
	var userModel *SqlcModel
	for i, m := range info.Models {
		if m.Name == "User" {
			userModel = &info.Models[i]
			break
		}
	}
	if userModel == nil {
		t.Fatal("User model not found")
		return
	}
	if len(userModel.Fields) != 4 {
		t.Fatalf("expected 4 fields on User, got %d", len(userModel.Fields))
	}

	// Check queries.
	if len(info.Queries) != 2 {
		t.Fatalf("expected 2 insert queries, got %d", len(info.Queries))
	}

	// Check delete queries.
	if len(info.DeleteQueries) != 1 {
		t.Fatalf("expected 1 delete query, got %d", len(info.DeleteQueries))
	}
	if info.DeleteQueries[0].Name != "DeleteUser" {
		t.Fatalf("expected delete query name %q, got %q", "DeleteUser", info.DeleteQueries[0].Name)
	}
}

func TestParseSqlcDir_CreatePrefix(t *testing.T) {
	dir := t.TempDir()
	writeSqlcFiles(t, dir, map[string]string{
		"models.go": `package db

type Item struct {
	ID   int64
	Name string
}
`,
		"query.sql.go": `package db

import "context"

type DBTX interface{}

type Queries struct{ db DBTX }

func New(db DBTX) *Queries { return &Queries{db: db} }

type CreateItemParams struct {
	Name string
}

func (q *Queries) CreateItem(ctx context.Context, arg CreateItemParams) (Item, error) {
	return Item{}, nil
}
`,
	})

	info, err := ParseSqlcDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(info.Queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(info.Queries))
	}
	if info.Queries[0].Name != "CreateItem" {
		t.Fatalf("expected query name %q, got %q", "CreateItem", info.Queries[0].Name)
	}
	if info.Queries[0].ReturnType != "Item" {
		t.Fatalf("expected return type %q, got %q", "Item", info.Queries[0].ReturnType)
	}
	if info.Queries[0].ParamType != "CreateItemParams" {
		t.Fatalf("expected param type %q, got %q", "CreateItemParams", info.Queries[0].ParamType)
	}
	if len(info.Queries[0].ParamFields) != 1 {
		t.Fatalf("expected 1 param field, got %d", len(info.Queries[0].ParamFields))
	}
}

func TestParseSqlcDir_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeSqlcFiles(t, dir, map[string]string{
		"models.go": `package db

type Foo struct {
	ID int64
}
`,
		"models_test.go": `package db

type TestOnly struct {
	X int
}
`,
	})

	info, err := ParseSqlcDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range info.Models {
		if m.Name == "TestOnly" {
			t.Fatal("should not include types from test files")
		}
	}
}

func TestParseSqlcDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := ParseSqlcDir(dir)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestFindQueryForTable(t *testing.T) {
	info := &SqlcInfo{
		Queries: []SqlcQuery{
			{Name: "InsertCompany", ReturnType: "Company", ParamType: "InsertCompanyParams"},
			{Name: "InsertUser", ReturnType: "User", ParamType: "InsertUserParams"},
		},
	}

	table := Table{GoName: "User"}
	q := info.FindQueryForTable(table)
	if q == nil {
		t.Fatal("expected to find query for User")
		return
	}
	if q.Name != "InsertUser" {
		t.Fatalf("expected %q, got %q", "InsertUser", q.Name)
	}

	table = Table{GoName: "NotExist"}
	if info.FindQueryForTable(table) != nil {
		t.Fatal("expected nil for non-existent table")
	}
}

func TestFindDeleteQueryForTable(t *testing.T) {
	info := &SqlcInfo{
		DeleteQueries: []SqlcDeleteQuery{
			{Name: "DeleteUser", ArgName: "id", ArgType: "int64"},
		},
	}

	table := Table{GoName: "User"}
	dq := info.FindDeleteQueryForTable(table)
	if dq == nil {
		t.Fatal("expected to find delete query for User")
	}
	if dq.Name != "DeleteUser" {
		t.Fatalf("expected %q, got %q", "DeleteUser", dq.Name)
	}
}

func TestGenerateSqlc_BasicOutput(t *testing.T) {
	schema := `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    company_id INTEGER NOT NULL REFERENCES companies(id)
);
`
	tables := ParseSchema(schema)

	sqlcInfo := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "Company", Fields: []SqlcField{{Name: "ID", Type: "int64"}, {Name: "Name", Type: "string"}}},
			{Name: "User", Fields: []SqlcField{{Name: "ID", Type: "int64"}, {Name: "Name", Type: "string"}, {Name: "Email", Type: "string"}, {Name: "CompanyID", Type: "int64"}}},
		},
		Queries: []SqlcQuery{
			{Name: "InsertCompany", ReturnType: "Company", ParamType: "InsertCompanyParams", ParamFields: []SqlcField{{Name: "Name", Type: "string"}}},
			{Name: "InsertUser", ReturnType: "User", ParamType: "InsertUserParams", ParamFields: []SqlcField{{Name: "Name", Type: "string"}, {Name: "Email", Type: "string"}, {Name: "CompanyID", Type: "int64"}}},
		},
		DeleteQueries: []SqlcDeleteQuery{
			{Name: "DeleteUser", ArgName: "id", ArgType: "int64"},
		},
	}

	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, sqlcInfo); err != nil {
		t.Fatalf("GenerateSqlc error: %v", err)
	}

	output := buf.String()

	// go/format aligns struct fields with tabs, so use flexible matching.
	checks := []struct {
		name   string
		substr string
	}{
		{"package", "package testutil"},
		{"seedling import", `"github.com/mhiro2/seedling"`},
		{"sqlc import", `db "github.com/myapp/internal/db"`},
		{"company blueprint type", "seedling.Blueprint[db.Company]"},
		{"user blueprint type", "seedling.Blueprint[db.User]"},
		{"company name", `"company"`},
		{"user name", `"user"`},
		{"company table", `"companies"`},
		{"user table", `"users"`},
		{"insert company call", "InsertCompany(ctx, db.InsertCompanyParams{"},
		{"insert user call", "InsertUser(ctx, db.InsertUserParams{"},
		{"dbtx param", "dbtx seedling.DBTX"},
		{"param field Name", "Name: v.Name"},
		{"param field Email", "v.Email"},
		{"param field CompanyID", "CompanyID: v.CompanyID"},
		{"belongs to relation", "seedling.BelongsTo"},
		{"local field", `LocalField: "CompanyID"`},
		{"ref blueprint", `RefBlueprint: "company"`},
		{"required (no Optional)", `RefBlueprint: "company"}`},
		{"delete function", "DeleteUser"},
		{"no struct definition", ""},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if check.substr != "" && !strings.Contains(output, check.substr) {
				t.Fatalf("expected output to contain %q\n\nGot:\n%s", check.substr, output)
			}
		})
	}

	// Ensure no struct definitions are generated (they come from sqlc).
	if strings.Contains(output, "type Company struct") {
		t.Fatal("should not generate struct definitions in sqlc mode")
	}
	if strings.Contains(output, "type User struct") {
		t.Fatal("should not generate struct definitions in sqlc mode")
	}

	// Ensure no "time" import (no time.Time in sqlc mode).
	if strings.Contains(output, `"time"`) {
		t.Fatal("should not import time in sqlc mode")
	}
}

func TestGenerateSqlc_NoMatchingQuery(t *testing.T) {
	schema := `
CREATE TABLE tags (
    id SERIAL PRIMARY KEY,
    label TEXT NOT NULL
);
`
	tables := ParseSchema(schema)

	sqlcInfo := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "Tag", Fields: []SqlcField{{Name: "ID", Type: "int64"}, {Name: "Label", Type: "string"}}},
		},
	}

	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, sqlcInfo); err != nil {
		t.Fatalf("GenerateSqlc error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "// TODO: implement") {
		t.Fatal("expected TODO comment when no matching query found")
	}
}

func TestGenerateSqlc_CompositePK(t *testing.T) {
	schema := `
CREATE TABLE article_tags (
    article_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (article_id, tag_id)
);
`
	tables := ParseSchema(schema)
	sqlcInfo := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "ArticleTag", Fields: []SqlcField{{Name: "ArticleID", Type: "int64"}, {Name: "TagID", Type: "int64"}}},
		},
	}

	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, sqlcInfo); err != nil {
		t.Fatalf("GenerateSqlc error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `PKFields: []string{"ArticleID", "TagID"}`) {
		t.Fatalf("expected composite PKFields, got:\n%s", output)
	}
}

func TestRun_SqlcMode(t *testing.T) {
	sqlcDir := t.TempDir()
	writeSqlcFiles(t, sqlcDir, map[string]string{
		"models.go": `package db

type Item struct {
	ID   int64
	Name string
}
`,
		"query.sql.go": `package db

import "context"

type DBTX interface{}

type Queries struct{ db DBTX }

func New(db DBTX) *Queries { return &Queries{db: db} }

type InsertItemParams struct {
	Name string
}

func (q *Queries) InsertItem(ctx context.Context, arg InsertItemParams) (Item, error) {
	return Item{}, nil
}
`,
	})

	schemaPath := writeSchemaFile(t, `
CREATE TABLE items (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{
		"-pkg", "testutil",
		"-sqlc", sqlcDir,
		"-sqlc-pkg", "github.com/myapp/internal/db",
		schemaPath,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", exitCode, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "package testutil") {
		t.Fatalf("expected package testutil, got:\n%s", output)
	}
	if !strings.Contains(output, `db "github.com/myapp/internal/db"`) {
		t.Fatalf("expected sqlc import, got:\n%s", output)
	}
	if !strings.Contains(output, "db.InsertItemParams") {
		t.Fatalf("expected InsertItemParams usage, got:\n%s", output)
	}
}

func TestRun_SqlcRequiresSqlcPkg(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	schemaPath := writeSchemaFile(t, `CREATE TABLE x (id INT);`)

	exitCode := run([]string{
		"-sqlc", "/some/dir",
		schemaPath,
	}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "-sqlc-pkg is required") {
		t.Fatalf("expected sqlc-pkg required error, got: %s", stderr.String())
	}
}

func TestExprToString(t *testing.T) {
	tests := []struct {
		goType string
		want   string
	}{
		{"int64", "int64"},
		{"string", "string"},
		{"[]byte", "[]byte"},
	}

	// This tests the internal function indirectly through parsing.
	dir := t.TempDir()
	for _, tt := range tests {
		writeSqlcFiles(t, dir, map[string]string{
			"models.go": `package db

type TestModel struct {
	Field ` + tt.goType + `
}
`,
		})

		info, err := ParseSqlcDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		if len(info.Models) == 0 {
			t.Fatal("no models found")
		}
		if len(info.Models[0].Fields) == 0 {
			t.Fatal("no fields found")
		}
		if got := info.Models[0].Fields[0].Type; got != tt.want {
			t.Errorf("exprToString(%q) = %q, want %q", tt.goType, got, tt.want)
		}
	}
}
