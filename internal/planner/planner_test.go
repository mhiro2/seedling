package planner_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/planner"
	"github.com/mhiro2/seedling/internal/testutil/plannertest"
	"github.com/mhiro2/seedling/seedlingtest"
)

type (
	Company    = seedlingtest.Company
	User       = seedlingtest.User
	Project    = seedlingtest.Project
	Task       = seedlingtest.Task
	Department = seedlingtest.Department
	Employee   = seedlingtest.Employee
	Article    = seedlingtest.Article
	Tag        = seedlingtest.Tag
	ArticleTag = seedlingtest.ArticleTag
)

type mockRegistry = plannertest.PlannerRegistry

func newMockRegistry() *mockRegistry {
	r := plannertest.NewPlannerRegistry()
	r.RegisterBasic()
	r.RegisterHasMany()
	r.RegisterManyToMany()
	return r
}

func TestPlan_SimpleCompany(t *testing.T) {
	// Arrange
	reg := newMockRegistry()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Company](), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if result.Graph.Root() == nil {
		t.Fatal("expected root node")
	}
	if result.Graph.Root().BlueprintName != "company" {
		t.Fatalf("got %v, want %v", result.Graph.Root().BlueprintName, "company")
	}
}

func TestPlan_UserExpandsCompany(t *testing.T) {
	// Arrange
	reg := newMockRegistry()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[User](), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	order, err := result.Graph.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 {
		t.Fatalf("got len %d, want %d", len(order), 2)
	}
	if order[0].BlueprintName != "company" {
		t.Fatalf("got %v, want %v", order[0].BlueprintName, "company")
	}
	if order[1].BlueprintName != "user" {
		t.Fatalf("got %v, want %v", order[1].BlueprintName, "user")
	}
}

func TestPlan_TaskDiamond(t *testing.T) {
	// Arrange
	reg := newMockRegistry()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Task](), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	order, err := result.Graph.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	// task, project, user, project.company, assignee.company = 5 nodes
	// (two separate company nodes since they have different node IDs)
	if len(order) < 4 {
		t.Fatalf("got len %d, want >= %d", len(order), 4)
	}
	// Task should be last
	last := order[len(order)-1]
	if last.BlueprintName != "task" {
		t.Fatalf("got %v, want %v", last.BlueprintName, "task")
	}
}

func TestPlan_SetOption(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets:  map[string]any{"Title": "custom title"},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	root := result.Graph.Root()
	task := root.Value.(Task)
	if task.Title != "custom title" {
		t.Fatalf("got %v, want %v", task.Title, "custom title")
	}
}

func TestPlan_RefOption(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
				Sets:  map[string]any{"Name": "custom-project"},
				Uses:  make(map[string]any),
				Refs:  make(map[string]*planner.OptionSet),
				Omits: make(map[string]bool),
			},
		},
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	projectNode := result.Graph.Node("task.project")
	if projectNode == nil {
		t.Fatal("expected project node")
	}
	proj := projectNode.Value.(Project)
	if proj.Name != "custom-project" {
		t.Fatalf("got %v, want %v", proj.Name, "custom-project")
	}
}

func TestPlan_UseOption(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	existingCompany := Company{ID: 99, Name: "existing-company"}
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
				Sets: make(map[string]any),
				Uses: map[string]any{
					"company": existingCompany,
				},
				Refs:  make(map[string]*planner.OptionSet),
				Omits: make(map[string]bool),
			},
		},
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	// The project's company should be the provided one.
	companyNode := result.Graph.Node("task.project.company")
	if companyNode == nil {
		t.Fatal("expected company node for project")
	}
	if !companyNode.IsProvided {
		t.Fatal("expected company to be marked as provided")
	}
	c := companyNode.Value.(Company)
	if c.ID != 99 {
		t.Fatalf("got %v, want %v", c.ID, 99)
	}
}

func TestPlan_UseOptionTypeMismatch(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
				Sets: make(map[string]any),
				Uses: map[string]any{
					"company": Tag{},
				},
				Refs:  make(map[string]*planner.OptionSet),
				Omits: make(map[string]bool),
			},
		},
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, errx.ErrTypeMismatch)
	}
}

