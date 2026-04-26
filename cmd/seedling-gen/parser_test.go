package main

import (
	"strings"
	"testing"
)

func TestParseSchema_DoubleQuoteEscape(t *testing.T) {
	// Arrange: ANSI quoted identifier where the actual name contains a double quote.
	sql := `CREATE TABLE "weird""name" ("id""col" INT PRIMARY KEY);`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if got, want := tables[0].Name, `weird"name`; got != want {
		t.Fatalf("table name: got %q, want %q", got, want)
	}
	if len(tables[0].Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(tables[0].Columns))
	}
	if got, want := tables[0].Columns[0].Name, `id"col`; got != want {
		t.Fatalf("column name: got %q, want %q", got, want)
	}
	if !tables[0].Columns[0].IsPK {
		t.Fatal("column should be PK")
	}
}

func TestParseSchema_BacktickEscape(t *testing.T) {
	// Arrange: MySQL backtick identifier with embedded backtick.
	sql := "CREATE TABLE `weird``name` (`id``col` INT PRIMARY KEY);"

	// Act
	tables, err := ParseSchemaWithDialect(sql, "mysql")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if got, want := tables[0].Name, "weird`name"; got != want {
		t.Fatalf("table name: got %q, want %q", got, want)
	}
	if got, want := tables[0].Columns[0].Name, "id`col"; got != want {
		t.Fatalf("column name: got %q, want %q", got, want)
	}
}

func TestParseSchema_SingleQuoteEscapeInDefault(t *testing.T) {
	// Arrange: doubled single quote inside a string literal must not terminate it.
	sql := `CREATE TABLE quoted (note TEXT DEFAULT 'it''s ok', id INT PRIMARY KEY);`

	// Act
	tables, err := ParseSchema(sql)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(tables[0].Columns))
	}
}

func TestParseSchema_DottedQualifiedTable(t *testing.T) {
	// Arrange
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "unquoted schema.table", sql: `CREATE TABLE public.users (id INT PRIMARY KEY);`, want: "users"},
		{name: "quoted schema.table", sql: `CREATE TABLE "public"."users" ("id" INT PRIMARY KEY);`, want: "users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			tables, err := ParseSchemaWithDialect(tt.sql, "postgres")
			if err != nil {
				t.Fatal(err)
			}

			// Assert
			if len(tables) != 1 {
				t.Fatalf("expected 1 table, got %d", len(tables))
			}
			if tables[0].Name != tt.want {
				t.Fatalf("table name: got %q, want %q", tables[0].Name, tt.want)
			}
		})
	}
}

func TestParseSchema_ReservedWordTable(t *testing.T) {
	// Arrange: reserved keywords used as identifiers must round-trip when quoted.
	sql := `CREATE TABLE "select" ("from" INT PRIMARY KEY, "where" TEXT);`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "select" {
		t.Fatalf("table name: got %q, want %q", tables[0].Name, "select")
	}
	if len(tables[0].Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(tables[0].Columns))
	}
	if tables[0].Columns[0].Name != "from" || tables[0].Columns[1].Name != "where" {
		t.Fatalf("column names: got %q,%q", tables[0].Columns[0].Name, tables[0].Columns[1].Name)
	}
}

func TestParseSchema_CommentLikeInsideQuotedIdentifier(t *testing.T) {
	// Arrange: -- inside a quoted identifier must not be stripped as a comment.
	sql := "CREATE TABLE users (\n  id INT,\n  \"col--name\" TEXT,\n  primary_key INT PRIMARY KEY\n);"

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	var found bool
	for _, col := range tables[0].Columns {
		if col.Name == "col--name" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, len(tables[0].Columns))
		for i, c := range tables[0].Columns {
			names[i] = c.Name
		}
		t.Fatalf("expected column %q, got %v", "col--name", names)
	}
}

func TestParseSchema_MultiLineCommentsAcrossBody(t *testing.T) {
	// Arrange
	sql := strings.TrimSpace(`
-- header comment
CREATE TABLE users (
  id INT, -- inline
  /* multi
     line */ name TEXT,
  email TEXT NOT NULL
);
`)

	// Act
	tables, err := ParseSchema(sql)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].Columns) != 3 {
		names := make([]string, len(tables[0].Columns))
		for i, c := range tables[0].Columns {
			names[i] = c.Name
		}
		t.Fatalf("expected 3 columns, got %d (%v)", len(tables[0].Columns), names)
	}
}

func TestStripSQLComments_PreservesCommentInsideQuotes(t *testing.T) {
	// Arrange & Act & Assert
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "single-quoted -- preserved", in: `'a -- b'`, want: `'a -- b'`},
		{name: "double-quoted -- preserved", in: `"a -- b"`, want: `"a -- b"`},
		{name: "backtick -- preserved", in: "`a -- b`", want: "`a -- b`"},
		{name: "outside quotes -- stripped", in: "a -- b\nc", want: "a \nc"},
		{name: "block comment outside quotes stripped", in: "a /* x */ b", want: "a   b"},
		{name: "block comment inside quotes preserved", in: `'a /* x */ b'`, want: `'a /* x */ b'`},
		{name: "doubled single quote escape", in: `'it''s --ok'`, want: `'it''s --ok'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripSQLComments(tt.in)
			if got != tt.want {
				t.Fatalf("stripSQLComments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTrimIdentifierQuotes_UnescapesDoubledQuotes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "double-quoted plain", in: `"foo"`, want: "foo"},
		{name: "double-quoted with escape", in: `"foo""bar"`, want: `foo"bar`},
		{name: "backtick plain", in: "`foo`", want: "foo"},
		{name: "backtick with escape", in: "`foo``bar`", want: "foo`bar"},
		{name: "bracket plain", in: "[foo]", want: "foo"},
		{name: "no quotes", in: "foo", want: "foo"},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimIdentifierQuotes(tt.in)
			if got != tt.want {
				t.Fatalf("trimIdentifierQuotes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
