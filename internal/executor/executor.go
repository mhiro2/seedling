package executor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

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

// Execute inserts all nodes in topological order, assigning referenced parent fields to child FKs.
// If execution fails after one or more nodes have succeeded, the returned Result
// contains those nodes so the caller can clean them up. If logFn is non-nil, it
// is called for each step in the execution order.
func Execute(ctx context.Context, db any, g *graph.Graph, lookup BlueprintLookup, logFn func(LogEntry)) (*Result, error) {
	result := &Result{Nodes: make(map[string]NodeResult)}
	if g == nil {
		return finalizeResult(result, nil, false), fmt.Errorf("execute graph: %w: graph must not be nil", errx.ErrInvalidOption)
	}

	order, err := g.TopoSort()
	if err != nil {
		return finalizeResult(result, g, false), fmt.Errorf("topologically sort graph: %w", err)
	}

	blueprints, err := preflight(ctx, order, lookup)
	if err != nil {
		return finalizeResult(result, g, false), err
	}

	for i, node := range order {
		if err := ctx.Err(); err != nil {
			return finalizeResult(result, g, false), fmt.Errorf("execute graph: %w", err)
		}

		// Collect FK bindings for logging before assignment.
		var bindings []FKBinding
		if logFn != nil {
			bindings = collectFKBindings(node)
		}

		// Assign referenced parent fields to this node's FK fields.
		if err := assignFKs(node); err != nil {
			return finalizeResult(result, g, false), fmt.Errorf("assign foreign keys for node %q: %w", node.ID, err)
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
			bp := blueprints[node.ID]
			inserted, err := bp.Insert(ctx, db, node.Value)
			if err != nil {
				return finalizeResult(result, g, false), fmt.Errorf("insert node %q: %w", node.ID, errx.InsertFailed(node.BlueprintName, err))
			}
			if isNilValue(inserted) {
				err := errors.New("insert returned a nil value")
				return finalizeResult(result, g, false), fmt.Errorf("insert node %q: %w", node.ID, errx.InsertFailed(node.BlueprintName, err))
			}
			if bp.ModelType != nil && reflect.TypeOf(inserted) != bp.ModelType {
				err := fmt.Errorf("%w: insert returned %T, want %s", errx.ErrTypeMismatch, inserted, bp.ModelType)
				return finalizeResult(result, g, false), fmt.Errorf("insert node %q: %w", node.ID, errx.InsertFailed(node.BlueprintName, err))
			}
			node.Value = inserted
		}

		result.Nodes[node.ID] = NodeResult{
			Name:  node.BlueprintName,
			Value: node.Value,
		}
	}

	return finalizeResult(result, g, true), nil
}

// preflight resolves every blueprint and validates all static value and FK
// bindings before the first Insert callback can create a database side effect.
func preflight(ctx context.Context, order []*graph.Node, lookup BlueprintLookup) (map[string]*planner.BlueprintDef, error) {
	blueprints := make(map[string]*planner.BlueprintDef, len(order))

	for _, node := range order {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("execute graph: %w", err)
		}
		if node == nil {
			return nil, fmt.Errorf("execute graph: %w: graph contains a nil node", errx.ErrInvalidOption)
		}
		// Planner-built nodes carry a struct or a non-nil pointer to one, but a
		// graph.Node assembled externally may violate that invariant. Reject nil
		// values before the reflection and typed callback paths can panic.
		if isNilValue(node.Value) {
			return nil, fmt.Errorf("execute node %q: node value must not be nil", node.ID)
		}

		bp, err := lookup.LookupByName(node.BlueprintName)
		if err != nil {
			return nil, fmt.Errorf("lookup blueprint %q: %w", node.BlueprintName, err)
		}
		if bp == nil {
			return nil, fmt.Errorf("lookup blueprint %q: %w: lookup returned nil", node.BlueprintName, errx.ErrInvalidOption)
		}
		if bp.ModelType != nil && reflect.TypeOf(node.Value) != bp.ModelType {
			return nil, fmt.Errorf("%w: node %q has value %T, want %s", errx.ErrTypeMismatch, node.ID, node.Value, bp.ModelType)
		}
		if !node.IsProvided && bp.Insert == nil {
			return nil, fmt.Errorf("execute node %q: %w: blueprint %q has no Insert function", node.ID, errx.ErrInvalidOption, node.BlueprintName)
		}
		blueprints[node.ID] = bp
	}

	for _, node := range order {
		if err := validateFKBindings(node); err != nil {
			return nil, fmt.Errorf("validate foreign keys for node %q: %w", node.ID, err)
		}
	}

	return blueprints, nil
}

