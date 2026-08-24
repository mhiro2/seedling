package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseEntSchemaDir_BasicSchema(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "user.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/edge"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").StructTag("json:\"display_name,omitempty\""),
		field.Int("age").Optional().Nillable(),
		field.String("email").Sensitive(),
		field.Int("company_uuid"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).Ref("users").Unique().Required().Field("company_uuid"),
	}
}
`)
	writeFile(t, dir, "company.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/edge"
)

type Company struct {
	ent.Schema
}

func (Company) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (Company) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
	}
}
`)

	// Act
	schemas, err := ParseEntSchemaDir(dir)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) < 2 {
		t.Fatalf("expected at least 2 schemas, got %d", len(schemas))
	}

	var user, company *EntSchema
	for i, s := range schemas {
		switch s.Name {
		case "User":
			user = &schemas[i]
		case "Company":
			company = &schemas[i]
		}
	}

	if user == nil {
		t.Fatal("User schema not found")
	}
	if len(user.Fields) != 4 {
		t.Fatalf("expected 4 fields on User, got %d", len(user.Fields))
	}
	if user.Fields[0].Name != "name" {
		t.Fatalf("expected first field 'name', got %q", user.Fields[0].Name)
	}
	if user.Fields[0].JSONName != "display_name" || user.Fields[2].JSONName != "-" {
		t.Fatalf("unexpected parsed JSON names: name=%q email=%q", user.Fields[0].JSONName, user.Fields[2].JSONName)
	}
	if !user.Fields[1].Optional {
		t.Fatal("expected age field to be optional")
	}
	if !user.Fields[1].Nillable || user.Fields[1].GoType != "*int" {
		t.Fatalf("expected age to be nillable *int, got %+v", user.Fields[1])
	}

	// Check edges.
	if len(user.Edges) != 1 {
		t.Fatalf("expected 1 edge on User, got %d", len(user.Edges))
	}
	edge := user.Edges[0]
	if edge.Direction != "From" {
		t.Fatalf("expected From direction, got %q", edge.Direction)
	}
	if edge.Type != "Company" {
		t.Fatalf("expected edge type Company, got %q", edge.Type)
	}
	if !edge.Unique {
		t.Fatal("expected edge to be unique")
	}
	if !edge.Required {
		t.Fatal("expected edge to be required")
	}
	if edge.Ref != "users" {
		t.Fatalf("expected ref 'users', got %q", edge.Ref)
	}
	if edge.Field != "company_uuid" {
		t.Fatalf("expected edge field %q, got %q", "company_uuid", edge.Field)
	}

	if company == nil {
		t.Fatal("Company schema not found")
	}
	if len(company.Fields) != 1 {
		t.Fatalf("expected 1 field on Company, got %d", len(company.Fields))
	}
}

func TestParseEntSchemaDir_PreservesDefaultJSONNameAndGoTypeOption(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "user.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("display_name").StructTag("db:\"display_name\"").GoType(CustomString("")),
	}
}
`)

	schemas, err := ParseEntSchemaDir(dir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}
	field := schemas[0].Fields[0]
	if field.JSONName != "display_name" || !field.CustomGoType {
		t.Fatalf("parsed field = %+v, want default JSON name and custom Go type", field)
	}
}

func TestParseEntSchemaDir_IncludesMixinOnlyAndEmptySchemas(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "schemas.go", `package schema

import "entgo.io/ent"

type Empty struct {
	ent.Schema
}

type MixinOnly struct {
	ent.Schema
}

func (MixinOnly) Mixin() []ent.Mixin {
	return nil
}
`)

	schemas, err := ParseEntSchemaDir(dir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}
	if len(schemas) != 2 || schemas[0].Name != "Empty" || schemas[1].Name != "MixinOnly" {
		t.Fatalf("schemas = %+v, want Empty and MixinOnly", schemas)
	}
}

func TestParseEntSchemaDir_EmptyDir(t *testing.T) {
	// Act & Assert
	dir := t.TempDir()
	_, err := ParseEntSchemaDir(dir)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestParseEntSchemaDir_RejectsUnexposedSingularEdge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "user.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

type User struct {
	ent.Schema
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("profile", Profile.Type).Unique(),
	}
}
`)

	_, err := ParseEntSchemaDir(dir)
	if err == nil || !strings.Contains(err.Error(), "requires .Field(...)") {
		t.Fatalf("expected exposed foreign-key error, got %v", err)
	}
}

