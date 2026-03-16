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
