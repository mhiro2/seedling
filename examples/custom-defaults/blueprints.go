package customdefaults

import (
	"context"
	"sync/atomic"

	"github.com/mhiro2/seedling"
)

var idSeq atomic.Int64

func nextID() int {
	return int(idSeq.Add(1))
}

// SetupBlueprints registers the User blueprint with sensible defaults.
func SetupBlueprints() {
	seedling.MustRegister(seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() User {
			return User{
				Name:   "test-user",
				Email:  "test@example.com",
				Role:   "member",
				Status: "active",
			}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = nextID()
			return v, nil
		},
	})
}

// AdminUser returns a With option that creates a user with admin role.
func AdminUser() seedling.Option {
	return seedling.With(func(u *User) {
		u.Role = "admin"
		u.Email = "admin@example.com"
	})
}

// InactiveUser returns a With option that creates an inactive user.
func InactiveUser() seedling.Option {
	return seedling.With(func(u *User) {
		u.Status = "inactive"
	})
}
