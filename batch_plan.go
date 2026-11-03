package seedling

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/debug"
	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/graph"
	"github.com/mhiro2/seedling/internal/planner"
)

// BatchPlan is the dependency graph of n records of type T, built without
// inserting anything, as [InsertMany] would insert them. Like [Plan], it can
// be executed multiple times, each run on a cloned graph, and AfterInsert
// callbacks captured at build time are shared across executions.
type BatchPlan[T any] struct {
	graph        *graph.Graph
	rootIDs      []string
	afterInserts [][]afterInsertFn // per record, in request order
	ctx          context.Context
	registry     *Registry
	db           DBTX
	logFn        func(InsertLog)
}

// BuildManyE constructs the plan of n records of type T without inserting
// anything. Seq options and batch sharing of belongs-to parents resolve as
// they do for [InsertManyE].
func BuildManyE[T any](n int, opts ...Option) (*BatchPlan[T], error) {
	return NewSession[T](nil).BuildManyE(n, opts...)
}

// BuildManyE constructs the plan of n records of type T without inserting
// anything. Seq options and batch sharing of belongs-to parents resolve as
// they do for [Session.InsertManyE].
func (s Session[T]) BuildManyE(n int, opts ...Option) (*BatchPlan[T], error) {
	if n < 0 {
		return nil, fmt.Errorf("validate insert count: n must be >= 0, got %d: %w", n, ErrInvalidOption)
	}
	plan := &BatchPlan[T]{
		graph:    graph.New(),
		rootIDs:  []string{},
		registry: s.registry,
		db:       s.db,
	}
	if n == 0 {
		return plan, nil
	}

	rootType := reflect.TypeFor[T]()
	collected := make([]*optionSet, n)
	internalOpts := make([]*planner.OptionSet, n)
	for i := range n {
		prepared, err := resolveRootOptions(s.registry, rootType, opts, i)
		if err != nil {
			return nil, err
		}
		collected[i] = prepared
		internalOpts[i] = toOptionSet(prepared)
	}

	// InScope is a root option, so every record resolves the same one.
	adapter := newRegistryAdapter(s.registry, resolveScope(collected[0].scope, s.scope))
	result, err := planner.PlanMany(adapter, rootType, internalOpts)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	plan.graph = result.Graph
	plan.rootIDs = result.RootIDs
	plan.afterInserts = make([][]afterInsertFn, n)
	for i, c := range collected {
		plan.afterInserts[i] = c.afterInserts
	}
	// WithContext and WithInsertLog are root options, so every record resolves
	// the same ones.
	plan.ctx = collected[0].ctx
	plan.logFn = collected[0].logFn
	return plan, nil
}

// BuildMany constructs the plan of n records of type T without inserting
// anything. Fails the test on error.
func BuildMany[T any](tb testing.TB, n int, opts ...Option) *BatchPlan[T] {
	tb.Helper()
	return NewSession[T](nil).BuildMany(tb, n, opts...)
}

// BuildMany constructs the plan of n records of type T without inserting
// anything. Fails the test on error.
func (s Session[T]) BuildMany(tb testing.TB, n int, opts ...Option) *BatchPlan[T] {
	tb.Helper()
	plan, err := s.forTest(tb).BuildManyE(n, opts...)
	if err != nil {
		tb.Fatal(err)
	}
	return plan
}

// Len returns the number of records of type T the plan inserts.
func (p *BatchPlan[T]) Len() int {
	return len(p.rootIDs)
}

// Insert executes the plan and inserts all records. Fails the test on error.
func (p *BatchPlan[T]) Insert(tb testing.TB, db DBTX) BatchResult[T] {
	tb.Helper()
	result, err := p.InsertE(tb.Context(), db)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// InsertE executes the plan and inserts all records, returning an error on
// failure. If execution fails after inserting some nodes, the returned
// BatchResult contains every successful node and only the roots that completed
// successfully.
func (p *BatchPlan[T]) InsertE(ctx context.Context, db DBTX) (BatchResult[T], error) {
	if len(p.rootIDs) == 0 {
		return emptyBatchResult[T](), nil
	}

	// Execute a clone, as Plan.InsertE does, so every node owns its values and
	// an Insert callback mutating a Set slice, map, or pointer cannot leak into
	// the caller's option value or another record of the batch.
	ctx = insertContext(ctx, p.ctx)
	effectiveDB := db
	if effectiveDB == nil {
		effectiveDB = p.db
	}
	execResult, err := executor.Execute(ctx, effectiveDB, p.graph.Clone(), newRegistryAdapter(p.registry, nil), toExecutorLogFn(p.logFn))
	result, resultErr := batchResultFromExecutor[T](p.registry, p.rootIDs, execResult, err == nil)
	if resultErr != nil {
		return result, resultErr
	}
	if err != nil {
		return result, fmt.Errorf("execute plan: %w", err)
	}

	for i, rootID := range p.rootIDs {
		node, ok := execResult.Nodes[rootID]
		if !ok {
			continue
		}
		for _, fn := range p.afterInserts[i] {
			if err := fn(node.Value, effectiveDB); err != nil {
				return result, fmt.Errorf("run after-insert callback: %w", err)
			}
		}
	}
	return result, nil
}

// DebugString returns a human-readable tree representation of the plan.
func (p *BatchPlan[T]) DebugString() string {
	return debug.TreeString(p.graph)
}

// DryRunString returns the planned INSERT execution order with FK assignments.
func (p *BatchPlan[T]) DryRunString() string {
	return debug.DryRunString(p.graph)
}

// Validate performs a dry-run of the plan, checking for type mismatches and
// constraint violations without inserting any records.
func (p *BatchPlan[T]) Validate() error {
	return validatePlan(p.graph)
}

// Snapshot returns a read-only description of every record the plan holds.
func (p *BatchPlan[T]) Snapshot() PlanSnapshot {
	return newPlanSnapshot(p.graph)
}
