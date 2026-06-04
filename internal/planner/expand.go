package planner

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

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
	active  []activeBlueprint
}

// activeBlueprint records a blueprint currently on the expansion stack. The
// expansion of a required relation that returns to an already-active
// (blueprint, options) pair would recurse forever, so it is reported as a
// cycle. Diamond dependencies are not affected: sibling branches leave the
// stack before the next branch is expanded, and shared nodes short-circuit on
// the visited map before reaching the cycle check.
type activeBlueprint struct {
	name string
	opts *OptionSet
}

func (e *expander) expandBlueprint(bp *BlueprintDef, nodeID string, opts *OptionSet, bindings map[string]*graph.Node, relationPath string) (*graph.Node, error) {
	if existing, ok := e.visited[nodeID]; ok {
		return existing, nil
	}

	if err := e.enterBlueprint(bp.Name, opts); err != nil {
		return nil, err
	}
	defer e.leaveBlueprint()

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

// enterBlueprint pushes a blueprint onto the active expansion stack, reporting
// ErrCycleDetected when the same (blueprint, options) pair is already active.
// A successful enter must be paired with leaveBlueprint; a returned error means
// nothing was pushed.
func (e *expander) enterBlueprint(name string, opts *OptionSet) error {
	for _, frame := range e.active {
		if frame.name == name && reflect.DeepEqual(frame.opts, opts) {
			path := make([]string, 0, len(e.active)+1)
			for _, f := range e.active {
				path = append(path, f.name)
			}
			path = append(path, name)
			return fmt.Errorf("expand relations: %w", errx.RelationCycle(path))
		}
	}
	e.active = append(e.active, activeBlueprint{name: name, opts: opts})
	return nil
}

func (e *expander) leaveBlueprint() {
	e.active = e.active[:len(e.active)-1]
}
