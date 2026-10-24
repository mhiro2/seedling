package withtx_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/mhiro2/seedling"
	withtx "github.com/mhiro2/seedling/examples/with-tx"
)

func setup(t *testing.T) *seedling.Registry {
	t.Helper()
	reg := seedling.NewRegistry()
	withtx.RegisterBlueprints(reg)
	return reg
}

func TestWithTx_InsertOneUser(t *testing.T) {
	// Arrange
	reg := setup(t)
	db := openDB(t)
	var tx *sql.Tx

	// Act: run the test body in a subtest so its t.Cleanup, which rolls the
	// transaction back, has finished by the time the outer test checks the database.
	// The transaction is begun with the outer test's context, which is still live
	// when the subtest ends, so only that cleanup can roll it back.
	outerCtx := t.Context()
	t.Run("inside transaction", func(t *testing.T) {
		tx = seedling.WithTx(liveContextT{T: t, ctx: outerCtx}, db)
		user := seedling.NewSession[withtx.User](reg).InsertOne(t, tx).Root()

		if user.ID == 0 {
			t.Fatal("expected user ID to be set")
		}
		if user.CompanyID == 0 {
			t.Fatal("expected CompanyID to be set")
		}
		if user.Name != "test-user" {
			t.Fatalf("expected Name = %q, got %q", "test-user", user.Name)
		}

		// The rows are visible through the transaction but not yet committed.
		assertRowCount(t, tx, "companies", 1)
		assertRowCount(t, tx, "users", 1)
		assertRowCount(t, db, "companies", 0)
		assertRowCount(t, db, "users", 0)
	})

	// Assert: WithTx rolled the transaction back, so nothing was persisted.
	if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("expected transaction to be finished by cleanup, got Commit error %v", err)
	}
	assertRowCount(t, db, "companies", 0)
	assertRowCount(t, db, "users", 0)
}

func TestWithTx_InsertRequiresTransaction(t *testing.T) {
	// Arrange
	reg := setup(t)
	db := openDB(t)

	// Act
	_, err := seedling.NewSession[withtx.Company](reg).InsertOneE(t.Context(), db)

	// Assert
	if !errors.Is(err, seedling.ErrInsertFailed) {
		t.Fatalf("expected ErrInsertFailed when passing *sql.DB instead of *sql.Tx, got %v", err)
	}
	assertRowCount(t, db, "companies", 0)
}

// liveContextT overrides Context so a transaction begun by WithTx is not
// rolled back by database/sql when the subtest's own context is canceled.
type liveContextT struct {
	*testing.T
	ctx context.Context
}

func (t liveContextT) Context() context.Context { return t.ctx }

// rowCounter is implemented by both *sql.DB and *sql.Tx.
type rowCounter interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func assertRowCount(t *testing.T, q rowCounter, table string, want int) {
	t.Helper()

	var got int
	if err := q.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("expected %d rows in %s, got %d", want, table, got)
	}
}
