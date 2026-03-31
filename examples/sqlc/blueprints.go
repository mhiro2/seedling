package sqlc

import (
	"context"
	"sync/atomic"

	"github.com/mhiro2/seedling"
)

// idSeq simulates auto-increment IDs that would normally come from the database.
var idSeq atomic.Int64

func nextID() int64 {
	return idSeq.Add(1)
}

// RegisterBlueprints registers Organization and Member blueprints.
//
// In a real project, the Insert functions would call sqlc-generated query methods:
//
//	Insert: func(ctx context.Context, db seedling.DBTX, v Organization) (Organization, error) {
//	    return queries.InsertOrganization(ctx, db.(*sql.DB), v)
//	}
//
// Here we use mock inserts that assign incrementing IDs.
func RegisterBlueprints() {
	seedling.MustRegister(seedling.Blueprint[Organization]{
		Name:    "organization",
		Table:   "organizations",
		PKField: "ID",
		Defaults: func() Organization {
			return Organization{Name: "test-org"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Organization) (Organization, error) {
			// In production: return queries.InsertOrganization(ctx, db.(*sql.DB), v)
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[Member]{
		Name:    "member",
		Table:   "members",
		PKField: "ID",
		Defaults: func() Member {
			return Member{Name: "test-member", Email: "member@example.com"}
		},
		Relations: []seedling.Relation{
			{
				Name:         "organization",
				Kind:         seedling.BelongsTo,
				LocalField:   "OrganizationID",
				RefBlueprint: "organization",
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Member) (Member, error) {
			// In production: return queries.InsertMember(ctx, db.(*sql.DB), v)
			v.ID = nextID()
			return v, nil
		},
	})
}
