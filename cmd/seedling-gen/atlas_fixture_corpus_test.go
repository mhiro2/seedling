package main

import (
	"strings"
	"testing"
)

func TestParseAtlasHCL_RealSchemaCorpus(t *testing.T) {
	// Arrange
	tests := []struct {
		name    string
		fixture string
		assert  func(t *testing.T, tables []Table)
	}{
		{
			name:    "service schema",
			fixture: "atlas/pass/service_schema.hcl",
			assert: func(t *testing.T, tables []Table) {
				t.Helper()

				if len(tables) != 2 {
					t.Fatalf("expected 2 tables, got %d", len(tables))
				}

				users := findTableByName(t, tables, "users")
				if len(users.Columns) != 5 {
					t.Fatalf("expected 5 user columns, got %d", len(users.Columns))
				}

				companyFK := findForeignKeyByColumns(t, users, "company_id")
				if companyFK.RefTable != "companies" {
					t.Fatalf("company FK ref = %q, want %q", companyFK.RefTable, "companies")
				}
				if !companyFK.NotNull {
					t.Fatal("expected company FK to be required")
				}

				managerFK := findForeignKeyByColumns(t, users, "manager_id")
				if managerFK.RefTable != "users" {
					t.Fatalf("manager FK ref = %q, want %q", managerFK.RefTable, "users")
				}
				if managerFK.NotNull {
					t.Fatal("expected manager FK to be optional")
				}

				createdAt := findColumnByName(t, users, "created_at")
				if createdAt.GoType != "time.Time" {
					t.Fatalf("created_at GoType = %q, want %q", createdAt.GoType, "time.Time")
				}
			},
		},
		{
			name:    "deployment schema",
			fixture: "atlas/pass/deployment_schema.hcl",
			assert: func(t *testing.T, tables []Table) {
				t.Helper()

				if len(tables) != 2 {
					t.Fatalf("expected 2 tables, got %d", len(tables))
				}

				deployments := findTableByName(t, tables, "deployments")
				if len(deployments.Columns) != 4 {
					t.Fatalf("expected 4 deployment columns, got %d", len(deployments.Columns))
				}
				if len(deployments.ForeignKeys) != 1 {
					t.Fatalf("expected 1 deployment FK, got %d", len(deployments.ForeignKeys))
				}

				regionFK := findForeignKeyByColumns(t, deployments, "region_country_code", "region_code")
				if regionFK.RefTable != "regions" {
					t.Fatalf("region FK ref = %q, want %q", regionFK.RefTable, "regions")
				}
				if !regionFK.NotNull {
					t.Fatal("expected region FK to be required")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			data := readSeedlingGenFixture(t, tt.fixture)

			// Act
			tables, err := ParseAtlasHCL(data)
			// Assert
			if err != nil {
				t.Fatalf("parse atlas fixture %q: %v", tt.fixture, err)
			}
			tt.assert(t, tables)
		})
	}
}

func TestParseAtlasHCL_IntentionalFailureCorpus(t *testing.T) {
	// Arrange
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			name:    "unclosed table block",
			fixture: "atlas/fail/unclosed_table.hcl",
			want:    "unclosed brace",
		},
		{
			name:    "unclosed column block",
			fixture: "atlas/fail/unclosed_column.hcl",
			want:    "unclosed brace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			data := readSeedlingGenFixture(t, tt.fixture)

			// Act
			_, err := ParseAtlasHCL(data)

			// Assert
			if err == nil {
				t.Fatal("expected fixture parsing to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}
