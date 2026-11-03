package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func findTable(t *testing.T, tables []Table, name string) Table {
	t.Helper()
	i := slices.IndexFunc(tables, func(table Table) bool { return table.Name == name })
	if i < 0 {
		t.Fatalf("table %q not found in %v", name, tableNames(tables))
	}
	return tables[i]
}

func schemaColumn(t *testing.T, table Table, name string) Column {
	t.Helper()
	i := slices.IndexFunc(table.Columns, func(col Column) bool { return col.Name == name })
	if i < 0 {
		t.Fatalf("column %q not found on %s", name, table.Name)
	}
	return table.Columns[i]
}

func tableNames(tables []Table) []string {
	names := make([]string, len(tables))
	for i, table := range tables {
		names[i] = table.Name
	}
	return names
}

func columnNames(table Table) []string {
	names := make([]string, len(table.Columns))
	for i, col := range table.Columns {
		names[i] = col.Name
	}
	return names
}

func foreignKeyTargets(table Table) []string {
	targets := make([]string, len(table.ForeignKeys))
	for i, fk := range table.ForeignKeys {
		targets[i] = strings.Join(fk.Columns, ",") + "->" + fk.RefTable + "(" + strings.Join(fk.RefColumns, ",") + ")"
	}
	return targets
}

func TestParseSchemaSources_AppliesMigrationsInOrder(t *testing.T) {
	// Arrange
	sources := []schemaSource{
		{Name: "001_init.sql", SQL: `
-- +goose Up
CREATE TABLE companies (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    nickname TEXT
);
-- +goose Down
DROP TABLE users;
DROP TABLE companies;
`},
		{Name: "002_alter.sql", SQL: `
-- +goose Up
ALTER TABLE users ADD COLUMN email VARCHAR(64) NOT NULL UNIQUE;
ALTER TABLE users RENAME COLUMN nickname TO display_name;
ALTER TABLE users ALTER COLUMN display_name SET NOT NULL;
ALTER TABLE companies RENAME TO organizations;
ALTER TABLE organizations RENAME COLUMN id TO org_id;
-- +goose Down
ALTER TABLE users DROP COLUMN email;
`},
	}

	// Act
	tables, err := parseSchemaSources(sources, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := tableNames(tables), []string{"organizations", "users"}; !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	org := findTable(t, tables, "organizations")
	if org.GoName != "Organization" || org.BlueprintID != "organization" {
		t.Fatalf("renamed table names = %s/%s", org.GoName, org.BlueprintID)
	}
	if pk := schemaColumn(t, org, "org_id"); !pk.IsPK || pk.GoName != "OrgID" {
		t.Fatalf("renamed key column = %+v", pk)
	}
	users := findTable(t, tables, "users")
	if got, want := columnNames(users), []string{"id", "company_id", "display_name", "email"}; !slices.Equal(got, want) {
		t.Fatalf("users columns = %v, want %v", got, want)
	}
	if got, want := foreignKeyTargets(users), []string{"company_id->organizations(org_id)"}; !slices.Equal(got, want) {
		t.Fatalf("users foreign keys = %v, want %v", got, want)
	}
	if col := schemaColumn(t, users, "company_id"); !col.IsFK || col.FKRefTable != "organizations" {
		t.Fatalf("company_id = %+v", col)
	}
	if email := schemaColumn(t, users, "email"); !email.NotNull || !email.Unique || email.MaxLength != 64 {
		t.Fatalf("email = %+v", email)
	}
	if name := schemaColumn(t, users, "display_name"); !name.NotNull || name.GoName != "DisplayName" {
		t.Fatalf("display_name = %+v", name)
	}
}

func TestParseSchemaSources_StripsRollbackSections(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{name: "goose", sql: "-- +goose Up\nCREATE TABLE users (id INT PRIMARY KEY);\n-- +goose Down\nDROP TABLE users;\n"},
		{name: "sql-migrate", sql: "-- +migrate Up\nCREATE TABLE users (id INT PRIMARY KEY);\n-- +migrate Down\nDROP TABLE users;\n"},
		{name: "dbmate", sql: "-- migrate:up\nCREATE TABLE users (id INT PRIMARY KEY);\n-- migrate:down\nDROP TABLE users;\n"},
		{name: "tern", sql: "CREATE TABLE users (id INT PRIMARY KEY);\n---- create above / drop below ----\nDROP TABLE users;\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			tables, err := parseSchemaSources([]schemaSource{{Name: "m.sql", SQL: tt.sql}}, "postgres")
			if err != nil {
				t.Fatal(err)
			}

			// Assert
			if got := tableNames(tables); !slices.Equal(got, []string{"users"}) {
				t.Fatalf("tables = %v, want [users]", got)
			}
		})
	}
}

