package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func setupCleanupBlueprints(tb testing.TB, deleted *[]string) {
	tb.Helper()
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v Company) error {
			*deleted = append(*deleted, fmt.Sprintf("company:%d", v.ID))
			return nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() User {
			return User{Name: "test-user"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v User) error {
			*deleted = append(*deleted, fmt.Sprintf("user:%d", v.ID))
			return nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[Project]{
		Name:    "project",
		Table:   "projects",
		PKField: "ID",
		Defaults: func() Project {
			return Project{Name: "test-project"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v Project) error {
			*deleted = append(*deleted, fmt.Sprintf("project:%d", v.ID))
			return nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[Task]{
		Name:    "task",
		Table:   "tasks",
		PKField: "ID",
		Defaults: func() Task {
			return Task{Title: "test-task", Status: "open"}
		},
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v Task) error {
			*deleted = append(*deleted, fmt.Sprintf("task:%d", v.ID))
			return nil
		},
	})

	useTestRegistry(tb, reg)
}

func TestCleanup(t *testing.T) {
	t.Run("deletes single record", func(t *testing.T) {
		var deleted []string
		setupCleanupBlueprints(t, &deleted)

		plan := build[Company](t)
		result := plan.Insert(t, nil)

		result.Cleanup(t, nil)

		if len(deleted) != 1 {
			t.Fatalf("expected 1 deletion, got %d: %v", len(deleted), deleted)
		}
		if deleted[0] != fmt.Sprintf("company:%d", result.Root().ID) {
			t.Fatalf("unexpected deletion: %s", deleted[0])
		}
	})

	t.Run("deletes in reverse dependency order", func(t *testing.T) {
		var deleted []string
		setupCleanupBlueprints(t, &deleted)

		plan := build[User](t)
		result := plan.Insert(t, nil)

		result.Cleanup(t, nil)

		// User depends on Company.
		// Topological insert order: company, user
		// Reverse order for cleanup: user, company
		if len(deleted) != 2 {
			t.Fatalf("expected 2 deletions, got %d: %v", len(deleted), deleted)
		}

		// User must be deleted first (child before parent)
		userID := result.Root().ID
		if deleted[0] != fmt.Sprintf("user:%d", userID) {
			t.Fatalf("expected user to be deleted first, got %s", deleted[0])
		}

		// Company must be deleted last
		company := result.MustNode("company")
		companyID := company.Value().(Company).ID
		if deleted[1] != fmt.Sprintf("company:%d", companyID) {
			t.Fatalf("expected company to be deleted last, got %s", deleted[1])
		}
	})

	t.Run("skips provided nodes", func(t *testing.T) {
		var deleted []string
		setupCleanupBlueprints(t, &deleted)

		existingCompany := Company{ID: 999, Name: "existing"}
		plan := build[User](t, seedling.Use("company", existingCompany))
		result := plan.Insert(t, nil)

		result.Cleanup(t, nil)

		// Only user should be deleted, not the provided company
		if len(deleted) != 1 {
			t.Fatalf("expected 1 deletion, got %d: %v", len(deleted), deleted)
		}
		if deleted[0] != fmt.Sprintf("user:%d", result.Root().ID) {
			t.Fatalf("unexpected deletion: %s", deleted[0])
		}
	})

	t.Run("no-op on empty result", func(t *testing.T) {
		var zero seedling.Result[Company]
		err := zero.CleanupE(t.Context(), nil)
		if err != nil {
			t.Fatalf("expected no error on empty result, got %v", err)
		}
	})
}

func TestCleanup_SnapshotDeleteFns(t *testing.T) {
	// Arrange
	var deleted []string
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v Company) error {
			deleted = append(deleted, fmt.Sprintf("company:%d", v.ID))
			return nil
		},
	})
	useTestRegistry(t, reg)

	plan := build[Company](t)
	result := plan.Insert(t, nil)

	// Act: reset registry after result creation
	reg.Reset()

	// Assert: cleanup still works using snapshotted delete functions
	err := result.CleanupE(t.Context(), nil)
	if err != nil {
		t.Fatalf("expected cleanup to succeed after registry reset, got %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deletion, got %d: %v", len(deleted), deleted)
	}
}

func TestCleanupE(t *testing.T) {
	t.Run("returns error when delete function not defined", func(t *testing.T) {
		setupBlueprints(t) // uses blueprints without Delete

		plan := build[Company](t)
		result := plan.Insert(t, nil)

		err := result.CleanupE(t.Context(), nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, seedling.ErrDeleteNotDefined) {
			t.Fatalf("expected ErrDeleteNotDefined, got %v", err)
		}
	})

	t.Run("returns error when delete callback fails", func(t *testing.T) {
		ids := seedlingtest.NewIDSequence()
		reg := seedling.NewRegistry()
		deleteErr := errors.New("delete failed")

		seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
			Name:    "company",
			Table:   "companies",
			PKField: "ID",
			Defaults: func() Company {
				return Company{Name: "test-company"}
			},
			Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
				v.ID = ids.Next()
				return v, nil
			},
			Delete: func(ctx context.Context, db seedling.DBTX, v Company) error {
				return deleteErr
			},
		})
		useTestRegistry(t, reg)

		plan := build[Company](t)
		result := plan.Insert(t, nil)

		err := result.CleanupE(t.Context(), nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, seedling.ErrDeleteFailed) {
			t.Fatalf("expected ErrDeleteFailed, got %v", err)
		}
		if !errors.Is(err, deleteErr) {
			t.Fatalf("expected wrapped deleteErr, got %v", err)
		}
	})

	t.Run("passes context and db to delete function", func(t *testing.T) {
		ids := seedlingtest.NewIDSequence()
		reg := seedling.NewRegistry()

		type ctxKey struct{}
		var capturedCtx context.Context
		var capturedDB seedling.DBTX

		seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
			Name:    "company",
			Table:   "companies",
			PKField: "ID",
			Defaults: func() Company {
				return Company{Name: "test-company"}
			},
			Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
				v.ID = ids.Next()
				return v, nil
			},
			Delete: func(ctx context.Context, db seedling.DBTX, v Company) error {
				capturedCtx = ctx
				capturedDB = db
				return nil
			},
		})
		useTestRegistry(t, reg)

		plan := build[Company](t)
		result := plan.Insert(t, nil)

		ctx := context.WithValue(t.Context(), ctxKey{}, "test-value")
		db := "test-db-handle"
		err := result.CleanupE(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedCtx.Value(ctxKey{}) != "test-value" {
			t.Fatal("context was not passed to delete function")
		}
		if capturedDB != seedling.DBTX("test-db-handle") {
			t.Fatal("db was not passed to delete function")
		}
	})
}
