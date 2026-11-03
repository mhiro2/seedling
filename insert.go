package seedling

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/executor"
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
	result, err := s.forTest(tb).InsertOneE(tb.Context(), db, opts...)
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
	plan, err := s.BuildE(opts...)
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
	result, err := s.forTest(tb).InsertManyE(tb.Context(), db, n, opts...)
	if err != nil {
		tb.Fatal(err)
	}
	return result.rootsView()
}

// InsertManyE creates and inserts n records of type T, returning a [BatchResult]
// for cleanup and graph inspection.
// When Seq options are present, the sequence function is called with the 0-based
// index for each record. Shared belongs-to dependencies are inserted once when
// their resolved options are identical across records.
// If execution fails after inserting some nodes, the returned BatchResult
// contains every successful node and only the roots that completed successfully.
func InsertManyE[T any](ctx context.Context, db DBTX, n int, opts ...Option) (BatchResult[T], error) {
	return NewSession[T](nil).InsertManyE(ctx, db, n, opts...)
}

// InsertManyE creates and inserts n records of type T, returning a [BatchResult]
// for cleanup and graph inspection.
// When Seq options are present, the sequence function is called with the 0-based
// index for each record. Shared belongs-to dependencies are inserted once when
// their resolved options are identical across records.
// If execution fails after inserting some nodes, the returned BatchResult
// contains every successful node and only the roots that completed successfully.
func (s Session[T]) InsertManyE(ctx context.Context, db DBTX, n int, opts ...Option) (BatchResult[T], error) {
	plan, err := s.BuildManyE(n, opts...)
	if err != nil {
		var zero BatchResult[T]
		return zero, err
	}
	return plan.InsertE(ctx, s.resolveDB(db))
}

func batchResultFromExecutor[T any](reg *Registry, rootIDs []string, execResult *executor.Result, requireAllRoots bool) (BatchResult[T], error) {
	if execResult == nil {
		var zero BatchResult[T]
		return zero, fmt.Errorf("%w: executor returned a nil result", ErrInvalidOption)
	}

	result := BatchResult[T]{
		roots:         make([]T, 0, len(rootIDs)),
		rootIndices:   make([]int, 0, len(rootIDs)),
		rootIDs:       append([]string(nil), rootIDs...),
		nodes:         execResult.Nodes,
		graph:         execResult.Graph,
		registry:      reg,
		deleteFns:     snapshotDeleteFns(reg, execResult.Nodes),
		cleanupValues: execResult.Inserted,
	}
	rootType := reflect.TypeFor[T]()
	for rootIndex, rootID := range rootIDs {
		node, ok := execResult.Nodes[rootID]
		if !ok {
			if requireAllRoots {
				return result, fmt.Errorf("seedling: root node %q not found in batch result", rootID)
			}
			continue
		}

		root, ok := node.Value.(T)
		if !ok {
			return result, fmt.Errorf("%w: root node %q has value %T, want %s", ErrTypeMismatch, rootID, node.Value, rootType)
		}
		result.roots = append(result.roots, root)
		result.rootIndices = append(result.rootIndices, rootIndex)
	}

	return result, nil
}

// insertContext picks the context an insert runs with. A WithContext option,
// resolved after trait expansion, overrides the context the caller passed.
func insertContext(ctx, override context.Context) context.Context {
	if override != nil {
		return override
	}
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
