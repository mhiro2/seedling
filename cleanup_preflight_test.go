package seedling_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func TestCleanupE_ChecksAllDeleteCallbacksBeforeDeleting(t *testing.T) {
	// Arrange: the child (user) has Delete, its parent (company) does not.
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	var deleted []string

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, v User) error {
			deleted = append(deleted, "user")
			return nil
		},
	})

	result, err := seedling.NewSession[User](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	err = result.CleanupE(t.Context(), nil)

	// Assert
	if !errors.Is(err, seedling.ErrDeleteNotDefined) {
		t.Fatalf("got %v, want %v", err, seedling.ErrDeleteNotDefined)
	}
	if len(deleted) != 0 {
		t.Fatalf("expected no deletes before the preflight error, got %v", deleted)
	}
}
