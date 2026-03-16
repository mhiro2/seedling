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
