package graph_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

func TestTopoSort_OrdersAcyclicGraphs(t *testing.T) {
	// Arrange
	tests := []struct {
		name      string
		build     func() *graph.Graph
		wantLen   int
		wantFirst string
		wantLast  string
	}{
		{
			name: "simple chain",
			build: func() *graph.Graph {
				g := graph.New()
				company := &graph.Node{ID: "company", BlueprintName: "company"}
				user := &graph.Node{ID: "user", BlueprintName: "user"}
				g.AddNode(company)
				g.AddNode(user)
				g.AddEdge(company, user, "CompanyID")
				return g
			},
			wantLen:   2,
			wantFirst: "company",
			wantLast:  "user",
		},
		{
			name: "diamond",
			build: func() *graph.Graph {
				g := graph.New()
				company := &graph.Node{ID: "company", BlueprintName: "company"}
				project := &graph.Node{ID: "project", BlueprintName: "project"}
				user := &graph.Node{ID: "user", BlueprintName: "user"}
				task := &graph.Node{ID: "task", BlueprintName: "task"}

				g.AddNode(task)
				g.AddNode(project)
				g.AddNode(user)
				g.AddNode(company)

				g.AddEdge(company, project, "CompanyID")
				g.AddEdge(company, user, "CompanyID")
				g.AddEdge(project, task, "ProjectID")
				g.AddEdge(user, task, "AssigneeUserID")
				return g
			},
			wantLen:   4,
			wantFirst: "company",
			wantLast:  "task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			order, err := tt.build().TopoSort()
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if len(order) != tt.wantLen {
				t.Fatalf("got len %d, want %d", len(order), tt.wantLen)
			}
			if order[0].ID != tt.wantFirst {
				t.Fatalf("got %v, want %v", order[0].ID, tt.wantFirst)
			}
			if order[len(order)-1].ID != tt.wantLast {
				t.Fatalf("got %v, want %v", order[len(order)-1].ID, tt.wantLast)
			}
		})
	}
}

func TestTopoSort_DetectsCycles(t *testing.T) {
	// Arrange
	g := graph.New()
	a := &graph.Node{ID: "a", BlueprintName: "a"}
	b := &graph.Node{ID: "b", BlueprintName: "b"}
	g.AddNode(a)
	g.AddNode(b)
	g.AddEdge(a, b, "AID")
	g.AddEdge(b, a, "BID")

	// Act
	_, err := g.TopoSort()

	// Assert
	if !errors.Is(err, errx.ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, errx.ErrCycleDetected)
	}
	msg := err.Error()
	if !strings.Contains(msg, "a") {
		t.Fatalf("expected error to contain %q, got %v", "a", msg)
	}
	if !strings.Contains(msg, "b") {
		t.Fatalf("expected error to contain %q, got %v", "b", msg)
	}
}

func TestTopoSort_UsesDeterministicNodeOrder(t *testing.T) {
	// Arrange
	g := graph.New()
	root := &graph.Node{ID: "root", BlueprintName: "root"}
	beta := &graph.Node{ID: "beta", BlueprintName: "beta"}
	alpha := &graph.Node{ID: "alpha", BlueprintName: "alpha"}

	g.AddNode(beta)
	g.AddNode(root)
	g.AddNode(alpha)

	g.AddEdge(root, beta, "RootID")
	g.AddEdge(root, alpha, "RootID")

	// Act
	order, err := g.TopoSort()
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 {
		t.Fatalf("got len %d, want %d", len(order), 3)
	}
	if order[0].ID != "root" {
		t.Fatalf("got %v, want %v", order[0].ID, "root")
	}
	if order[1].ID != "alpha" {
		t.Fatalf("got %v, want %v", order[1].ID, "alpha")
	}
	if order[2].ID != "beta" {
		t.Fatalf("got %v, want %v", order[2].ID, "beta")
	}
}

func TestGraph_RootReturnsFirstNode(t *testing.T) {
	// Arrange
	g := graph.New()
	n := &graph.Node{ID: "root", BlueprintName: "root"}
	g.AddNode(n)

	// Act & Assert
	if g.Root() != n {
		t.Fatalf("got %v, want %v", g.Root(), n)
	}
}

func TestGraph_Node(t *testing.T) {
	// Arrange
	g := graph.New()
	n := &graph.Node{ID: "users", BlueprintName: "user"}
	g.AddNode(n)

	// Act & Assert
	if g.Node("users") != n {
		t.Fatal("Node did not return the expected node")
	}
	if g.Node("nonexistent") != nil {
		t.Fatal("Node returned non-nil for missing ID")
	}
}

func TestGraph_Nodes(t *testing.T) {
	// Arrange
	g := graph.New()
	a := &graph.Node{ID: "a", BlueprintName: "a"}
	b := &graph.Node{ID: "b", BlueprintName: "b"}
	c := &graph.Node{ID: "c", BlueprintName: "c"}
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)

	// Act
	nodes := g.Nodes()

	// Assert
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = true
	}
	for _, id := range []string{"a", "b", "c"} {
		if !ids[id] {
			t.Errorf("missing node %q", id)
		}
	}
}

