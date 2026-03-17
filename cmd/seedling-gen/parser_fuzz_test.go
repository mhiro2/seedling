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
