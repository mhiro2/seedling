package seedling_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mhiro2/seedling"
)

var errStubPgxUnsupported = errors.New("stub pgx operation")

type stubPgxBeginner struct {
	tx          pgx.Tx
	lastOptions pgx.TxOptions
}

func (s *stubPgxBeginner) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.tx, nil
}

func (s *stubPgxBeginner) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	s.lastOptions = txOptions
	return s.tx, nil
}

type stubPgxTx struct {
	rollbackCalls int
}

func (s *stubPgxTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return s, nil
}

func (s *stubPgxTx) Commit(ctx context.Context) error {
	_ = s
	return nil
}

func (s *stubPgxTx) Rollback(ctx context.Context) error {
	s.rollbackCalls++
	return nil
}

func (s *stubPgxTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	_ = s
	return 0, errStubPgxUnsupported
}

func (s *stubPgxTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	_ = s
	return nil
}

func (s *stubPgxTx) LargeObjects() pgx.LargeObjects {
	_ = s
	return pgx.LargeObjects{}
}

func (s *stubPgxTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	_ = s
	return nil, errStubPgxUnsupported
}

func (s *stubPgxTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	_ = s
	var tag pgconn.CommandTag
	return tag, nil
}

func (s *stubPgxTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	_ = s
	return nil, errStubPgxUnsupported
}

func (s *stubPgxTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	_ = s
	return nil
}

func (s *stubPgxTx) Conn() *pgx.Conn {
	_ = s
	return nil
}

func TestWithPgxTx_RollsBackTransactionOnCleanup(t *testing.T) {
	// Arrange
	tx := &stubPgxTx{}
	db := &stubPgxBeginner{tx: tx}

	// Act
	t.Run("transactional helper", func(t *testing.T) {
		got := seedling.WithPgxTx(t, db)

		// Assert
		if got != tx {
			t.Fatalf("got %v, want %v", got, tx)
		}
	})

	// Assert
	if got, want := tx.rollbackCalls, 1; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNewPgxTestSession_BindsTransactionAndRollsBackOnCleanup(t *testing.T) {
	// Arrange
	tx := &stubPgxTx{}
	db := &stubPgxBeginner{tx: tx}
	txOptions := pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	}

	// Act
	t.Run("session helper", func(t *testing.T) {
		sess := seedling.NewPgxTestSession[Company](t, nil, db, txOptions)

		// Assert
		bound, ok := sess.DB().(pgx.Tx)
		if !ok {
			t.Fatalf("expected pgx.Tx, got %T", sess.DB())
		}
		if bound != tx {
			t.Fatalf("got %v, want %v", bound, tx)
		}
	})

	// Assert
	if got, want := db.lastOptions.AccessMode, pgx.ReadOnly; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, want := tx.rollbackCalls, 1; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}