func TestNode_DependenciesAndDependents(t *testing.T) {
	// Arrange
	g := graph.New()
	parent := &graph.Node{ID: "parent", BlueprintName: "parent", PKField: "ID"}
	child := &graph.Node{ID: "child", BlueprintName: "child"}
	g.AddNode(parent)
	g.AddNode(child)
	g.AddEdge(parent, child, "ParentID")

	// Act & Assert
	deps := child.Dependencies()
	if len(deps) != 1 {
		t.Fatalf("got %d dependencies, want 1", len(deps))
	}
	if deps[0].Parent != parent {
		t.Fatal("dependency parent mismatch")
	}

	dependents := parent.Dependents()
	if len(dependents) != 1 {
		t.Fatalf("got %d dependents, want 1", len(dependents))
	}
	if dependents[0].Child != child {
		t.Fatal("dependent child mismatch")
	}
}

func TestNode_PrimaryKeyFields(t *testing.T) {
	tests := []struct {
		name     string
		node     graph.Node
		wantLen  int
		wantNil  bool
		wantKeys []string
	}{
		{
			name:    "no PK",
			node:    graph.Node{ID: "a"},
			wantNil: true,
		},
		{
			name:     "single PKField",
			node:     graph.Node{ID: "a", PKField: "ID"},
			wantLen:  1,
			wantKeys: []string{"ID"},
		},
		{
			name:     "multi PKFields",
			node:     graph.Node{ID: "a", PKFields: []string{"Code", "Number"}},
			wantLen:  2,
			wantKeys: []string{"Code", "Number"},
		},
		{
			name:     "PKFields takes precedence",
			node:     graph.Node{ID: "a", PKField: "ID", PKFields: []string{"A", "B"}},
			wantLen:  2,
			wantKeys: []string{"A", "B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result := tt.node.PrimaryKeyFields()

			// Assert
			if tt.wantNil {
				if result != nil {
					t.Fatalf("got %v, want nil", result)
				}
				return
			}
			if len(result) != tt.wantLen {
				t.Fatalf("got len %d, want %d", len(result), tt.wantLen)
			}
			for i, key := range tt.wantKeys {
				if result[i] != key {
					t.Errorf("result[%d] = %q, want %q", i, result[i], key)
				}
			}
		})
	}
}

func TestNode_PrimaryKeyFields_ReturnsCopy(t *testing.T) {
	// Arrange
	node := graph.Node{ID: "a", PKFields: []string{"Code", "Number"}}

	// Act
	result := node.PrimaryKeyFields()
	result[0] = "mutated"

	// Assert
	if node.PKFields[0] != "Code" {
		t.Fatal("PrimaryKeyFields did not return a copy")
	}
}

func TestClone_Simple(t *testing.T) {
	// Arrange
	type User struct {
		ID   int
		Name string
	}
	g := graph.New()
	company := &graph.Node{ID: "company", BlueprintName: "company", PKField: "ID", Value: struct{ ID int }{1}}
	user := &graph.Node{ID: "user", BlueprintName: "user", PKField: "ID", Value: User{ID: 2, Name: "Alice"}}
	g.AddNode(company)
	g.AddNode(user)
	g.AddEdge(company, user, "CompanyID")

	// Act
	cloned := g.Clone()

	// Assert
	if cloned == g {
		t.Fatal("Clone returned same graph pointer")
	}
	if cloned.Root() == g.Root() {
		t.Fatal("Clone returned same root pointer")
	}
	if cloned.Root().ID != "company" {
		t.Fatalf("cloned root ID = %q, want %q", cloned.Root().ID, "company")
	}
	if cloned.Node("user") == g.Node("user") {
		t.Fatal("Clone returned same user node pointer")
	}
	if cloned.Node("user").BlueprintName != "user" {
		t.Fatalf("cloned user blueprint = %q, want %q", cloned.Node("user").BlueprintName, "user")
	}

	// Verify edges are cloned
	userDeps := cloned.Node("user").Dependencies()
	if len(userDeps) != 1 {
		t.Fatalf("cloned user has %d dependencies, want 1", len(userDeps))
	}
	if userDeps[0].Parent != cloned.Node("company") {
		t.Fatal("cloned edge points to original node instead of cloned node")
	}
}

func TestClone_NilGraph(t *testing.T) {
	// Arrange
	var g *graph.Graph

	// Act
	cloned := g.Clone()

	// Assert
	if cloned != nil {
		t.Fatal("Clone of nil graph should return nil")
	}
}

