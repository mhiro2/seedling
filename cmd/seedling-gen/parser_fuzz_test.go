package main

import (
	"bytes"
	"strings"
	"testing"
)

func FuzzParseSchemaWithDialect(f *testing.F) {
	// Seed corpus
	f.Add("", "auto")
	f.Add("CREATE TABLE users (id INT PRIMARY KEY);", "auto")
	f.Add("CREATE TABLE users (id INT, name TEXT", "postgres")
	f.Add("CREATE TABLE `users` (`id` INTEGER PRIMARY KEY, `name` VARCHAR(255));", "mysql")
	f.Add(`CREATE TABLE "users" ("id" SERIAL PRIMARY KEY, "name" TEXT NOT NULL);`, "postgres")
	f.Add("CREATE TYPE mood AS ENUM ('sad', 'ok'); CREATE TABLE tasks (id INT, mood mood NOT NULL);", "postgres")
	f.Add("CREATE TABLE x (\x00id INT, note TEXT DEFAULT 'a(b)c');", "sqlite")
	f.Add(strings.Repeat("CREATE TABLE t (id INT, note TEXT DEFAULT '(');", 32), "auto")
	f.Add("CREATE TABLE users (id INT PRIMARY KEY);", "oracle")
	f.Add(strings.Repeat("A", 20000), strings.Repeat("B", 128))
	// Doubled quotes inside ANSI / MySQL quoted identifiers must be treated as escapes.
	f.Add(`CREATE TABLE "weird""name" ("id""col" INT PRIMARY KEY);`, "postgres")
	f.Add("CREATE TABLE `weird``name` (`id``col` INT PRIMARY KEY);", "mysql")
	// Doubled single quotes are SQL string-literal escapes.
	f.Add(`CREATE TABLE quoted (note TEXT DEFAULT 'it''s ok');`, "auto")
	// Schema-qualified (dotted) identifiers, including quoted segments.
	f.Add(`CREATE TABLE public.users (id INT PRIMARY KEY);`, "postgres")
	f.Add(`CREATE TABLE "public"."users" ("id" INT PRIMARY KEY);`, "postgres")
	// Reserved-word table / column names quoted to disambiguate.
	f.Add(`CREATE TABLE "select" ("from" INT PRIMARY KEY, "where" TEXT);`, "postgres")
	f.Add("CREATE TABLE `order` (`group` INT PRIMARY KEY, `select` TEXT);", "mysql")
	// Comments interleaved across multi-line CREATE TABLE bodies.
	f.Add("-- header\nCREATE TABLE users (\n  id INT, -- inline\n  /* block */ name TEXT\n);", "auto")
	f.Add("CREATE TABLE users (\n  id INT,\n  -- comment-like inside identifier:\n  \"col--name\" TEXT\n);", "postgres")

	f.Fuzz(func(t *testing.T, sql, dialect string) {
		// Act
		tables, err := ParseSchemaWithDialect(sql, dialect)

		// Assert
		if !isSupportedDialect(dialect) {
			if err == nil {
				t.Fatal("expected unsupported dialect error")
			}
			if !strings.Contains(err.Error(), "unsupported dialect") {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}

		// Supported dialect: parse errors are allowed for malformed input,
		// but must not panic.
		if err != nil {
			return
		}

		var buf bytes.Buffer
		_ = Generate(&buf, "blueprints", tables)
	})
}

func isSupportedDialect(dialect string) bool {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "auto", "postgres", "mysql", "sqlite":
		return true
	default:
		return false
	}
}
