package seedling

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// PgxBeginner begins pgx transactions for [WithPgxTx].
type PgxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// PgxTxBeginner begins pgx transactions with options for [NewPgxTestSession].
type PgxTxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// NewPgxTestSession starts a pgx transaction, binds it to the session, and
// rolls it back during test cleanup.
func NewPgxTestSession[T any](tb testing.TB, reg *Registry, db PgxTxBeginner, txOptions pgx.TxOptions) Session[T] {
	tb.Helper()

	tx, err := db.BeginTx(tb.Context(), txOptions)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := tx.Rollback(tb.Context()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			tb.Errorf("seedling: rollback test transaction: %v", err)
		}
	})

	return NewSession[T](reg).WithDB(tx)
}

// WithPgxTx starts a pgx transaction and rolls it back during test cleanup.
func WithPgxTx(tb testing.TB, db PgxBeginner) pgx.Tx {
	tb.Helper()

	tx, err := db.Begin(tb.Context())
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := tx.Rollback(tb.Context()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			tb.Errorf("seedling: rollback test transaction: %v", err)
		}
	})

	return tx
}
