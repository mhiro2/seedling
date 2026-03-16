//go:build integration

package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	// Register the database/sql SQLite driver.
	_ "modernc.org/sqlite"

	"github.com/mhiro2/seedling"
)

type Harness struct {
	DB       *sql.DB
	Registry *seedling.Registry
}

func NewHarness(tb testing.TB) *Harness {
	tb.Helper()

	db := openSQLiteDB(tb)
	reg := registerBlueprints(tb)

	return &Harness{
		DB:       db,
		Registry: reg,
	}
}

func openSQLiteDB(tb testing.TB) *sql.DB {
	tb.Helper()

	db, err := sql.Open("sqlite", "file:seedling_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		tb.Fatalf("open sqlite db: %v", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Errorf("close sqlite db: %v", err)
		}
	})

	if err := db.PingContext(context.Background()); err != nil {
		tb.Fatalf("ping sqlite db: %v", err)
	}

	if err := applySchema(context.Background(), db); err != nil {
		tb.Fatalf("apply sqlite schema: %v", err)
	}

	return db
}

func applySchema(ctx context.Context, db *sql.DB) error {
	schema, err := os.ReadFile(schemaPath())
	if err != nil {
		return fmt.Errorf("read sqlite schema: %w", err)
	}

	for statement := range strings.SplitSeq(string(schema), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply sqlite statement %q: %w", statement, err)
		}
	}

	return nil
}

func schemaPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("sqlite testutil: unable to resolve schema path")
	}

	return filepath.Join(filepath.Dir(filepath.Dir(file)), "testdata", "schema.sql")
}
