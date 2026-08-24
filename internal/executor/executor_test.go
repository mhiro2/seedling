package executor_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
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

type CompanyEmbeddedFields struct {
	ID int
}

type CompanyWithNilEmbeddedID struct {
	*CompanyEmbeddedFields
	Name string
}

type CompanyWithUnexportedID struct {
	id   int
	Name string
}

type User struct {
	ID        int
	CompanyID int
	Name      string
}

type UserWithStringCompanyID struct {
	ID        int
	CompanyID string
	Name      string
}

type UserEmbeddedFields struct {
	CompanyID int
}

type UserWithNilEmbeddedCompanyID struct {
	*UserEmbeddedFields
	ID   int
	Name string
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
	result, err := executor.Execute(ctx, nil, g, lookup, nil)

	// Assert
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if got, want := len(result.Nodes), 1; got != want {
		t.Fatalf("got %d completed nodes, want %d", got, want)
	}
	if _, ok := result.Nodes["company"]; !ok {
		t.Fatal("expected successfully inserted company in partial result")
	}
	if _, ok := result.Nodes["user"]; ok {
		t.Fatal("did not expect canceled user in partial result")
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

func TestExecute_InsertErrorReturnsPartialResult(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
	g.AddNode(company)
	g.AddNode(user)
	g.AddEdge(company, user, "CompanyID")

	insertErr := errors.New("insert user")
	lookup := newTestLookup()
	lookup.bps["user"].Insert = func(ctx context.Context, db, v any) (any, error) {
		return nil, insertErr
	}

	// Act
	result, err := executor.Execute(t.Context(), nil, g, lookup, nil)

	// Assert
	if !errors.Is(err, insertErr) {
		t.Fatalf("got %v, want wrapped %v", err, insertErr)
	}
	if got, want := len(result.Nodes), 1; got != want {
		t.Fatalf("got %d completed nodes, want %d", got, want)
	}
	insertedCompany, ok := result.Nodes["company"]
	if !ok {
		t.Fatal("expected successfully inserted company in partial result")
	}
	if insertedCompany.Value.(Company).ID == 0 {
		t.Fatal("expected partial result to retain inserted company ID")
	}
	partialRoot, ok := result.Root.(Company)
	if !ok {
		t.Fatalf("got partial root %T, want Company", result.Root)
	}
	if partialRoot.ID == 0 {
		t.Fatal("expected partial result to retain the successfully inserted root")
	}
	if got, want := len(result.Graph.Nodes()), 1; got != want {
		t.Fatalf("got %d nodes in partial graph, want %d", got, want)
	}
	if result.Graph.Node("user") != nil {
		t.Fatal("partial graph must not contain the failed user node")
	}
	if root := result.Graph.Root(); root == nil || root.ID != "company" {
		t.Fatalf("partial graph root = %v, want the company node", root)
	}
}

func TestExecute_PartialGraphHasNoRootWhenRootFailed(t *testing.T) {
	// Arrange
	// The root depends on a parent, so the parent succeeds and the root does not.
	g := graph.New()
	user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
	company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	insertErr := errors.New("insert user")
	lookup := newTestLookup()
	lookup.bps["user"].Insert = func(ctx context.Context, db, v any) (any, error) {
		return nil, insertErr
	}

	// Act
	result, err := executor.Execute(t.Context(), nil, g, lookup, nil)

	// Assert
	if !errors.Is(err, insertErr) {
		t.Fatalf("got %v, want wrapped %v", err, insertErr)
	}
	if root := result.Graph.Root(); root != nil {
		t.Fatalf("partial graph root = %q, want none because the root never completed", root.ID)
	}
	if got, want := len(result.Graph.Nodes()), 1; got != want {
		t.Fatalf("got %d nodes in partial graph, want %d", got, want)
	}
	if result.Graph.Node("company") == nil {
		t.Fatal("partial graph must retain the successfully inserted company")
	}
}

func TestExecute_PreflightRejectsInvalidGraphBeforeInsert(t *testing.T) {
	tests := []struct {
		name      string
		configure func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup)
		wantError error
	}{
		{
			name: "missing child blueprint",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				delete(lookup.bps, "user")
				g.AddEdge(company, user, "CompanyID")
			},
			wantError: errx.ErrBlueprintNotFound,
		},
		{
			name: "missing foreign key field",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				g.AddEdge(company, user, "MissingCompanyID")
			},
			wantError: errx.ErrFieldNotFound,
		},
		{
			name: "missing referenced parent field",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				g.AddEdgeBindings(company, user, []graph.FieldBinding{
					{ParentField: "MissingID", ChildField: "CompanyID"},
				})
			},
			wantError: errx.ErrFieldNotFound,
		},
		{
			name: "foreign key field type mismatch",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				user.Value = UserWithStringCompanyID{Name: "wrong foreign key type"}
				lookup.bps["user"].ModelType = reflect.TypeFor[UserWithStringCompanyID]()
				g.AddEdge(company, user, "CompanyID")
			},
			wantError: errx.ErrTypeMismatch,
		},
		{
			name: "nil embedded pointer in foreign key field",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				user.Value = UserWithNilEmbeddedCompanyID{Name: "nil embedded destination"}
				lookup.bps["user"].ModelType = reflect.TypeFor[UserWithNilEmbeddedCompanyID]()
				g.AddEdge(company, user, "CompanyID")
			},
			wantError: errx.ErrInvalidOption,
		},
		{
			name: "unexported referenced parent field",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				company.Value = CompanyWithUnexportedID{id: 7, Name: "unexported source"}
				lookup.bps["company"].ModelType = reflect.TypeFor[CompanyWithUnexportedID]()
				g.AddEdgeBindings(company, user, []graph.FieldBinding{
					{ParentField: "id", ChildField: "CompanyID"},
				})
			},
			wantError: errx.ErrFieldNotFound,
		},
		{
			name: "node value type mismatch",
			configure: func(g *graph.Graph, company, user *graph.Node, lookup *mockLookup) {
				user.Value = Project{Name: "wrong type"}
				g.AddEdge(company, user, "CompanyID")
			},
			wantError: errx.ErrTypeMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			g := graph.New()
			company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
			user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
			g.AddNode(user)
			g.AddNode(company)

			lookup := newTestLookup()
			insertCalls := 0
			companyInsert := lookup.bps["company"].Insert
			lookup.bps["company"].Insert = func(ctx context.Context, db, v any) (any, error) {
				insertCalls++
				return companyInsert(ctx, db, v)
			}
			userInsert := lookup.bps["user"].Insert
			lookup.bps["user"].Insert = func(ctx context.Context, db, v any) (any, error) {
				insertCalls++
				return userInsert(ctx, db, v)
			}
			tt.configure(g, company, user, lookup)

			// Act
			result, err := executor.Execute(t.Context(), nil, g, lookup, nil)

			// Assert
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("got %v, want %v", err, tt.wantError)
			}
			if insertCalls != 0 {
				t.Fatalf("got %d Insert calls before validation failed, want 0", insertCalls)
			}
			if len(result.Nodes) != 0 {
				t.Fatalf("got %d completed nodes, want 0", len(result.Nodes))
			}
		})
	}
}

