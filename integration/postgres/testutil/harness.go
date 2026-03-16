//go:build integration

package testutil

import (
	"context"
	"database/sql"
	"testing"
	"time"

	// Register the database/sql pgx driver.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mhiro2/seedling"
)

const (
	startupTimeout = 2 * time.Minute
	cleanupTimeout = 30 * time.Second
)

type Harness struct {
	DB       *sql.DB
	Registry *seedling.Registry
}

func NewHarness(tb testing.TB) *Harness {
	tb.Helper()

	db := openPostgresDB(tb)
	reg := registerBlueprints(tb)

	return &Harness{
		DB:       db,
		Registry: reg,
	}
}

func openPostgresDB(tb testing.TB) *sql.DB {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	tb.Cleanup(cancel)

	container, err := runPostgresContainer(ctx)
	if err != nil {
		if shouldSkipDockerError(err) {
			tb.Skipf("skip postgres integration test: %v", err)
		}
		tb.Fatalf("start postgres container: %v", err)
	}

	tb.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			tb.Errorf("terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		tb.Fatalf("build connection string: %v", err)
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		tb.Fatalf("open postgres db: %v", err)
	}

	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Errorf("close postgres db: %v", err)
		}
	})

	if err := db.PingContext(ctx); err != nil {
		tb.Fatalf("ping postgres db: %v", err)
	}

	return db
}
