package seedlingpgx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingpgx"
)

var errStubPgxUnsupported = errors.New("stub pgx operation")

type company struct {
	ID int
}

type stubBeginner struct {
	tx          pgx.Tx
	lastOptions pgx.TxOptions
}

func (s *stubBeginner) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.tx, nil
}

func (s *stubBeginner) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	s.lastOptions = txOptions
	return s.tx, nil
}

type stubTx struct {
	rollbackCalls int
}

func (s *stubTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return s, nil
}

func (s *stubTx) Commit(ctx context.Context) error {
	_ = s
	return nil
}

func (s *stubTx) Rollback(ctx context.Context) error {
	s.rollbackCalls++
	return nil
}

func (s *stubTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	_ = s
	return 0, errStubPgxUnsupported
}

func (s *stubTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	_ = s
	return nil
}

func (s *stubTx) LargeObjects() pgx.LargeObjects {
	_ = s
	return pgx.LargeObjects{}
}

func (s *stubTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	_ = s
	return nil, errStubPgxUnsupported
}

func (s *stubTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	_ = s
	var tag pgconn.CommandTag
	return tag, nil
}

func (s *stubTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	_ = s
	return nil, errStubPgxUnsupported
}

func (s *stubTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	_ = s
	return nil
}

func (s *stubTx) Conn() *pgx.Conn {
	_ = s
	return nil
}

func TestWithTx_RollsBackTransactionOnCleanup(t *testing.T) {
	// Arrange
	tx := &stubTx{}
	db := &stubBeginner{tx: tx}

	// Act
	t.Run("transactional helper", func(t *testing.T) {
		got := seedlingpgx.WithTx(t, db)

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

func TestNewTestSession_BindsTransactionAndRollsBackOnCleanup(t *testing.T) {
	// Arrange
	tx := &stubTx{}
	db := &stubBeginner{tx: tx}
	txOptions := pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	}

	// Act
	t.Run("session helper", func(t *testing.T) {
		sess := seedlingpgx.NewTestSession[company](t, nil, db, txOptions)

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

func TestNewTestSession_ReturnsSeedlingSession(t *testing.T) {
	// Arrange
	tx := &stubTx{}
	db := &stubBeginner{tx: tx}

	// Act
	sess := seedlingpgx.NewTestSession[company](t, nil, db, pgx.TxOptions{})

	// Assert
	if _, ok := any(sess).(seedling.Session[company]); !ok {
		t.Fatal("expected seedling.Session")
	}
}
