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

	g := graph.New()
	visited := make(map[string]*graph.Node)

	_, err = expand(reg, bp, bp.Name, opts, g, visited, nil)
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
	visited := make(map[string]*graph.Node)
	shared := newBatchShareState()
	rootIDs := make([]string, 0, len(opts))

	for i, opt := range opts {
		rootID := fmt.Sprintf("root[%d]", i)
		rootIDs = append(rootIDs, rootID)

		exp := &expander{
			reg:     reg,
			graph:   g,
			visited: visited,
			share:   shared,
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
