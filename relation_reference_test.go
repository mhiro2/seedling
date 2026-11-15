package seedling_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mhiro2/seedling"
)

type referenceCountry struct {
	ID   int
	Code string
}

type referenceCity struct {
	ID          int
	CountryCode *string
}

func TestInsert_BelongsToReferencedField(t *testing.T) {
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[referenceCountry]{
		Name:    "reference_country",
		PKField: "ID",
		Defaults: func(*seedling.Scope) referenceCountry {
			return referenceCountry{Code: "JP"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, country referenceCountry) (referenceCountry, error) {
			country.ID = 1
			return country, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[referenceCity]{
		Name:    "reference_city",
		PKField: "ID",
		Relations: []seedling.Relation{
			{
				Name:         "country",
				Kind:         seedling.BelongsTo,
				LocalField:   "CountryCode",
				RefField:     "Code",
				RefBlueprint: "reference_country",
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, city referenceCity) (referenceCity, error) {
			city.ID = 2
			return city, nil
		},
	})

	plan, err := seedling.NewSession[referenceCity](reg).BuildE()
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate plan: %v", err)
	}
	result, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Root().CountryCode; got == nil || *got != "JP" {
		t.Fatalf("CountryCode = %v, want pointer to %q", got, "JP")
	}
}

type referenceOrg struct {
	ID   string
	Code string
}

type referenceMember struct {
	ID      int
	OrgCode string
}

// registerReferenceOrgMembers registers an org whose has-many members point
// back at it through OrgCode, which holds the org's Code rather than its ID.
func registerReferenceOrgMembers(tb testing.TB, memberOrg seedling.Relation) *seedling.Registry {
	tb.Helper()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[referenceOrg]{
		Name:    "reference_org",
		PKField: "ID",
		Defaults: func(*seedling.Scope) referenceOrg {
			return referenceOrg{Code: "ACME"}
		},
		Relations: []seedling.Relation{{
			Name: "members", Kind: seedling.HasMany, LocalField: "OrgCode", RefField: "Code",
			RefBlueprint: "reference_member", Count: 1,
		}},
		Insert: func(_ context.Context, _ seedling.DBTX, org referenceOrg) (referenceOrg, error) {
			org.ID = "org-1"
			return org, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[referenceMember]{
		Name:      "reference_member",
		PKField:   "ID",
		Relations: []seedling.Relation{memberOrg},
		Insert: func(_ context.Context, _ seedling.DBTX, member referenceMember) (referenceMember, error) {
			member.ID = 2
			return member, nil
		},
	})
	return reg
}

func TestInsert_HasManyReferencedFieldWithBackReference(t *testing.T) {
	// Arrange
	reg := registerReferenceOrgMembers(t, seedling.Relation{
		Name: "org", Kind: seedling.BelongsTo, LocalField: "OrgCode", RefField: "Code", RefBlueprint: "reference_org",
	})

	// Act
	result, err := seedling.NewSession[referenceOrg](reg).InsertOneE(t.Context(), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	member, ok := result.Node("reference_member")
	if !ok {
		t.Fatal("expected a member node")
	}
	if got := member.Value().(referenceMember).OrgCode; got != "ACME" {
		t.Fatalf("OrgCode = %q, want the org's Code %q", got, "ACME")
	}
}

func TestBuild_HasManyAndBackReferenceDisagreeOnReferencedField(t *testing.T) {
	// Arrange: members fill OrgCode from Code, but the member's own relation
	// says OrgCode holds the org's ID.
	reg := registerReferenceOrgMembers(t, seedling.Relation{
		Name: "org", Kind: seedling.BelongsTo, LocalField: "OrgCode", RefBlueprint: "reference_org",
	})

	// Act
	_, err := seedling.NewSession[referenceOrg](reg).BuildE()

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestBuild_EmptyHasManySkipsBackReferenceCheck(t *testing.T) {
	// Arrange: the definitions disagree, but no member is planned.
	reg := registerReferenceOrgMembers(t, seedling.Relation{
		Name: "org", Kind: seedling.BelongsTo, LocalField: "OrgCode", RefBlueprint: "reference_org",
	})

	// Act
	_, err := seedling.NewSession[referenceOrg](reg).BuildE(seedling.Ref("members", seedling.Count(0)))
	// Assert
	if err != nil {
		t.Fatalf("expected an empty has-many to plan, got %v", err)
	}
}
