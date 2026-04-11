package main

import (
	"strings"
	"testing"
)

func TestBuildDefaultLiteral_FillsSupportedScalarFields(t *testing.T) {
	// Arrange
	model := normalizedModel{
		ZeroValueExpr: "User{}",
		BlueprintID:   "user",
		Fields: []normalizedField{
			{GoName: "ID", GoType: "int64", IsPK: true},
			{GoName: "Name", GoType: "string"},
			{GoName: "Active", GoType: "bool"},
			{GoName: "Score", GoType: "float64"},
			{GoName: "Blob", GoType: "[]byte"},
			{GoName: "CreatedAt", GoType: "time.Time"},
			{GoName: "CompanyID", GoType: "int64", IsRelationFK: true},
			{GoName: "Token", GoType: "uuid.UUID"},
		},
	}

	// Act
	got := buildDefaultLiteral(model)

	// Assert
	tests := []struct {
		name    string
		substr  string
		missing bool
	}{
		{name: "string field", substr: `Name: "user-name"`},
		{name: "bool field", substr: `Active: true`},
		{name: "float field", substr: `Score: 1`},
		{name: "byte slice field", substr: `Blob: []byte("user-blob")`},
		{name: "time field", substr: `CreatedAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)`},
		{name: "primary key skipped", substr: `ID:`, missing: true},
		{name: "relation foreign key skipped", substr: `CompanyID:`, missing: true},
		{name: "unsupported type skipped", substr: `Token:`, missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contains := strings.Contains(got, tt.substr)
			if tt.missing && contains {
				t.Fatalf("got %q, expected substring %q to be absent", got, tt.substr)
			}
			if !tt.missing && !contains {
				t.Fatalf("got %q, expected substring %q", got, tt.substr)
			}
		})
	}
}

func TestBuildDefaultLiteral_FallsBackToZeroValueWhenNothingIsFillable(t *testing.T) {
	// Arrange
	model := normalizedModel{
		ZeroValueExpr: "&ent.Company{}",
		BlueprintID:   "company",
		Fields: []normalizedField{
			{GoName: "ID", GoType: "int64", IsPK: true},
			{GoName: "Token", GoType: "uuid.UUID"},
		},
	}

	// Act
	got := buildDefaultLiteral(model)

	// Assert
	if got != "&ent.Company{}" {
		t.Fatalf("got %q, want %q", got, "&ent.Company{}")
	}
}

func TestNormalizedModelsNeedTimeImport(t *testing.T) {
	tests := []struct {
		name   string
		models []normalizedModel
		want   bool
	}{
		{
			name: "without time fields",
			models: []normalizedModel{
				{Fields: []normalizedField{{GoName: "Name", GoType: "string"}}},
			},
			want: false,
		},
		{
			name: "with time field",
			models: []normalizedModel{
				{Fields: []normalizedField{{GoName: "CreatedAt", GoType: "time.Time"}}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := normalizedModelsNeedTimeImport(tt.models)

			// Assert
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
