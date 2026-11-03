package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/mhiro2/seedling"
)

type collectionOrder struct {
	ID     int
	Status string
}

type collectionItem struct {
	ID       int
	OrderID  int
	Position int
	Status   string
}

type collectionMember struct {
	ID   int
	Name string
}

type collectionRole struct {
	ID   int
	Name string
}

type collectionMembership struct {
	MemberID int
	RoleID   int
	Active   bool
}

// collectionDB records every insert so tests can check which rows were written.
type collectionDB struct {
	items       []collectionItem
	roles       []collectionRole
	memberships []collectionMembership
}

func newCollectionRegistry(t *testing.T, itemCount int) *seedling.Registry {
	t.Helper()

	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionOrder]{
		Name:    "order",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "items", Kind: seedling.HasMany, LocalField: "OrderID", RefBlueprint: "item", Count: itemCount, Optional: true},
		},
		Defaults: func() collectionOrder { return collectionOrder{Status: "open"} },
		Insert: func(_ context.Context, _ seedling.DBTX, v collectionOrder) (collectionOrder, error) {
			v.ID = nextID()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionItem]{
		Name:    "item",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "order", Kind: seedling.BelongsTo, LocalField: "OrderID", RefBlueprint: "order"},
		},
		Defaults: func() collectionItem { return collectionItem{Status: "open"} },
		Traits: map[string][]seedling.Option{
			"cancelled": {seedling.Set("Status", "cancelled")},
		},
		Insert: func(_ context.Context, db seedling.DBTX, v collectionItem) (collectionItem, error) {
			v.ID = nextID()
			if rec, ok := db.(*collectionDB); ok {
				rec.items = append(rec.items, v)
			}
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionMember]{
		Name:    "member",
		PKField: "ID",
		Relations: []seedling.Relation{
			{
				Name:             "roles",
				Kind:             seedling.ManyToMany,
				LocalField:       "MemberID",
				RemoteField:      "RoleID",
				RefBlueprint:     "role",
				ThroughBlueprint: "membership",
				Count:            1,
				Optional:         true,
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v collectionMember) (collectionMember, error) {
			v.ID = nextID()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionRole]{
		Name:     "role",
		PKField:  "ID",
		Defaults: func() collectionRole { return collectionRole{Name: "role"} },
		Insert: func(_ context.Context, db seedling.DBTX, v collectionRole) (collectionRole, error) {
			v.ID = nextID()
			if rec, ok := db.(*collectionDB); ok {
				rec.roles = append(rec.roles, v)
			}
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionMembership]{
		Name:     "membership",
		PKFields: []string{"MemberID", "RoleID"},
		Relations: []seedling.Relation{
			{Name: "member", Kind: seedling.BelongsTo, LocalField: "MemberID", RefBlueprint: "member"},
			{Name: "role", Kind: seedling.BelongsTo, LocalField: "RoleID", RefBlueprint: "role"},
		},
		Defaults: func() collectionMembership { return collectionMembership{Active: true} },
		Insert: func(_ context.Context, db seedling.DBTX, v collectionMembership) (collectionMembership, error) {
			if rec, ok := db.(*collectionDB); ok {
				rec.memberships = append(rec.memberships, v)
			}
			return v, nil
		},
	})
	return reg
}

func itemStatuses(items []collectionItem) []string {
	statuses := make([]string, len(items))
	for i, item := range items {
		statuses[i] = item.Status
	}
	return statuses
}

func TestRef_CountOverridesRelationCount(t *testing.T) {
	t.Parallel()

	for _, count := range []int{0, 1, 21} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			t.Parallel()

			db := &collectionDB{}
			reg := newCollectionRegistry(t, 2)
			order := seedling.NewSession[collectionOrder](reg).InsertOne(t, db,
				seedling.Ref("items", seedling.Count(count)),
			).Root()

			if len(db.items) != count {
				t.Fatalf("got %d items, want %d", len(db.items), count)
			}
			for _, item := range db.items {
				if item.OrderID != order.ID {
					t.Fatalf("got OrderID %d, want %d", item.OrderID, order.ID)
				}
			}
		})
	}
}

func TestRef_AtConfiguresSingleElement(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := newCollectionRegistry(t, 0)
	seedling.NewSession[collectionOrder](reg).InsertOne(t, db,
		seedling.Ref("items",
			seedling.Count(3),
			seedling.Set("Status", "paid"),
			seedling.At(1, seedling.BlueprintTrait("cancelled")),
		),
	)

	if got, want := itemStatuses(db.items), []string{"paid", "cancelled", "paid"}; !slices.Equal(got, want) {
		t.Fatalf("got statuses %v, want %v", got, want)
	}
}

