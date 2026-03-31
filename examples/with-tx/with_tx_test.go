package withtx_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
	"testing"

	"github.com/mhiro2/seedling"
	withtx "github.com/mhiro2/seedling/examples/with-tx"
)

func setup(t *testing.T) {
	t.Helper()
	seedling.ResetRegistry()
	withtx.ResetIDs()
	withtx.RegisterBlueprints()
}

func TestWithTx_InsertOneUser(t *testing.T) {
	// Arrange
	setup(t)
	db := openDB(t)

	// Act
	tx := seedling.WithTx(t, db)
	user := seedling.InsertOne[withtx.User](t, tx).Root()

	// Assert
	if user.ID == 0 {
		t.Fatal("expected user ID to be set")
	}
	if user.CompanyID == 0 {
		t.Fatal("expected CompanyID to be set")
	}
	if user.Name != "test-user" {
		t.Fatalf("expected Name = %q, got %q", "test-user", user.Name)
	}
}

var registerDriverOnce sync.Once

func openDB(t *testing.T) *sql.DB {
	t.Helper()

	registerDriverOnce.Do(func() {
		sql.Register("seedling-example-tx", stubDriver{})
	})

	db, err := sql.Open("seedling-example-tx", "")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	return db
}

type stubDriver struct{}

func (stubDriver) Open(name string) (driver.Conn, error) {
	return stubConn{}, nil
}

type stubConn struct{}

func (stubConn) Prepare(query string) (driver.Stmt, error) {
	return stubStmt{}, nil
}

func (stubConn) Close() error {
	return nil
}

func (stubConn) Begin() (driver.Tx, error) {
	return stubTx{}, nil
}

func (stubConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return stubTx{}, nil
}

type stubStmt struct{}

func (stubStmt) Close() error {
	return nil
}

func (stubStmt) NumInput() int {
	return -1
}

func (stubStmt) Exec(args []driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (stubStmt) Query(args []driver.Value) (driver.Rows, error) {
	return stubRows{}, nil
}

type stubRows struct{}

func (stubRows) Columns() []string {
	return nil
}

func (stubRows) Close() error {
	return nil
}

func (stubRows) Next(dest []driver.Value) error {
	return io.EOF
}

type stubTx struct{}

func (stubTx) Commit() error {
	return nil
}

func (stubTx) Rollback() error {
	return nil
}
