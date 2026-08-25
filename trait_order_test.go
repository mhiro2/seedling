package seedling_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func TestBlueprintTrait_AppliesInWrittenOrder(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerCompanyWithTraits(t, reg, map[string][]seedling.Option{
		"big": {seedling.Set("Name", "big-corp")},
	})
	sess := seedling.NewSession[Company](reg)

	tests := []struct {
		name string
		opts []seedling.Option
		want string
	}{
		{
			name: "explicit Set after trait wins",
			opts: []seedling.Option{seedling.BlueprintTrait("big"), seedling.Set("Name", "explicit")},
			want: "explicit",
		},
		{
			name: "trait after explicit Set wins",
			opts: []seedling.Option{seedling.Set("Name", "explicit"), seedling.BlueprintTrait("big")},
			want: "big-corp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result, err := sess.InsertOneE(t.Context(), nil, tt.opts...)
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Root().Name; got != tt.want {
				t.Fatalf("got Name %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlueprintTrait_RegistryCopiesTraitDefinitions(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	traitOpts := []seedling.Option{seedling.Set("Name", "big-corp")}
	traits := map[string][]seedling.Option{"big": traitOpts}
	registerCompanyWithTraits(t, reg, traits)

	// Mutate the caller-owned map and slice after registration.
	traitOpts[0] = seedling.Set("Name", "mutated")
	traits["big"] = []seedling.Option{seedling.Set("Name", "replaced")}
	delete(traits, "big")

	// Act
	result, err := seedling.NewSession[Company](reg).InsertOneE(t.Context(), nil, seedling.BlueprintTrait("big"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Root().Name; got != "big-corp" {
		t.Fatalf("got Name %q, want %q", got, "big-corp")
	}
}

func TestBlueprintTrait_SelfReferenceIsRejected(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerCompanyWithTraits(t, reg, map[string][]seedling.Option{
		"a": {seedling.BlueprintTrait("b")},
		"b": {seedling.BlueprintTrait("a")},
	})

	// Act
	_, err := seedling.NewSession[Company](reg).BuildE(seedling.BlueprintTrait("a"))

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
	if err == nil || !strings.Contains(err.Error(), "references itself") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBlueprintTrait_CycleThroughRefIsRejected(t *testing.T) {
	// Arrange: a self-referential blueprint whose trait re-applies itself on
	// the parent it references.
	type Employee struct {
		ID        int
		ManagerID int
		Name      string
	}
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Employee]{
		Name:    "employee",
		Table:   "employees",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "manager", Kind: seedling.BelongsTo, LocalField: "ManagerID", RefBlueprint: "employee", Optional: true},
		},
		Traits: map[string][]seedling.Option{
			"chain": {seedling.Ref("manager", seedling.BlueprintTrait("chain"))},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Employee) (Employee, error) { return v, nil },
	})

	// Act
	_, err := seedling.NewSession[Employee](reg).BuildE(seedling.BlueprintTrait("chain"))

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
	if err == nil || !strings.Contains(err.Error(), "references itself") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBlueprintTrait_RegistryCopiesNestedRefOptions(t *testing.T) {
	// Arrange: the nested Ref option slice is caller-owned and mutated later.
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	registerCompanyWithTraits(t, reg, map[string][]seedling.Option{
		"big": {seedling.Set("Name", "big-corp")},
	})
	nested := []seedling.Option{seedling.BlueprintTrait("big")}
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Traits: map[string][]seedling.Option{
			"corporate": {seedling.Ref("company", nested...)},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	nested[0] = seedling.Set("Name", "mutated")

	// Act
	result, err := seedling.NewSession[User](reg).InsertOneE(t.Context(), nil, seedling.BlueprintTrait("corporate"))
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	company := seedling.MustNodeAs[Company](result, "company")
	if company.Name != "big-corp" {
		t.Fatalf("got company Name %q, want %q", company.Name, "big-corp")
	}
}