func TestRef_ElementOptionsApplyInWrittenOrder(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := newCollectionRegistry(t, 2)
	seedling.NewSession[collectionOrder](reg).InsertOne(t, db,
		seedling.Ref("items", seedling.At(0, seedling.Set("Status", "cancelled"))),
		seedling.Ref("items", seedling.Set("Status", "paid")),
	)

	if got, want := itemStatuses(db.items), []string{"paid", "paid"}; !slices.Equal(got, want) {
		t.Fatalf("got statuses %v, want %v", got, want)
	}
}

func TestRef_EachReceivesElementIndex(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := newCollectionRegistry(t, 0)
	seedling.NewSession[collectionOrder](reg).InsertOne(t, db,
		seedling.Ref("items",
			seedling.Count(3),
			seedling.Each(func(i int) []seedling.Option {
				return []seedling.Option{seedling.Set("Position", i+1)}
			}),
		),
	)

	positions := make([]int, len(db.items))
	for i, item := range db.items {
		positions[i] = item.Position
	}
	slices.Sort(positions)
	if want := []int{1, 2, 3}; !slices.Equal(positions, want) {
		t.Fatalf("got positions %v, want %v", positions, want)
	}
}

func TestRef_ElementOptionsFromTrait(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionOrder]{
		Name:    "order",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "items", Kind: seedling.HasMany, LocalField: "OrderID", RefBlueprint: "item"},
		},
		Traits: map[string][]seedling.Option{
			"partially_cancelled": {
				seedling.Ref("items", seedling.Count(2), seedling.At(1, seedling.Set("Status", "cancelled"))),
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v collectionOrder) (collectionOrder, error) {
			v.ID = nextID()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[collectionItem]{
		Name:     "item",
		PKField:  "ID",
		Defaults: func() collectionItem { return collectionItem{Status: "open"} },
		Relations: []seedling.Relation{
			{Name: "order", Kind: seedling.BelongsTo, LocalField: "OrderID", RefBlueprint: "order"},
		},
		Insert: func(_ context.Context, db seedling.DBTX, v collectionItem) (collectionItem, error) {
			v.ID = nextID()
			db.(*collectionDB).items = append(db.(*collectionDB).items, v)
			return v, nil
		},
	})

	seedling.NewSession[collectionOrder](reg).InsertOne(t, db, seedling.BlueprintTrait("partially_cancelled"))

	if got, want := itemStatuses(db.items), []string{"open", "cancelled"}; !slices.Equal(got, want) {
		t.Fatalf("got statuses %v, want %v", got, want)
	}
}

func TestRef_ThroughConfiguresJoinRecords(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := newCollectionRegistry(t, 0)
	member := seedling.NewSession[collectionMember](reg).InsertOne(t, db,
		seedling.Ref("roles",
			seedling.Count(2),
			seedling.Set("Name", "editor"),
			seedling.At(1, seedling.Through(seedling.Set("Active", false))),
		),
	).Root()

	if len(db.roles) != 2 || len(db.memberships) != 2 {
		t.Fatalf("got %d roles and %d memberships, want 2 and 2", len(db.roles), len(db.memberships))
	}
	active := make(map[int]bool, len(db.memberships))
	for _, m := range db.memberships {
		if m.MemberID != member.ID {
			t.Fatalf("got MemberID %d, want %d", m.MemberID, member.ID)
		}
		active[m.RoleID] = m.Active
	}
	for _, role := range db.roles {
		if role.Name != "editor" {
			t.Fatalf("got role name %q, want %q", role.Name, "editor")
		}
	}
	if active[db.roles[0].ID] == active[db.roles[1].ID] {
		t.Fatalf("want exactly one inactive membership, got %v", db.memberships)
	}
}

func TestUse_ManyToManyLinksExistingRecords(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := newCollectionRegistry(t, 0)
	admin := collectionRole{ID: 9001, Name: "admin"}
	viewer := &collectionRole{ID: 9002, Name: "viewer"}

	result := seedling.NewSession[collectionMember](reg).InsertOne(t, db,
		seedling.Use("roles", []any{admin, viewer}),
		seedling.Ref("roles", seedling.At(1, seedling.Through(seedling.Set("Active", false)))),
	)
	member := result.Root()

	if len(db.roles) != 0 {
		t.Fatalf("got %d inserted roles, want none", len(db.roles))
	}
	want := []collectionMembership{
		{MemberID: member.ID, RoleID: admin.ID, Active: true},
		{MemberID: member.ID, RoleID: viewer.ID, Active: false},
	}
	got := slices.Clone(db.memberships)
	slices.SortFunc(got, func(a, b collectionMembership) int { return a.RoleID - b.RoleID })
	if !slices.Equal(got, want) {
		t.Fatalf("got memberships %v, want %v", got, want)
	}
	if roles := result.Nodes("role"); len(roles) != 2 {
		t.Fatalf("got %d role nodes, want 2 provided nodes", len(roles))
	}
}

