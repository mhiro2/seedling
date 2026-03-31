package batchinsert_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mhiro2/seedling"
	batchinsert "github.com/mhiro2/seedling/examples/batch-insert"
)

func setup(t *testing.T) {
	t.Helper()
	seedling.ResetRegistry()
	batchinsert.ResetIDs()
	batchinsert.RegisterBlueprints()
}

func TestInsertManyE_SharedProject(t *testing.T) {
	// Arrange
	setup(t)

	// Act
	result, err := seedling.InsertManyE[batchinsert.Task](context.Background(), nil, 2,
		seedling.Ref("project", seedling.Set("Name", "shared-project")),
	)
	if err != nil {
		t.Fatal(err)
	}

	project0Node, ok := result.NodeAt(0, "project")
	if !ok {
		t.Fatal("expected project for root 0")
	}
	project0, ok := project0Node.Value().(batchinsert.Project)
	if !ok {
		t.Fatalf("expected project value, got %T", project0Node.Value())
	}

	project1Node, ok := result.NodeAt(1, "project")
	if !ok {
		t.Fatal("expected project for root 1")
	}
	project1, ok := project1Node.Value().(batchinsert.Project)
	if !ok {
		t.Fatalf("expected project value, got %T", project1Node.Value())
	}

	// Assert
	if result.Len() != 2 {
		t.Fatalf("expected 2 tasks, got %d", result.Len())
	}
	if project0.Name != "shared-project" {
		t.Fatalf("expected Name = %q, got %q", "shared-project", project0.Name)
	}
	if project0.ID != project1.ID {
		t.Fatalf("expected shared project ID, got %d and %d", project0.ID, project1.ID)
	}
}

func TestInsertManyE_SeqRefCreatesDistinctProjects(t *testing.T) {
	// Arrange
	setup(t)

	// Act
	result, err := seedling.InsertManyE[batchinsert.Task](context.Background(), nil, 2,
		seedling.SeqRef("project", func(i int) []seedling.Option {
			return []seedling.Option{seedling.Set("Name", fmt.Sprintf("project-%d", i))}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	project0Node, ok := result.NodeAt(0, "project")
	if !ok {
		t.Fatal("expected project for root 0")
	}
	project0, ok := project0Node.Value().(batchinsert.Project)
	if !ok {
		t.Fatalf("expected project value, got %T", project0Node.Value())
	}

	project1Node, ok := result.NodeAt(1, "project")
	if !ok {
		t.Fatal("expected project for root 1")
	}
	project1, ok := project1Node.Value().(batchinsert.Project)
	if !ok {
		t.Fatalf("expected project value, got %T", project1Node.Value())
	}

	// Assert
	if project0.Name != "project-0" {
		t.Fatalf("expected Name = %q, got %q", "project-0", project0.Name)
	}
	if project1.Name != "project-1" {
		t.Fatalf("expected Name = %q, got %q", "project-1", project1.Name)
	}
	if project0.ID == project1.ID {
		t.Fatalf("expected distinct project IDs, got %d", project0.ID)
	}
}