func TestParseEntSchemaDir_AcceptsSingularEdgeExposedByCounterpart(t *testing.T) {
	// Ent only allows .Field(...) on the schema holding the FK column, so the
	// owning side of a one-to-one cannot expose it itself.
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
	return []ent.Field{field.String("name")}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("card", Card.Type).Unique(),
	}
}

type Card struct {
	ent.Schema
}

func (Card) Fields() []ent.Field {
	return []ent.Field{field.Int("user_id")}
}

func (Card) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("card").Unique().Required().Field("user_id"),
	}
}
`)

	schemas, err := ParseEntSchemaDir(dir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}
	if len(schemas) != 2 {
		t.Fatalf("got %d schemas, want 2", len(schemas))
	}
}

func TestGenerateEnt_RequiresExposedForeignKeyForSingularEdges(t *testing.T) {
	tests := []struct {
		name string
		edge EntEdge
	}{
		{name: "inverse unique", edge: EntEdge{Name: "company", Type: "Company", Direction: "From", Unique: true}},
		{name: "inverse required", edge: EntEdge{Name: "company", Type: "Company", Direction: "From", Required: true}},
		{name: "owning unique", edge: EntEdge{Name: "profile", Type: "Profile", Direction: "To", Unique: true}},
		{name: "owning required", edge: EntEdge{Name: "profile", Type: "Profile", Direction: "To", Required: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemas := []EntSchema{{Name: "User", Edges: []EntEdge{tt.edge}}}

			var buf bytes.Buffer
			err := GenerateEnt(&buf, "testutil", "github.com/myapp/ent", schemas)
			if err == nil || !strings.Contains(err.Error(), "requires .Field(...)") {
				t.Fatalf("expected exposed foreign-key error, got %v", err)
			}
		})
	}
}

func TestEntTypeToGoType(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"String", "string"},
		{"Int", "int"},
		{"Int64", "int64"},
		{"Bool", "bool"},
		{"Time", "time.Time"},
		{"Float64", "float64"},
		{"UUID", "uuid.UUID"},
		{"Bytes", "[]byte"},
		{"Enum", "string"},
	}
	for _, tt := range tests {
		got := entTypeToGoType(tt.input)
		if got != tt.want {
			t.Errorf("entTypeToGoType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerateEnt_BasicOutput(t *testing.T) {
	// Arrange
	schemas := []EntSchema{
		{
			Name: "Company",
			Fields: []EntField{
				{Name: "name", GoName: "Name", Type: "String", GoType: "string"},
			},
		},
		{
			Name: "User",
			Fields: []EntField{
				{Name: "name", GoName: "Name", Type: "String", GoType: "string"},
				{Name: "age", GoName: "Age", Type: "Int", GoType: "*int", Optional: true, Nillable: true},
				{Name: "company_uuid", GoName: "CompanyUUID", Type: "Int", GoType: "int"},
			},
			Edges: []EntEdge{
				{Name: "company", Type: "Company", Direction: "From", Ref: "users", Field: "company_uuid", Unique: true, Required: true},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := GenerateEnt(&buf, "testutil", "github.com/myapp/ent", schemas)
	// Assert
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	checks := []struct {
		name   string
		substr string
	}{
		{"package", "package testutil"},
		{"seedling import", `"github.com/mhiro2/seedling"`},
		{"ent import", `"github.com/myapp/ent"`},
		{"company pointer type", "*ent.Company"},
		{"user pointer type", "*ent.User"},
		{"insert builder", ".Create()"},
		{"save call", ".Save(ctx)"},
		{"delete call", ".DeleteOneID(v.ID)"},
		{"belongs to from edge", "seedling.BelongsTo"},
		{"explicit edge field", `LocalField: "CompanyUUID"`},
		{"acronym-aware field setter", "SetCompanyUUID(v.CompanyUUID)"},
		{"nillable setter", "SetNillableAge(v.Age)"},
		{"ref blueprint", `RefBlueprint: "company"`},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(output, check.substr) {
				t.Fatalf("expected output to contain %q\n\nGot:\n%s", check.substr, output)
			}
		})
	}
}

func TestGenerateEnt_SkipsZeroOptionalNonNillableForeignKey(t *testing.T) {
	schemas := []EntSchema{
		{
			Name: "Company",
			Fields: []EntField{
				{Name: "name", GoName: "Name", Type: "String", GoType: "string"},
			},
		},
		{
			Name: "User",
			Fields: []EntField{
				{Name: "company_id", GoName: "CompanyID", Type: "Int", GoType: "int", Optional: true},
			},
			Edges: []EntEdge{
				{Name: "company", Type: "Company", Direction: "From", Field: "company_id", Unique: true},
			},
		},
	}

	var output bytes.Buffer
	if err := GenerateEnt(&output, "testutil", "github.com/myapp/ent", schemas); err != nil {
		t.Fatalf("GenerateEnt: %v", err)
	}
	generated := output.String()
	if !strings.Contains(generated, `"reflect"`) {
		t.Fatalf("generated output does not import reflect:\n%s", generated)
	}
	if !strings.Contains(generated, `if !reflect.ValueOf(v.CompanyID).IsZero() {`) {
		t.Fatalf("generated output does not guard the optional FK setter:\n%s", generated)
	}
}

func TestGenerateEnt_PointerForeignKeyUsesNilGuardWithoutReflect(t *testing.T) {
	schemas := []EntSchema{
		{
			Name: "Company",
			Fields: []EntField{
				{Name: "name", GoName: "Name", Type: "String", GoType: "string"},
			},
		},
		{
			Name: "User",
			Fields: []EntField{
				{
					Name:         "company_id",
					GoName:       "CompanyID",
					Type:         "Int",
					GoType:       "*mytypes.CompanyID",
					Optional:     true,
					CustomGoType: true,
					SetterName:   "SetCompanyID",
					SetterDeref:  true,
				},
			},
			Edges: []EntEdge{
				{Name: "company", Type: "Company", Direction: "From", Field: "company_id", Unique: true},
			},
		},
	}

	var output bytes.Buffer
	if err := GenerateEnt(&output, "testutil", "github.com/myapp/ent", schemas); err != nil {
		t.Fatalf("GenerateEnt: %v", err)
	}
	generated := output.String()
	if strings.Contains(generated, `"reflect"`) {
		t.Fatalf("generated output imports reflect for a nil-guarded FK setter:\n%s", generated)
	}
	if !strings.Contains(generated, `if v.CompanyID != nil {`) {
		t.Fatalf("generated output does not nil-guard the pointer FK setter:\n%s", generated)
	}
}

func TestRun_EntRequiresImportPath(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"ent", "--dir", "/some/dir"}, &stdout, &stderr)

	// Assert
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--import-path is required") {
		t.Fatalf("expected import-path required error, got: %s", stderr.String())
	}
}

func TestGenerateEnt_SkipsDefaultForExplicitIDField(t *testing.T) {
	// A schema that declares its own id still gets its ID from Ent or the
	// database, so generated Defaults must leave it alone.
	tests := []struct {
		name  string
		field EntField
	}{
		{name: "int id", field: EntField{Name: "id", GoName: "ID", Type: "Int", GoType: "int", DefaultType: "int"}},
		{name: "string id", field: EntField{Name: "id", GoName: "ID", Type: "String", GoType: "string", DefaultType: "string"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			schemas := []EntSchema{{
				Name: "Item",
				Fields: []EntField{
					tt.field,
					{Name: "label", GoName: "Label", Type: "String", GoType: "string", DefaultType: "string"},
				},
			}}

			// Act
			var buf bytes.Buffer
			if err := GenerateEnt(&buf, "testutil", "github.com/myapp/ent", schemas); err != nil {
				t.Fatal(err)
			}

			// Assert
			output := buf.String()
			if strings.Contains(output, "ID:") {
				t.Fatalf("expected no default for the explicit id field\n\nGot:\n%s", output)
			}
			if !strings.Contains(output, `Label: "item-label"`) {
				t.Fatalf("expected other fields to keep their defaults\n\nGot:\n%s", output)
			}
		})
	}
}

func TestGenerateEnt_DefaultsAutofillSupportedFields(t *testing.T) {
	// Arrange
	schemas := []EntSchema{
		{
			Name: "Company",
			Fields: []EntField{
				{Name: "name", GoName: "Name", Type: "String", GoType: "string"},
			},
		},
		{
			Name: "User",
			Fields: []EntField{
				{Name: "name", GoName: "Name", Type: "String", GoType: "string"},
				{Name: "created_at", GoName: "CreatedAt", Type: "Time", GoType: "time.Time"},
				{Name: "token", GoName: "Token", Type: "UUID", GoType: "uuid.UUID"},
				{Name: "company_id", GoName: "CompanyID", Type: "Int", GoType: "int"},
			},
			Edges: []EntEdge{
				{Name: "company", Type: "Company", Direction: "From", Ref: "users", Field: "company_id", Unique: true, Required: true},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	if err := GenerateEnt(&buf, "testutil", "github.com/myapp/ent", schemas); err != nil {
		t.Fatal(err)
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
		{name: "unsupported uuid skipped", substr: `Token:`, missing: true},
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
