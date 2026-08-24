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

func (q *Queries) InsertUser(ctx context.Context, db DBTX, arg *InsertUserParams) (*User, error) {
	return &User{}, nil
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
	for _, query := range info.Queries {
		if query.Name == "InsertUser" && (!query.ParamPointer || !query.ReturnPointer || !query.DBArgument) {
			t.Fatalf("expected InsertUser pointer and DB metadata, got %+v", query)
		}
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

func TestParseSqlcDir_ScalarInsertAndCompositeDeleteParams(t *testing.T) {
	dir := t.TempDir()
	writeSqlcFiles(t, dir, map[string]string{
		"models.go": `package db

type Country struct {
	Code string
}

type Membership struct {
	OrganizationID int64
	UserID int64
}
`,
		"query.sql.go": `package db

import "context"

type DBTX any

type Queries struct{}

func (*Queries) InsertCountry(ctx context.Context, code string) (Country, error) {
	return Country{}, nil
}

type DeleteMembershipParams struct {
	OrganizationID int64
	UserID int64
}

func (*Queries) DeleteMembership(ctx context.Context, db DBTX, arg *DeleteMembershipParams) error {
	return nil
}
`,
	})

	info, err := ParseSqlcDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Queries) != 1 {
		t.Fatalf("insert queries = %d, want 1", len(info.Queries))
	}
	insert := info.Queries[0]
	if insert.ParamType != "" || insert.ArgName != "code" || insert.ArgType != "string" {
		t.Fatalf("unexpected scalar insert metadata: %+v", insert)
	}
	if len(info.DeleteQueries) != 1 {
		t.Fatalf("delete queries = %d, want 1", len(info.DeleteQueries))
	}
	deleteQuery := info.DeleteQueries[0]
	if deleteQuery.ParamType != "DeleteMembershipParams" || !deleteQuery.ParamPointer || !deleteQuery.DBArgument || len(deleteQuery.ParamFields) != 2 {
		t.Fatalf("unexpected composite delete metadata: %+v", deleteQuery)
	}
}

func TestGenerateSqlc_ScalarInsertAndCompositeDeleteParams(t *testing.T) {
	tables := []Table{
		{
			Name: "countries", GoName: "Country", BlueprintID: "country",
			Columns: []Column{
				{Name: "code", GoName: "Code", GoType: "string", IsPK: true, NotNull: true},
			},
		},
		{
			Name: "memberships", GoName: "Membership", BlueprintID: "membership",
			Columns: []Column{
				{Name: "organization_id", GoName: "OrganizationID", GoType: "int64", IsPK: true, NotNull: true},
				{Name: "user_id", GoName: "UserID", GoType: "int64", IsPK: true, NotNull: true},
			},
		},
	}
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "Country", Fields: []SqlcField{{Name: "Code", Type: "string"}}},
			{Name: "Membership", Fields: []SqlcField{
				{Name: "OrganizationID", Type: "int64"},
				{Name: "UserID", Type: "int64"},
			}},
		},
		Queries: []SqlcQuery{
			{Name: "InsertCountry", ReturnType: "Country", ArgName: "column_1", ArgType: "string"},
		},
		DeleteQueries: []SqlcDeleteQuery{
			{
				Name:      "DeleteMembership",
				ParamType: "DeleteMembershipParams",
				ParamFields: []SqlcField{
					{Name: "OrganizationID", Type: "int64"},
					{Name: "UserID", Type: "int64"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "InsertCountry(ctx, v.Code)") {
		t.Fatalf("expected scalar insert argument, got:\n%s", output)
	}
	if !strings.Contains(output, "DeleteMembership(ctx, db.DeleteMembershipParams{") ||
		!strings.Contains(output, "OrganizationID: v.OrganizationID") ||
		!strings.Contains(output, "UserID:         v.UserID") {
		t.Fatalf("expected composite delete params, got:\n%s", output)
	}
}

func TestGenerateSqlc_ValidatesCompositeDeletePrimaryKeyCoverage(t *testing.T) {
	tables := []Table{{
		Name: "memberships", GoName: "Membership", BlueprintID: "membership",
		Columns: []Column{
			{Name: "organization_id", GoName: "OrganizationID", GoType: "int64", IsPK: true, NotNull: true},
			{Name: "user_id", GoName: "UserID", GoType: "int64", IsPK: true, NotNull: true},
			{Name: "role", GoName: "Role", GoType: "string", NotNull: true},
		},
	}}
	model := SqlcModel{Name: "Membership", Fields: []SqlcField{
		{Name: "OrganizationID", Type: "int64"},
		{Name: "UserID", Type: "int64"},
		{Name: "Role", Type: "string"},
	}}

	tests := []struct {
		name  string
		query SqlcDeleteQuery
		want  string
	}{
		{
			name: "missing primary key",
			query: SqlcDeleteQuery{
				Name:        "DeleteMembership",
				ParamType:   "DeleteMembershipParams",
				ParamFields: []SqlcField{{Name: "OrganizationID", Type: "int64"}},
			},
			want: `does not accept primary-key model field "UserID"`,
		},
		{
			name: "non primary key",
			query: SqlcDeleteQuery{
				Name:      "DeleteMembership",
				ParamType: "DeleteMembershipParams",
				ParamFields: []SqlcField{
					{Name: "OrganizationID", Type: "int64"},
					{Name: "UserID", Type: "int64"},
					{Name: "Role", Type: "string"},
				},
			},
			want: `params field "Role" maps to non-primary-key model field "Role"`,
		},
		{
			name: "duplicate primary key",
			query: SqlcDeleteQuery{
				Name:      "DeleteMembership",
				ParamType: "DeleteMembershipParams",
				ParamFields: []SqlcField{
					{Name: "OrganizationID", Type: "int64"},
					{Name: "Organization_Id", Type: "int64"},
					{Name: "UserID", Type: "int64"},
				},
			},
			want: `model field "OrganizationID" more than once`,
		},
		{
			name: "scalar composite key",
			query: SqlcDeleteQuery{
				Name:    "DeleteMembership",
				ArgName: "organizationID",
				ArgType: "int64",
			},
			want: "scalar argument cannot cover 2 primary-key fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &SqlcInfo{
				Package:       "db",
				Models:        []SqlcModel{model},
				DeleteQueries: []SqlcDeleteQuery{tt.query},
			}
			var buf bytes.Buffer
			err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestGenerateSqlc_RejectsScalarDeletePrimaryKeyTypeMismatch(t *testing.T) {
	tables := []Table{{
		Name: "labels", GoName: "Label", BlueprintID: "label",
		Columns: []Column{{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true}},
	}}
	info := &SqlcInfo{
		Package: "db",
		Models:  []SqlcModel{{Name: "Label", Fields: []SqlcField{{Name: "ID", Type: "int64"}}}},
		DeleteQueries: []SqlcDeleteQuery{{
			Name:    "DeleteLabel",
			ArgName: "id",
			ArgType: "string",
		}},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), `type string does not match primary-key model field "ID" type int64`) {
		t.Fatalf("expected scalar primary-key type error, got %v", err)
	}
}

func TestGenerateSqlc_RejectsScalarDeleteByNonPrimaryKey(t *testing.T) {
	tables := []Table{{
		Name: "users", GoName: "User", BlueprintID: "user",
		Columns: []Column{
			{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
			{Name: "external_id", GoName: "ExternalID", GoType: "int64", NotNull: true},
		},
	}}
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{{Name: "User", Fields: []SqlcField{
			{Name: "ID", Type: "int64"},
			{Name: "ExternalID", Type: "int64"},
		}}},
		DeleteQueries: []SqlcDeleteQuery{{
			Name:    "DeleteUser",
			ArgName: "externalID",
			ArgType: "int64",
		}},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), `scalar argument "externalID" identifies non-primary-key model field "ExternalID"`) {
		t.Fatalf("expected scalar non-primary-key error, got %v", err)
	}
}

func TestGenerateSqlc_RejectsAmbiguousScalarInsertArgument(t *testing.T) {
	tables := []Table{
		{
			Name: "countries", GoName: "Country", BlueprintID: "country",
			Columns: []Column{
				{Name: "id", GoName: "ID", GoType: "int64", IsPK: true},
				{Name: "code", GoName: "Code", GoType: "string"},
				{Name: "name", GoName: "Name", GoType: "string"},
			},
		},
	}
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "Country", Fields: []SqlcField{
				{Name: "ID", Type: "int64"},
				{Name: "Code", Type: "string"},
				{Name: "Name", Type: "string"},
			}},
		},
		Queries: []SqlcQuery{
			{Name: "InsertCountry", ReturnType: "Country", ArgName: "column_1", ArgType: "string"},
		},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), "cannot map scalar argument") {
		t.Fatalf("expected ambiguous scalar mapping error, got %v", err)
	}
}