func TestPlan_InvalidRelation(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: map[string]any{
			"nonexistent": Company{},
		},
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrRelationNotFound) {
		t.Fatalf("got %v, want %v", err, errx.ErrRelationNotFound)
	}
	msg := err.Error()
	if !strings.Contains(msg, "available relations") {
		t.Errorf("expected error to contain %q, got %v", "available relations", msg)
	}
	if !strings.Contains(msg, "project") {
		t.Errorf("expected error to contain %q, got %v", "project", msg)
	}
	if !strings.Contains(msg, "assignee") {
		t.Errorf("expected error to contain %q, got %v", "assignee", msg)
	}
}

func TestPlan_HasManyAutoExpand(t *testing.T) {
	// Arrange
	reg := newMockRegistry()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Department](), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	order, err := result.Graph.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 {
		t.Fatalf("got len %d, want %d", len(order), 3)
	}
	if order[0].BlueprintName != "department" {
		t.Fatalf("got %v, want %v", order[0].BlueprintName, "department")
	}

	for i := 1; i < len(order); i++ {
		if order[i].BlueprintName != "employee" {
			t.Fatalf("got %v, want %v", order[i].BlueprintName, "employee")
		}
		if len(order[i].Dependencies()) != 1 {
			t.Fatalf("got len %d, want %d", len(order[i].Dependencies()), 1)
		}
		if order[i].Dependencies()[0].Parent.ID != "department" {
			t.Fatalf("got %v, want %v", order[i].Dependencies()[0].Parent.ID, "department")
		}
	}
}

func TestPlan_HasManyRefAppliesToChildren(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"employees": {
				Sets:  map[string]any{"Name": "custom-employee"},
				Uses:  make(map[string]any),
				Refs:  make(map[string]*planner.OptionSet),
				Omits: make(map[string]bool),
			},
		},
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Department](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []string{"department.employees[0]", "department.employees[1]"} {
		node := result.Graph.Node(nodeID)
		if node == nil {
			t.Fatalf("expected node %q", nodeID)
		}
		employee := node.Value.(Employee)
		if employee.Name != "custom-employee" {
			t.Fatalf("%s Name mismatch: got %v, want %v", nodeID, employee.Name, "custom-employee")
		}
	}
}

func TestPlan_NestedRefValidation(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
				Sets: make(map[string]any),
				Uses: make(map[string]any),
				Refs: map[string]*planner.OptionSet{
					"missing": {
						Sets:  map[string]any{"Name": "x"},
						Uses:  make(map[string]any),
						Refs:  make(map[string]*planner.OptionSet),
						Omits: make(map[string]bool),
					},
				},
				Omits: make(map[string]bool),
			},
		},
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrRelationNotFound) {
		t.Fatalf("got %v, want %v", err, errx.ErrRelationNotFound)
	}
}

func TestPlan_UseOnHasManyIsInvalid(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: map[string]any{
			"employees": []Employee{{ID: 1, Name: "existing"}},
		},
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Department](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
	}
}

func TestPlan_ManyToManyAutoExpand(t *testing.T) {
	// Arrange
	reg := newMockRegistry()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Article](), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []string{
		"article.tags[0]",
		"article.tags[0].article_tag",
		"article.tags[1]",
		"article.tags[1].article_tag",
	} {
		if result.Graph.Node(nodeID) == nil {
			t.Fatalf("expected node %q", nodeID)
		}
	}

	for _, nodeID := range []string{"article.tags[0].article_tag", "article.tags[1].article_tag"} {
		node := result.Graph.Node(nodeID)
		if node == nil {
			t.Fatalf("expected join node %q", nodeID)
		}
		if len(node.Dependencies()) != 2 {
			t.Fatalf("%q should depend on article and tag: got len %d, want %d", nodeID, len(node.Dependencies()), 2)
		}
	}
}

func TestPlan_ManyToManyRefAppliesToChildren(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Sets: make(map[string]any),
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"tags": {
				Sets:  map[string]any{"Name": "custom-tag"},
				Uses:  make(map[string]any),
				Refs:  make(map[string]*planner.OptionSet),
				Omits: make(map[string]bool),
			},
		},
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Article](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []string{"article.tags[0]", "article.tags[1]"} {
		node := result.Graph.Node(nodeID)
		if node == nil {
			t.Fatalf("expected node %q", nodeID)
		}
		tag := node.Value.(Tag)
		if tag.Name != "custom-tag" {
			t.Fatalf("%s Name mismatch: got %v, want %v", nodeID, tag.Name, "custom-tag")
		}
	}
}
