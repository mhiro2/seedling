package seedling_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/mhiro2/seedling"
)

type isolatedRecord struct {
	ID    int
	Tags  []string
	Attrs map[string]string
	Score *int
}

func TestInsertMany_SetValuesAreIndependentAcrossRecords(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	nextID := 0
	seedling.MustRegisterTo(reg, seedling.Blueprint[isolatedRecord]{
		Name:    "isolated_record",
		Table:   "isolated_records",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v isolatedRecord) (isolatedRecord, error) {
			nextID++
			v.ID = nextID
			// Each insert mutates the reference values it was handed.
			v.Tags[0] = "inserted-" + strconv.Itoa(nextID)
			v.Attrs["by"] = strconv.Itoa(nextID)
			*v.Score = nextID * 10
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	tags := []string{"original"}
	attrs := map[string]string{"by": "caller"}
	score := 7

	// Act
	records := insertMany[isolatedRecord](t, nil, 2,
		seedling.Set("Tags", tags),
		seedling.Set("Attrs", attrs),
		seedling.Set("Score", &score),
	)

	// Assert
	if tags[0] != "original" || attrs["by"] != "caller" || score != 7 {
		t.Fatalf("caller values changed: tags=%v attrs=%v score=%d", tags, attrs, score)
	}
	if got := records[0].Tags[0]; got != "inserted-1" {
		t.Fatalf("records[0].Tags[0] = %q, want %q", got, "inserted-1")
	}
	if got := records[0].Attrs["by"]; got != "1" {
		t.Fatalf("records[0].Attrs[by] = %q, want %q", got, "1")
	}
	if got := *records[0].Score; got != 10 {
		t.Fatalf("*records[0].Score = %d, want %d", got, 10)
	}
	if got := records[1].Tags[0]; got != "inserted-2" {
		t.Fatalf("records[1].Tags[0] = %q, want %q", got, "inserted-2")
	}
	if records[0].Score == records[1].Score {
		t.Fatal("records share the same Score pointer")
	}
}

type pointerKeyCompany struct {
	ID *int
}

type pointerKeyMember struct {
	ID        *int
	CompanyID *int
}

func TestInsertOne_PointerForeignKeyDoesNotAliasParentKey(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	var deletedCompanyIDs []int
	seedling.MustRegisterTo(reg, seedling.Blueprint[pointerKeyCompany]{
		Name:    "pointer_key_company",
		Table:   "pointer_key_companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v pointerKeyCompany) (pointerKeyCompany, error) {
			id := 1
			v.ID = &id
			return v, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, v pointerKeyCompany) error {
			deletedCompanyIDs = append(deletedCompanyIDs, *v.ID)
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[pointerKeyMember]{
		Name:    "pointer_key_member",
		Table:   "pointer_key_members",
		PKField: "ID",
		Relations: []seedling.Relation{
			seedling.BelongsToRelation("company", "pointer_key_company", false, "CompanyID"),
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v pointerKeyMember) (pointerKeyMember, error) {
			id := 2
			v.ID = &id
			// Rewrite the FK through the pointer the member was handed.
			*v.CompanyID = 99
			return v, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, _ pointerKeyMember) error {
			return nil
		},
	})
	useTestRegistry(t, reg)

	// Act
	result, err := session[pointerKeyMember](t).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.CleanupE(t.Context(), nil); err != nil {
		t.Fatal(err)
	}

	// Assert
	company := seedling.MustNodeAs[pointerKeyCompany](result, "pointer_key_company")
	if *company.ID != 1 {
		t.Fatalf("company ID = %d, want %d", *company.ID, 1)
	}
	if got := *result.Root().CompanyID; got != 99 {
		t.Fatalf("member CompanyID = %d, want %d", got, 99)
	}
	if len(deletedCompanyIDs) != 1 || deletedCompanyIDs[0] != 1 {
		t.Fatalf("deleted company IDs = %v, want [1]", deletedCompanyIDs)
	}
}

func TestCleanup_UsesValueCapturedWhenInsertReturned(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	var insertedCompany *pointerCompany
	var deletedCompanyIDs []int
	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerCompany]{
		Name:    "pointer_company",
		Table:   "pointer_companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) (*pointerCompany, error) {
			company.ID = 1
			insertedCompany = company
			return company, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) error {
			deletedCompanyIDs = append(deletedCompanyIDs, company.ID)
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerUser]{
		Name:    "pointer_user",
		Table:   "pointer_users",
		PKField: "ID",
		Defaults: func() *pointerUser {
			return &pointerUser{}
		},
		Relations: []seedling.Relation{
			seedling.BelongsToRelation("company", "pointer_company", false, "CompanyID"),
		},
		Insert: func(_ context.Context, _ seedling.DBTX, user *pointerUser) (*pointerUser, error) {
			user.ID = 2
			// A later insert rewrites the already-inserted parent it can reach.
			insertedCompany.ID = 99
			return user, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, _ *pointerUser) error {
			return nil
		},
	})
	useTestRegistry(t, reg)

	// Act
	result, err := session[*pointerUser](t).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.CleanupE(t.Context(), nil); err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(deletedCompanyIDs) != 1 || deletedCompanyIDs[0] != 1 {
		t.Fatalf("deleted company IDs = %v, want [1]", deletedCompanyIDs)
	}
}