// validateFKBindings checks every FK binding on a node before any insert runs.
// Parent fields are matched by type only, because a parent's Insert callback has
// not run yet and may still allocate the embedded struct that holds the
// referenced field.
func validateFKBindings(node *graph.Node) error {
	for _, edge := range node.Dependencies() {
		parent := edge.Parent
		parentType := reflect.TypeOf(parent.Value)
		for _, binding := range edge.Bindings {
			if err := field.CanBind(parentType, binding.ParentField, node.Value, binding.ChildField); err != nil {
				return fmt.Errorf("bind parent %q field %q to node %q field %q: %w",
					parent.ID, binding.ParentField, node.ID, binding.ChildField, err)
			}
		}
	}
	return nil
}

func finalizeResult(result *Result, g *graph.Graph, complete bool) *Result {
	if g == nil {
		result.Graph = graph.New()
		return result
	}

	if root := g.Root(); root != nil {
		if insertedRoot, ok := result.Nodes[root.ID]; ok {
			result.Root = insertedRoot.Value
		}
	}

	if complete {
		result.Graph = g
		return result
	}

	result.Graph = resultGraph(g, result.Nodes)
	return result
}

// resultGraph snapshots only nodes that completed successfully. Cleanup can
// therefore traverse a partial result without attempting to delete the failed
// node or any nodes that execution never reached.
func resultGraph(g *graph.Graph, results map[string]NodeResult) *graph.Graph {
	partial := graph.New()
	if len(results) == 0 {
		return partial
	}

	nodes := g.Nodes()
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	copied := make(map[string]*graph.Node, len(results))
	addNode := func(node *graph.Node) {
		if _, exists := copied[node.ID]; exists {
			return
		}
		nodeResult, ok := results[node.ID]
		if !ok {
			return
		}
		clonedNode := &graph.Node{
			ID:            node.ID,
			BlueprintName: node.BlueprintName,
			Table:         node.Table,
			Value:         nodeResult.Value,
			IsProvided:    node.IsProvided,
			PKField:       node.PKField,
			PKFields:      append([]string(nil), node.PKFields...),
			SetFields:     append([]string(nil), node.SetFields...),
		}
		partial.AddNode(clonedNode)
		copied[node.ID] = clonedNode
	}

	for _, node := range nodes {
		addNode(node)
	}

	// AddNode treats whichever node landed first as the root, which for a
	// partial result is an arbitrary survivor. Restore the real root, or clear
	// it when the root itself never completed.
	partial.SetRoot(nil)
	if root := g.Root(); root != nil {
		partial.SetRoot(copied[root.ID])
	}

	for _, child := range nodes {
		copiedChild, ok := copied[child.ID]
		if !ok {
			continue
		}
		for _, edge := range child.Dependencies() {
			copiedParent, ok := copied[edge.Parent.ID]
			if !ok {
				continue
			}
			partial.AddEdgeBindings(copiedParent, copiedChild, edge.Bindings)
		}
	}

	return partial
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
//
// Value nodes use a single allocation to materialize an addressable struct.
// Pointer nodes are already addressable and are mutated in place. The copy
// goes through [field.Copy], which uses cached field-index paths and avoids
// boxing the PK through `any` once per binding.
func assignFKs(node *graph.Node) error {
	deps := node.Dependencies()
	if len(deps) == 0 {
		return nil
	}

	value := reflect.ValueOf(node.Value)
	target := node.Value
	if value.Kind() != reflect.Pointer {
		ptr := reflect.New(value.Type())
		ptr.Elem().Set(value)
		target = ptr.Interface()
	}

	for _, edge := range deps {
		parent := edge.Parent
		for _, binding := range edge.Bindings {
			if err := field.Copy(parent.Value, binding.ParentField, target, binding.ChildField); err != nil {
				return fmt.Errorf("bind parent %q field %q to node %q field %q: %w",
					parent.ID, binding.ParentField, node.ID, binding.ChildField, err)
			}
		}
	}
	if value.Kind() != reflect.Pointer {
		node.Value = reflect.ValueOf(target).Elem().Interface()
	}
	return nil
}

// isNilValue reports whether value is a nil interface or a nil pointer.
func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
