package withtx

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mhiro2/seedling"
)

// RegisterBlueprints registers the Company and User blueprints for SQL transaction examples in reg.
//
// The Insert functions expect the *sql.Tx returned by seedling.WithTx, so every
// row a test inserts belongs to that transaction and disappears when the
// transaction is rolled back at test cleanup.
func RegisterBlueprints(reg *seedling.Registry) {
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			tx, err := txFrom(db)
			if err != nil {
				return Company{}, err
			}
			err = tx.QueryRowContext(ctx,
				"INSERT INTO companies (name) VALUES ($1) RETURNING id",
				v.Name,
			).Scan(&v.ID)
			if err != nil {
				return Company{}, fmt.Errorf("insert company: %w", err)
			}
			return v, nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() User {
			return User{Name: "test-user", Email: "test@example.com"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			tx, err := txFrom(db)
			if err != nil {
				return User{}, err
			}
			err = tx.QueryRowContext(ctx,
				"INSERT INTO users (company_id, name, email) VALUES ($1, $2, $3) RETURNING id",
				v.CompanyID, v.Name, v.Email,
			).Scan(&v.ID)
			if err != nil {
				return User{}, fmt.Errorf("insert user: %w", err)
			}
			return v, nil
		},
	})
}

// txFrom narrows the opaque seedling.DBTX handle to the *sql.Tx these blueprints require.
func txFrom(db seedling.DBTX) (*sql.Tx, error) {
	tx, ok := db.(*sql.Tx)
	if !ok {
		return nil, fmt.Errorf("withtx: expected *sql.Tx from seedling.WithTx, got %T", db)
	}
	return tx, nil
}
