package graph

import "github.com/mhiro2/seedling/internal/clone"

// Clone returns a structurally independent copy of the graph. The copy keeps
// the order nodes were added and the order of every node's edges, so it sorts
// and executes exactly like the original.
func (g *Graph) Clone() *Graph {
	if g == nil {
		return nil
	}

	cloned := New()
	nodeMap := make(map[*Node]*Node, len(g.order))

	for _, node := range g.order {
		copied := &Node{
			ID:            node.ID,
			BlueprintName: node.BlueprintName,
			Table:         node.Table,
			Value:         clone.Value(node.Value),
			IsProvided:    node.IsProvided,
			PKField:       node.PKField,
			PKFields:      append([]string(nil), node.PKFields...),
			SetFields:     append([]string(nil), node.SetFields...),
		}
		cloned.AddNode(copied)
		nodeMap[node] = copied
	}
	cloned.root = nodeMap[g.root]

	edgeMap := make(map[*Edge]*Edge)
	cloneEdge := func(edge *Edge) *Edge {
		if copied, ok := edgeMap[edge]; ok {
			return copied
		}
		copied := &Edge{
			Parent:     nodeMap[edge.Parent],
			Child:      nodeMap[edge.Child],
			LocalField: edge.LocalField,
			Bindings:   append([]FieldBinding(nil), edge.Bindings...),
		}
		edgeMap[edge] = copied
		return copied
	}
	for _, node := range g.order {
		copied := nodeMap[node]
		copied.Origin = node.Origin
		copied.Origin.From = nodeMap[node.Origin.From]
		for _, edge := range node.dependencies {
			copied.dependencies = append(copied.dependencies, cloneEdge(edge))
		}
		for _, edge := range node.dependents {
			copied.dependents = append(copied.dependents, cloneEdge(edge))
		}
	}

	return cloned
}
