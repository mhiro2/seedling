package graph

// Prune returns a subgraph containing only the nodes whose IDs are in keepIDs.
// Edges are preserved only when both endpoints are in keepIDs.
// The root is set to the original root if it is in keepIDs, otherwise nil.
func (g *Graph) Prune(keepIDs map[string]bool) *Graph {
	pruned := New()

	// Re-create kept nodes (reuse original *Node pointers).
	for id, node := range g.nodes {
		if !keepIDs[id] {
			continue
		}
		pruned.nodes[id] = node
		if g.root != nil && g.root.ID == id {
			pruned.root = node
		}
	}

	// Rebuild edge lists: only keep edges where both ends are retained.
	for _, node := range pruned.nodes {
		var deps []*Edge
		for _, e := range node.dependencies {
			if keepIDs[e.Parent.ID] {
				deps = append(deps, e)
			}
		}
		node.dependencies = deps

		var dependents []*Edge
		for _, e := range node.dependents {
			if keepIDs[e.Child.ID] {
				dependents = append(dependents, e)
			}
		}
		node.dependents = dependents
	}

	return pruned
}
