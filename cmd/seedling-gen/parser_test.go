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

func columnByName(t *testing.T, table Table, name string) Column {
	t.Helper()
	for _, c := range table.Columns {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("column %q not found", name)
	return Column{}
}

func TestParseSchema_StringLiteralDefaultDoesNotTriggerConstraints(t *testing.T) {
	// Arrange: SQL keywords sitting inside DEFAULT string literals must not be
	// mistaken for real constraints.
	sql := `CREATE TABLE notes (
		id INT PRIMARY KEY,
		body TEXT DEFAULT 'this is NOT NULL really',
		tag TEXT DEFAULT 'see REFERENCES manual',
		label TEXT DEFAULT 'PRIMARY KEY note'
	);`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if body := columnByName(t, tables[0], "body"); body.NotNull {
		t.Error("body must not be NOT NULL from a string literal")
	}
	if tag := columnByName(t, tables[0], "tag"); tag.IsFK {
		t.Errorf("tag must not be a FK from a string literal, got ref %q", tag.FKRefTable)
	}
	if label := columnByName(t, tables[0], "label"); label.IsPK {
		t.Error("label must not be PK from a string literal")
	}
	if id := columnByName(t, tables[0], "id"); !id.IsPK {
		t.Error("id should remain a real PK")
	}
	if len(tables[0].ForeignKeys) != 0 {
		t.Fatalf("expected no foreign keys, got %d", len(tables[0].ForeignKeys))
	}
}

func TestParseSchema_RealConstraintsCoexistWithStringLiterals(t *testing.T) {
	// Arrange: a real NOT NULL / REFERENCES alongside literal text that contains
	// the same keywords. The quoted REFERENCES target must survive intact.
	sql := `CREATE TABLE items (
		id INT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT 'has NOT NULL inside',
		owner_id INT NOT NULL REFERENCES "users",
		note TEXT DEFAULT 'mentions REFERENCES users'
	);`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if name := columnByName(t, tables[0], "name"); !name.NotNull {
		t.Error("name should keep its real NOT NULL")
	}
	owner := columnByName(t, tables[0], "owner_id")
	if !owner.IsFK || owner.FKRefTable != "users" {
		t.Fatalf("owner_id: got FK=%v ref=%q, want FK to users", owner.IsFK, owner.FKRefTable)
	}
	if note := columnByName(t, tables[0], "note"); note.IsFK {
		t.Errorf("note must not be a FK from a string literal, got ref %q", note.FKRefTable)
	}
}

func TestParseSchema_TableConstraintStringLiteralDoesNotTrigger(t *testing.T) {
	// Arrange: constraint keywords sitting inside a CHECK body string literal must
	// not be applied as real table-level PRIMARY KEY / FOREIGN KEY constraints.
	sql := `CREATE TABLE t (
		id INT,
		body TEXT,
		CONSTRAINT ck CHECK (body <> 'PRIMARY KEY (id)'),
		CONSTRAINT ck2 CHECK (body <> 'FOREIGN KEY (id) REFERENCES other')
	);`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if id := columnByName(t, tables[0], "id"); id.IsPK {
		t.Error("id must not be PK from a CHECK string literal")
	}
	if len(tables[0].ForeignKeys) != 0 {
		t.Fatalf("expected no foreign keys, got %d", len(tables[0].ForeignKeys))
	}
}

func TestParseSchema_TableConstraintRealKeysStillDetected(t *testing.T) {
	// Arrange: a real table-level PRIMARY KEY / FOREIGN KEY must still be detected.
	sql := `CREATE TABLE t (
		id INT,
		owner_id INT,
		PRIMARY KEY (id),
		FOREIGN KEY (owner_id) REFERENCES owners (id)
	);`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if id := columnByName(t, tables[0], "id"); !id.IsPK {
		t.Error("id should be detected as PK")
	}
	owner := columnByName(t, tables[0], "owner_id")
	if !owner.IsFK || owner.FKRefTable != "owners" {
		t.Fatalf("owner_id: got FK=%v ref=%q, want FK to owners", owner.IsFK, owner.FKRefTable)
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
