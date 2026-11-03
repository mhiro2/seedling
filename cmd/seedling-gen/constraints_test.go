package main

import (
	"slices"
	"strings"
	"testing"
)

func TestParseSchema_ReadsColumnConstraints(t *testing.T) {
	// Arrange
	schema := `
CREATE TYPE user_role AS ENUM ('member', 'admin', 'it''s');

CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    email VARCHAR(64) NOT NULL UNIQUE,
    handle CHARACTER VARYING (20) NOT NULL,
    role user_role NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'active')),
    kind ENUM('a', 'b') NOT NULL,
    code TEXT NOT NULL,
    slug TEXT NOT NULL,
    tenant_id BIGINT NOT NULL,
    note TEXT NOT NULL DEFAULT 'UNIQUE CHECK (note IN (''x''))',
    priority TEXT NOT NULL,
    CONSTRAINT users_code_key UNIQUE (code),
    UNIQUE (tenant_id, handle),
    CHECK (priority IN ('low', 'high'))
);

CREATE UNIQUE INDEX users_slug_idx ON users (slug);
CREATE UNIQUE INDEX users_tenant_note_idx ON users (tenant_id, note);
`

	// Act
	tables, err := ParseSchema(schema)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	tests := []struct {
		column     string
		enumValues []string
		maxLength  int
		unique     bool
	}{
		{column: "email", maxLength: 64, unique: true},
		{column: "handle", maxLength: 20},
		{column: "role", enumValues: []string{"member", "admin", "it's"}},
		{column: "status", enumValues: []string{"draft", "active"}},
		{column: "kind", enumValues: []string{"a", "b"}},
		{column: "code", unique: true},
		{column: "slug", unique: true},
		{column: "tenant_id"},
		{column: "note"},
		{column: "priority", enumValues: []string{"low", "high"}},
		{column: "id"},
	}
	for _, tt := range tests {
		t.Run(tt.column, func(t *testing.T) {
			col := columnByName(t, tables[0], tt.column)
			if !slices.Equal(col.EnumValues, tt.enumValues) {
				t.Fatalf("EnumValues = %q, want %q", col.EnumValues, tt.enumValues)
			}
			if col.MaxLength != tt.maxLength {
				t.Fatalf("MaxLength = %d, want %d", col.MaxLength, tt.maxLength)
			}
			if col.Unique != tt.unique {
				t.Fatalf("Unique = %v, want %v", col.Unique, tt.unique)
			}
		})
	}
}

func TestParseStringLiterals_DecodesEscapes(t *testing.T) {
	tests := []struct {
		list      string
		backslash bool
		want      []string
	}{
		{list: `'it''s', 'b'`, want: []string{"it's", "b"}},
		{list: `'a\n', 'b'`, want: []string{`a\n`, "b"}},
		{list: `'a\n', 'it\'s'`, backslash: true, want: []string{"a\n", "it's"}},
		{list: `E'a\tb', 'c\d'`, want: []string{"a\tb", `c\d`}},
	}
	for _, tt := range tests {
		got, ok := parseStringLiterals(tt.list, tt.backslash)
		if !ok || !slices.Equal(got, tt.want) {
			t.Fatalf("parseStringLiterals(%q, %v) = %q, %v; want %q", tt.list, tt.backslash, got, ok, tt.want)
		}
	}
}

func TestParseSchema_IntersectsEnumConstraintsAndReadsOrderedIndexes(t *testing.T) {
	// Arrange
	schema := `
CREATE TYPE level AS ENUM ('low', 'mid', 'high');
CREATE TABLE tasks (
    id BIGINT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('a', 'b')),
    lvl level NOT NULL CHECK (lvl IN ('high', 'mid')),
    email TEXT NOT NULL,
    slug TEXT NOT NULL,
    CHECK (status IN ('c', 'b'))
);
CREATE UNIQUE INDEX tasks_email_idx ON tasks (email ASC);
CREATE UNIQUE INDEX tasks_slug_idx ON tasks USING btree (slug COLLATE "C" text_pattern_ops);
`

	// Act
	tables, err := ParseSchema(schema)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got := columnByName(t, tables[0], "status").EnumValues; !slices.Equal(got, []string{"b"}) {
		t.Fatalf("status enum = %q, want [b]", got)
	}
	if got := columnByName(t, tables[0], "lvl").EnumValues; !slices.Equal(got, []string{"mid", "high"}) {
		t.Fatalf("lvl enum = %q, want [mid high]", got)
	}
	for _, name := range []string{"email", "slug"} {
		if !columnByName(t, tables[0], name).Unique {
			t.Fatalf("%s is not unique", name)
		}
	}
}

