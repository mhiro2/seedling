package reuseparent_test

import (
	"testing"

	"github.com/mhiro2/seedling"
	reuseparent "github.com/mhiro2/seedling/examples/reuse-parent"
)

func setup(t *testing.T) {
	t.Helper()
	seedling.ResetRegistry()
	reuseparent.SetupBlueprints()
}

func TestUse_ShareParentCompany(t *testing.T) {
	setup(t)

	// First, create a shared Company.
	company := seedling.InsertOne[reuseparent.Company](t, nil,
		seedling.Set("Name", "Shared Corp"),
	)

	// Create two Projects under the same Company using Use().
	projectA := seedling.InsertOne[reuseparent.Project](t, nil,
		seedling.Set("Name", "Project Alpha"),
		seedling.Use("company", company.Root()),
	)
	projectB := seedling.InsertOne[reuseparent.Project](t, nil,
		seedling.Set("Name", "Project Beta"),
		seedling.Use("company", company.Root()),
	)

	// Both projects should reference the same Company.
	if projectA.Root().CompanyID != company.Root().ID {
		t.Fatalf("projectA.CompanyID = %d, want %d", projectA.Root().CompanyID, company.Root().ID)
	}
	if projectB.Root().CompanyID != company.Root().ID {
		t.Fatalf("projectB.CompanyID = %d, want %d", projectB.Root().CompanyID, company.Root().ID)
	}
	if projectA.Root().Name != "Project Alpha" {
		t.Fatalf("projectA.Name = %q, want %q", projectA.Root().Name, "Project Alpha")
	}
	if projectB.Root().Name != "Project Beta" {
		t.Fatalf("projectB.Name = %q, want %q", projectB.Root().Name, "Project Beta")
	}
}

func TestUse_ShareParentProject(t *testing.T) {
	setup(t)

	// Create a shared Project (which auto-creates a Company).
	project := seedling.InsertOne[reuseparent.Project](t, nil,
		seedling.Set("Name", "Shared Project"),
	)

	// Create multiple Tasks under the same Project using Use().
	task1 := seedling.InsertOne[reuseparent.Task](t, nil,
		seedling.Set("Title", "Task 1"),
		seedling.Use("project", project.Root()),
	)
	task2 := seedling.InsertOne[reuseparent.Task](t, nil,
		seedling.Set("Title", "Task 2"),
		seedling.Use("project", project.Root()),
	)
	task3 := seedling.InsertOne[reuseparent.Task](t, nil,
		seedling.Set("Title", "Task 3"),
		seedling.Use("project", project.Root()),
	)

	// All tasks should reference the same Project.
	for i, task := range []reuseparent.Task{task1.Root(), task2.Root(), task3.Root()} {
		if task.ProjectID != project.Root().ID {
			t.Fatalf("task[%d].ProjectID = %d, want %d", i, task.ProjectID, project.Root().ID)
		}
	}
}
