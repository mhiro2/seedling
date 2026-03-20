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
	logFn        func(InsertLog)
}

// BuildE constructs a dependency plan for type T without inserting anything.
func BuildE[T any](opts ...Option) (*Plan[T], error) {
	return NewSession[T](nil).BuildE(opts...)
}

// BuildE constructs a dependency plan for type T without inserting anything.
func (s Session[T]) BuildE(opts ...Option) (*Plan[T], error) {
	rootType := reflect.TypeFor[T]()

	collected, err := prepareRootOptions(s.registry, rootType, opts)
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
	ctx := p.ctx
	if ctx == nil {
		ctx = tb.Context()
	}
	result, err := p.InsertE(ctx, db)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// InsertE executes the plan and inserts all records, returning an error on failure.
func (p *Plan[T]) InsertE(ctx context.Context, db DBTX) (Result[T], error) {
	if ctx == nil {
		ctx = context.Background()
	}
	adapter := newRegistryAdapter(p.registry)
	g := p.graph.Clone()
	execResult, err := executor.Execute(ctx, db, g, adapter, p.toExecutorLogFn())
	if err != nil {
		var zero Result[T]
		return zero, fmt.Errorf("execute plan: %w", err)
	}

	root := execResult.Root.(T)

	result := Result[T]{
		root:      root,
		nodes:     execResult.Nodes,
		graph:     execResult.Graph,
		registry:  p.registry,
		deleteFns: snapshotDeleteFns(p.registry, execResult.Nodes),
	}

	// Run AfterInsert callbacks.
	// On failure the result is still returned so callers can clean up
	// already-inserted records via Result.Cleanup.
	for _, fn := range p.afterInserts {
		switch cb := fn.(type) {
		case func(T, DBTX):
			cb(root, db)
		case func(T, DBTX) error:
			if err := cb(root, db); err != nil {
				return result, fmt.Errorf("run after-insert callback: %w", err)
			}
		}
	}

	return result, nil
}

// DebugString returns a human-readable tree representation of the plan.
func (p *Plan[T]) DebugString() string {
	return debug.TreeString(p.graph)
}

// DryRunString returns the planned INSERT execution order with FK assignments.
// Each step shows which table will be inserted and how FK fields are populated
// from parent PK values. Provided nodes (via [Use]) are marked as skipped.
//
// This is useful for understanding how seedling will resolve dependencies
// before actually executing inserts.
func (p *Plan[T]) DryRunString() string {
	return debug.DryRunString(p.graph)
}

func (p *Plan[T]) toExecutorLogFn() func(executor.LogEntry) {
	return toExecutorLogFn(p.logFn)
}

func prepareRootOptions(reg *Registry, rootType reflect.Type, opts []Option) (*optionSet, error) {
	collected := collectOptions(opts)

	r := resolveRegistry(reg).reg
	def, err := r.lookupByType(rootType)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	if err := resolveAllTraits(collected, def, r); err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	if err := validateResolvedOptions(collected); err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	return collected, nil
}

// resolveAllTraits resolves trait names on the optionSet and recursively on
// all nested Ref options. For each Ref, the target blueprint is determined
// from the relation definition so that traits can be looked up correctly.
func resolveAllTraits(os *optionSet, def *blueprintDef, r *registry) error {
	if err := resolveTraits(os, def); err != nil {
		return err
	}
	for name, refOpts := range os.refs {
		if !hasTraits(refOpts) {
			continue
		}
		refBP, err := findRefBlueprint(def, name, r)
		if err != nil {
			// Unknown relation names will be caught by the planner validator.
			continue
		}
		refCollected := collectOptions(refOpts)
		if err := resolveAllTraits(refCollected, refBP, r); err != nil {
			return fmt.Errorf("resolve traits for ref %q: %w", name, err)
		}
		os.refs[name] = reconstructOptions(refCollected)
	}
	return nil
}

// hasTraits checks if any option in the slice is a trait option.
func hasTraits(opts []Option) bool {
	for _, o := range opts {
		switch o := o.(type) {
		case blueprintTraitOption:
			return true
		case refOption:
			if hasTraits(o.opts) {
				return true
			}
		case inlineTraitOption:
			if hasTraits(o.opts) {
				return true
			}
		}
	}
	return false
}

// findRefBlueprint looks up the blueprint targeted by a relation name.
func findRefBlueprint(def *blueprintDef, relationName string, r *registry) (*blueprintDef, error) {
	for _, rel := range def.relations {
		if rel.name == relationName {
			return r.lookupByName(rel.refBlueprint)
		}
	}
	return nil, fmt.Errorf("relation %q not found on blueprint %q", relationName, def.name)
}

// reconstructOptions converts an optionSet back into a slice of Options so
// it can be stored in refs and later re-collected by toOptionSet.
func reconstructOptions(os *optionSet) []Option {
	var opts []Option
	for field, value := range os.sets {
		opts = append(opts, Set(field, value))
	}
	for name, value := range os.uses {
		opts = append(opts, Use(name, value))
	}
	for name, refOpts := range os.refs {
		opts = append(opts, Ref(name, refOpts...))
	}
	for name := range os.omits {
		opts = append(opts, Omit(name))
	}
	for name, fn := range os.whens {
		opts = append(opts, whenOption{name: name, fn: fn})
	}
	for _, fn := range os.withFns {
		opts = append(opts, withFnOption{fn: fn})
	}
	for _, fn := range os.afterInserts {
		opts = append(opts, rawAfterInsertOption{fn: fn})
	}
	for _, fn := range os.genFns {
		opts = append(opts, rawGenerateOption{fn: fn})
	}
	if os.rand != nil {
		opts = append(opts, WithRand(os.rand))
	}
	if os.ctx != nil {
		opts = append(opts, WithContext(os.ctx))
	}
	if os.logFn != nil {
		opts = append(opts, WithInsertLog(os.logFn))
	}
	return opts
}

// resolveTraits expands deferred trait names by looking them up from the
// blueprint's registered traits and applying their options to the optionSet.
// Traits are resolved iteratively to support trait-of-trait references.
func resolveTraits(os *optionSet, def *blueprintDef) error {
	seen := make(map[string]bool)
	for len(os.traits) > 0 {
		pending := os.traits
		os.traits = nil
		for _, name := range pending {
			if seen[name] {
				continue
			}
			seen[name] = true
			traitOpts, ok := def.traits[name]
			if !ok {
				return fmt.Errorf("%w: trait %q not defined on blueprint %q", ErrInvalidOption, name, def.name)
			}
			for _, o := range traitOpts {
				o.applyOption(os)
			}
		}
	}
	return nil
}

func toOptionSet(os *optionSet) *planner.OptionSet {
	if os == nil {
		return nil
	}

	refs := make(map[string]*planner.OptionSet, len(os.refs))
	for name, refOpts := range os.refs {
		refs[name] = toOptionSet(collectOptions(refOpts))
	}
	withFns := make([]planner.WithFn, len(os.withFns))
	for i, fn := range os.withFns {
		withFns[i] = planner.WithFn(fn)
	}
	genFns := make([]planner.GenerateFn, len(os.genFns))
	for i, fn := range os.genFns {
		genFns[i] = planner.GenerateFn(fn)
	}

	return &planner.OptionSet{
		Sets:    os.sets,
		Uses:    os.uses,
		Refs:    refs,
		Omits:   os.omits,
		Whens:   os.whens,
		WithFns: withFns,
		Seqs:    os.seqs,
		GenFns:  genFns,
		Rand:    os.rand,
		Only:    os.only,
	}
}

func validateResolvedOptions(os *optionSet) error {
	if os == nil {
		return nil
	}
	if len(os.seqs) > 0 || len(os.seqRefs) > 0 || len(os.seqUses) > 0 {
		return fmt.Errorf("%w: Seq, SeqRef, and SeqUse are only supported by InsertMany", ErrInvalidOption)
	}
	for name, refOpts := range os.refs {
		if err := validateResolvedOptions(collectOptions(refOpts)); err != nil {
			return fmt.Errorf("%w: ref %q", err, name)
		}
	}
	return nil
}
