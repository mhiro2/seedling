package seedling_test

import (
	"context"
	"testing"

	"github.com/mhiro2/seedling"
)

func TestRegisterTo_DoesNotInvokeDefaults(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	calls := 0

	// Act
	err := seedling.RegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			calls++
			return Company{}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) { return v, nil },
	})
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("Defaults was invoked %d times during registration, want 0", calls)
	}

	result, err := seedling.NewSession[Company](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("Defaults was invoked %d times for one record, want 1", calls)
	}
	_ = result
}
