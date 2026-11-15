package graph

import (
	"container/heap"
	"fmt"
	"sort"

	"github.com/mhiro2/seedling/internal/errx"
)

// TopoSort returns the nodes in topological order (dependencies first).
// Returns an error if a cycle is detected.
func (g *Graph) TopoSort() ([]*Node, error) {
	// Kahn's algorithm
	// "dependencies" edges point from child to parent (child depends on parent).
	// We want parents inserted first, so we compute in-degree based on
	// parent→child direction: a node's in-degree is the number of parents
	// it depends on (len of its dependencies edges).

	//
	// Among nodes whose dependencies are all met, the one added to the graph
	// first comes first, so records are inserted in the order they were
	// planned: the roots of a batch in index order, and the children of a
	// has-many relation in element order.
	inDegree := make(map[string]int, len(g.nodes))
	queue := make(nodeHeap, 0, len(g.nodes))
	for _, n := range g.order {
		// n has edges to its dependencies.
		// In topological terms, each dependency is a prerequisite.
		inDegree[n.ID] = len(n.dependencies)
		if len(n.dependencies) == 0 {
			heap.Push(&queue, n)
		}
	}

	var result []*Node
	for len(queue) > 0 {
		n := heap.Pop(&queue).(*Node)
		result = append(result, n)

		// For each node that depends on n (dependents edges = nodes where n is a dependency).
		for _, e := range n.dependents {
			child := e.Child
			inDegree[child.ID]--
			if inDegree[child.ID] == 0 {
				heap.Push(&queue, child)
			}
		}
	}

	if len(result) != len(g.nodes) {
		resultSet := make(map[string]bool, len(result))
		for _, n := range result {
			resultSet[n.ID] = true
		}
		var cycleNodes []string
		for _, n := range g.order {
			if !resultSet[n.ID] {
				cycleNodes = append(cycleNodes, n.ID)
			}
		}
		sort.Strings(cycleNodes)
		return nil, fmt.Errorf("topologically sort graph: %w", errx.CycleDetected(cycleNodes))
	}

	return result, nil
}

type nodeHeap []*Node

func (h nodeHeap) Len() int {
	return len(h)
}

func (h nodeHeap) Less(i, j int) bool {
	return h[i].seq < h[j].seq
}

func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *nodeHeap) Push(x any) {
	*h = append(*h, x.(*Node))
}

func (h *nodeHeap) Pop() any {
	old := *h
	n := len(old)
	node := old[n-1]
	*h = old[:n-1]
	return node
}
