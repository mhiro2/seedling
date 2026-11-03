package seedling

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	scope    *Scope
	cleanup  *CleanupTracker
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
// pgx), use the companion package github.com/mhiro2/seedling/seedlingpgx or
// begin and defer-rollback the transaction yourself and pass it via [NewSession]
// + [Session.WithDB].
func NewTestSession[T any](tb testing.TB, reg *Registry, db TxBeginner, txOptions *sql.TxOptions) Session[T] {
	tb.Helper()

	tx, err := beginTestTx(tb, db, txOptions)
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

// WithDB returns a copy of the session bound to db. Plans built from the
// returned session retain this binding; a non-nil DB passed to Plan.Insert or
// Plan.InsertE overrides it.
func (s Session[T]) WithDB(db DBTX) Session[T] {
	s.db = db
	return s
}

// WithScope returns a copy of the session that plans records in scope, for
// the testing APIs as well as the error-returning ones. An [InScope] option
// overrides it.
func (s Session[T]) WithScope(scope *Scope) Session[T] {
	s.scope = scope
	return s
}

// WithCleanup returns a copy of the session that records every insert in
// tracker, for the testing APIs as well as the error-returning ones. A
// [WithCleanup] option overrides it.
func (s Session[T]) WithCleanup(tracker *CleanupTracker) Session[T] {
	s.cleanup = tracker
	return s
}

// forTest returns the session the testing APIs use for tb: one planning in
// the test's scope unless a scope is already bound, and recording in the
// tracker AutoCleanup enabled for the test unless one is already bound.
func (s Session[T]) forTest(tb testing.TB) Session[T] {
	tb.Helper()
	if s.scope == nil {
		s.scope = TestScope(tb)
	}
	if s.cleanup == nil {
		s.cleanup = cleanupTrackerFor(tb)
	}
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

	tx, err := beginTestTx(tb, db, nil)
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

// beginTestTx begins a transaction that outlives tb's context. database/sql
// rolls a transaction back as soon as the context it began with is canceled,
// and tb.Context() is canceled before cleanup functions run, so a transaction
// begun with it would already be gone, mid-rollback, when the cleanup that
// rolls it back, or one that still reads through it, runs.
func beginTestTx(tb testing.TB, db TxBeginner, opts *sql.TxOptions) (*sql.Tx, error) {
	tb.Helper()
	tx, err := db.BeginTx(context.WithoutCancel(tb.Context()), opts)
	if err != nil {
		return nil, fmt.Errorf("seedling: begin test transaction: %w", err)
	}
	return tx, nil
}