func TestClone_ValueIndependence(t *testing.T) {
	// Arrange
	type Inner struct {
		Name string
	}
	type Item struct {
		ID    int
		Tags  []string
		Meta  map[string]string
		Inner *Inner
		Arr   [2]int
	}
	g := graph.New()
	node := &graph.Node{ID: "item", BlueprintName: "item", Value: Item{
		ID:    1,
		Tags:  []string{"a", "b"},
		Meta:  map[string]string{"x": "y"},
		Inner: &Inner{Name: "inner"},
		Arr:   [2]int{10, 20},
	}}
	g.AddNode(node)

	// Act
	cloned := g.Clone()
	clonedValue := cloned.Node("item").Value.(Item)
	clonedValue.Tags[0] = "mutated"
	clonedValue.Meta["x"] = "mutated"
	clonedValue.Inner.Name = "mutated"
	clonedValue.Arr[0] = 99

	// Assert
	origValue := g.Node("item").Value.(Item)
	if origValue.Tags[0] != "a" {
		t.Fatal("Clone did not deep-copy slice value")
	}
	if origValue.Meta["x"] != "y" {
		t.Fatal("Clone did not deep-copy map value")
	}
	if origValue.Inner.Name != "inner" {
		t.Fatal("Clone did not deep-copy pointer value")
	}
}

func TestClone_NilValue(t *testing.T) {
	// Arrange
	g := graph.New()
	node := &graph.Node{ID: "n", BlueprintName: "n", Value: nil}
	g.AddNode(node)

	// Act
	cloned := g.Clone()

	// Assert
	if cloned.Node("n").Value != nil {
		t.Fatal("Clone should preserve nil Value")
	}
}

func TestClone_NilFieldsInValue(t *testing.T) {
	// Arrange
	type S struct {
		Ptr   *int
		Slice []string
		Map   map[string]int
	}
	g := graph.New()
	node := &graph.Node{ID: "n", BlueprintName: "n", Value: S{}}
	g.AddNode(node)

	// Act
	cloned := g.Clone()

	// Assert
	v := cloned.Node("n").Value.(S)
	if v.Ptr != nil {
		t.Fatal("expected nil Ptr")
	}
	if v.Slice != nil {
		t.Fatal("expected nil Slice")
	}
	if v.Map != nil {
		t.Fatal("expected nil Map")
	}
}

func TestClone_StructWithUnexportedField(t *testing.T) {
	// Arrange
	type withUnexported struct {
		ID   int
		name string //nolint:unused // tests that clone skips unexported fields
	}
	g := graph.New()
	node := &graph.Node{ID: "n", BlueprintName: "n", Value: withUnexported{ID: 1}}
	g.AddNode(node)

	// Act
	cloned := g.Clone()

	// Assert
	v := cloned.Node("n").Value.(withUnexported)
	if v.ID != 1 {
		t.Fatalf("expected ID=1, got %d", v.ID)
	}
}

func TestClone_SetFieldsAndPKFields(t *testing.T) {
	// Arrange
	g := graph.New()
	node := &graph.Node{
		ID:            "n",
		BlueprintName: "n",
		PKFields:      []string{"A", "B"},
		SetFields:     []string{"X"},
		Value:         struct{}{},
	}
	g.AddNode(node)

	// Act
	cloned := g.Clone()

	// Assert
	cn := cloned.Node("n")
	if len(cn.PKFields) != 2 || cn.PKFields[0] != "A" {
		t.Fatal("PKFields not cloned correctly")
	}
	if len(cn.SetFields) != 1 || cn.SetFields[0] != "X" {
		t.Fatal("SetFields not cloned correctly")
	}

	// Verify independence
	cn.PKFields[0] = "mutated"
	if node.PKFields[0] != "A" {
		t.Fatal("PKFields not independent after clone")
	}
}

func TestPrune_KeepsSelectedNodes(t *testing.T) {
	// Arrange
	g := graph.New()
	a := &graph.Node{ID: "a", BlueprintName: "a", PKField: "ID"}
	b := &graph.Node{ID: "b", BlueprintName: "b"}
	c := &graph.Node{ID: "c", BlueprintName: "c"}
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.AddEdge(a, b, "AID")
	g.AddEdge(a, c, "AID")

	// Act
	pruned := g.Prune(map[string]bool{"a": true, "b": true})

	// Assert
	nodes := pruned.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}
	if pruned.Node("c") != nil {
		t.Fatal("pruned graph should not contain node c")
	}
	if pruned.Root() == nil || pruned.Root().ID != "a" {
		t.Fatal("pruned graph should have root a")
	}

	// Edge to c should be removed from a's dependents
	aDeps := pruned.Node("a").Dependents()
	if len(aDeps) != 1 {
		t.Fatalf("got %d dependents for a, want 1", len(aDeps))
	}
	if aDeps[0].Child.ID != "b" {
		t.Fatalf("expected dependent to be b, got %s", aDeps[0].Child.ID)
	}
}

func TestPrune_RootPruned(t *testing.T) {
	// Arrange
	g := graph.New()
	a := &graph.Node{ID: "a", BlueprintName: "a"}
	b := &graph.Node{ID: "b", BlueprintName: "b"}
	g.AddNode(a)
	g.AddNode(b)

	// Act
	pruned := g.Prune(map[string]bool{"b": true})

	// Assert
	if pruned.Root() != nil {
		t.Fatal("pruned graph root should be nil when original root is pruned")
	}
}
