package quickstart

import (
	"context"
	"sync/atomic"

	"github.com/mhiro2/seedling"
)

var idSeq atomic.Int64

func nextID() int {
	return int(idSeq.Add(1))
}

// RegisterBlueprints registers the generated-style Company and User blueprints.
func RegisterBlueprints() {
	seedling.MustRegister(seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[User]{
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
			v.ID = nextID()
			return v, nil
		},
	})
}

// ResetIDs resets the in-memory ID sequence used by this example package.
func ResetIDs() {
	idSeq.Store(0)
}
