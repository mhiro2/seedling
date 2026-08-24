package seedlingpgx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/mhiro2/seedling"
)

// minRollbackTimeout is the floor for the rollback budget. Tests run without a
// deadline (`go test -timeout 0`) get exactly this.
const minRollbackTimeout = 5 * time.Second

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
		ctx, cancel := rollbackContext(tb)
		defer cancel()

		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
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
		ctx, cancel := rollbackContext(tb)
		defer cancel()

		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			tb.Errorf("seedlingpgx: rollback test transaction: %v", err)
		}
	})

	return tx
}

// rollbackContext derives a context that stays active while test cleanup runs.
// tb.Context() is already canceled by the time tb.Cleanup callbacks execute, so
// the deadline has to be rebuilt rather than inherited. It follows the test
// binary's own remaining deadline so a slow connection is not cut short by an
// unrelated constant.
func rollbackContext(tb testing.TB) (context.Context, context.CancelFunc) {
	tb.Helper()
	return context.WithTimeout(context.WithoutCancel(tb.Context()), rollbackTimeout(tb))
}

func rollbackTimeout(tb testing.TB) time.Duration {
	tb.Helper()

	deadliner, ok := tb.(interface{ Deadline() (time.Time, bool) })
	if !ok {
		return minRollbackTimeout
	}
	deadline, ok := deadliner.Deadline()
	if !ok {
		return minRollbackTimeout
	}
	if remaining := time.Until(deadline); remaining > minRollbackTimeout {
		return remaining
	}
	return minRollbackTimeout
}
