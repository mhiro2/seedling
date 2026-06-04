package graph

import "github.com/mhiro2/seedling/internal/clone"

// Clone returns a structurally independent copy of the graph.
func (g *Graph) Clone() *Graph {
	if g == nil {
		return nil
	}

	cloned := New()
	nodeMap := make(map[*Node]*Node, len(g.nodes))

	for _, node := range g.nodes {
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
		cloned.nodes[copied.ID] = copied
		nodeMap[node] = copied
		if g.root == node {
			cloned.root = copied
		}
	}

	for _, node := range g.nodes {
		parent := nodeMap[node]
		for _, edge := range node.dependents {
			copied := &Edge{
				Parent:     parent,
				Child:      nodeMap[edge.Child],
				LocalField: edge.LocalField,
				Bindings:   append([]FieldBinding(nil), edge.Bindings...),
			}
			parent.dependents = append(parent.dependents, copied)
			copied.Child.dependencies = append(copied.Child.dependencies, copied)
		}
	}

	return cloned
}
