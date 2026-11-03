package planner

import (
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

type expander struct {
	reg      Registry
	graph    *graph.Graph
	visited  map[nodeIdentity]*graph.Node
	share    *batchShareState
	roots    []*graph.Node   // roots planned so far by PlanRoots, which Links refer to by index
	only     map[string]bool // nil = expand all; non-nil = skip root-level relations not in set
	maxNodes int             // zero = unlimited
	active   []activeBlueprint
}

// activeBlueprint records a blueprint currently on the expansion stack. The
// expansion of a required relation that returns to an already-active
// (blueprint, options, bound relations) frame would recurse forever, so it is
// reported as a cycle. Diamond dependencies are not affected: sibling branches
// leave the stack before the next branch is expanded, and shared nodes
// short-circuit on the visited map before reaching the cycle check.
//
// bound lists the relations already resolved by a structural back-reference
// (a has-many child bound to the parent that created it). Those relations are
// never expanded, so a frame that binds them terminates differently from one
// that expands them: department -> employees[0] binds the child's department
// relation instead of recursing into another department. Comparing relation
// names rather than the bound nodes keeps a recursion that rebinds a fresh
// parent on every level detectable.
type activeBlueprint struct {
	name  string
	opts  *OptionSet
	bound []string
}

func (e *expander) expandBlueprint(bp *BlueprintDef, nodeID nodeIdentity, opts *OptionSet, bindings map[string]*graph.Node, relationPath relationPath, origin graph.Origin) (*graph.Node, error) {
	if existing, ok := e.visited[nodeID]; ok {
		return existing, nil
	}

	if err := e.enterBlueprint(bp.Name, opts, bindings); err != nil {
		return nil, err
	}
	defer e.leaveBlueprint()

	node, err := newBlueprintNode(bp, nodeID.String(), opts)
	if err != nil {
		return nil, err
	}
	node.Origin = origin
	if err := e.addNode(nodeID, node); err != nil {
		return nil, err
	}

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

// addNode adds a planned node to the graph, failing once the plan would hold
// more than maxNodes nodes. The limit is checked as nodes are added, so a
// relation that would expand into a huge graph stops at the limit instead of
// being planned in full first.
func (e *expander) addNode(nodeID nodeIdentity, node *graph.Node) error {
	if e.maxNodes > 0 && e.graph.Len() >= e.maxNodes {
		return fmt.Errorf("expand %s: %w", node.Path(), errx.TooManyNodes(e.maxNodes))
	}
	e.graph.AddNode(node)
	e.visited[nodeID] = node
	return nil
}

// reserve fails when n more nodes under node's relation would exceed
// maxNodes, so a large collection is rejected before its elements are
// allocated.
func (e *expander) reserve(node *graph.Node, relation string, n int) error {
	if e.maxNodes > 0 && e.graph.Len()+n > e.maxNodes {
		return fmt.Errorf("expand %s.%s: %w", node.Path(), relation, errx.TooManyNodes(e.maxNodes))
	}
	return nil
}

// enterBlueprint pushes a blueprint onto the active expansion stack, reporting
// ErrCycleDetected when the same (blueprint, options, bound relations) frame is
// already active. A successful enter must be paired with leaveBlueprint; a
// returned error means nothing was pushed.
func (e *expander) enterBlueprint(name string, opts *OptionSet, bindings map[string]*graph.Node) error {
	bound := slices.Sorted(maps.Keys(bindings))
	for _, frame := range e.active {
		if frame.name == name && slices.Equal(frame.bound, bound) && reflect.DeepEqual(frame.opts, opts) {
			path := make([]string, 0, len(e.active)+1)
			for _, f := range e.active {
				path = append(path, f.name)
			}
			path = append(path, name)
			return fmt.Errorf("expand relations: %w", errx.RelationCycle(path))
		}
	}
	e.active = append(e.active, activeBlueprint{name: name, opts: opts, bound: bound})
	return nil
}

func (e *expander) leaveBlueprint() {
	e.active = e.active[:len(e.active)-1]
}
