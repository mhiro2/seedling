package seedling

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/mhiro2/seedling/internal/debug"
	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/graph"
)

// deleteFn holds a snapshot of a blueprint's delete function captured at
// Result creation time. This prevents cleanup behavior from changing if
// the registry is reset or re-registered after the Result is created.
type deleteFn struct {
	fn func(ctx context.Context, db, v any) error
}

// Result holds all created nodes after insertion.
type Result[T any] struct {
	root      T
	nodes     map[string]executor.NodeResult
	graph     *graph.Graph
	registry  *Registry
	deleteFns map[string]deleteFn // blueprint name → delete function snapshot
}

// Root returns the root record that was inserted.
func (r Result[T]) Root() T {
	return r.root
}

// Node returns a named node from the dependency graph by blueprint name.
// If multiple nodes share the same blueprint name, the one with the
// lexicographically smallest node ID is returned. Node IDs are constructed
// as "root.relation" paths, so the smallest ID is typically the node
// closest to the root in the dependency graph.
//
// To retrieve all nodes that match a given blueprint name, use [Result.Nodes].
func (r Result[T]) Node(name string) (NodeResult, bool) {
	var matchIDs []string
	for id, nr := range r.nodes {
		if nr.Name == name {
			matchIDs = append(matchIDs, id)
		}
	}
	if len(matchIDs) == 0 {
		return NodeResult{}, false
	}

	sort.Strings(matchIDs)
	nr := r.nodes[matchIDs[0]]
	return NodeResult{name: nr.Name, value: nr.Value}, true
}

// Nodes returns all nodes that match the given blueprint name, sorted by
// node ID (lexicographic order). Returns nil if no nodes match.
func (r Result[T]) Nodes(name string) []NodeResult {
	var matchIDs []string
	for id, nr := range r.nodes {
		if nr.Name == name {
			matchIDs = append(matchIDs, id)
		}
	}
	if len(matchIDs) == 0 {
		return nil
	}

	sort.Strings(matchIDs)
	results := make([]NodeResult, len(matchIDs))
	for i, id := range matchIDs {
		nr := r.nodes[id]
		results[i] = NodeResult{name: nr.Name, value: nr.Value}
	}
	return results
}

// MustNode returns a named node or panics.
func (r Result[T]) MustNode(name string) NodeResult {
	nr, ok := r.Node(name)
	if !ok {
		panic(fmt.Sprintf("seedling: node %q not found in result", name))
	}
	return nr
}

// All returns all nodes in the result as a map keyed by node ID.
// This is useful for inspecting every record that was created during insertion.
func (r Result[T]) All() map[string]NodeResult {
	all := make(map[string]NodeResult, len(r.nodes))
	for id, nr := range r.nodes {
		all[id] = NodeResult{name: nr.Name, value: nr.Value}
	}
	return all
}

// DebugString returns a human-readable tree of the execution result,
// showing each node's state (inserted/provided) and PK value.
func (r Result[T]) DebugString() string {
	if r.graph == nil {
		return "(empty)"
	}
	return debug.ResultString(r.graph)
}

// Cleanup deletes all records that were inserted by seedling in reverse
// dependency order (children before parents). Records provided via [Use]
// are skipped because they were not created by seedling.
//
// Every blueprint whose records appear in the result must have a [Blueprint.Delete]
// function defined; otherwise Cleanup returns [ErrDeleteNotDefined].
//
// Cleanup is useful when transaction rollback is not available, such as when
// using testcontainers or external databases.
func (r Result[T]) Cleanup(tb testing.TB, db DBTX) {
	tb.Helper()
	if err := r.CleanupE(tb.Context(), db); err != nil {
		tb.Fatal(err)
	}
}

// CleanupE deletes all records that were inserted by seedling in reverse
// dependency order (children before parents). Records provided via [Use]
// are skipped because they were not created by seedling.
//
// Every blueprint whose records appear in the result must have a [Blueprint.Delete]
// function defined; otherwise CleanupE returns [ErrDeleteNotDefined].
//
// Delete functions are captured at result creation time, so cleanup behavior
// is not affected by subsequent registry resets or re-registrations.
func (r Result[T]) CleanupE(ctx context.Context, db DBTX) error {
	if r.graph == nil {
		return nil
	}

	order, err := r.graph.TopoSort()
	if err != nil {
		return fmt.Errorf("sort cleanup graph: %w", err)
	}

	// Delete in reverse topological order: children first, then parents.
	for i := len(order) - 1; i >= 0; i-- {
		node := order[i]
		if node.IsProvided {
			continue
		}

		df, ok := r.deleteFns[node.BlueprintName]
		if !ok || df.fn == nil {
			return fmt.Errorf("cleanup blueprint %q: %w", node.BlueprintName, errx.DeleteNotDefined(node.BlueprintName))
		}

		if err := df.fn(ctx, db, node.Value); err != nil {
			return fmt.Errorf("cleanup blueprint %q: %w", node.BlueprintName, errx.DeleteFailed(node.BlueprintName, err))
		}
	}

	return nil
}

// snapshotDeleteFns captures the delete functions from the registry for all
// blueprint names present in the execution result. This ensures cleanup uses
// the delete functions that existed at result creation time.
func snapshotDeleteFns(reg *Registry, nodes map[string]executor.NodeResult) map[string]deleteFn {
	r := resolveRegistry(reg).reg
	r.mu.RLock()
	defer r.mu.RUnlock()

	fns := make(map[string]deleteFn, len(nodes))
	for _, nr := range nodes {
		if _, ok := fns[nr.Name]; ok {
			continue
		}
		def, exists := r.byName[nr.Name]
		if exists && def.delete != nil {
			fns[nr.Name] = deleteFn{fn: def.delete}
		}
	}
	return fns
}

// NodeResult holds the result of a single node.
type NodeResult struct {
	name  string
	value any
}

// Name returns the blueprint name of this node.
func (n NodeResult) Name() string {
	return n.name
}

// Value returns the inserted value.
func (n NodeResult) Value() any {
	return n.value
}

// NodeAs returns a named node cast to T.
func NodeAs[T any](lookup interface {
	Node(name string) (NodeResult, bool)
}, name string,
) (T, bool, error) {
	var zero T

	node, ok := lookup.Node(name)
	if !ok {
		return zero, false, nil
	}
	value, ok := node.value.(T)
	if !ok {
		return zero, true, fmt.Errorf("%w: node %q has value %T, want %s", ErrTypeMismatch, name, node.value, reflect.TypeFor[T]())
	}
	return value, true, nil
}

// MustNodeAs returns a named node cast to T or panics.
func MustNodeAs[T any](lookup interface {
	Node(name string) (NodeResult, bool)
}, name string,
) T {
	value, ok, err := NodeAs[T](lookup, name)
	if err != nil {
		panic(err)
	}
	if !ok {
		panic(fmt.Sprintf("seedling: node %q not found in result", name))
	}
	return value
}

// NodesAs returns all named nodes cast to T.
func NodesAs[T any](lookup interface {
	Nodes(name string) []NodeResult
}, name string,
) ([]T, error) {
	nodes := lookup.Nodes(name)
	if len(nodes) == 0 {
		return nil, nil
	}

	values := make([]T, len(nodes))
	for i, node := range nodes {
		value, ok := node.value.(T)
		if !ok {
			return nil, fmt.Errorf("%w: node %q at index %d has value %T, want %s", ErrTypeMismatch, name, i, node.value, reflect.TypeFor[T]())
		}
		values[i] = value
	}
	return values, nil
}