func TestExecute_ParentInsertAllocatesEmbeddedReferencedField(t *testing.T) {
	// Arrange
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Value: CompanyWithNilEmbeddedID{Name: "acme"}, PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Value: User{Name: "alice"}, PKField: "ID"}
	g.AddNode(company)
	g.AddNode(user)
	g.AddEdge(company, user, "CompanyID")

	lookup := newTestLookup()
	lookup.bps["company"].ModelType = reflect.TypeFor[CompanyWithNilEmbeddedID]()
	lookup.bps["company"].Insert = func(ctx context.Context, db, v any) (any, error) {
		c := v.(CompanyWithNilEmbeddedID)
		c.CompanyEmbeddedFields = &CompanyEmbeddedFields{ID: 11}
		return c, nil
	}

	// Act
	result, err := executor.Execute(t.Context(), nil, g, lookup, nil)
	// Assert
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	inserted, ok := result.Nodes["user"].Value.(User)
	if !ok {
		t.Fatalf("user node value = %T, want User", result.Nodes["user"].Value)
	}
	if inserted.CompanyID != 11 {
		t.Fatalf("user CompanyID = %d, want 11", inserted.CompanyID)
	}
}

func TestExecute_NilNodeValue(t *testing.T) {
	// Arrange: a child node assembled outside the planner carries a nil value
	// and depends on a parent, which would panic in assignFKs on
	// reflect.New(nil). The executor must return an error instead.
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", Value: Company{Name: "acme"}, PKField: "ID"}
	user := &graph.Node{ID: "user", BlueprintName: "user", Value: nil, PKField: "ID"}
	g.AddNode(user)
	g.AddNode(company)
	g.AddEdge(company, user, "CompanyID")

	// Act
	_, err := executor.Execute(t.Context(), nil, g, newTestLookup(), nil)

	// Assert
	if err == nil {
		t.Fatal("expected error for nil node value")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil-value error, got %v", err)
	}
}
