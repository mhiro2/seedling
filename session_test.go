package seedling_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func TestSession_WithDB_UsesBoundDBWhenCallSiteDBIsNil(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	boundDB := &struct{ name string }{name: "bound"}
	var capturedDB seedling.DBTX

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func(*seedling.Scope) Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			capturedDB = db
			v.ID = ids.Next()
			return v, nil
		},
	})

	sess := seedling.NewSession[Company](reg).WithDB(boundDB)

	// Act
	company := sess.InsertOne(t, nil).Root()

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if boundDB != capturedDB {
		t.Fatalf("got %p, want %p (same pointer)", capturedDB, boundDB)
	}
}

func TestSession_WithDB_ExplicitDBOverridesBoundDB(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	boundDB := &struct{ name string }{name: "bound"}
	explicitDB := &struct{ name string }{name: "explicit"}
	var capturedDB seedling.DBTX

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func(*seedling.Scope) Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			capturedDB = db
			v.ID = ids.Next()
			return v, nil
		},
	})

	sess := seedling.NewSession[Company](reg).WithDB(boundDB)

	// Act
	company := sess.InsertOne(t, explicitDB).Root()

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if explicitDB != capturedDB {
		t.Fatalf("got %p, want %p (same pointer)", capturedDB, explicitDB)
	}
}

func TestNewTestSession_RollsBackTransactionOnCleanup(t *testing.T) {
	// Arrange
	db, err := openStubSQLDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	_, err = db.Exec(`CREATE TABLE companies (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}

	reg := seedlingtest.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func(*seedling.Scope) Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			tx, ok := db.(*sql.Tx)
			if !ok {
				return Company{}, fmt.Errorf("expected *sql.Tx, got %T", db)
			}

			result, err := tx.ExecContext(ctx, `INSERT INTO companies(name) VALUES (?)`, v.Name)
			if err != nil {
				return Company{}, fmt.Errorf("insert company: %w", err)
			}

			id, err := result.LastInsertId()
			if err != nil {
				return Company{}, fmt.Errorf("read inserted company id: %w", err)
			}

			v.ID = int(id)
			return v, nil
		},
	})

	// Act
	t.Run("transactional insert", func(t *testing.T) {
		// Arrange
		sess := seedling.NewTestSession[Company](t, reg, db, nil)
		plan, err := sess.BuildE()
		if err != nil {
			t.Fatal(err)
		}

		// Act
		result, err := plan.InsertE(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		company := result.Root()

		// Assert
		if company.ID == 0 {
			t.Fatal("expected non-zero ID")
		}

		tx, ok := sess.DB().(*sql.Tx)
		if !ok {
			t.Fatal("expected bound transaction")
		}

		var count int
		err = tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM companies`).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("got %v, want %v", count, 1)
		}
	})

	// Assert
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("expected zero count after rollback")
	}
}

func TestWithTx_StaysOpenUntilItsCleanup(t *testing.T) {
	// Arrange
	db, err := openStubSQLDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
	var cleanupErr error

	// Act: a cleanup registered after WithTx runs before its rollback, once
	// the test context is canceled.
	t.Run("test", func(t *testing.T) {
		tx := seedling.WithTx(t, db)
		t.Cleanup(func() {
			// database/sql rolls back a transaction whose begin context is
			// canceled from a goroutine, so give it time to do so.
			time.Sleep(50 * time.Millisecond)
			_, cleanupErr = tx.ExecContext(context.Background(), `INSERT INTO companies(name) VALUES (?)`, "late")
		})
	})

	// Assert
	if cleanupErr != nil {
		t.Fatalf("transaction closed before its cleanup: %v", cleanupErr)
	}
}
