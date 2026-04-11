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
		field.String("name"),
		field.Int("age").Optional(),
		field.String("email"),
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
	if len(user.Fields) != 3 {
		t.Fatalf("expected 3 fields on User, got %d", len(user.Fields))
	}
	if user.Fields[0].Name != "name" {
		t.Fatalf("expected first field 'name', got %q", user.Fields[0].Name)
	}
	if !user.Fields[1].Optional {
		t.Fatal("expected age field to be optional")
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

	if company == nil {
		t.Fatal("Company schema not found")
	}
	if len(company.Fields) != 1 {
		t.Fatalf("expected 1 field on Company, got %d", len(company.Fields))
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
				{Name: "name", Type: "String", GoType: "string"},
			},
		},
		{
			Name: "User",
			Fields: []EntField{
				{Name: "name", Type: "String", GoType: "string"},
				{Name: "age", Type: "Int", GoType: "int", Optional: true},
			},
			Edges: []EntEdge{
				{Name: "company", Type: "Company", Direction: "From", Ref: "users", Unique: true, Required: true},
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

func TestGenerateEnt_DefaultsAutofillSupportedFields(t *testing.T) {
	// Arrange
	schemas := []EntSchema{
		{
			Name: "Company",
			Fields: []EntField{
				{Name: "name", Type: "String", GoType: "string"},
			},
		},
		{
			Name: "User",
			Fields: []EntField{
				{Name: "name", Type: "String", GoType: "string"},
				{Name: "created_at", Type: "Time", GoType: "time.Time"},
				{Name: "token", Type: "UUID", GoType: "uuid.UUID"},
			},
			Edges: []EntEdge{
				{Name: "company", Type: "Company", Direction: "From", Ref: "users", Unique: true, Required: true},
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
