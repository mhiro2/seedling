package seedling

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
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
	return lookupNodeResult(r.nodes, name)
}

// Nodes returns all nodes that match the given blueprint name, sorted by
// node ID (lexicographic order). Returns nil if no nodes match.
func (r Result[T]) Nodes(name string) []NodeResult {
	return lookupNodeResults(r.nodes, name)
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
	return cloneNodeResults(r.nodes)
}

// DebugString returns a human-readable tree of the execution result,
// showing each node's state (inserted/provided) and PK value.
func (r Result[T]) DebugString() string {
	return debugResultString(r.graph)
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
//
// CleanupE is fail-fast: it stops at the first delete error and returns it.
func (r Result[T]) CleanupE(ctx context.Context, db DBTX) error {
	return cleanupResultGraph(ctx, r.graph, r.deleteFns, db)
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

// BatchResult holds all created nodes after batch insertion.
type BatchResult[T any] struct {
	roots     []T
	rootIDs   []string
	nodes     map[string]executor.NodeResult
	graph     *graph.Graph
	registry  *Registry
	deleteFns map[string]deleteFn // blueprint name → delete function snapshot
}

// Len returns the number of inserted root records.
func (r BatchResult[T]) Len() int {
	return len(r.roots)
}

// Roots returns the inserted root records.
func (r BatchResult[T]) Roots() []T {
	return slices.Clone(r.roots)
}

func (r BatchResult[T]) rootsView() []T {
	return r.roots
}

// RootAt returns the inserted root record at index.
func (r BatchResult[T]) RootAt(index int) (T, bool) {
	var zero T
	if index < 0 || index >= len(r.roots) {
		return zero, false
	}
	return r.roots[index], true
}

// MustRootAt returns the inserted root record at index or panics.
func (r BatchResult[T]) MustRootAt(index int) T {
	root, ok := r.RootAt(index)
	if !ok {
		panic(fmt.Sprintf("seedling: root index %d out of range", index))
	}
	return root
}

// Node returns a named node from the full batch dependency graph by blueprint name.
// If multiple nodes share the same blueprint name, the one with the
// lexicographically smallest node ID is returned across all roots.
//
// For root-scoped lookups, use [BatchResult.NodeAt] or [BatchResult.NodesForRoot].
func (r BatchResult[T]) Node(name string) (NodeResult, bool) {
	return lookupNodeResult(r.nodes, name)
}

// Nodes returns all nodes that match the given blueprint name across the full batch.
func (r BatchResult[T]) Nodes(name string) []NodeResult {
	return lookupNodeResults(r.nodes, name)
}

// NodeAt returns the named node associated with the root at index.
// Shared belongs-to dependencies are included when that root references them.
func (r BatchResult[T]) NodeAt(rootIndex int, name string) (NodeResult, bool) {
	return lookupNodeResult(r.nodesForRoot(rootIndex), name)
}

// NodesForRoot returns all named nodes associated with the root at index,
// sorted by node ID. Shared belongs-to dependencies are included when that
// root references them.
func (r BatchResult[T]) NodesForRoot(rootIndex int, name string) []NodeResult {
	return lookupNodeResults(r.nodesForRoot(rootIndex), name)
}

// MustNodeAt returns the named node associated with the root at index or panics.
func (r BatchResult[T]) MustNodeAt(rootIndex int, name string) NodeResult {
	nr, ok := r.NodeAt(rootIndex, name)
	if !ok {
		panic(fmt.Sprintf("seedling: node %q not found for root index %d", name, rootIndex))
	}
	return nr
}

// MustNode returns a named node or panics.
func (r BatchResult[T]) MustNode(name string) NodeResult {
	nr, ok := r.Node(name)
	if !ok {
		panic(fmt.Sprintf("seedling: node %q not found in result", name))
	}
	return nr
}

// All returns all nodes in the result as a map keyed by node ID.
func (r BatchResult[T]) All() map[string]NodeResult {
	return cloneNodeResults(r.nodes)
}

// DebugString returns a human-readable tree of the execution result.
func (r BatchResult[T]) DebugString() string {
	return debugResultString(r.graph)
}

// Cleanup deletes all records that were inserted by seedling in reverse dependency order.
func (r BatchResult[T]) Cleanup(tb testing.TB, db DBTX) {
	tb.Helper()
	if err := r.CleanupE(tb.Context(), db); err != nil {
		tb.Fatal(err)
	}
}

// CleanupE deletes all records that were inserted by seedling in reverse dependency order.
// CleanupE is fail-fast: it stops at the first delete error and returns it.
func (r BatchResult[T]) CleanupE(ctx context.Context, db DBTX) error {
	return cleanupResultGraph(ctx, r.graph, r.deleteFns, db)
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

func lookupNodeResult(nodes map[string]executor.NodeResult, name string) (NodeResult, bool) {
	var matchIDs []string
	for id, nr := range nodes {
		if nr.Name == name {
			matchIDs = append(matchIDs, id)
		}
	}
	if len(matchIDs) == 0 {
		return NodeResult{}, false
	}

	sort.Strings(matchIDs)
	nr := nodes[matchIDs[0]]
	return NodeResult{name: nr.Name, value: nr.Value}, true
}

func lookupNodeResults(nodes map[string]executor.NodeResult, name string) []NodeResult {
	var matchIDs []string
	for id, nr := range nodes {
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
		nr := nodes[id]
		results[i] = NodeResult{name: nr.Name, value: nr.Value}
	}
	return results
}

func cloneNodeResults(nodes map[string]executor.NodeResult) map[string]NodeResult {
	all := make(map[string]NodeResult, len(nodes))
	for id, nr := range nodes {
		all[id] = NodeResult{name: nr.Name, value: nr.Value}
	}
	return all
}

func debugResultString(g *graph.Graph) string {
	if g == nil {
		return "(empty)"
	}
	return debug.ResultString(g)
}

func emptyBatchResult[T any]() BatchResult[T] {
	return BatchResult[T]{
		roots:   []T{},
		rootIDs: []string{},
	}
}

func (r BatchResult[T]) nodesForRoot(rootIndex int) map[string]executor.NodeResult {
	rootID, ok := r.rootNodeID(rootIndex)
	if !ok {
		return nil
	}

	selected := make(map[string]struct{})
	var pending []string

	for id := range r.nodes {
		if id == rootID || strings.HasPrefix(id, rootID+".") {
			if _, exists := selected[id]; exists {
				continue
			}
			selected[id] = struct{}{}
			pending = append(pending, id)
		}
	}

	if r.graph == nil {
		return selectNodeResults(r.nodes, selected)
	}

	for len(pending) > 0 {
		last := len(pending) - 1
		id := pending[last]
		pending = pending[:last]

		node := r.graph.Node(id)
		if node == nil {
			continue
		}

		for _, edge := range node.Dependencies() {
			parentID := edge.Parent.ID
			if _, exists := selected[parentID]; exists {
				continue
			}
			selected[parentID] = struct{}{}
			pending = append(pending, parentID)
		}
	}

	return selectNodeResults(r.nodes, selected)
}

func selectNodeResults(nodes map[string]executor.NodeResult, selected map[string]struct{}) map[string]executor.NodeResult {
	scoped := make(map[string]executor.NodeResult, len(selected))
	for id := range selected {
		nr, ok := nodes[id]
		if !ok {
			continue
		}
		scoped[id] = nr
	}
	return scoped
}

func (r BatchResult[T]) rootNodeID(rootIndex int) (string, bool) {
	if rootIndex < 0 || rootIndex >= len(r.roots) {
		return "", false
	}
	if len(r.rootIDs) == len(r.roots) {
		return r.rootIDs[rootIndex], true
	}
	return fmt.Sprintf("root[%d]", rootIndex), true
}

func cleanupResultGraph(ctx context.Context, g *graph.Graph, deleteFns map[string]deleteFn, db DBTX) error {
	if g == nil {
		return nil
	}

	order, err := g.TopoSort()
	if err != nil {
		return fmt.Errorf("sort cleanup graph: %w", err)
	}

	for i := len(order) - 1; i >= 0; i-- {
		node := order[i]
		if node.IsProvided {
			continue
		}

		df, ok := deleteFns[node.BlueprintName]
		if !ok || df.fn == nil {
			return fmt.Errorf("cleanup blueprint %q: %w", node.BlueprintName, errx.DeleteNotDefined(node.BlueprintName))
		}

		if err := df.fn(ctx, db, node.Value); err != nil {
			return fmt.Errorf("cleanup blueprint %q: %w", node.BlueprintName, errx.DeleteFailed(node.BlueprintName, err))
		}
	}

	return nil
}