func TestParseSchemaSources_DropsAndConstraints(t *testing.T) {
	// Arrange
	sql := `
CREATE TABLE companies (id INT PRIMARY KEY);
CREATE TABLE teams (id INT PRIMARY KEY);
CREATE TABLE users (
    id INT PRIMARY KEY,
    company_id INT NOT NULL REFERENCES companies(id),
    team_id INT,
    legacy_id INT,
    status TEXT CHECK (status IN ('active', 'banned', 'invited')),
    CONSTRAINT users_team_fk FOREIGN KEY (team_id) REFERENCES teams(id)
);
CREATE TABLE audit (id INT PRIMARY KEY, user_legacy_id INT REFERENCES users(legacy_id));
ALTER TABLE users DROP CONSTRAINT users_company_id_fkey;
ALTER TABLE users DROP CONSTRAINT users_team_fk, ADD CONSTRAINT users_team_fk2 FOREIGN KEY (team_id) REFERENCES teams(id);
ALTER TABLE users DROP CONSTRAINT users_status_check, ADD CONSTRAINT users_status_check2 CHECK (status IN ('active', 'invited'));
ALTER TABLE users DROP COLUMN legacy_id;
DROP TABLE IF EXISTS teams CASCADE;
DROP TABLE IF EXISTS ghosts;
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := tableNames(tables), []string{"companies", "users", "audit"}; !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
	users := findTable(t, tables, "users")
	if got := foreignKeyTargets(users); len(got) != 0 {
		t.Fatalf("users foreign keys = %v, want none", got)
	}
	if col := schemaColumn(t, users, "company_id"); col.IsFK {
		t.Fatalf("company_id still marked as a foreign key: %+v", col)
	}
	if status := schemaColumn(t, users, "status"); !slices.Equal(status.EnumValues, []string{"active", "invited"}) {
		t.Fatalf("status enum = %v", status.EnumValues)
	}
	if got := foreignKeyTargets(findTable(t, tables, "audit")); len(got) != 0 {
		t.Fatalf("audit keeps a foreign key to a dropped column: %v", got)
	}
}

func TestParseSchemaSources_ColumnTypeAndUniqueIndexes(t *testing.T) {
	// Arrange
	sql := `
CREATE TABLE users (id INT PRIMARY KEY, email TEXT, code VARCHAR(8) NOT NULL, handle TEXT UNIQUE);
CREATE UNIQUE INDEX users_email_idx ON users (email);
ALTER TABLE users ALTER COLUMN code TYPE VARCHAR(32) USING code::varchar(32);
ALTER TABLE users ALTER COLUMN code DROP NOT NULL, ALTER COLUMN code SET DEFAULT 'x';
ALTER TABLE users DROP CONSTRAINT users_handle_key;
ALTER INDEX users_email_idx RENAME TO users_email_uniq;
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	users := findTable(t, tables, "users")
	if code := schemaColumn(t, users, "code"); code.MaxLength != 32 || code.NotNull {
		t.Fatalf("code = %+v", code)
	}
	if !schemaColumn(t, users, "email").Unique {
		t.Fatal("email lost its unique index")
	}
	if schemaColumn(t, users, "handle").Unique {
		t.Fatal("handle kept a dropped unique constraint")
	}

	// Act: the renamed index is dropped by its new name.
	tables, err = ParseSchemaWithDialect(sql+"DROP INDEX users_email_uniq;", "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if schemaColumn(t, findTable(t, tables, "users"), "email").Unique {
		t.Fatal("email kept a dropped unique index")
	}
}

