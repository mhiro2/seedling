package seedling_test

import (
	"context"
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
		Defaults: func() referenceCountry {
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
