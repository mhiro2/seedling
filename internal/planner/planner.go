package planner

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/graph"
)

// Plan builds a dependency graph for the given root type with options.
func Plan(reg Registry, rootType reflect.Type, opts *OptionSet) (*PlanResult, error) {
	bp, err := reg.LookupByType(rootType)
	if err != nil {
		return nil, fmt.Errorf("lookup root blueprint %s: %w", rootType, err)
	}
	if err := validatePlan(reg, bp, opts); err != nil {
		return nil, err
	}

	g := graph.New()
	visited := make(map[nodeIdentity]*graph.Node)

	var only map[string]bool
	if opts != nil {
		only = opts.Only
	}

	_, err = expand(reg, bp, rootNodeIdentity(bp.Name), opts, g, visited, nil, only)
	if err != nil {
		return nil, err
	}

	return &PlanResult{Graph: g}, nil
}

// PlanMany builds a dependency graph for multiple roots of the same type.
func PlanMany(reg Registry, rootType reflect.Type, opts []*OptionSet) (*PlanManyResult, error) {
	bp, err := reg.LookupByType(rootType)
	if err != nil {
		return nil, fmt.Errorf("lookup root blueprint %s: %w", rootType, err)
	}

	g := graph.New()
	visited := make(map[nodeIdentity]*graph.Node)
	shared := newBatchShareState()
	rootIDs := make([]string, 0, len(opts))

	for i, opt := range opts {
		if err := validatePlan(reg, bp, opt); err != nil {
			return nil, fmt.Errorf("validate root[%d]: %w", i, err)
		}
	}

	for i, opt := range opts {
		rootID := batchRootNodeIdentity(i)
		rootIDs = append(rootIDs, rootID.String())

		exp := &expander{
			reg:     reg,
			graph:   g,
			visited: visited,
			share:   shared,
			only:    opt.Only,
		}
		if _, err := exp.expandBlueprint(bp, rootID, opt, nil, ""); err != nil {
			return nil, err
		}
	}

	return &PlanManyResult{
		Graph:   g,
		RootIDs: rootIDs,
	}, nil
}

// PlanRoots builds one dependency graph for roots of any blueprint. Roots are
// planned in order and never share nodes implicitly; a root's Links bind its
// belongs-to relations to roots planned before it. The graph has no single
// root, so RootIDs identifies each root's node in order.
func PlanRoots(reg Registry, roots []Root) (*PlanManyResult, error) {
	g := graph.New()
	visited := make(map[nodeIdentity]*graph.Node)
	rootIDs := make([]string, 0, len(roots))
	rootNodes := make([]*graph.Node, 0, len(roots))

	for i, root := range roots {
		bp, err := reg.LookupByType(root.Type)
		if err != nil {
			return nil, fmt.Errorf("lookup root[%d] blueprint %s: %w", i, root.Type, err)
		}
		if err := validatePlan(reg, bp, root.Opts); err != nil {
			return nil, fmt.Errorf("validate root[%d]: %w", i, err)
		}

		var only map[string]bool
		if root.Opts != nil {
			only = root.Opts.Only
		}
		rootID := batchRootNodeIdentity(i)
		exp := &expander{
			reg:     reg,
			graph:   g,
			visited: visited,
			roots:   rootNodes,
			only:    only,
		}
		node, err := exp.expandBlueprint(bp, rootID, root.Opts, nil, "")
		if err != nil {
			return nil, fmt.Errorf("expand root[%d]: %w", i, err)
		}
		rootIDs = append(rootIDs, rootID.String())
		rootNodes = append(rootNodes, node)
	}
	g.SetRoot(nil)

	return &PlanManyResult{
		Graph:   g,
		RootIDs: rootIDs,
	}, nil
}
