package planner_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
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

func newEmptyOptionSet() *planner.OptionSet {
	return &planner.OptionSet{
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}
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

func TestPlan_OptionalBelongsToRefExpandsRelation(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	userBP, err := reg.LookupByName("user")
	if err != nil {
		t.Fatal(err)
	}
	userBP.Relations[0].Required = false

	opts := newEmptyOptionSet()
	opts.Refs["company"] = &planner.OptionSet{
		Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-company"}},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[User](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	companyNode := result.Graph.Node("user.company")
	if companyNode == nil {
		t.Fatal("expected company node")
	}
	company := companyNode.Value.(Company)
	if company.Name != "custom-company" {
		t.Fatalf("got %v, want %v", company.Name, "custom-company")
	}
}

func TestPlan_OptionalBelongsToWhenTrueExpandsRelation(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	userBP, err := reg.LookupByName("user")
	if err != nil {
		t.Fatal(err)
	}
	userBP.Relations[0].Required = false

	opts := newEmptyOptionSet()
	opts.Sets = append(opts.Sets, planner.FieldSet{Field: "Name", Value: "expand-company"})
	opts.Whens = map[string]func(any) (bool, error){
		"company": func(v any) (bool, error) {
			user, ok := v.(User)
			if !ok {
				return false, errx.ErrTypeMismatch
			}
			return user.Name == "expand-company", nil
		},
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[User](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if result.Graph.Node("user.company") == nil {
		t.Fatal("expected company node")
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
		Sets:  []planner.FieldSet{{Field: "Title", Value: "custom title"}},
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
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
				Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-project"}},
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
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
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
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
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

func TestPlan_OnlyUnknownRelationIsInvalid(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := newEmptyOptionSet()
	opts.Only = map[string]bool{"missing": true}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrRelationNotFound) {
		t.Fatalf("got %v, want %v", err, errx.ErrRelationNotFound)
	}
}

func TestPlan_NestedOnlyIsInvalid(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := newEmptyOptionSet()
	opts.Refs["project"] = &planner.OptionSet{
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
		Only:  map[string]bool{"company": true},
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
	}
}

func TestPlan_OnlyWithRefOnExcludedRelationIsInvalid(t *testing.T) {
	// Arrange: Only("project") excludes assignee, but a Ref configures it. The
	// contradiction must surface instead of silently discarding the Ref.
	reg := newMockRegistry()
	opts := newEmptyOptionSet()
	opts.Only = map[string]bool{"project": true}
	opts.Refs["assignee"] = &planner.OptionSet{
		Sets:  []planner.FieldSet{{Field: "Name", Value: "ignored"}},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
	}
	if !strings.Contains(err.Error(), "assignee") {
		t.Fatalf("expected error to mention %q, got %v", "assignee", err.Error())
	}
}

func TestPlan_EmptyOnlyWithRefIsInvalid(t *testing.T) {
	// Arrange: Only() with no relations is a non-nil empty set; a Ref on any
	// relation is still excluded and must be reported, not silently dropped.
	reg := newMockRegistry()
	opts := newEmptyOptionSet()
	opts.Only = map[string]bool{}
	opts.Refs["project"] = &planner.OptionSet{
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
	}
}

func TestPlan_OnlyWithUseOnExcludedRelationIsInvalid(t *testing.T) {
	// Arrange: Only("project") excludes assignee, but a Use provides it.
	reg := newMockRegistry()
	opts := newEmptyOptionSet()
	opts.Only = map[string]bool{"project": true}
	opts.Uses["assignee"] = User{ID: 99, Name: "existing-user"}

	// Act & Assert
	_, err := planner.Plan(reg, reflect.TypeFor[Task](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
	}
}

func TestPlan_OmitAndRefConflict(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	userBP, err := reg.LookupByName("user")
	if err != nil {
		t.Fatal(err)
	}
	userBP.Relations[0].Required = false

	opts := newEmptyOptionSet()
	opts.Omits["company"] = true
	opts.Refs["company"] = &planner.OptionSet{
		Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-company"}},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act & Assert
	_, err = planner.Plan(reg, reflect.TypeFor[User](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
	}
}

func TestPlan_OmitAndUseConflict(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	userBP, err := reg.LookupByName("user")
	if err != nil {
		t.Fatal(err)
	}
	userBP.Relations[0].Required = false

	opts := newEmptyOptionSet()
	opts.Omits["company"] = true
	opts.Uses["company"] = Company{ID: 99, Name: "existing-company"}

	// Act & Assert
	_, err = planner.Plan(reg, reflect.TypeFor[User](), opts)
	if !errors.Is(err, errx.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, errx.ErrInvalidOption)
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

func TestPlan_HasManyChildBackReferenceBindsDespiteWhen(t *testing.T) {
	// Arrange: a false When on the child's belongs_to(department) back-reference
	// must not strip the structural FK that ties an auto-generated employee to
	// the department that created it.
	reg := newMockRegistry()
	opts := newEmptyOptionSet()
	opts.Refs["employees"] = &planner.OptionSet{
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
		Whens: map[string]func(any) (bool, error){
			"department": func(any) (bool, error) { return false, nil },
		},
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
		deps := node.Dependencies()
		if len(deps) != 1 {
			t.Fatalf("%s: got %d dependencies, want 1 (department back-reference)", nodeID, len(deps))
		}
		if deps[0].Parent.ID != "department" {
			t.Fatalf("%s: got parent %q, want %q", nodeID, deps[0].Parent.ID, "department")
		}
	}
}

func TestPlan_HasManyRefAppliesToChildren(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"employees": {
				Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-employee"}},
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

func TestPlan_OptionalHasManyRefExpandsRelation(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	departmentBP, err := reg.LookupByName("department")
	if err != nil {
		t.Fatal(err)
	}
	departmentBP.Relations[0].Required = false

	opts := newEmptyOptionSet()
	opts.Refs["employees"] = &planner.OptionSet{
		Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-employee"}},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
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
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"project": {
				Uses: make(map[string]any),
				Refs: map[string]*planner.OptionSet{
					"missing": {
						Sets:  []planner.FieldSet{{Field: "Name", Value: "x"}},
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
		"article.tags[0].%join:article_tag",
		"article.tags[1]",
		"article.tags[1].%join:article_tag",
	} {
		if result.Graph.Node(nodeID) == nil {
			t.Fatalf("expected node %q", nodeID)
		}
	}

	for _, nodeID := range []string{"article.tags[0].%join:article_tag", "article.tags[1].%join:article_tag"} {
		node := result.Graph.Node(nodeID)
		if node == nil {
			t.Fatalf("expected join node %q", nodeID)
		}
		if len(node.Dependencies()) != 2 {
			t.Fatalf("%q should depend on article and tag: got len %d, want %d", nodeID, len(node.Dependencies()), 2)
		}
	}
}

func TestPlanMany_MultipleRoots(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := []*planner.OptionSet{
		{
			Sets:  []planner.FieldSet{{Field: "Name", Value: "user-1"}},
			Uses:  make(map[string]any),
			Refs:  make(map[string]*planner.OptionSet),
			Omits: make(map[string]bool),
		},
		{
			Sets:  []planner.FieldSet{{Field: "Name", Value: "user-2"}},
			Uses:  make(map[string]any),
			Refs:  make(map[string]*planner.OptionSet),
			Omits: make(map[string]bool),
		},
	}

	// Act
	result, err := planner.PlanMany(reg, reflect.TypeFor[User](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RootIDs) != 2 {
		t.Fatalf("got %d root IDs, want 2", len(result.RootIDs))
	}
	if result.RootIDs[0] != "root[0]" {
		t.Errorf("root[0] = %q, want %q", result.RootIDs[0], "root[0]")
	}
	if result.RootIDs[1] != "root[1]" {
		t.Errorf("root[1] = %q, want %q", result.RootIDs[1], "root[1]")
	}

	// Each root should have a user node
	for _, rootID := range result.RootIDs {
		node := result.Graph.Node(rootID)
		if node == nil {
			t.Fatalf("expected node %q", rootID)
		}
		if node.BlueprintName != "user" {
			t.Errorf("node %q blueprint = %q, want %q", rootID, node.BlueprintName, "user")
		}
	}

	order, err := result.Graph.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	// 2 users + shared company(s)
	if len(order) < 3 {
		t.Fatalf("got %d nodes, want >= 3", len(order))
	}
}

func TestPlanMany_NonShareableOptions(t *testing.T) {
	// Arrange: each root has a WithFn on the company relation, making it non-shareable
	reg := newMockRegistry()
	opts := []*planner.OptionSet{
		{
			Sets: []planner.FieldSet{{Field: "Name", Value: "user-1"}},
			Uses: make(map[string]any),
			Refs: map[string]*planner.OptionSet{
				"company": {
					Sets:    []planner.FieldSet{{Field: "Name", Value: "company-a"}},
					Uses:    make(map[string]any),
					Refs:    make(map[string]*planner.OptionSet),
					Omits:   make(map[string]bool),
					WithFns: []planner.WithFn{func(v any) (any, error) { return v, nil }},
				},
			},
			Omits: make(map[string]bool),
		},
		{
			Sets: []planner.FieldSet{{Field: "Name", Value: "user-2"}},
			Uses: make(map[string]any),
			Refs: map[string]*planner.OptionSet{
				"company": {
					Sets:    []planner.FieldSet{{Field: "Name", Value: "company-b"}},
					Uses:    make(map[string]any),
					Refs:    make(map[string]*planner.OptionSet),
					Omits:   make(map[string]bool),
					WithFns: []planner.WithFn{func(v any) (any, error) { return v, nil }},
				},
			},
			Omits: make(map[string]bool),
		},
	}

	// Act
	result, err := planner.PlanMany(reg, reflect.TypeFor[User](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RootIDs) != 2 {
		t.Fatalf("got %d root IDs, want 2", len(result.RootIDs))
	}

	// With WithFns, companies should NOT be shared — each user gets its own
	nodes := result.Graph.Nodes()
	companyCount := 0
	for _, n := range nodes {
		if n.BlueprintName == "company" {
			companyCount++
		}
	}
	if companyCount < 2 {
		t.Fatalf("expected at least 2 company nodes (non-shared), got %d", companyCount)
	}
}

func TestPlanMany_ShareableWithSets(t *testing.T) {
	// Arrange: both roots have the exact same static Ref opts, so companies should be shared
	reg := newMockRegistry()
	sharedRef := &planner.OptionSet{
		Sets:  []planner.FieldSet{{Field: "Name", Value: "shared-company"}},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}
	opts := []*planner.OptionSet{
		{
			Sets:  []planner.FieldSet{{Field: "Name", Value: "user-1"}},
			Uses:  make(map[string]any),
			Refs:  map[string]*planner.OptionSet{"company": sharedRef},
			Omits: make(map[string]bool),
		},
		{
			Sets:  []planner.FieldSet{{Field: "Name", Value: "user-2"}},
			Uses:  make(map[string]any),
			Refs:  map[string]*planner.OptionSet{"company": sharedRef},
			Omits: make(map[string]bool),
		},
	}

	// Act
	result, err := planner.PlanMany(reg, reflect.TypeFor[User](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}

	// With identical static options, companies should be shared — only 1 company node
	companyCount := 0
	for _, n := range result.Graph.Nodes() {
		if n.BlueprintName == "company" {
			companyCount++
		}
	}
	if companyCount != 1 {
		t.Fatalf("expected 1 shared company node, got %d", companyCount)
	}
}

func TestPlan_ManyToManyRefAppliesToChildren(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	opts := &planner.OptionSet{
		Uses: make(map[string]any),
		Refs: map[string]*planner.OptionSet{
			"tags": {
				Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-tag"}},
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

type cycleNode struct {
	ID       int64
	ParentID int64
}

type cycleLeft struct {
	ID      int64
	RightID int64
}

type cycleRight struct {
	ID     int64
	LeftID int64
}

func registerCycleSelfRef(reg *mockRegistry) {
	reg.Register(&planner.BlueprintDef{
		Name:     "cycle_node",
		Table:    "cycle_nodes",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "parent", Kind: planner.BelongsTo, LocalFields: []string{"ParentID"}, RefBlueprint: "cycle_node", Required: true},
		},
		Defaults:  func() any { return cycleNode{} },
		ModelType: reflect.TypeFor[cycleNode](),
	})
}

func registerCycleMutual(reg *mockRegistry) {
	reg.Register(&planner.BlueprintDef{
		Name:     "cycle_left",
		Table:    "cycle_lefts",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "right", Kind: planner.BelongsTo, LocalFields: []string{"RightID"}, RefBlueprint: "cycle_right", Required: true},
		},
		Defaults:  func() any { return cycleLeft{} },
		ModelType: reflect.TypeFor[cycleLeft](),
	})
	reg.Register(&planner.BlueprintDef{
		Name:     "cycle_right",
		Table:    "cycle_rights",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "left", Kind: planner.BelongsTo, LocalFields: []string{"LeftID"}, RefBlueprint: "cycle_left", Required: true},
		},
		Defaults:  func() any { return cycleRight{} },
		ModelType: reflect.TypeFor[cycleRight](),
	})
}

func TestPlan_RequiredSelfReferenceReturnsCycleError(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	registerCycleSelfRef(reg)

	// Act
	_, err := planner.Plan(reg, reflect.TypeFor[cycleNode](), nil)

	// Assert
	if !errors.Is(err, errx.ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, errx.ErrCycleDetected)
	}
}

func TestPlan_RequiredMutualReferenceReturnsCycleError(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	registerCycleMutual(reg)

	// Act
	_, err := planner.Plan(reg, reflect.TypeFor[cycleLeft](), nil)

	// Assert
	if !errors.Is(err, errx.ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, errx.ErrCycleDetected)
	}
}

func TestPlan_HasManyBackReferenceThroughBelongsToIsNotACycle(t *testing.T) {
	// Arrange: employee -> department -> employees[i]. Each generated employee
	// binds its department relation to the department that created it, so the
	// graph is finite even though employee appears twice on the expansion path.
	reg := newMockRegistry()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Employee](), nil)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Graph.Nodes()); got != 4 {
		t.Fatalf("got %d nodes, want %d", got, 4)
	}
	department := result.Graph.Node("employee.department")
	if department == nil {
		t.Fatal("expected department node")
	}
	for _, nodeID := range []string{"employee.department.employees[0]", "employee.department.employees[1]"} {
		child := result.Graph.Node(nodeID)
		if child == nil {
			t.Fatalf("expected node %q", nodeID)
		}
		if result.Graph.Node(nodeID+".department") != nil {
			t.Fatalf("%s must bind its department instead of expanding a new one", nodeID)
		}
		if !slices.ContainsFunc(child.Dependencies(), func(edge *graph.Edge) bool { return edge.Parent == department }) {
			t.Fatalf("%s must depend on the department that created it", nodeID)
		}
	}
}

type cycleCategory struct {
	ID       int64
	ParentID int64
}

func TestPlan_RequiredHasManySelfReferenceReturnsCycleError(t *testing.T) {
	// Arrange: every category creates a child category bound to it, which in
	// turn creates its own child forever.
	reg := newMockRegistry()
	reg.Register(&planner.BlueprintDef{
		Name:     "category",
		Table:    "categories",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "children", Kind: planner.HasMany, LocalFields: []string{"ParentID"}, RefBlueprint: "category", Required: true, Count: 1},
			{Name: "parent", Kind: planner.BelongsTo, LocalFields: []string{"ParentID"}, RefBlueprint: "category"},
		},
		Defaults:  func() any { return cycleCategory{} },
		ModelType: reflect.TypeFor[cycleCategory](),
	})

	// Act
	_, err := planner.Plan(reg, reflect.TypeFor[cycleCategory](), nil)

	// Assert
	if !errors.Is(err, errx.ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, errx.ErrCycleDetected)
	}
}

func TestPlan_RequiredMutualHasManyReturnsCycleError(t *testing.T) {
	// Arrange: left creates rights bound to it, and each right creates lefts
	// bound to it, so every level rebinds a fresh parent.
	reg := newMockRegistry()
	reg.Register(&planner.BlueprintDef{
		Name:     "cycle_left",
		Table:    "cycle_lefts",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "rights", Kind: planner.HasMany, LocalFields: []string{"LeftID"}, RefBlueprint: "cycle_right", Required: true, Count: 1},
			{Name: "right", Kind: planner.BelongsTo, LocalFields: []string{"RightID"}, RefBlueprint: "cycle_right"},
		},
		Defaults:  func() any { return cycleLeft{} },
		ModelType: reflect.TypeFor[cycleLeft](),
	})
	reg.Register(&planner.BlueprintDef{
		Name:     "cycle_right",
		Table:    "cycle_rights",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "lefts", Kind: planner.HasMany, LocalFields: []string{"RightID"}, RefBlueprint: "cycle_left", Required: true, Count: 1},
			{Name: "left", Kind: planner.BelongsTo, LocalFields: []string{"LeftID"}, RefBlueprint: "cycle_left"},
		},
		Defaults:  func() any { return cycleRight{} },
		ModelType: reflect.TypeFor[cycleRight](),
	})

	// Act
	_, err := planner.Plan(reg, reflect.TypeFor[cycleLeft](), nil)

	// Assert
	if !errors.Is(err, errx.ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, errx.ErrCycleDetected)
	}
}

func TestPlan_OptionalSelfReferenceExpandsOneLevel(t *testing.T) {
	// Arrange: an optional self-reference explicitly Ref'd expands exactly one
	// level and must not be misreported as a cycle.
	reg := newMockRegistry()
	reg.Register(&planner.BlueprintDef{
		Name:     "tree_node",
		Table:    "tree_nodes",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "parent", Kind: planner.BelongsTo, LocalFields: []string{"ParentID"}, RefBlueprint: "tree_node"},
		},
		Defaults:  func() any { return cycleNode{} },
		ModelType: reflect.TypeFor[cycleNode](),
	})

	opts := newEmptyOptionSet()
	opts.Refs["parent"] = newEmptyOptionSet()

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[cycleNode](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if result.Graph.Node("tree_node") == nil {
		t.Fatal("expected root node")
	}
	if result.Graph.Node("tree_node.parent") == nil {
		t.Fatal("expected one expanded parent node")
	}
	if result.Graph.Node("tree_node.parent.parent") != nil {
		t.Fatal("optional self-reference should not expand a second level")
	}
}

func TestPlan_OptionalManyToManyRefExpandsRelation(t *testing.T) {
	// Arrange
	reg := newMockRegistry()
	articleBP, err := reg.LookupByName("article")
	if err != nil {
		t.Fatal(err)
	}
	articleBP.Relations[0].Required = false

	opts := newEmptyOptionSet()
	opts.Refs["tags"] = &planner.OptionSet{
		Sets:  []planner.FieldSet{{Field: "Name", Value: "custom-tag"}},
		Uses:  make(map[string]any),
		Refs:  make(map[string]*planner.OptionSet),
		Omits: make(map[string]bool),
	}

	// Act
	result, err := planner.Plan(reg, reflect.TypeFor[Article](), opts)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []string{
		"article.tags[0]",
		"article.tags[0].%join:article_tag",
		"article.tags[1]",
		"article.tags[1].%join:article_tag",
	} {
		if result.Graph.Node(nodeID) == nil {
			t.Fatalf("expected node %q", nodeID)
		}
	}
}
