package seedling_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func registerCompanyWithTraits(tb testing.TB, reg *seedling.Registry, traits map[string][]seedling.Option) {
	tb.Helper()
	ids := seedlingtest.NewIDSequence()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:     "company",
		Table:    "companies",
		PKField:  "ID",
		Defaults: func() Company { return Company{Name: "default"} },
		Traits:   traits,
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
}

func TestBuildE_RejectsAfterInsertForOtherType(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerCompanyWithTraits(t, reg, nil)

	// Act
	_, err := seedling.NewSession[Company](reg).BuildE(
		seedling.AfterInsert(func(_ User, _ seedling.DBTX) {}),
	)

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
	if err == nil || !strings.Contains(err.Error(), "after-insert callback has type") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBuildE_RejectsNilAfterInsert(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	registerCompanyWithTraits(t, reg, nil)
	var cb func(Company, seedling.DBTX)

	// Act
	_, err := seedling.NewSession[Company](reg).BuildE(seedling.AfterInsert(cb))

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
	if err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
