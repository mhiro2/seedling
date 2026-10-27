package quickstart

//go:generate go run github.com/mhiro2/seedling/cmd/seedling-gen sql --pkg quickstart --out blueprints_gen.go schema.sql

import (
	"context"
	"sync/atomic"

	"github.com/mhiro2/seedling"
)

var idSeq atomic.Int64

func nextID() int {
	return int(idSeq.Add(1))
}

// NewRegistry returns a local registry populated with this example's blueprints.
//
// blueprints_gen.go is owned by seedling-gen and rewritten on every run. This
// hand-written file adjusts the generated blueprints before registering them,
// so regenerating after a schema change never overwrites these callbacks.
func NewRegistry() *seedling.Registry {
	blueprints := NewBlueprints()

	blueprints.Company.Insert = func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
		v.ID = nextID() // your DB call
		return v, nil
	}

	blueprints.User.Insert = func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
		v.ID = nextID() // your DB call
		return v, nil
	}
	generatedUserDefaults := blueprints.User.Defaults
	blueprints.User.Defaults = func() User {
		user := generatedUserDefaults()
		user.Name = "test-user"
		user.Email = "test@example.com"
		return user
	}

	reg := seedling.NewRegistry()
	blueprints.MustRegister(reg)
	return reg
}

// ResetIDs resets the in-memory ID sequence used by this example package.
func ResetIDs() {
	idSeq.Store(0)
}