func TestParseSchemaSources_EnumTypeChanges(t *testing.T) {
	// Arrange
	sql := `
CREATE TYPE status AS ENUM ('draft', 'done');
CREATE TABLE tasks (id INT PRIMARY KEY, status status NOT NULL);
ALTER TYPE status ADD VALUE 'review' BEFORE 'done';
ALTER TYPE status ADD VALUE IF NOT EXISTS 'archived';
ALTER TYPE status RENAME VALUE 'draft' TO 'open';
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	got := schemaColumn(t, findTable(t, tables, "tasks"), "status").EnumValues
	if want := []string{"open", "review", "done", "archived"}; !slices.Equal(got, want) {
		t.Fatalf("status enum = %v, want %v", got, want)
	}
}

func TestParseSchemaSources_MySQLAlterForms(t *testing.T) {
	// Arrange
	sql := "CREATE TABLE `companies` (`id` BIGINT PRIMARY KEY);\n" +
		"CREATE TABLE `users` (\n" +
		"  `id` BIGINT PRIMARY KEY,\n" +
		"  `company_id` BIGINT,\n" +
		"  `name` VARCHAR(20),\n" +
		"  FOREIGN KEY (`company_id`) REFERENCES `companies` (`id`)\n" +
		") ENGINE=InnoDB;\n" +
		"ALTER TABLE `users` DROP FOREIGN KEY `users_ibfk_1`;\n" +
		"ALTER TABLE `users` MODIFY `name` VARCHAR(40) NOT NULL, CHANGE COLUMN `company_id` `org_id` BIGINT NOT NULL;\n" +
		"ALTER TABLE `users` ADD COLUMN `email` VARCHAR(64) AFTER `id`, ADD UNIQUE KEY `users_email` (`email`), ADD INDEX `users_name` (`name`);\n" +
		"ALTER TABLE `users` ADD CONSTRAINT `users_org_fk` FOREIGN KEY (`org_id`) REFERENCES `companies` (`id`), ENGINE=InnoDB;\n" +
		"RENAME TABLE `companies` TO `orgs`;\n"

	// Act
	tables, err := ParseSchemaWithDialect(sql, "mysql")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	users := findTable(t, tables, "users")
	if got, want := columnNames(users), []string{"id", "email", "org_id", "name"}; !slices.Equal(got, want) {
		t.Fatalf("users columns = %v, want %v", got, want)
	}
	if got, want := foreignKeyTargets(users), []string{"org_id->orgs(id)"}; !slices.Equal(got, want) {
		t.Fatalf("users foreign keys = %v, want %v", got, want)
	}
	if !users.ForeignKeys[0].NotNull {
		t.Fatal("org_id foreign key should be required after CHANGE ... NOT NULL")
	}
	if name := schemaColumn(t, users, "name"); name.MaxLength != 40 || !name.NotNull {
		t.Fatalf("name = %+v", name)
	}
	if !schemaColumn(t, users, "email").Unique {
		t.Fatal("email should be unique")
	}
}

func TestParseSchemaSources_PgDumpOutput(t *testing.T) {
	// Arrange: the shape pg_dump --schema-only writes, with keys added after
	// the tables.
	sql := `
SET statement_timeout = 0;
SELECT pg_catalog.set_config('search_path', '', false);
CREATE TABLE public.companies (id bigint NOT NULL, name text NOT NULL);
ALTER TABLE public.companies OWNER TO postgres;
CREATE SEQUENCE public.companies_id_seq START WITH 1;
ALTER SEQUENCE public.companies_id_seq OWNED BY public.companies.id;
CREATE TABLE public.users (id bigint NOT NULL, company_id bigint NOT NULL, email text NOT NULL);
ALTER TABLE ONLY public.companies ALTER COLUMN id SET DEFAULT nextval('public.companies_id_seq'::regclass);
ALTER TABLE public.users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (SEQUENCE NAME public.users_id_seq START WITH 1);
ALTER TABLE ONLY public.companies ADD CONSTRAINT companies_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.users ADD CONSTRAINT users_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.users ADD CONSTRAINT users_email_key UNIQUE (email);
CREATE INDEX users_company_id_idx ON public.users USING btree (company_id);
ALTER TABLE ONLY public.users ADD CONSTRAINT users_company_id_fkey FOREIGN KEY (company_id) REFERENCES public.companies(id) ON DELETE CASCADE;
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	users := findTable(t, tables, "users")
	if !schemaColumn(t, users, "id").IsPK || !schemaColumn(t, findTable(t, tables, "companies"), "id").IsPK {
		t.Fatal("primary keys added by ALTER TABLE were not applied")
	}
	if got, want := foreignKeyTargets(users), []string{"company_id->companies(id)"}; !slices.Equal(got, want) {
		t.Fatalf("users foreign keys = %v, want %v", got, want)
	}
	if !schemaColumn(t, users, "email").Unique {
		t.Fatal("email should be unique")
	}
}