func TestCollectionOptions_InsertManyResolvesPerRecord(t *testing.T) {
	t.Parallel()

	db := &collectionDB{}
	reg := newCollectionRegistry(t, 0)
	seedling.NewSession[collectionOrder](reg).InsertMany(t, db, 2,
		seedling.SeqRef("items", func(i int) []seedling.Option {
			return []seedling.Option{seedling.Count(i + 1)}
		}),
	)

	if len(db.items) != 3 {
		t.Fatalf("got %d items, want 3", len(db.items))
	}
}

func TestCollectionOptions_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []seedling.Option
		root string
	}{
		{name: "count outside ref", opts: []seedling.Option{seedling.Count(1)}},
		{name: "count nested in at", opts: []seedling.Option{seedling.Ref("items", seedling.Count(1), seedling.At(0, seedling.Count(2)))}},
		{name: "negative count", opts: []seedling.Option{seedling.Ref("items", seedling.Count(-1))}},
		{name: "at outside collection", opts: []seedling.Option{seedling.At(0, seedling.Set("Status", "x"))}},
		{name: "at under belongs-to ref", root: "item", opts: []seedling.Option{seedling.Ref("order", seedling.At(0))}},
		{name: "at out of range", opts: []seedling.Option{seedling.Ref("items", seedling.Count(2), seedling.At(2, seedling.Set("Status", "x")))}},
		{name: "negative at", opts: []seedling.Option{seedling.Ref("items", seedling.Count(2), seedling.At(-1))}},
		{name: "each outside collection", opts: []seedling.Option{seedling.Each(func(int) []seedling.Option { return nil })}},
		{name: "nil each", opts: []seedling.Option{seedling.Ref("items", seedling.Count(1), seedling.Each(nil))}},
		{name: "through on has-many", opts: []seedling.Option{seedling.Ref("items", seedling.Count(1), seedling.Through())}},
		{name: "options without records", opts: []seedling.Option{seedling.Ref("items", seedling.Set("Status", "x"))}},
		{name: "use on has-many", opts: []seedling.Option{seedling.Use("items", []collectionItem{{ID: 1}})}},
		{name: "use non-slice on many-to-many", root: "member", opts: []seedling.Option{seedling.Use("roles", collectionRole{ID: 1})}},
		{name: "use wrong element type", root: "member", opts: []seedling.Option{seedling.Use("roles", []collectionItem{{ID: 1}})}},
		{name: "use nil element", root: "member", opts: []seedling.Option{seedling.Use("roles", []*collectionRole{nil})}},
		{name: "use count mismatch", root: "member", opts: []seedling.Option{
			seedling.Use("roles", []collectionRole{{ID: 1}}),
			seedling.Ref("roles", seedling.Count(2)),
		}},
		{name: "use with element options", root: "member", opts: []seedling.Option{
			seedling.Use("roles", []collectionRole{{ID: 1}}),
			seedling.Ref("roles", seedling.Set("Name", "x")),
		}},
		{name: "omit with ref", opts: []seedling.Option{seedling.Omit("items"), seedling.Ref("items", seedling.Count(1))}},
		{name: "only excludes ref", opts: []seedling.Option{seedling.Only(), seedling.Ref("items", seedling.Count(1))}},
		{name: "unknown field in element", opts: []seedling.Option{seedling.Ref("items", seedling.Count(1), seedling.At(0, seedling.Set("Missing", 1)))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reg := newCollectionRegistry(t, 0)
			var err error
			switch tt.root {
			case "item":
				_, err = seedling.NewSession[collectionItem](reg).BuildE(tt.opts...)
			case "member":
				_, err = seedling.NewSession[collectionMember](reg).BuildE(tt.opts...)
			default:
				_, err = seedling.NewSession[collectionOrder](reg).BuildE(tt.opts...)
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, seedling.ErrInvalidOption) && !errors.Is(err, seedling.ErrTypeMismatch) && !errors.Is(err, seedling.ErrFieldNotFound) {
				t.Fatalf("got %v, want ErrInvalidOption, ErrTypeMismatch, or ErrFieldNotFound", err)
			}
		})
	}
}
