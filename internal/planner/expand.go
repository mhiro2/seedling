package planner

import "github.com/mhiro2/seedling/internal/graph"

// expand recursively creates nodes for a blueprint and its required relations.
// visited tracks node IDs already expanded to handle diamond dependencies.
func expand(
	reg Registry,
	bp *BlueprintDef,
	nodeID string,
	opts *OptionSet,
	g *graph.Graph,
	visited map[string]*graph.Node,
	bindings map[string]*graph.Node,
) (*graph.Node, error) {
	return (&expander{
		reg:     reg,
		graph:   g,
		visited: visited,
	}).expandBlueprint(bp, nodeID, opts, bindings, "")
}

type expander struct {
	reg     Registry
	graph   *graph.Graph
	visited map[string]*graph.Node
	share   *batchShareState
}

func (e *expander) expandBlueprint(bp *BlueprintDef, nodeID string, opts *OptionSet, bindings map[string]*graph.Node, relationPath string) (*graph.Node, error) {
	if err := validate(bp, opts); err != nil {
		return nil, err
	}

	if existing, ok := e.visited[nodeID]; ok {
		return existing, nil
	}

	node, err := newBlueprintNode(bp, nodeID, opts)
	if err != nil {
		return nil, err
	}

	e.graph.AddNode(node)
	e.visited[nodeID] = node

	for _, rel := range bp.Relations {
		if err := e.expandRelation(bp, node, nodeID, relationPath, rel, opts, bindings); err != nil {
			return nil, err
		}
	}

	return node, nil
}
