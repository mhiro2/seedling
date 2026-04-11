package executor_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/graph"
	"github.com/mhiro2/seedling/internal/planner"
)

type Company struct {
	ID   int
	Name string
}

type User struct {
	ID        int
	CompanyID int
	Name      string
}

type Task struct {
	ID             int
	ProjectID      int
	AssigneeUserID int
	Title          string
}

type Project struct {
	ID        int
	CompanyID int
	Name      string
}

type mockLookup struct {
	bps map[string]*planner.BlueprintDef
}

func (m *mockLookup) LookupByName(name string) (*planner.BlueprintDef, error) {
	bp, ok := m.bps[name]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint %q: %w", name, errx.BlueprintNotFound(name))
	}
	return bp, nil
}

func newTestLookup() *mockLookup {
	idCounter := 0
	return &mockLookup{
		bps: map[string]*planner.BlueprintDef{
			"company": {
				Name:     "company",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					idCounter++
					c := v.(Company)
					c.ID = idCounter
					return c, nil
				},
				ModelType: reflect.TypeFor[Company](),
			},
			"user": {
				Name:     "user",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					idCounter++
					u := v.(User)
					u.ID = idCounter
					return u, nil
				},
				ModelType: reflect.TypeFor[User](),
			},
			"project": {
				Name:     "project",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					idCounter++
					p := v.(Project)
					p.ID = idCounter
					return p, nil
				},
				ModelType: reflect.TypeFor[Project](),
			},
			"task": {
				Name:     "task",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					idCounter++
					tk := v.(Task)
					tk.ID = idCounter
					return tk, nil
				},
				ModelType: reflect.TypeFor[Task](),
			},
		},
	}
}

func TestExecute_InsertsAndAssignsForeignKeys(t *testing.T) {
	tests := []struct {
		name      string
		build     func() *graph.Graph
		assertion func(t *testing.T, result *executor.Result)
	}{
		{
			name: "inserts parent before child",
			build: func() *graph.Graph {
				g := graph.New()
				company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
				user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
				g.AddNode(user)
				g.AddNode(company)
				g.AddEdge(company, user, "CompanyID")
				return g
			},
			assertion: func(t *testing.T, result *executor.Result) {
				t.Helper()
				// Assert
				u := result.Nodes["user"].Value.(User)
				c := result.Nodes["company"].Value.(Company)
				if c.ID == 0 {
					t.Fatal("company ID should be set")
				}
				if u.CompanyID != c.ID {
					t.Fatalf("user.CompanyID should equal company.ID: got %v, want %v", u.CompanyID, c.ID)
				}
			},
		},
		{
			name: "provided node skips insert",
			build: func() *graph.Graph {
				g := graph.New()
				company := &graph.Node{
					ID:            "company",
					BlueprintName: "company",
					Value:         Company{ID: 99, Name: "existing"},
					PKField:       "ID",
					IsProvided:    true,
				}
				user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "bob"}, PKField: "ID"}
				g.AddNode(user)
				g.AddNode(company)
				g.AddEdge(company, user, "CompanyID")
				return g
			},
			assertion: func(t *testing.T, result *executor.Result) {
				t.Helper()
				// Assert
				c := result.Nodes["company"].Value.(Company)
				if c.ID != 99 {
					t.Fatalf("provided company ID should remain 99: got %v, want %v", c.ID, 99)
				}
				u := result.Nodes["user"].Value.(User)
				if u.CompanyID != 99 {
					t.Fatalf("user.CompanyID should be 99: got %v, want %v", u.CompanyID, 99)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g := tt.build()

			// Act
			result, err := executor.Execute(t.Context(), nil, g, newTestLookup(), nil)
			if err != nil {
				t.Fatal(err)
			}

			// Assert
			tt.assertion(t, result)
		})
	}
}

func TestExecute_WithLogFn(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Table: "companies", Value: Company{Name: "acme"}, PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Table: "users", Value: User{Name: "alice"}, PKField: "ID"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	var logs []executor.LogEntry

	// Act
	result, err := executor.Execute(t.Context(), nil, g, newTestLookup(), func(entry executor.LogEntry) {
		logs = append(logs, entry)
	})
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("got %d log entries, want 2", len(logs))
	}

	// First log should be company (no FK bindings)
	if logs[0].Blueprint != "company" {
		t.Errorf("log[0] blueprint = %q, want %q", logs[0].Blueprint, "company")
	}
	if logs[0].Step != 1 {
		t.Errorf("log[0] step = %d, want 1", logs[0].Step)
	}
	if len(logs[0].FKBindings) != 0 {
		t.Errorf("log[0] should have 0 FK bindings, got %d", len(logs[0].FKBindings))
	}

	// Second log should be user with FK binding to company
	if logs[1].Blueprint != "user" {
		t.Errorf("log[1] blueprint = %q, want %q", logs[1].Blueprint, "user")
	}
	if len(logs[1].FKBindings) != 1 {
		t.Fatalf("log[1] should have 1 FK binding, got %d", len(logs[1].FKBindings))
	}
	binding := logs[1].FKBindings[0]
	if binding.ChildField != "CompanyID" {
		t.Errorf("binding child field = %q, want %q", binding.ChildField, "CompanyID")
	}
	if binding.ParentBlueprint != "company" {
		t.Errorf("binding parent blueprint = %q, want %q", binding.ParentBlueprint, "company")
	}
	if binding.ParentTable != "companies" {
		t.Errorf("binding parent table = %q, want %q", binding.ParentTable, "companies")
	}

	// FK value should be set
	companyID := result.Nodes["company"].Value.(Company).ID
	if binding.Value != companyID {
		t.Errorf("binding value = %v, want %v", binding.Value, companyID)
	}
}

func TestExecute_ContextCanceled(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Act
	_, err := executor.Execute(ctx, nil, g, newTestLookup(), nil)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestExecute_ContextCanceledDuringInsert(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	ctx, cancel := context.WithCancel(context.Background())

	lookup := &mockLookup{
		bps: map[string]*planner.BlueprintDef{
			"company": {
				Name:     "company",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					cancel() // cancel after first insert
					c := v.(Company)
					c.ID = 1
					return c, nil
				},
				ModelType: reflect.TypeFor[Company](),
			},
			"user": {
				Name:     "user",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					u := v.(User)
					u.ID = 2
					return u, nil
				},
				ModelType: reflect.TypeFor[User](),
			},
		},
	}

	// Act
	_, err := executor.Execute(ctx, nil, g, lookup, nil)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestExecute_InsertError(t *testing.T) {
	// Arrange
	g := graph.New()
	g.AddNode(&graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "fail"}, PKField: "ID"})

	lookup := &mockLookup{
		bps: map[string]*planner.BlueprintDef{
			"company": {
				Name:     "company",
				PKFields: []string{"ID"},
				Insert: func(ctx context.Context, db, v any) (any, error) {
					return nil, errors.New("db error")
				},
			},
		},
	}

	// Act
	_, err := executor.Execute(t.Context(), nil, g, lookup, nil)

	// Assert
	if !errors.Is(err, errx.ErrInsertFailed) {
		t.Fatalf("got %v, want %v", err, errx.ErrInsertFailed)
	}
}
