package main

import (
	"strings"
	"testing"
)

func TestParseSchema_RealSchemaCorpus(t *testing.T) {
	// Arrange
	tests := []struct {
		name    string
		fixture string
		dialect string
		assert  func(t *testing.T, tables []Table)
	}{
		{
			name:    "postgres accounts and users",
			fixture: "sql/pass/postgres_accounts_users.sql",
			dialect: "postgres",
			assert: func(t *testing.T, tables []Table) {
				t.Helper()

				if len(tables) != 2 {
					t.Fatalf("expected 2 tables, got %d", len(tables))
				}

				companies := findTableByName(t, tables, "companies")
				if len(companies.Columns) != 4 {
					t.Fatalf("expected 4 company columns, got %d", len(companies.Columns))
				}

				users := findTableByName(t, tables, "users")
				if len(users.Columns) != 7 {
					t.Fatalf("expected 7 user columns, got %d", len(users.Columns))
				}

				status := findColumnByName(t, users, "status")
				if status.GoType != "string" {
					t.Fatalf("status GoType = %q, want %q", status.GoType, "string")
				}
				if !status.NotNull {
					t.Fatal("expected status to be NOT NULL")
				}

				searchName := findColumnByName(t, users, "search_name")
				if searchName.GoType != "string" {
					t.Fatalf("search_name GoType = %q, want %q", searchName.GoType, "string")
				}

				createdAt := findColumnByName(t, users, "created_at")
				if createdAt.GoType != "time.Time" {
					t.Fatalf("created_at GoType = %q, want %q", createdAt.GoType, "time.Time")
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
			},
		},
		{
			name:    "mysql orders",
			fixture: "sql/pass/mysql_orders.sql",
			dialect: "mysql",
			assert: func(t *testing.T, tables []Table) {
				t.Helper()

				if len(tables) != 2 {
					t.Fatalf("expected 2 tables, got %d", len(tables))
				}

				orders := findTableByName(t, tables, "orders")
				if len(orders.Columns) != 6 {
					t.Fatalf("expected 6 order columns, got %d", len(orders.Columns))
				}

				grossTotal := findColumnByName(t, orders, "gross_total")
				if grossTotal.GoType != "float64" {
					t.Fatalf("gross_total GoType = %q, want %q", grossTotal.GoType, "float64")
				}

				accountID := findColumnByName(t, orders, "account_id")
				if !accountID.IsFK {
					t.Fatal("expected account_id to be marked as FK")
				}

				accountFK := findForeignKeyByColumns(t, orders, "account_id")
				if accountFK.RefTable != "accounts" {
					t.Fatalf("account FK ref = %q, want %q", accountFK.RefTable, "accounts")
				}
				if !accountFK.NotNull {
					t.Fatal("expected account FK to be required")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			schema := readSeedlingGenFixture(t, tt.fixture)

			// Act
			tables, err := ParseSchemaWithDialect(schema, tt.dialect)
			// Assert
			if err != nil {
				t.Fatalf("parse schema fixture %q: %v", tt.fixture, err)
			}
			tt.assert(t, tables)
		})
	}
}

func TestParseSchema_IntentionalFailureCorpus(t *testing.T) {
	// Arrange
	tests := []struct {
		name    string
		fixture string
		dialect string
		want    string
	}{
		{
			name:    "postgres truncated create table",
			fixture: "sql/fail/postgres_truncated_users.sql",
			dialect: "postgres",
			want:    "unclosed parenthesis",
		},
		{
			name:    "mysql truncated second table",
			fixture: "sql/fail/mysql_truncated_orders.sql",
			dialect: "mysql",
			want:    "unclosed parenthesis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			schema := readSeedlingGenFixture(t, tt.fixture)

			// Act
			_, err := ParseSchemaWithDialect(schema, tt.dialect)

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