func TestParseSchemaWithDialect_MySQLEnumEscapes(t *testing.T) {
	tables, err := ParseSchemaWithDialect("CREATE TABLE t (id BIGINT PRIMARY KEY, kind ENUM('it\\'s', 'b') NOT NULL);", "mysql")
	if err != nil {
		t.Fatal(err)
	}
	if got := columnByName(t, tables[0], "kind").EnumValues; !slices.Equal(got, []string{"it's", "b"}) {
		t.Fatalf("kind enum = %q, want [it's b]", got)
	}
}

func TestParseStringLiterals_RejectsExpressions(t *testing.T) {
	for _, list := range []string{"", "1, 2", "'a', b", "'a' 'b'", "'unterminated"} {
		if values, ok := parseStringLiterals(list, false); ok {
			t.Fatalf("parseStringLiterals(%q) = %q, want rejection", list, values)
		}
	}
}

func TestParseAtlasHCL_ReadsColumnConstraints(t *testing.T) {
	// Arrange
	schema := `
table "users" {
  schema = schema.public
  column "id" {
    type = bigint
  }
  column "email" {
    type = varchar(80)
  }
  column "name" {
    type = text
  }
  primary_key {
    columns = [column.id]
  }
  index "users_email_key" {
    unique  = true
    columns = [column.email]
  }
  index "users_name_idx" {
    columns = [column.name]
  }
}
`

	// Act
	tables, err := ParseAtlasHCL(schema)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	email := columnByName(t, tables[0], "email")
	if email.MaxLength != 80 || !email.Unique {
		t.Fatalf("email = %+v, want MaxLength 80 and Unique", email)
	}
	if name := columnByName(t, tables[0], "name"); name.Unique {
		t.Fatal("name is unique, want a plain index to leave it non-unique")
	}
}

func TestParseGormDir_ReadsSizeAndUniqueTags(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type User struct {
	ID    uint   `+"`"+`gorm:"primaryKey"`+"`"+`
	Email string `+"`"+`gorm:"size:40;unique;not null"`+"`"+`
}
`)

	// Act
	models, err := ParseGormDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	for _, field := range models[0].Fields {
		if field.Name == "Email" && (field.MaxLength != 40 || !field.Unique) {
			t.Fatalf("Email = %+v, want MaxLength 40 and Unique", field)
		}
	}
}

func TestGenerate_ConstraintAwareDefaultsInsertDistinctValidRows(t *testing.T) {
	// Arrange
	tables, err := ParseSchema(`
CREATE TABLE accounts (
    id BIGINT PRIMARY KEY,
    email VARCHAR(40) NOT NULL UNIQUE,
    code CHAR(3) NOT NULL UNIQUE,
    number INTEGER NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'active')),
    token UUID NOT NULL,
    settings JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);`)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	var buf strings.Builder
	if err := Generate(&buf, "compile", tables); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Assert
	ensureCompilesAndRuns(t, "constraint_defaults", buf.String(), constraintDefaultsTest)
}

const constraintDefaultsTest = `package compile

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/mhiro2/seedling"
)

func TestConstraintAwareDefaults(t *testing.T) {
	reg := seedling.NewRegistry()
	blueprints := NewBlueprints()
	var nextID int64
	blueprints.Account.Insert = func(_ context.Context, _ seedling.DBTX, v Account) (Account, error) {
		nextID++
		v.ID = nextID
		return v, nil
	}
	blueprints.MustRegister(reg)

	accounts := seedling.NewSession[Account](reg)
	all := append(accounts.InsertMany(t, nil, 2).Roots(), accounts.InsertOne(t, nil).Root())

	uuidPattern := regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
	emails, codes, numbers := map[string]bool{}, map[string]bool{}, map[int]bool{}
	for _, a := range all {
		if len(a.Email) > 40 || emails[a.Email] {
			t.Fatalf("email %q is too long or repeated", a.Email)
		}
		if len(a.Code) > 3 || codes[a.Code] {
			t.Fatalf("code %q is too long or repeated", a.Code)
		}
		if numbers[a.Number] {
			t.Fatalf("number %d is repeated", a.Number)
		}
		emails[a.Email], codes[a.Code], numbers[a.Number] = true, true, true
		if a.Status != "pending" {
			t.Fatalf("status = %q, want the first allowed value", a.Status)
		}
		if !uuidPattern.MatchString(a.Token) {
			t.Fatalf("token %q is not a UUID", a.Token)
		}
		if !json.Valid([]byte(a.Settings)) {
			t.Fatalf("settings %q is not JSON", a.Settings)
		}
		if !a.CreatedAt.Equal(seedling.TestScope(t).Now()) {
			t.Fatalf("created_at = %v, want the scope clock", a.CreatedAt)
		}
	}
}
`
