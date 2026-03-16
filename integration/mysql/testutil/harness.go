//go:build integration

package testutil

import (
	"context"
	"database/sql"
	"testing"
	"time"

	// Register the database/sql MySQL driver.
	_ "github.com/go-sql-driver/mysql"

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

	db := openMySQLDB(tb)
	reg := registerBlueprints(tb)

	return &Harness{
		DB:       db,
		Registry: reg,
	}
}

func openMySQLDB(tb testing.TB) *sql.DB {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	tb.Cleanup(cancel)

	container, err := runMySQLContainer(ctx)
	if err != nil {
		if shouldSkipDockerError(err) {
			tb.Skipf("skip mysql integration test: %v", err)
		}
		tb.Fatalf("start mysql container: %v", err)
	}

	tb.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			tb.Errorf("terminate mysql container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		tb.Fatalf("build connection string: %v", err)
	}

	db, err := sql.Open("mysql", connStr)
	if err != nil {
		tb.Fatalf("open mysql db: %v", err)
	}

	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Errorf("close mysql db: %v", err)
		}
	})

	if err := db.PingContext(ctx); err != nil {
		tb.Fatalf("ping mysql db: %v", err)
	}

	return db
}