func TestGenerateSqlc_UsesParsedModelFieldNames(t *testing.T) {
	tables := mustParseSchema(t, `
CREATE TABLE companies (
    id BIGINT PRIMARY KEY,
    spotify_url TEXT UNIQUE NOT NULL
);
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    profile_name TEXT NOT NULL,
    company_spotify_url TEXT NOT NULL REFERENCES companies(spotify_url)
);
`)
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "Company", Fields: []SqlcField{
				{Name: "ID", Type: "int64"},
				{Name: "SpotifyUrl", Type: "string"},
			}},
			{Name: "User", Fields: []SqlcField{
				{Name: "ID", Type: "int64"},
				{Name: "DisplayLabel", Type: "string"},
				{Name: "CompanySpotifyUrl", Type: "string"},
			}},
		},
		Queries: []SqlcQuery{
			{Name: "InsertCompany", ReturnType: "Company", ArgName: "spotifyUrl", ArgType: "string"},
			{
				Name:       "InsertUser",
				ReturnType: "User",
				ParamType:  "InsertUserParams",
				ParamFields: []SqlcField{
					{Name: "DisplayLabel", Type: "string"},
					{Name: "CompanySpotifyUrl", Type: "string"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		`SpotifyUrl: "company-spotify_url"`,
		`DisplayLabel: "user-display_label"`,
		`LocalField: "CompanySpotifyUrl"`,
		`RefField: "SpotifyUrl"`,
		`InsertCompany(ctx, v.SpotifyUrl)`,
		`DisplayLabel:      v.DisplayLabel`,
		`CompanySpotifyUrl: v.CompanySpotifyUrl`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected generated output to contain %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, `LocalField: "CompanySpotifyURL"`) || strings.Contains(output, `RefField: "SpotifyURL"`) {
		t.Fatalf("generated output used schema-derived initialism spelling:\n%s", output)
	}
}

func TestMapSqlcModelFields_UsesOrdinalForRenames(t *testing.T) {
	// A renamed column shares its name with nothing, so position still binds it.
	table := Table{Columns: []Column{
		{Name: "primary_name"},
		{Name: "secondary_name"},
	}}
	model := &SqlcModel{Fields: []SqlcField{
		{Name: "Headline", Type: "string"},
		{Name: "Subtitle", Type: "string"},
	}}

	fields, err := mapSqlcModelFields(table, model)
	if err != nil {
		t.Fatal(err)
	}
	if fields["primary_name"].Name != "Headline" || fields["secondary_name"].Name != "Subtitle" {
		t.Fatalf("ordinal mapping = primary:%s secondary:%s", fields["primary_name"].Name, fields["secondary_name"].Name)
	}
}

func TestMapSqlcModelFields_RejectsColumnOrderDisagreement(t *testing.T) {
	// Both names are present but at swapped positions, so this schema and this
	// model disagree about the column order. Binding by position here would
	// generate hooks that read and delete by the wrong column.
	table := Table{Columns: []Column{
		{Name: "primary_name"},
		{Name: "secondary_name"},
	}}
	model := &SqlcModel{Fields: []SqlcField{
		{Name: "SecondaryName", Type: "string"},
		{Name: "PrimaryName", Type: "string"},
	}}

	_, err := mapSqlcModelFields(table, model)
	if err == nil {
		t.Fatal("expected an error for disagreeing column order")
	}
	if !strings.Contains(err.Error(), "regenerate sqlc") {
		t.Fatalf("error = %v, want it to mention regenerating sqlc", err)
	}
}

func TestGenerateSqlc_UsesActualMethodSignatures(t *testing.T) {
	tables := mustParseSchema(t, `
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    spotify_url TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
`)
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "AccountRecord", Fields: []SqlcField{
				{Name: "ID", Type: "int64"},
				{Name: "SpotifyUrl", Type: "string"},
				{Name: "CreatedAt", Type: "time.Time"},
			}},
		},
		Queries: []SqlcQuery{
			{
				Name:          "InsertUser",
				ReturnType:    "AccountRecord",
				ReturnPointer: true,
				DBArgument:    true,
				ParamType:     "InsertUserParams",
				ParamPointer:  true,
				ParamFields: []SqlcField{
					{Name: "SpotifyURL", Type: "string"},
					{Name: "RecordedAt", Type: "time.Time"},
				},
			},
		},
		DeleteQueries: []SqlcDeleteQuery{
			{
				Name:         "DeleteUser",
				DBArgument:   true,
				ParamType:    "DeleteUserParams",
				ParamPointer: true,
				ParamFields:  []SqlcField{{Name: "Identifier", Type: "int64"}},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{
		`seedling.Blueprint[db.AccountRecord]`,
		`return db.AccountRecord{`,
		`inserted, err := db.New().InsertUser(ctx, dbtx.(db.DBTX), &db.InsertUserParams{`,
		`SpotifyURL: v.SpotifyUrl`,
		`RecordedAt: v.CreatedAt`,
		`return *inserted, nil`,
		`DeleteUser(ctx, dbtx.(db.DBTX), &db.DeleteUserParams{`,
		`Identifier: v.ID`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected generated output to contain %q:\n%s", want, output)
		}
	}
}

func TestGenerateSqlc_RejectsAmbiguousParamsField(t *testing.T) {
	tables := mustParseSchema(t, `
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL
);
`)
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "User", Fields: []SqlcField{
				{Name: "ID", Type: "int64"},
				{Name: "Code", Type: "string"},
				{Name: "Name", Type: "string"},
			}},
		},
		Queries: []SqlcQuery{
			{
				Name:        "InsertUser",
				ReturnType:  "User",
				ParamType:   "InsertUserParams",
				ParamFields: []SqlcField{{Name: "Value", Type: "string"}},
			},
		},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), `cannot map params field "Value" (string) to exactly one model field`) {
		t.Fatalf("expected ambiguous params mapping error, got %v", err)
	}
}

