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
	if got := tables[0].ForeignKeys[0].RefColumns; len(got) != 1 || got[0] != "id" {
		t.Fatalf("referenced columns = %v, want [id]", got)
	}
}

func TestParseSchema_RetainsNonPrimaryReferencedColumns(t *testing.T) {
	sql := `
CREATE TABLE countries (
  id INT PRIMARY KEY,
  code TEXT UNIQUE NOT NULL
);
CREATE TABLE cities (
  id INT PRIMARY KEY,
  country_code TEXT NOT NULL REFERENCES countries(code)
);`

	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	fk := tables[1].ForeignKeys[0]
	if got := fk.RefColumns; len(got) != 1 || got[0] != "code" {
		t.Fatalf("referenced columns = %v, want [code]", got)
	}
	if got := tables[1].Columns[1].FKRefColumns; len(got) != 1 || got[0] != "code" {
		t.Fatalf("column referenced fields = %v, want [code]", got)
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

func TestSQLDialectMask(t *testing.T) {
	tests := []struct {
		name       string
		dialect    string
		in         string
		wantText   string
		wantMasked string
	}{
		{
			name:       "line and block comments",
			dialect:    "auto",
			in:         "a -- b\nc /* d\n */ e",
			wantText:   "a     \nc     \n    e",
			wantMasked: "a     \nc     \n    e",
		},
		{
			name:       "string literal with doubled quote",
			dialect:    "auto",
			in:         `x 'it''s -- (' y`,
			wantText:   `x 'it''s -- (' y`,
			wantMasked: `x '          ' y`,
		},
		{
			name:       "quoted identifiers keep one token",
			dialect:    "auto",
			in:         "\"a b\" `c(d`",
			wantText:   "\"a b\" `c(d`",
			wantMasked: "\"___\" `___`",
		},
		{
			name:       "postgres dollar quotes",
			dialect:    "postgres",
			in:         `d $$NOT NULL$$ $tag$ ) $$ $tag$ e`,
			wantText:   `d $$NOT NULL$$ $tag$ ) $$ $tag$ e`,
			wantMasked: `d $$        $$ $tag$      $tag$ e`,
		},
		{
			name:       "dollar inside an identifier is not a quote",
			dialect:    "postgres",
			in:         `a$b$ NOT NULL`,
			wantText:   `a$b$ NOT NULL`,
			wantMasked: `a$b$ NOT NULL`,
		},
		{
			name:       "postgres escape string",
			dialect:    "postgres",
			in:         `E'it\'s' NOT NULL`,
			wantText:   `E'it\'s' NOT NULL`,
			wantMasked: `E'     ' NOT NULL`,
		},
		{
			name:       "mysql backslash escape",
			dialect:    "mysql",
			in:         `'it\'s' NOT NULL`,
			wantText:   `'it\'s' NOT NULL`,
			wantMasked: `'     ' NOT NULL`,
		},
		{
			name:       "sqlite has no dollar quotes",
			dialect:    "sqlite",
			in:         `$$x$$`,
			wantText:   `$$x$$`,
			wantMasked: `$$x$$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			d, err := newSQLDialect(tt.dialect)
			if err != nil {
				t.Fatal(err)
			}

			// Act
			text, masked := d.mask(tt.in)

			// Assert
			if text != tt.wantText {
				t.Fatalf("text = %q, want %q", text, tt.wantText)
			}
			if masked != tt.wantMasked {
				t.Fatalf("masked = %q, want %q", masked, tt.wantMasked)
			}
		})
	}
}

func TestParseSchema_IgnoresSQLInsideStringLiterals(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		sql     string
	}{
		{
			name:    "CREATE TABLE inside a string",
			dialect: "auto",
			sql:     `SELECT 'CREATE TABLE ghosts (id INTEGER PRIMARY KEY)';`,
		},
		{
			name:    "CREATE TABLE inside a dollar quote",
			dialect: "postgres",
			sql:     `DO $body$ BEGIN EXECUTE 'x'; END $body$; SELECT $$CREATE TABLE ghosts (id INT)$$;`,
		},
		{
			name:    "CREATE TABLE inside a comment",
			dialect: "auto",
			sql:     "/* CREATE TABLE ghosts (id INT); */ -- CREATE TABLE ghosts (id INT);\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sql := tt.sql + "\nCREATE TABLE users (id INTEGER PRIMARY KEY);"

			// Act
			tables, err := ParseSchemaWithDialect(sql, tt.dialect)
			if err != nil {
				t.Fatal(err)
			}

			// Assert
			if len(tables) != 1 || tables[0].Name != "users" {
				t.Fatalf("got tables %+v, want only users", tables)
			}
		})
	}
}

func TestParseSchema_IgnoresConstraintsInsideQuotedDefaults(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		column  string
	}{
		{name: "dollar quote", dialect: "postgres", column: `note TEXT DEFAULT $$NOT NULL$$`},
		{name: "tagged dollar quote", dialect: "postgres", column: `note TEXT DEFAULT $d$REFERENCES users(id) NOT NULL$d$`},
		{name: "escape string", dialect: "postgres", column: `note TEXT DEFAULT E'it\'s NOT NULL'`},
		{name: "mysql backslash", dialect: "mysql", column: `note TEXT DEFAULT 'it\'s NOT NULL'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			sql := "CREATE TABLE notes (id INT NOT NULL, " + tt.column + ", owner_id INT NOT NULL);"

			// Act
			tables, err := ParseSchemaWithDialect(sql, tt.dialect)
			if err != nil {
				t.Fatal(err)
			}

			// Assert
			notes := findTableByName(t, tables, "notes")
			if len(notes.Columns) != 3 {
				t.Fatalf("got columns %+v, want id, note, owner_id", notes.Columns)
			}
			note := findColumnByName(t, notes, "note")
			if note.NotNull || note.IsFK {
				t.Fatalf("note column = %+v, want nullable and not a foreign key", note)
			}
			if !findColumnByName(t, notes, "owner_id").NotNull {
				t.Fatal("expected owner_id to stay NOT NULL")
			}
		})
	}
}

func TestParseSchema_PrimaryKeyForeignKeyIsRequired(t *testing.T) {
	const users = "CREATE TABLE users (id INTEGER PRIMARY KEY);\n"
	tests := []struct {
		name        string
		dialect     string
		sql         string
		fkColumns   []string
		wantNotNull bool
	}{
		{
			name:        "postgres inline key",
			dialect:     "postgres",
			sql:         "CREATE TABLE profiles (user_id INTEGER PRIMARY KEY REFERENCES users(id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: true,
		},
		{
			name:        "postgres table-level key",
			dialect:     "postgres",
			sql:         "CREATE TABLE profiles (user_id INTEGER REFERENCES users(id), PRIMARY KEY (user_id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: true,
		},
		{
			name:        "mysql composite key with table-level foreign key",
			dialect:     "mysql",
			sql:         "CREATE TABLE memberships (user_id INT, team_id INT, PRIMARY KEY (user_id, team_id), FOREIGN KEY (user_id) REFERENCES users(id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: true,
		},
		{
			name:        "auto follows standard SQL",
			dialect:     "auto",
			sql:         "CREATE TABLE profiles (user_id INT PRIMARY KEY REFERENCES users(id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: true,
		},
		{
			name:        "sqlite INTEGER PRIMARY KEY is the rowid",
			dialect:     "sqlite",
			sql:         "CREATE TABLE profiles (user_id INTEGER PRIMARY KEY REFERENCES users(id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: true,
		},
		{
			name:        "sqlite non-rowid key may be NULL",
			dialect:     "sqlite",
			sql:         "CREATE TABLE profiles (user_id INT PRIMARY KEY REFERENCES users(id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: false,
		},
		{
			name:        "sqlite composite key may be NULL",
			dialect:     "sqlite",
			sql:         "CREATE TABLE memberships (user_id INTEGER REFERENCES users(id), team_id INTEGER, PRIMARY KEY (user_id, team_id));",
			fkColumns:   []string{"user_id"},
			wantNotNull: false,
		},
		{
			name:        "sqlite WITHOUT ROWID key is NOT NULL",
			dialect:     "sqlite",
			sql:         "CREATE TABLE memberships (user_id INT REFERENCES users(id), team_id INT, PRIMARY KEY (user_id, team_id)) WITHOUT ROWID;",
			fkColumns:   []string{"user_id"},
			wantNotNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			tables, err := ParseSchemaWithDialect(users+tt.sql, tt.dialect)
			if err != nil {
				t.Fatal(err)
			}

			// Assert
			fk := findForeignKeyByColumns(t, tables[1], tt.fkColumns...)
			if fk.NotNull != tt.wantNotNull {
				t.Fatalf("foreign key NotNull = %v, want %v", fk.NotNull, tt.wantNotNull)
			}
			if got := findColumnByName(t, tables[1], tt.fkColumns[0]).NotNull; got != tt.wantNotNull {
				t.Fatalf("column NotNull = %v, want %v", got, tt.wantNotNull)
			}
		})
	}
}
