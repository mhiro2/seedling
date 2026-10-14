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

// Plan represents a dependency graph ready for insertion.
// A plan can be executed multiple times. Each execution operates on a cloned
// graph so the built plan remains unchanged.
//
// Note: AfterInsert callbacks registered via options are captured at Build time
// and shared across executions. Go closures cannot be cloned, so reusing a
// plan also reuses any callback state captured by those closures. Prefer
// stateless callbacks, or rebuild the plan when callback state must be isolated.
type Plan[T any] struct {
	graph        *graph.Graph
	afterInserts []any // func(T, DBTX) closures
	ctx          context.Context
	registry     *Registry
	db           DBTX
	logFn        func(InsertLog)
}

// BuildE constructs a dependency plan for type T without inserting anything.
func BuildE[T any](opts ...Option) (*Plan[T], error) {
	return NewSession[T](nil).BuildE(opts...)
}

// BuildE constructs a dependency plan for type T without inserting anything.
func (s Session[T]) BuildE(opts ...Option) (*Plan[T], error) {
	rootType := reflect.TypeFor[T]()

	collected, err := resolveRootOptions(s.registry, rootType, opts, noSequenceIndex)
	if err != nil {
		return nil, err
	}

	adapter := newRegistryAdapter(s.registry)
	optSet := toOptionSet(collected)

	result, err := planner.Plan(adapter, rootType, optSet)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	return &Plan[T]{
		graph:        result.Graph,
		afterInserts: collected.afterInserts,
		ctx:          collected.ctx,
		registry:     s.registry,
		db:           s.db,
		logFn:        collected.logFn,
	}, nil
}

// Build constructs a dependency plan for type T without inserting anything.
// Fails the test on error.
func Build[T any](tb testing.TB, opts ...Option) *Plan[T] {
	tb.Helper()
	return NewSession[T](nil).Build(tb, opts...)
}

// Build constructs a dependency plan for type T without inserting anything.
// Fails the test on error.
func (s Session[T]) Build(tb testing.TB, opts ...Option) *Plan[T] {
	tb.Helper()
	plan, err := s.BuildE(opts...)
	if err != nil {
		tb.Fatal(err)
	}
	return plan
}

// Insert executes the plan and inserts all records. Fails the test on error.
func (p *Plan[T]) Insert(tb testing.TB, db DBTX) Result[T] {
	tb.Helper()
	result, err := p.InsertE(tb.Context(), db)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// InsertE executes the plan and inserts all records, returning an error on failure.
// A [WithContext] option given when the plan was built overrides ctx.
// If execution fails after inserting dependencies, the returned Result contains
// the successful nodes and can be passed to [Result.CleanupE].
func (p *Plan[T]) InsertE(ctx context.Context, db DBTX) (Result[T], error) {
	ctx = insertContext(ctx, p.ctx)
	if p.graph != nil {
		if root := p.graph.Root(); root != nil {
			if _, ok := root.Value.(T); !ok {
				var zero Result[T]
				return zero, fmt.Errorf("%w: root node has value %T, want %s", ErrTypeMismatch, root.Value, reflect.TypeFor[T]())
			}
		}
	}

	adapter := newRegistryAdapter(p.registry)
	g := p.graph.Clone()
	effectiveDB := p.resolveDB(db)
	execResult, err := executor.Execute(ctx, effectiveDB, g, adapter, p.toExecutorLogFn())
	result, resultErr := p.resultFromExecutor(execResult, err == nil)
	if resultErr != nil {
		return result, resultErr
	}
	if err != nil {
		return result, fmt.Errorf("execute plan: %w", err)
	}

	root := result.root

	// Run AfterInsert callbacks.
	// On failure the result is still returned so callers can clean up
	// already-inserted records via Result.Cleanup.
	for _, fn := range p.afterInserts {
		switch cb := fn.(type) {
		case func(T, DBTX):
			cb(root, effectiveDB)
		case func(T, DBTX) error:
			if err := cb(root, effectiveDB); err != nil {
				return result, fmt.Errorf("run after-insert callback: %w", err)
			}
		default:
			// BuildE rejects mismatched callbacks up front; this guards against a
			// plan assembled through any other path.
			return result, afterInsertTypeError[T](fn)
		}
	}

	return result, nil
}

func (p *Plan[T]) resultFromExecutor(execResult *executor.Result, requireRoot bool) (Result[T], error) {
	if execResult == nil {
		var zero Result[T]
		return zero, fmt.Errorf("%w: executor returned a nil result", ErrInvalidOption)
	}

	result := Result[T]{
		nodes:         execResult.Nodes,
		graph:         execResult.Graph,
		registry:      p.registry,
		deleteFns:     snapshotDeleteFns(p.registry, execResult.Nodes),
		cleanupValues: execResult.Inserted,
	}
	if execResult.Root == nil {
		if requireRoot {
			return result, fmt.Errorf("%w: root node has value <nil>, want %s", ErrTypeMismatch, reflect.TypeFor[T]())
		}
		return result, nil
	}

	root, ok := execResult.Root.(T)
	if !ok {
		return result, fmt.Errorf("%w: root node has value %T, want %s", ErrTypeMismatch, execResult.Root, reflect.TypeFor[T]())
	}
	result.root = root
	return result, nil
}

func (p *Plan[T]) resolveDB(db DBTX) DBTX {
	if db != nil {
		return db
	}
	return p.db
}

// DebugString returns a human-readable tree representation of the plan.
func (p *Plan[T]) DebugString() string {
	return debug.TreeString(p.graph)
}

// DryRunString returns the planned INSERT execution order with FK assignments.
// Each step shows which table will be inserted and how FK fields are populated
// from referenced parent field values. Provided nodes (via [Use]) are marked as skipped.
//
// This is useful for understanding how seedling will resolve dependencies
// before actually executing inserts.
func (p *Plan[T]) DryRunString() string {
	return debug.DryRunString(p.graph)
}

func (p *Plan[T]) toExecutorLogFn() func(executor.LogEntry) {
	return toExecutorLogFn(p.logFn)
}

func afterInsertTypeError[T any](fn any) error {
	return afterInsertTypeMismatch(reflect.TypeFor[T](), fn)
}

func afterInsertTypeMismatch(rootType reflect.Type, fn any) error {
	return fmt.Errorf("%w: after-insert callback has type %T; expected func(%s, seedling.DBTX) or func(%s, seedling.DBTX) error", ErrInvalidOption, fn, rootType, rootType)
}
