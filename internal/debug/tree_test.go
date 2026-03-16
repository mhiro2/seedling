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

func TestTreeString_Reused(t *testing.T) {
	// Arrange: diamond shape where company is reachable from two paths
	g := graph.New()
	task := &graph.Node{ID: "task", BlueprintName: "task"}
	project := &graph.Node{ID: "project", BlueprintName: "project"}
	user := &graph.Node{ID: "user", BlueprintName: "user"}
	company := &graph.Node{ID: "company", BlueprintName: "company"}
	g.AddNode(task)
	g.AddNode(project)
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(project, task, "ProjectID")
	g.AddEdge(user, task, "UserID")
	g.AddEdge(company, project, "CompanyID")
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.TreeString(g)
	t.Log(out)

	// Assert: company is visited from project first, then should be [reused] from user
	if !strings.Contains(out, "[reused]") {
		t.Errorf("expected output to contain %q, got:\n%s", "[reused]", out)
	}
}

func TestTreeString_SetFields(t *testing.T) {
	// Arrange
	g := graph.New()
	user := &graph.Node{ID: "user", BlueprintName: "user", SetFields: []string{"Name", "Email"}}
	g.AddNode(user)

	// Act
	out := debug.TreeString(g)

	// Assert
	if !strings.Contains(out, "Set:") {
		t.Errorf("expected output to contain SetFields, got:\n%s", out)
	}
	if !strings.Contains(out, "Email") || !strings.Contains(out, "Name") {
		t.Errorf("expected output to contain field names, got:\n%s", out)
	}
}

func TestTreeString_Empty(t *testing.T) {
	// Arrange
	g := graph.New()

	// Act
	out := debug.TreeString(g)

	// Assert
	if out != "(empty)" {
		t.Errorf("got %q, want %q", out, "(empty)")
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

func TestResultString_Simple(t *testing.T) {
	// Arrange
	type Company struct{ ID int }
	type User struct{ ID int }
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", PKField: "ID", Value: Company{ID: 42}}
	user := &graph.Node{ID: "user", BlueprintName: "user", PKField: "ID", Value: User{ID: 7}}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.ResultString(g)
	t.Log(out)

	// Assert
	if !strings.Contains(out, "user") {
		t.Errorf("expected output to contain %q", "user")
	}
	if !strings.Contains(out, "company") {
		t.Errorf("expected output to contain %q", "company")
	}
	if !strings.Contains(out, "[inserted]") {
		t.Errorf("expected output to contain %q", "[inserted]")
	}
	if !strings.Contains(out, "ID=42") {
		t.Errorf("expected output to contain PK value ID=42, got:\n%s", out)
	}
	if !strings.Contains(out, "ID=7") {
		t.Errorf("expected output to contain PK value ID=7, got:\n%s", out)
	}
}

func TestResultString_Provided(t *testing.T) {
	// Arrange
	type Company struct{ ID int }
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", PKField: "ID", Value: Company{ID: 1}, IsProvided: true}
	g.AddNode(company)

	// Act
	out := debug.ResultString(g)

	// Assert
	if !strings.Contains(out, "[provided]") {
		t.Errorf("expected output to contain %q, got:\n%s", "[provided]", out)
	}
}

func TestResultString_Empty(t *testing.T) {
	// Arrange
	g := graph.New()

	// Act
	out := debug.ResultString(g)

	// Assert
	if out != "(empty)" {
		t.Errorf("got %q, want %q", out, "(empty)")
	}
}

func TestResultString_Reused(t *testing.T) {
	// Arrange: diamond shape where company is reachable from two paths
	type Company struct{ ID int }
	type User struct{ ID int }
	type Project struct{ ID int }
	g := graph.New()
	task := &graph.Node{ID: "task", BlueprintName: "task", PKField: "ID", Value: struct{ ID int }{1}}
	project := &graph.Node{ID: "project", BlueprintName: "project", PKField: "ID", Value: Project{ID: 2}}
	user := &graph.Node{ID: "user", BlueprintName: "user", PKField: "ID", Value: User{ID: 3}}
	company := &graph.Node{ID: "company", BlueprintName: "company", PKField: "ID", Value: Company{ID: 4}}
	g.AddNode(task)
	g.AddNode(project)
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(project, task, "ProjectID")
	g.AddEdge(user, task, "UserID")
	g.AddEdge(company, project, "CompanyID")
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.ResultString(g)
	t.Log(out)

	// Assert
	if !strings.Contains(out, "[reused]") {
		t.Errorf("expected output to contain %q, got:\n%s", "[reused]", out)
	}
	if !strings.Contains(out, "[inserted]") {
		t.Errorf("expected output to contain %q", "[inserted]")
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

func TestDryRunString_ParentFallbackToBlueprint(t *testing.T) {
	// Arrange: parent has no Table, FK binding should use BlueprintName
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Table: "users"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	out := debug.DryRunString(g)

	// Assert
	if !strings.Contains(out, "SET CompanyID ← company.ID") {
		t.Errorf("expected parent fallback to blueprint name, got:\n%s", out)
	}
}
