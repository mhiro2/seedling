package seedling_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

const stubSQLDriverName = "seedling_stub_sql"

var (
	stubSQLRegisterOnce sync.Once
	stubSQLStates       sync.Map
)

type stubSQLState struct {
	mu     sync.Mutex
	count  int
	nextID int64
}

type stubSQLDriver struct{}

type stubSQLConn struct {
	state *stubSQLState
	tx    *stubSQLTxState
}

type stubSQLTxState struct {
	pendingCount int
	lastInsertID int64
}

type stubSQLTx struct {
	conn *stubSQLConn
}

type stubSQLResult struct {
	lastInsertID int64
}

type stubSQLRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func openStubSQLDB(tb testing.TB) (*sql.DB, error) {
	tb.Helper()

	stubSQLRegisterOnce.Do(func() {
		sql.Register(stubSQLDriverName, stubSQLDriver{})
	})

	dsn := tb.Name()
	stubSQLStates.Store(dsn, &stubSQLState{})

	tb.Cleanup(func() {
		stubSQLStates.Delete(dsn)
	})

	db, err := sql.Open(stubSQLDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open stub sql db: %w", err)
	}
	return db, nil
}

func (stubSQLDriver) Open(name string) (driver.Conn, error) {
	state, ok := stubSQLStates.Load(name)
	if !ok {
		return nil, fmt.Errorf("open stub sql db %q: state not found", name)
	}
	return &stubSQLConn{state: state.(*stubSQLState)}, nil
}

func (c *stubSQLConn) Prepare(query string) (driver.Stmt, error) {
	_ = c
	return nil, fmt.Errorf("prepare query %q: unsupported", query)
}

func (c *stubSQLConn) Close() error {
	_ = c
	return nil
}

func (c *stubSQLConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *stubSQLConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.tx != nil {
		return nil, fmt.Errorf("begin tx: transaction already active")
	}
	c.tx = &stubSQLTxState{}
	return &stubSQLTx{conn: c}, nil
}

func (c *stubSQLConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	normalized := strings.ToUpper(strings.TrimSpace(query))

	switch {
	case strings.HasPrefix(normalized, "CREATE TABLE"):
		return stubSQLResult{}, nil
	case strings.HasPrefix(normalized, "INSERT INTO COMPANIES"):
		if c.tx != nil {
			c.tx.pendingCount++
			c.state.mu.Lock()
			c.state.nextID++
			c.tx.lastInsertID = c.state.nextID
			c.state.mu.Unlock()
			return stubSQLResult{lastInsertID: c.tx.lastInsertID}, nil
		}

		c.state.mu.Lock()
		defer c.state.mu.Unlock()
		c.state.count++
		c.state.nextID++
		return stubSQLResult{lastInsertID: c.state.nextID}, nil
	default:
		return nil, fmt.Errorf("exec query %q: unsupported", query)
	}
}

func (c *stubSQLConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	normalized := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(normalized, "SELECT COUNT(*) FROM COMPANIES") {
		return nil, fmt.Errorf("query %q: unsupported", query)
	}

	c.state.mu.Lock()
	count := c.state.count
	c.state.mu.Unlock()

	if c.tx != nil {
		count += c.tx.pendingCount
	}

	return &stubSQLRows{
		columns: []string{"count"},
		values:  [][]driver.Value{{int64(count)}},
	}, nil
}

func (tx *stubSQLTx) Commit() error {
	if tx.conn.tx == nil {
		return fmt.Errorf("commit tx: transaction not active")
	}

	tx.conn.state.mu.Lock()
	tx.conn.state.count += tx.conn.tx.pendingCount
	tx.conn.state.mu.Unlock()
	tx.conn.tx = nil
	return nil
}

func (tx *stubSQLTx) Rollback() error {
	if tx.conn.tx == nil {
		return nil
	}
	tx.conn.tx = nil
	return nil
}

func (r stubSQLResult) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

func (r stubSQLResult) RowsAffected() (int64, error) {
	if r.lastInsertID == 0 {
		return 0, nil
	}
	return 1, nil
}

func (r *stubSQLRows) Columns() []string {
	return append([]string(nil), r.columns...)
}

func (r *stubSQLRows) Close() error {
	_ = r
	return nil
}

func (r *stubSQLRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
