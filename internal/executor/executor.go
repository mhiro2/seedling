package executor

import (
	"context"
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/field"
	"github.com/mhiro2/seedling/internal/graph"
	"github.com/mhiro2/seedling/internal/planner"
)

// Result holds the inserted nodes after execution.
type Result struct {
	Root  any
	Nodes map[string]NodeResult
	Graph *graph.Graph
}

// NodeResult holds the result of a single inserted node.
type NodeResult struct {
	Name  string
	Value any
}

// InsertFunc is the type-erased insert function stored in BlueprintDef.
type InsertFunc = func(ctx context.Context, db, v any) (any, error)

// BlueprintLookup resolves blueprint definitions by name.
type BlueprintLookup interface {
	LookupByName(name string) (*planner.BlueprintDef, error)
}

// LogEntry holds information about a single insert operation.
type LogEntry struct {
	Step       int
	Blueprint  string
	Table      string
	Provided   bool
	FKBindings []FKBinding
}

// FKBinding describes a single FK assignment made before an insert.
type FKBinding struct {
	ChildField      string
	ParentBlueprint string
	ParentTable     string
	ParentField     string
	Value           any
}

// Execute inserts all nodes in topological order, assigning parent PKs to child FKs.
// If logFn is non-nil, it is called for each step in the execution order.
func Execute(ctx context.Context, db any, g *graph.Graph, lookup BlueprintLookup, logFn func(LogEntry)) (*Result, error) {
	order, err := g.TopoSort()
	if err != nil {
		return nil, fmt.Errorf("topologically sort graph: %w", err)
	}

	result := &Result{
		Nodes: make(map[string]NodeResult),
	}

	for i, node := range order {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("execute graph: %w", err)
		}

		// Collect FK bindings for logging before assignment.
		var bindings []FKBinding
		if logFn != nil {
			bindings = collectFKBindings(node)
		}

		// Assign parent PKs to this node's FK fields.
		if err := assignFKs(node); err != nil {
			return nil, fmt.Errorf("assign foreign keys for node %q: %w", node.ID, err)
		}

		// Fill in actual FK values after assignment.
		if logFn != nil {
			for j := range bindings {
				if pkVal, err := field.GetField(node.Value, bindings[j].ChildField); err == nil {
					bindings[j].Value = pkVal
				}
			}

			logFn(LogEntry{
				Step:       i + 1,
				Blueprint:  node.BlueprintName,
				Table:      node.Table,
				Provided:   node.IsProvided,
				FKBindings: bindings,
			})
		}

		if !node.IsProvided {
			bp, err := lookup.LookupByName(node.BlueprintName)
			if err != nil {
				return nil, fmt.Errorf("lookup blueprint %q: %w", node.BlueprintName, err)
			}

			inserted, err := bp.Insert(ctx, db, node.Value)
			if err != nil {
				return nil, fmt.Errorf("insert node %q: %w", node.ID, errx.InsertFailed(node.BlueprintName, err))
			}
			node.Value = inserted
		}

		result.Nodes[node.ID] = NodeResult{
			Name:  node.BlueprintName,
			Value: node.Value,
		}
	}

	// Set root and graph.
	if root := g.Root(); root != nil {
		result.Root = root.Value
	}
	result.Graph = g

	return result, nil
}

// collectFKBindings gathers FK binding metadata from a node's dependency edges.
func collectFKBindings(node *graph.Node) []FKBinding {
	var bindings []FKBinding
	for _, edge := range node.Dependencies() {
		for _, b := range edge.Bindings {
			bindings = append(bindings, FKBinding{
				ChildField:      b.ChildField,
				ParentBlueprint: edge.Parent.BlueprintName,
				ParentTable:     edge.Parent.Table,
				ParentField:     b.ParentField,
			})
		}
	}
	return bindings
}

// assignFKs sets FK fields on the node based on its parent edges.
func assignFKs(node *graph.Node) error {
	for _, edge := range node.Dependencies() {
		parent := edge.Parent

		ptr := reflect.New(reflect.TypeOf(node.Value))
		ptr.Elem().Set(reflect.ValueOf(node.Value))

		for _, binding := range edge.Bindings {
			pkVal, err := field.GetField(parent.Value, binding.ParentField)
			if err != nil {
				return fmt.Errorf("get parent field %q for node %q: %w", binding.ParentField, parent.ID, err)
			}
			if err := field.SetField(ptr.Interface(), binding.ChildField, pkVal); err != nil {
				return fmt.Errorf("set child field %q for node %q: %w", binding.ChildField, node.ID, err)
			}
		}
		node.Value = ptr.Elem().Interface()
	}
	return nil
}