func TestGenerateSqlc_RejectsUnmappableModelFields(t *testing.T) {
	tables := mustParseSchema(t, `
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    display_name TEXT NOT NULL
);
`)
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "User", Fields: []SqlcField{{Name: "ID", Type: "int64"}}},
		},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), "has 1 fields but schema table has 2 columns") {
		t.Fatalf("expected explicit field mapping error, got %v", err)
	}
}

func TestGenerateSqlc_RejectsMissingRenamedModel(t *testing.T) {
	tables := mustParseSchema(t, `CREATE TABLE users (id BIGINT PRIMARY KEY);`)
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "AccountRecord", Fields: []SqlcField{{Name: "ID", Type: "int64"}}},
		},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), `sqlc model "User" not found`) {
		t.Fatalf("expected explicit renamed model mapping error, got %v", err)
	}
}

func TestGenerateSqlc_RejectsQueryRowAsRenamedTableModel(t *testing.T) {
	tables := mustParseSchema(t, `CREATE TABLE users (id BIGINT PRIMARY KEY, name TEXT NOT NULL);`)
	info := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{{Name: "InsertUserRow", Fields: []SqlcField{
			{Name: "ID", Type: "int64"},
			{Name: "Name", Type: "string"},
		}}},
		Queries: []SqlcQuery{{
			Name:        "InsertUser",
			ReturnType:  "InsertUserRow",
			ParamType:   "InsertUserParams",
			ParamFields: []SqlcField{{Name: "Name", Type: "string"}},
		}},
	}

	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, info)
	if err == nil || !strings.Contains(err.Error(), `query "InsertUser" returns query-specific row type "InsertUserRow"`) {
		t.Fatalf("expected query-specific row rejection, got %v", err)
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

func TestParseSqlcDir_DeterministicModelOrdering(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeSqlcFiles(t, dir, map[string]string{
		"models.go": `package db

type Zebra struct {
	ID int64
}

type Apple struct {
	ID int64
}

type Mango struct {
	ID int64
}

type Banana struct {
	ID int64
}
`,
	})

	// Act
	var first []string
	for i := range 8 {
		info, err := ParseSqlcDir(dir)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		names := make([]string, len(info.Models))
		for j, m := range info.Models {
			names[j] = m.Name
		}
		if i == 0 {
			first = names
			continue
		}
		// Assert
		for j, name := range names {
			if first[j] != name {
				t.Fatalf("iteration %d: model order is not deterministic\nfirst: %v\ngot:   %v", i, first, names)
			}
		}
	}

	// Assert: explicit alphabetical order.
	want := []string{"Apple", "Banana", "Mango", "Zebra"}
	if len(first) != len(want) {
		t.Fatalf("expected %d models, got %d", len(want), len(first))
	}
	for i, name := range want {
		if first[i] != name {
			t.Fatalf("expected sorted model order %v, got %v", want, first)
		}
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
	tables := mustParseSchema(t, schema)

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
	tables := mustParseSchema(t, schema)

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

func TestGenerateSqlc_DefaultsAutofillSupportedFields(t *testing.T) {
	// Arrange
	schema := `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    company_id INTEGER NOT NULL REFERENCES companies(id)
);
`
	tables := mustParseSchema(t, schema)
	sqlcInfo := &SqlcInfo{
		Package: "db",
		Models: []SqlcModel{
			{Name: "Company", Fields: []SqlcField{{Name: "ID", Type: "int64"}, {Name: "Name", Type: "string"}}},
			{Name: "User", Fields: []SqlcField{{Name: "ID", Type: "int64"}, {Name: "Name", Type: "string"}, {Name: "CreatedAt", Type: "time.Time"}, {Name: "CompanyID", Type: "int64"}}},
		},
		Queries: []SqlcQuery{
			{Name: "InsertUser", ReturnType: "User", ParamType: "InsertUserParams", ParamFields: []SqlcField{{Name: "Name", Type: "string"}, {Name: "CreatedAt", Type: "time.Time"}, {Name: "CompanyID", Type: "int64"}}},
		},
	}

	// Act
	var buf bytes.Buffer
	if err := GenerateSqlc(&buf, "testutil", "github.com/myapp/internal/db", tables, sqlcInfo); err != nil {
		t.Fatalf("GenerateSqlc error: %v", err)
	}

	// Assert
	output := buf.String()
	tests := []struct {
		name    string
		substr  string
		missing bool
	}{
		{name: "time import", substr: `"time"`},
		{name: "string default", substr: `Name: "user-name"`},
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

func TestGenerateSqlc_CompositePK(t *testing.T) {
	schema := `
CREATE TABLE article_tags (
    article_id INTEGER NOT NULL,
    tag_id INTEGER NOT NULL,
    PRIMARY KEY (article_id, tag_id)
);
`
	tables := mustParseSchema(t, schema)
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
		"sqlc",
		"-pkg", "testutil",
		"--dir", sqlcDir,
		"--import-path", "github.com/myapp/internal/db",
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

func TestRun_SqlcRequiresImportPath(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	schemaPath := writeSchemaFile(t, `CREATE TABLE x (id INT);`)

	// Act
	exitCode := run([]string{
		"sqlc",
		"--dir", "/some/dir",
		schemaPath,
	}, &stdout, &stderr)

	// Assert
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--import-path is required") {
		t.Fatalf("expected import-path required error, got: %s", stderr.String())
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
		{"*int64", "*int64"},
		{"[4]byte", "[4]byte"},
		{"interface{}", "interface{}"},
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

func TestResolveSqlcParamFields_NameMatchesWinOverTypeFallback(t *testing.T) {
	// "Header" matches no model field and falls back to the only remaining
	// string. Resolving parameters in order would let it claim "Title" first and
	// then reject the later exact match as a duplicate.
	params := []SqlcField{
		{Name: "Header", Type: "string"},
		{Name: "Title", Type: "string"},
	}
	modelFields := []SqlcField{
		{Name: "Title", Type: "string"},
		{Name: "Subtitle", Type: "string"},
	}

	resolved, err := resolveSqlcParamFields(params, modelFields)
	if err != nil {
		t.Fatalf("resolveSqlcParamFields: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved = %d fields, want 2", len(resolved))
	}
	if resolved[1].paramName != "Title" || resolved[1].modelField != "Title" {
		t.Fatalf("Title mapped to %q, want the model field of the same name", resolved[1].modelField)
	}
	if resolved[0].modelField != "Subtitle" {
		t.Fatalf("Header mapped to %q, want the field left over after name matching", resolved[0].modelField)
	}
}
