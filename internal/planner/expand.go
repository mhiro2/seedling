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
	only map[string]bool,
) (*graph.Node, error) {
	return (&expander{
		reg:     reg,
		graph:   g,
		visited: visited,
		only:    only,
	}).expandBlueprint(bp, nodeID, opts, bindings, "")
}

type expander struct {
	reg     Registry
	graph   *graph.Graph
	visited map[string]*graph.Node
	share   *batchShareState
	only    map[string]bool // nil = expand all; non-nil = skip root-level relations not in set
}

func (e *expander) expandBlueprint(bp *BlueprintDef, nodeID string, opts *OptionSet, bindings map[string]*graph.Node, relationPath string) (*graph.Node, error) {
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
		// Lazy evaluation: at root level, skip relations not in the only set.
		if e.only != nil && relationPath == "" && !e.only[rel.Name] {
			continue
		}
		if err := e.expandRelation(bp, node, nodeID, relationPath, rel, opts, bindings); err != nil {
			return nil, err
		}
	}

	return node, nil
}
