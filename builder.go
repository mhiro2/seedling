package seedling

import (
	"context"
	"math/rand/v2"
	"testing"
)

// Builder provides a fluent API for constructing and inserting records.
//
//	seedling.For[Task]().Set("Title", "urgent").Ref("project", seedling.Set("Name", "x")).Insert(t, db)
type Builder[T any] struct {
	session Session[T]
	opts    []Option
}

// For creates a new Builder for type T using the default registry.
func For[T any]() *Builder[T] {
	return &Builder[T]{session: NewSession[T](nil)}
}

// ForSession creates a new Builder for type T using a specific session.
func ForSession[T any](s Session[T]) *Builder[T] {
	return &Builder[T]{session: s}
}

// Set overrides a struct field value by its Go field name.
func (b *Builder[T]) Set(field string, value any) *Builder[T] {
	b.opts = append(b.opts, Set(field, value))
	return b
}

// Use provides an existing record for a direct relation, skipping auto-creation.
func (b *Builder[T]) Use(name string, value any) *Builder[T] {
	b.opts = append(b.opts, Use(name, value))
	return b
}

// Ref applies nested options to a specific relation's blueprint.
func (b *Builder[T]) Ref(name string, opts ...Option) *Builder[T] {
	b.opts = append(b.opts, Ref(name, opts...))
	return b
}

// Omit prevents auto-creation of an optional relation.
func (b *Builder[T]) Omit(name string) *Builder[T] {
	b.opts = append(b.opts, Omit(name))
	return b
}

// With applies a type-safe modification function to the root struct.
func (b *Builder[T]) With(fn func(*T)) *Builder[T] {
	b.opts = append(b.opts, With(fn))
	return b
}

// BlueprintTrait applies a named trait defined on the target blueprint.
func (b *Builder[T]) BlueprintTrait(name string) *Builder[T] {
	b.opts = append(b.opts, BlueprintTrait(name))
	return b
}

// InlineTrait applies an inline trait composed from explicit options.
func (b *Builder[T]) InlineTrait(opts ...Option) *Builder[T] {
	b.opts = append(b.opts, InlineTrait(opts...))
	return b
}

// Generate applies a rand-driven mutation function.
func (b *Builder[T]) Generate(fn func(*rand.Rand, *T)) *Builder[T] {
	b.opts = append(b.opts, Generate(fn))
	return b
}

// GenerateE applies a rand-driven mutation function that can return an error.
func (b *Builder[T]) GenerateE(fn func(*rand.Rand, *T) error) *Builder[T] {
	b.opts = append(b.opts, GenerateE(fn))
	return b
}

// WithContext sets the context used for insert operations.
func (b *Builder[T]) WithContext(ctx context.Context) *Builder[T] {
	b.opts = append(b.opts, WithContext(ctx))
	return b
}

// AfterInsert registers a callback that runs after the root record is inserted.
func (b *Builder[T]) AfterInsert(fn func(T, DBTX)) *Builder[T] {
	b.opts = append(b.opts, AfterInsert(fn))
	return b
}

// AfterInsertE registers an error-returning callback that runs after the root record is inserted.
func (b *Builder[T]) AfterInsertE(fn func(T, DBTX) error) *Builder[T] {
	b.opts = append(b.opts, AfterInsertE(fn))
	return b
}

// WithRand sets the RNG used by Generate options.
func (b *Builder[T]) WithRand(r *rand.Rand) *Builder[T] {
	b.opts = append(b.opts, WithRand(r))
	return b
}

// WithSeed sets the RNG seed used by Generate options.
func (b *Builder[T]) WithSeed(seed uint64) *Builder[T] {
	b.opts = append(b.opts, WithSeed(seed))
	return b
}

// Apply appends arbitrary options. Use this for options that require
// additional type parameters (e.g., Seq, SeqUse) which cannot be
// expressed as methods on Builder[T].
func (b *Builder[T]) Apply(opts ...Option) *Builder[T] {
	b.opts = append(b.opts, opts...)
	return b
}

// Insert creates and inserts a single record. Fails the test on error.
func (b *Builder[T]) Insert(tb testing.TB, db DBTX) Result[T] {
	tb.Helper()
	return b.session.InsertOne(tb, db, b.opts...)
}

// InsertE creates and inserts a single record, returning an error on failure.
func (b *Builder[T]) InsertE(ctx context.Context, db DBTX) (Result[T], error) {
	return b.session.InsertOneE(ctx, db, b.opts...)
}

// InsertMany creates and inserts n records. Shared belongs-to dependencies are
// inserted once when their resolved options are identical across records. Fails
// the test on error.
func (b *Builder[T]) InsertMany(tb testing.TB, db DBTX, n int) []T {
	tb.Helper()
	return b.session.InsertMany(tb, db, n, b.opts...)
}

// InsertManyE creates and inserts n records, returning an error on failure.
// Shared belongs-to dependencies are inserted once when their resolved options
// are identical across records.
func (b *Builder[T]) InsertManyE(ctx context.Context, db DBTX, n int) ([]T, error) {
	return b.session.InsertManyE(ctx, db, n, b.opts...)
}

// Build constructs a dependency plan without inserting anything. Fails the test on error.
func (b *Builder[T]) Build(tb testing.TB) *Plan[T] {
	tb.Helper()
	return b.session.Build(tb, b.opts...)
}

// BuildE constructs a dependency plan without inserting anything.
func (b *Builder[T]) BuildE() (*Plan[T], error) {
	return b.session.BuildE(b.opts...)
}
