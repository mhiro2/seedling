package debug_test

import (
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/debug"
	"github.com/mhiro2/seedling/internal/graph"
)

func TestTreeString_Simple(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company"}
	user := &graph.Node{ID: "user", BlueprintName: "user"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.TreeString(g)

	// Assert
	if !strings.Contains(out, "user") {
		t.Errorf("expected output to contain %q", "user")
	}
	if !strings.Contains(out, "company") {
		t.Errorf("expected output to contain %q", "company")
	}
}

func TestTreeString_Diamond(t *testing.T) {
	// Arrange
	g := graph.New()
	task := &graph.Node{ID: "task", BlueprintName: "task"}
	project := &graph.Node{ID: "task.project", BlueprintName: "project"}
	user := &graph.Node{ID: "task.assignee", BlueprintName: "user"}
	companyP := &graph.Node{ID: "task.project.company", BlueprintName: "company"}
	companyU := &graph.Node{ID: "task.assignee.company", BlueprintName: "company"}

	g.AddNode(task)
	g.AddNode(project)
	g.AddNode(user)
	g.AddNode(companyP)
	g.AddNode(companyU)

	g.AddEdge(project, task, "ProjectID")
	g.AddEdge(user, task, "AssigneeUserID")
	g.AddEdge(companyP, project, "CompanyID")
	g.AddEdge(companyU, user, "CompanyID")

	// Act
	out := debug.TreeString(g)
	t.Log(out)

	// Assert
	if !strings.Contains(out, "task") {
		t.Fatalf("expected output to contain %q", "task")
	}
	if !strings.Contains(out, "project") {
		t.Fatalf("expected output to contain %q", "project")
	}
	if !strings.Contains(out, "company") {
		t.Fatalf("expected output to contain %q", "company")
	}
}

func TestTreeString_Provided(t *testing.T) {
	// Arrange
	g := graph.New()
	user := &graph.Node{ID: "user", BlueprintName: "user"}
	company := &graph.Node{ID: "company", BlueprintName: "company", IsProvided: true}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.TreeString(g)

	// Assert
	if !strings.Contains(out, "[provided]") {
		t.Fatalf("expected output to contain %q", "[provided]")
	}
}

func TestTreeString_HasMany(t *testing.T) {
	// Arrange
	g := graph.New()
	department := &graph.Node{ID: "department", BlueprintName: "department"}
	employee0 := &graph.Node{ID: "department.employees[0]", BlueprintName: "employee"}
	employee1 := &graph.Node{ID: "department.employees[1]", BlueprintName: "employee"}

	g.AddNode(department)
	g.AddNode(employee0)
	g.AddNode(employee1)
	g.AddEdge(department, employee0, "DepartmentID")
	g.AddEdge(department, employee1, "DepartmentID")

	// Act
	out := debug.TreeString(g)

	// Assert
	if !strings.Contains(out, "department") {
		t.Fatalf("expected output to contain %q", "department")
	}
	if got := strings.Count(out, "employee"); got != 2 {
		t.Errorf("got %v, want %v", got, 2)
	}
}

func TestDryRunString_Simple(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Table: "companies", PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Table: "users"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.DryRunString(g)
	t.Log(out)

	// Assert
	if !strings.Contains(out, "Step 1: INSERT INTO companies (blueprint: company)") {
		t.Errorf("expected Step 1 for companies, got:\n%s", out)
	}
	if !strings.Contains(out, "Step 2: INSERT INTO users (blueprint: user)") {
		t.Errorf("expected Step 2 for users, got:\n%s", out)
	}
	if !strings.Contains(out, "SET CompanyID ← companies.ID") {
		t.Errorf("expected FK binding, got:\n%s", out)
	}
}

func TestDryRunString_Provided(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Table: "companies", PKField: "ID", IsProvided: true}
	user := &graph.Node{ID: "user", BlueprintName: "user", Table: "users"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.DryRunString(g)
	t.Log(out)

	// Assert
	if !strings.Contains(out, "SKIP companies (provided)") {
		t.Errorf("expected SKIP for provided node, got:\n%s", out)
	}
}

func TestDryRunString_Empty(t *testing.T) {
	g := graph.New()
	out := debug.DryRunString(g)
	if out != "(empty)" {
		t.Errorf("got %q, want %q", out, "(empty)")
	}
}

func TestDryRunString_CompositeFK(t *testing.T) {
	// Arrange
	g := graph.New()
	region := &graph.Node{ID: "region", BlueprintName: "region", Table: "regions", PKFields: []string{"Code", "Number"}}
	deployment := &graph.Node{ID: "deployment", BlueprintName: "deployment", Table: "deployments"}
	g.AddNode(deployment)
	g.AddNode(region)
	g.AddEdgeBindings(region, deployment, []graph.FieldBinding{
		{ParentField: "Code", ChildField: "RegionCode"},
		{ParentField: "Number", ChildField: "RegionNumber"},
	})

	// Act
	out := debug.DryRunString(g)
	t.Log(out)

	// Assert
	if !strings.Contains(out, "SET RegionCode ← regions.Code") {
		t.Errorf("expected RegionCode binding, got:\n%s", out)
	}
	if !strings.Contains(out, "SET RegionNumber ← regions.Number") {
		t.Errorf("expected RegionNumber binding, got:\n%s", out)
	}
}

func TestDryRunString_FallbackToBlueprint(t *testing.T) {
	// Arrange: no Table set, should fallback to BlueprintName
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", PKField: "ID"}
	g.AddNode(company)

	// Act
	out := debug.DryRunString(g)

	// Assert
	if !strings.Contains(out, "INSERT INTO company (blueprint: company)") {
		t.Errorf("expected fallback to blueprint name, got:\n%s", out)
	}
}
