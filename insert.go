package seedling

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/planner"
)

// InsertOne creates and inserts a single record of type T with all required
// dependencies automatically resolved. Fails the test on error.
func InsertOne[T any](tb testing.TB, db DBTX, opts ...Option) Result[T] {
	tb.Helper()
	return NewSession[T](nil).InsertOne(tb, db, opts...)
}

// InsertOne creates and inserts a single record of type T with all required
// dependencies automatically resolved. Fails the test on error.
func (s Session[T]) InsertOne(tb testing.TB, db DBTX, opts ...Option) Result[T] {
	tb.Helper()
	ctx, filtered := extractContext(tb.Context(), opts)
	result, err := s.InsertOneE(ctx, db, filtered...)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// InsertOneE creates and inserts a single record of type T, returning an error on failure.
func InsertOneE[T any](ctx context.Context, db DBTX, opts ...Option) (Result[T], error) {
	return NewSession[T](nil).InsertOneE(ctx, db, opts...)
}

// InsertOneE creates and inserts a single record of type T, returning an error on failure.
func (s Session[T]) InsertOneE(ctx context.Context, db DBTX, opts ...Option) (Result[T], error) {
	ctx, filtered := extractContext(ctx, opts)
	plan, err := s.BuildE(filtered...)
	if err != nil {
		var zero Result[T]
		return zero, err
	}
	return plan.InsertE(ctx, s.resolveDB(db))
}

// InsertMany creates and inserts n records of type T with the same options.
// Shared belongs-to dependencies are inserted once when their resolved options
// are identical across records. Fails the test on error.
func InsertMany[T any](tb testing.TB, db DBTX, n int, opts ...Option) []T {
	tb.Helper()
	return NewSession[T](nil).InsertMany(tb, db, n, opts...)
}

// InsertMany creates and inserts n records of type T with the same options.
// Shared belongs-to dependencies are inserted once when their resolved options
// are identical across records. Fails the test on error.
func (s Session[T]) InsertMany(tb testing.TB, db DBTX, n int, opts ...Option) []T {
	tb.Helper()
	ctx, filtered := extractContext(tb.Context(), opts)
	result, err := s.InsertManyE(ctx, db, n, filtered...)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// InsertManyE creates and inserts n records of type T, returning an error on failure.
// When Seq options are present, the sequence function is called with the 0-based
// index for each record. Shared belongs-to dependencies are inserted once when
// their resolved options are identical across records.
func InsertManyE[T any](ctx context.Context, db DBTX, n int, opts ...Option) ([]T, error) {
	return NewSession[T](nil).InsertManyE(ctx, db, n, opts...)
}

// InsertManyE creates and inserts n records of type T, returning an error on failure.
// When Seq options are present, the sequence function is called with the 0-based
// index for each record. Shared belongs-to dependencies are inserted once when
// their resolved options are identical across records.
func (s Session[T]) InsertManyE(ctx context.Context, db DBTX, n int, opts ...Option) ([]T, error) {
	ctx, opts = extractContext(ctx, opts)
	if n < 0 {
		return nil, fmt.Errorf("validate insert count: n must be >= 0, got %d: %w", n, ErrInvalidOption)
	}
	if n == 0 {
		return []T{}, nil
	}

	// Validate InsertMany-incompatible options before doing any work.
	precheck := collectOptions(opts)
	if err := validateInsertManyOptions(precheck); err != nil {
		return nil, err
	}

	rootType := reflect.TypeFor[T]()
	collected := make([]*optionSet, n)
	internalOpts := make([]*planner.OptionSet, n)

	for i := range n {
		resolved := resolveSeqs(opts, i)
		prepared, err := prepareRootOptions(s.registry, rootType, resolved)
		if err != nil {
			return nil, err
		}
		collected[i] = prepared
		internalOpts[i] = toOptionSet(prepared)
	}

	adapter := newRegistryAdapter(s.registry)
	plan, err := planner.PlanMany(adapter, rootType, internalOpts)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	// Extract logFn from the first collected optionSet (all share the same logFn).
	var execLogFn func(executor.LogEntry)
	if collected[0].logFn != nil {
		logFn := collected[0].logFn
		execLogFn = func(entry executor.LogEntry) {
			bindings := make([]FKBinding, len(entry.FKBindings))
			for i, b := range entry.FKBindings {
				bindings[i] = FKBinding{
					ChildField:      b.ChildField,
					ParentBlueprint: b.ParentBlueprint,
					ParentTable:     b.ParentTable,
					ParentField:     b.ParentField,
					Value:           b.Value,
				}
			}
			logFn(InsertLog{
				Step:       entry.Step,
				Blueprint:  entry.Blueprint,
				Table:      entry.Table,
				Provided:   entry.Provided,
				FKBindings: bindings,
			})
		}
	}

	execResult, err := executor.Execute(ctx, s.resolveDB(db), plan.Graph, adapter, execLogFn)
	if err != nil {
		return nil, fmt.Errorf("execute plan: %w", err)
	}

	results := make([]T, len(plan.RootIDs))
	for i, rootID := range plan.RootIDs {
		node, ok := execResult.Nodes[rootID]
		if !ok {
			return results, fmt.Errorf("seedling: root node %q not found in batch result", rootID)
		}

		root, ok := node.Value.(T)
		if !ok {
			return results, fmt.Errorf("%w: root node %q has value %T, want %s", ErrTypeMismatch, rootID, node.Value, rootType)
		}

		results[i] = root
		for _, fn := range collected[i].afterInserts {
			switch cb := fn.(type) {
			case func(T, DBTX):
				cb(root, s.resolveDB(db))
			case func(T, DBTX) error:
				if err := cb(root, s.resolveDB(db)); err != nil {
					return results, fmt.Errorf("run after-insert callback: %w", err)
				}
			}
		}
	}

	return results, nil
}

// extractContext extracts a WithContext option from opts and returns
// the context and the remaining options.
func extractContext(defaultCtx context.Context, opts []Option) (context.Context, []Option) {
	ctx := defaultCtx
	filtered := make([]Option, 0, len(opts))
	for _, o := range opts {
		if co, ok := o.(contextOption); ok {
			ctx = co.ctx
			continue
		}
		filtered = append(filtered, o)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx, filtered
}

// resolveSeqs converts Seq options into Set options by calling the sequence
// function with the given index. Non-Seq options are passed through.
func resolveSeqs(opts []Option, index int) []Option {
	resolved := make([]Option, 0, len(opts))
	for _, o := range opts {
		switch sq := o.(type) {
		case seqOption:
			resolved = append(resolved, Set(sq.field, sq.fn(index)))
		case seqRefOption:
			resolved = append(resolved, Ref(sq.name, resolveSeqs(sq.fn(index), index)...))
		case seqUseOption:
			resolved = append(resolved, Use(sq.name, sq.fn(index)))
		case refOption:
			resolved = append(resolved, Ref(sq.name, resolveSeqs(sq.opts, index)...))
		case inlineTraitOption:
			resolved = append(resolved, InlineTrait(resolveSeqs(sq.opts, index)...))
		default:
			resolved = append(resolved, o)
		}
	}
	return resolved
}
