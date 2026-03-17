package seedling

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TxBeginner begins SQL transactions for [NewTestSession].
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Session binds seedling operations for type T to a specific registry and optional database handle.
type Session[T any] struct {
	registry *Registry
	db       DBTX
}

// NewSession returns a typed session backed by the provided registry.
// If reg is nil, the package default registry is used.
func NewSession[T any](reg *Registry) Session[T] {
	return Session[T]{registry: resolveRegistry(reg)}
}

// NewTestSession starts a SQL transaction, binds it to the session, and rolls
// it back during test cleanup.
//
// This helper is specific to database/sql. If you use a different driver (e.g.
// pgx), use [NewPgxTestSession] / [WithPgxTx] or begin and defer-rollback the
// transaction yourself and pass it via [NewSession] + [Session.WithDB].
func NewTestSession[T any](tb testing.TB, reg *Registry, db TxBeginner, txOptions *sql.TxOptions) Session[T] {
	tb.Helper()

	tx, err := db.BeginTx(tb.Context(), txOptions)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			tb.Errorf("seedling: rollback test transaction: %v", err)
		}
	})

	return NewSession[T](reg).WithDB(tx)
}

// WithDB returns a copy of the session bound to db.
func (s Session[T]) WithDB(db DBTX) Session[T] {
	s.db = db
	return s
}

// DB returns the database handle bound to the session.
func (s Session[T]) DB() DBTX {
	return s.db
}

func (s Session[T]) resolveDB(db DBTX) DBTX {
	if db != nil {
		return db
	}
	return s.db
}

// WithTx starts a SQL transaction and rolls it back during test cleanup.
// This is a convenience wrapper for tests that need a transaction without
// creating a full [Session].
//
//	func TestSomething(t *testing.T) {
//	    tx := seedling.WithTx(t, db)
//	    result := seedling.InsertOne[Task](t, tx)
//	    // tx auto-rollbacks at cleanup
//	}
func WithTx(tb testing.TB, db TxBeginner) *sql.Tx {
	tb.Helper()

	tx, err := db.BeginTx(tb.Context(), nil)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			tb.Errorf("seedling: rollback test transaction: %v", err)
		}
	})

	return tx
}
