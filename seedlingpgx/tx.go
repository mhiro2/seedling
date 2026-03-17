package seedlingpgx

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mhiro2/seedling"
)

// Beginner begins pgx transactions for [WithTx].
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// TxBeginner begins pgx transactions with options for [NewTestSession].
type TxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// NewTestSession starts a pgx transaction, binds it to the session, and rolls
// it back during test cleanup.
func NewTestSession[T any](tb testing.TB, reg *seedling.Registry, db TxBeginner, txOptions pgx.TxOptions) seedling.Session[T] {
	tb.Helper()

	tx, err := db.BeginTx(tb.Context(), txOptions)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := tx.Rollback(tb.Context()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			tb.Errorf("seedlingpgx: rollback test transaction: %v", err)
		}
	})

	return seedling.NewSession[T](reg).WithDB(tx)
}

// WithTx starts a pgx transaction and rolls it back during test cleanup.
func WithTx(tb testing.TB, db Beginner) pgx.Tx {
	tb.Helper()

	tx, err := db.Begin(tb.Context())
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := tx.Rollback(tb.Context()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			tb.Errorf("seedlingpgx: rollback test transaction: %v", err)
		}
	})

	return tx
}
