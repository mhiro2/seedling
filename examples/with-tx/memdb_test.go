package withtx_test

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

// memdb is a tiny in-memory database/sql driver that stands in for a real
// database in this example. It understands just the statements the blueprints
// and tests issue, and gives transactions their own uncommitted view: rows
// inserted in a transaction are visible to it alone until Commit, and Rollback
// discards them.

const memDriverName = "seedling-example-memdb"

var (
	registerMemDriverOnce sync.Once
	memStores             sync.Map // DSN -> *memStore
)

// openDB returns a *sql.DB backed by a fresh in-memory store private to t.
func openDB(t *testing.T) *sql.DB {
	t.Helper()

	registerMemDriverOnce.Do(func() {
		sql.Register(memDriverName, memDriver{})
	})

	dsn := t.Name()
	memStores.Store(dsn, &memStore{committed: map[string]int{}})
	t.Cleanup(func() { memStores.Delete(dsn) })

	db, err := sql.Open(memDriverName, dsn)
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

// memStore holds the committed row counts per table, shared by all connections.
type memStore struct {
	mu        sync.Mutex
	nextID    int64
	committed map[string]int
}

type memDriver struct{}

func (memDriver) Open(dsn string) (driver.Conn, error) {
	store, ok := memStores.Load(dsn)
	if !ok {
		return nil, fmt.Errorf("memdb: unknown dsn %q", dsn)
	}
	return &memConn{store: store.(*memStore)}, nil
}

// memConn is one connection. pending is non-nil while a transaction is open
// and counts the rows that transaction inserted.
type memConn struct {
	store   *memStore
	pending map[string]int
}

func (*memConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("memdb: prepared statements are not supported: %q", query)
}

func (*memConn) Close() error { return nil }

func (c *memConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *memConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	if c.pending != nil {
		return nil, fmt.Errorf("memdb: transaction already open")
	}
	c.pending = map[string]int{}
	return memTx{conn: c}, nil
}

// QueryContext handles "INSERT INTO <table> ... RETURNING id" and
// "SELECT COUNT(*) FROM <table>".
func (c *memConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	fields := strings.Fields(query)
	switch {
	case len(fields) > 2 && strings.EqualFold(fields[0], "INSERT"):
		table := fields[2]
		c.store.mu.Lock()
		defer c.store.mu.Unlock()
		c.store.nextID++
		if c.pending != nil {
			c.pending[table]++
		} else {
			c.store.committed[table]++
		}
		return &memRows{column: "id", value: c.store.nextID}, nil

	case len(fields) == 4 && strings.EqualFold(fields[0], "SELECT"):
		table := fields[3]
		c.store.mu.Lock()
		defer c.store.mu.Unlock()
		count := c.store.committed[table] + c.pending[table]
		return &memRows{column: "count", value: int64(count)}, nil

	default:
		return nil, fmt.Errorf("memdb: unsupported query %q", query)
	}
}

type memTx struct {
	conn *memConn
}

func (tx memTx) Commit() error {
	store := tx.conn.store
	store.mu.Lock()
	defer store.mu.Unlock()
	for table, n := range tx.conn.pending {
		store.committed[table] += n
	}
	tx.conn.pending = nil
	return nil
}

func (tx memTx) Rollback() error {
	tx.conn.pending = nil
	return nil
}

// memRows is a single-row, single-column result set.
type memRows struct {
	column string
	value  int64
	done   bool
}

func (r *memRows) Columns() []string { return []string{r.column} }

func (*memRows) Close() error { return nil }

func (r *memRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = r.value
	r.done = true
	return nil
}
