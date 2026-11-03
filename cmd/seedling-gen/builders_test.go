package main

import (
	"slices"
	"strings"
	"testing"
)

func TestSetterTypeImports(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goType  string
		ok      bool
		imports []string
	}{
		{goType: "string", ok: true},
		{goType: "*int64", ok: true},
		{goType: "[]byte", ok: true},
		{goType: "map[string]int", ok: true},
		{goType: "any", ok: false},
		{goType: "[4]uint8", ok: true},
		{goType: "time.Time", ok: true, imports: []string{`"time"`}},
		{goType: "*time.Time", ok: true, imports: []string{`"time"`}},
		{goType: "sql.NullString", ok: true, imports: []string{`"database/sql"`}},
		{goType: "json.RawMessage", ok: true, imports: []string{`"encoding/json"`}},
		{goType: "pgtype.Text", ok: false},
		{goType: "uuid.UUID", ok: false},
		{goType: "Status", ok: false},
		{goType: "[]Status", ok: false},
		{goType: "func()", ok: false},
		{goType: "struct{}", ok: false},
		{goType: "chan int", ok: false},
		{goType: "not a type(", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			t.Parallel()

			imports, ok := setterTypeImports(tt.goType)
			if ok != tt.ok {
				t.Fatalf("got ok=%v, want %v", ok, tt.ok)
			}
			if ok && !slices.Equal(imports, tt.imports) {
				t.Fatalf("got imports %v, want %v", imports, tt.imports)
			}
		})
	}
}

func TestExportedName(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"company":        "Company",
		"assignee_tasks": "AssigneeTasks",
		"ownedPets":      "OwnedPets",
		"api_key":        "APIKey",
		"owner__id":      "OwnerID",
	} {
		if got := exportedName(name); got != want {
			t.Errorf("exportedName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestBuildBuilderModels_ResolvesNameCollisions(t *testing.T) {
	t.Parallel()

	// Arrange: a field named like the relation method, a field named like a
	// fixed builder method, a relation FK, and a relation to an unknown model.
	models := []normalizedModel{
		{FieldName: "Company", TypeExpr: "Company", BlueprintID: "company"},
		{
			FieldName: "User", TypeExpr: "*ent.User", BlueprintID: "user",
			Fields: []normalizedField{
				{GoName: "Company", GoType: "string"},
				{GoName: "Insert", GoType: "string"},
				{GoName: "CompanyID", GoType: "int", IsRelationFK: true},
				{GoName: "Status", GoType: "user.Status"},
				{GoName: "Name", GoType: "string"},
			},
			Relations: []normalizedRelation{
				{Name: "company", RefBlueprint: "company"},
				{Name: "team", RefBlueprint: "external"},
			},
		},
	}

	// Act
	builders, imports := buildBuilderModels(withReverseRelations(models), true)

	// Assert
	if len(imports) != 0 {
		t.Fatalf("got imports %v, want none", imports)
	}
	user := builders[1]
	if user.ElemExpr != "ent.User" {
		t.Fatalf("got ElemExpr %q, want %q", user.ElemExpr, "ent.User")
	}
	if len(user.BelongsTo) != 1 || user.BelongsTo[0].Method != "Company" || user.BelongsTo[0].TargetType != "Company" {
		t.Fatalf("got belongs-to methods %+v, want only Company", user.BelongsTo)
	}
	setters := make([]string, 0, len(user.Setters))
	for _, setter := range user.Setters {
		setters = append(setters, setter.Method)
	}
	if !slices.Equal(setters, []string{"Name"}) {
		t.Fatalf("got setters %v, want only Name", setters)
	}
	company := builders[0]
	if len(company.HasMany) != 1 || company.HasMany[0].Method != "Users" || company.HasMany[0].TargetBuilder != "UserBuilder" {
		t.Fatalf("got has-many methods %+v, want Users", company.HasMany)
	}
}

func TestValidateBuilderNames(t *testing.T) {
	t.Parallel()

	models := []normalizedModel{
		{FieldName: "User", StructName: "User"},
		{FieldName: "UserBuilder", StructName: "UserBuilder"},
	}
	err := validateBuilderNames(models, true)
	if err == nil || !strings.Contains(err.Error(), `builder type "UserBuilder" collides`) {
		t.Fatalf("got %v, want a builder name collision", err)
	}
	if err := validateBuilderNames(models[:1], true); err != nil {
		t.Fatalf("got %v, want no error", err)
	}
}

func TestBuildBuilderModels_FieldTypes(t *testing.T) {
	t.Parallel()

	// Arrange: a direct CreatedAt shadows the one promoted from gorm.Model.
	embedded := &embeddedStruct{FieldName: "Model", TypeExpr: "gorm.Model"}
	models := []normalizedModel{{
		FieldName: "Item", TypeExpr: "models.Item", BlueprintID: "item",
		Fields: []normalizedField{
			{GoName: "CreatedAt", GoType: "time.Time", Embedded: embedded},
			{GoName: "UpdatedAt", GoType: "time.Time", Embedded: embedded},
			{GoName: "CreatedAt", GoType: "int64"},
			{GoName: "Metadata", GoType: "any"},
		},
	}}

	tests := []struct {
		name           string
		qualifiedTypes bool
		want           []string
	}{
		{name: "qualified types allowed", qualifiedTypes: true, want: []string{"UpdatedAt time.Time", "CreatedAt int64"}},
		{name: "qualified types rejected", qualifiedTypes: false, want: []string{"CreatedAt int64"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			builders, _ := buildBuilderModels(models, tt.qualifiedTypes)

			// Assert
			got := make([]string, 0, len(builders[0].Setters))
			for _, setter := range builders[0].Setters {
				got = append(got, setter.Method+" "+setter.Type)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got setters %v, want %v", got, tt.want)
			}
		})
	}
}