func TestParseSchemaSources_ReportsUnsupportedChanges(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "unknown action",
			sql:  "CREATE TABLE users (id INT PRIMARY KEY);\n\nALTER TABLE users INHERITS FROM people;",
			want: `m.sql:3: ALTER TABLE users: unsupported action "INHERITS FROM people"`,
		},
		{
			name: "unknown table",
			sql:  "ALTER TABLE users ADD COLUMN email TEXT;",
			want: "m.sql:1: ALTER TABLE users: table is not defined by an earlier CREATE TABLE",
		},
		{
			name: "unknown constraint",
			sql:  "CREATE TABLE users (id INT PRIMARY KEY);\nALTER TABLE users DROP CONSTRAINT users_magic;",
			want: "m.sql:2: ALTER TABLE users: DROP CONSTRAINT users_magic: constraint is not defined",
		},
		{
			name: "unknown column",
			sql:  "CREATE TABLE users (id INT PRIMARY KEY);\nALTER TABLE users DROP COLUMN email;",
			want: "m.sql:2: ALTER TABLE users: DROP COLUMN email: column is not defined",
		},
		{
			name: "duplicate table",
			sql:  "CREATE TABLE users (id INT);\nCREATE TABLE users (id INT);",
			want: "m.sql:2: CREATE TABLE users: table is already defined",
		},
		{
			name: "drop unknown table",
			sql:  "DROP TABLE users;",
			want: "m.sql:1: DROP TABLE users: table is not defined",
		},
		{
			name: "unknown column action",
			sql:  "CREATE TABLE users (id INT);\nALTER TABLE users ALTER COLUMN id SET MAGIC;",
			want: `m.sql:2: ALTER TABLE users: ALTER COLUMN id: unsupported action "SET MAGIC"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := parseSchemaSources([]schemaSource{{Name: "m.sql", SQL: tt.sql}}, "postgres")

			// Assert
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestParseSchemaSources_TolerantForms(t *testing.T) {
	// Arrange: statements that are valid in a migration history and leave
	// the tables as they are.
	sql := `
CREATE TABLE users (id INT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS users (id INT PRIMARY KEY, ignored TEXT);
ALTER TABLE IF EXISTS ghosts ADD COLUMN x INT;
ALTER TABLE ghosts OWNER TO postgres;
ALTER TABLE users ADD COLUMN IF NOT EXISTS id INT;
ALTER TABLE users DROP COLUMN IF EXISTS email, DROP CONSTRAINT IF EXISTS users_magic;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS some_plain_index;
CREATE VIEW active_users AS SELECT * FROM users;
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got := columnNames(findTable(t, tables, "users")); !slices.Equal(got, []string{"id"}) {
		t.Fatalf("users columns = %v, want [id]", got)
	}
}

func TestReadSchemaFiles_ReportsFileOfFailingMigration(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	files := map[string]string{
		"001_init.up.sql":   "CREATE TABLE users (id INT PRIMARY KEY);\n",
		"001_init.down.sql": "DROP TABLE users;\n",
		"002_bad.up.sql":    "-- comment\nALTER TABLE users DROP COLUMN missing;\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := listSchemaDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := readSchemaFiles(paths)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	_, err = parseSchemaSources(sources, "postgres")

	// Assert
	want := filepath.Join(dir, "002_bad.up.sql") + ":2: ALTER TABLE users: DROP COLUMN missing"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want it to contain %q", err, want)
	}
}

func TestParseSchemaSources_RollbackMarkerInsideLiteral(t *testing.T) {
	// Arrange
	sql := "CREATE TABLE users (id INT PRIMARY KEY);\nSELECT $body$\n-- +goose Down\n$body$;\nCREATE TABLE events (id INT PRIMARY KEY);\n"

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := tableNames(tables), []string{"users", "events"}; !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
}

func TestParseSchemaSources_EnumTypeIdentity(t *testing.T) {
	// Arrange: the column keeps the renamed type, not the new type that
	// reuses its old name.
	sql := `
CREATE TYPE mood AS ENUM ('happy');
CREATE TABLE users (id INT PRIMARY KEY, mood mood NOT NULL);
ALTER TYPE mood RENAME TO old_mood;
CREATE TYPE mood AS ENUM ('sad');
ALTER TYPE old_mood ADD VALUE 'calm';
CREATE TABLE posts (id INT PRIMARY KEY, mood mood NOT NULL);
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := schemaColumn(t, findTable(t, tables, "users"), "mood").EnumValues, []string{"happy", "calm"}; !slices.Equal(got, want) {
		t.Fatalf("users.mood = %v, want %v", got, want)
	}
	if got, want := schemaColumn(t, findTable(t, tables, "posts"), "mood").EnumValues, []string{"sad"}; !slices.Equal(got, want) {
		t.Fatalf("posts.mood = %v, want %v", got, want)
	}
}

func TestParseSchemaSources_DropTypeCascade(t *testing.T) {
	// Arrange
	sql := `
CREATE TYPE mood AS ENUM ('happy');
CREATE TABLE users (id INT PRIMARY KEY, mood mood);
`

	// Act
	tables, err := ParseSchemaWithDialect(sql+"DROP TYPE mood CASCADE;", "postgres")
	if err != nil {
		t.Fatal(err)
	}
	_, restrictErr := ParseSchemaWithDialect(sql+"DROP TYPE mood;", "postgres")

	// Assert
	if got := columnNames(findTable(t, tables, "users")); !slices.Equal(got, []string{"id"}) {
		t.Fatalf("users columns = %v, want [id]", got)
	}
	if restrictErr == nil || !strings.Contains(restrictErr.Error(), "column users.mood still uses the type") {
		t.Fatalf("err = %v, want a type in use error", restrictErr)
	}
}

func TestParseSchemaSources_DroppedPrimaryKeyKeepsNotNull(t *testing.T) {
	// Arrange
	sql := `
CREATE TABLE users (id INT PRIMARY KEY, replacement INT);
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE users ADD PRIMARY KEY (replacement);
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	users := findTable(t, tables, "users")
	if id := schemaColumn(t, users, "id"); id.IsPK || !id.NotNull {
		t.Fatalf("id = %+v, want a NOT NULL non-key column", id)
	}
	if !schemaColumn(t, users, "replacement").IsPK {
		t.Fatal("replacement should be the primary key")
	}
}

func TestParseSchemaSources_DeclaredConstraintNameWins(t *testing.T) {
	// Arrange: users_check names the explicit constraint, so dropping it
	// keeps the unnamed check on status.
	sql := `
CREATE TABLE users (
    id INT PRIMARY KEY,
    status TEXT CHECK (status IN ('active', 'inactive')),
    CONSTRAINT users_check CHECK (id > 0)
);
ALTER TABLE users DROP CONSTRAINT users_check;
`

	// Act
	tables, err := ParseSchemaWithDialect(sql, "postgres")
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := schemaColumn(t, findTable(t, tables, "users"), "status").EnumValues, []string{"active", "inactive"}; !slices.Equal(got, want) {
		t.Fatalf("status enum = %v, want %v", got, want)
	}
}
