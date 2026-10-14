package seedling

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
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

	collected, err := prepareRootOptions(s.registry, rootType, opts, noSequenceIndex)
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

// noSequenceIndex marks an option expansion outside InsertMany, where Seq,
// SeqRef, and SeqUse stay unresolved and are rejected by validation.
const noSequenceIndex = -1

// prepareRootOptions expands and validates the root options. seqIndex is the
// 0-based record index of an InsertMany batch, or noSequenceIndex.
func prepareRootOptions(reg *Registry, rootType reflect.Type, opts []Option, seqIndex int) (*optionSet, error) {
	r := resolveRegistry(reg).reg
	def, err := r.lookupByType(rootType)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	expanded, err := (&optionExpander{registry: r, seqIndex: seqIndex}).expand(opts, def, nil)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	collected := collectOptions(expanded)
	if err := validateResolvedOptions(collected, true); err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	if err := validateAfterInserts(rootType, collected.afterInserts); err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	return collected, nil
}

// optionExpander resolves an option stream into the plain options the planner
// consumes. Traits expand in place, so a trait contributes its options exactly
// where it was written and later options keep overriding earlier ones. Options
// nested under Ref are expanded against the blueprint the relation targets.
// Inside InsertMany, sequence options are resolved for the record at seqIndex in
// the same pass, so a Seq defined in a trait and a trait returned by SeqRef
// behave like options written directly.
type optionExpander struct {
	registry *registry
	seqIndex int
}

// traitKey identifies a trait on a specific blueprint while it is being expanded.
type traitKey struct {
	blueprint string
	trait     string
}

func (k traitKey) String() string { return k.blueprint + ":" + k.trait }

// expand is the recursive worker of optionExpander. def is nil under a relation
// name that does not resolve. active holds the chain of traits currently being
// expanded, across Ref boundaries, so that a trait that (transitively)
// references itself — on the same blueprint or through a relation back to it —
// is reported instead of looping.
func (x *optionExpander) expand(opts []Option, def *blueprintDef, active []traitKey) ([]Option, error) {
	out := make([]Option, 0, len(opts))
	for _, o := range opts {
		if x.seqIndex != noSequenceIndex {
			o = x.resolveSequence(o)
		}
		switch o := o.(type) {
		case blueprintTraitOption:
			key := traitKey{blueprint: def.name, trait: o.name}
			if slices.Contains(active, key) {
				chain := make([]string, 0, len(active)+1)
				for _, k := range append(slices.Clone(active), key) {
					chain = append(chain, k.String())
				}
				return nil, fmt.Errorf("%w: trait %q on blueprint %q references itself via %s", ErrInvalidOption, o.name, def.name, strings.Join(chain, " -> "))
			}
			traitOpts, ok := def.traits[o.name]
			if !ok {
				return nil, fmt.Errorf("%w: trait %q not defined on blueprint %q", ErrInvalidOption, o.name, def.name)
			}
			expanded, err := x.expand(traitOpts, def, append(slices.Clone(active), key))
			if err != nil {
				return nil, err
			}
			out = append(out, expanded...)
		case inlineTraitOption:
			expanded, err := x.expand(o.opts, def, active)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded...)
		case refOption:
			refBP, err := findRefBlueprint(def, o.name, x.registry)
			if err != nil {
				if hasTraits(o.opts) {
					return nil, fmt.Errorf("resolve traits for ref %q: %w", o.name, err)
				}
				// Unknown relation names without traits are reported by the planner
				// validator with its usual hints.
				refBP = nil
			}
			expanded, err := x.expand(o.opts, refBP, active)
			if err != nil {
				return nil, fmt.Errorf("resolve traits for ref %q: %w", o.name, err)
			}
			out = append(out, refOption{name: o.name, opts: expanded})
		default:
			out = append(out, o)
		}
	}
	return out, nil
}

// resolveSequence turns a sequence option into the plain option it produces for
// the current record. Other options are returned unchanged.
func (x *optionExpander) resolveSequence(o Option) Option {
	switch sq := o.(type) {
	case seqOption:
		return setOption{field: sq.field, value: sq.fn(x.seqIndex)}
	case seqRefOption:
		return refOption{name: sq.name, opts: sq.fn(x.seqIndex)}
	case seqUseOption:
		return useOption{name: sq.name, value: sq.fn(x.seqIndex)}
	default:
		return o
	}
}

// hasTraits reports whether opts contains a blueprint trait option at any depth.
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

// findRefBlueprint looks up the blueprint targeted by a relation name. A nil
// def stands for an unresolved relation, under which no name resolves.
func findRefBlueprint(def *blueprintDef, relationName string, r *registry) (*blueprintDef, error) {
	if def == nil {
		return nil, fmt.Errorf("%w: relation %q not found", ErrInvalidOption, relationName)
	}
	for _, rel := range def.relations {
		if rel.name == relationName {
			return r.lookupByName(rel.refBlueprint)
		}
	}
	return nil, fmt.Errorf("%w: relation %q not found on blueprint %q", ErrInvalidOption, relationName, def.name)
}

// validateAfterInserts rejects AfterInsert callbacks whose record type does not
// match the root blueprint type, e.g. AfterInsert[Post] passed to InsertOne[User],
// and nil callbacks that would panic when invoked.
func validateAfterInserts(rootType reflect.Type, fns []any) error {
	dbType := reflect.TypeFor[DBTX]()
	for _, fn := range fns {
		fv := reflect.ValueOf(fn)
		if !fv.IsValid() || fv.Kind() != reflect.Func {
			return afterInsertTypeMismatch(rootType, fn)
		}
		if fv.IsNil() {
			return fmt.Errorf("%w: after-insert callback must not be nil", ErrInvalidOption)
		}
		ft := fv.Type()
		if ft.NumIn() != 2 || ft.In(0) != rootType || ft.In(1) != dbType {
			return afterInsertTypeMismatch(rootType, fn)
		}
	}
	return nil
}

func afterInsertTypeError[T any](fn any) error {
	return afterInsertTypeMismatch(reflect.TypeFor[T](), fn)
}

func afterInsertTypeMismatch(rootType reflect.Type, fn any) error {
	return fmt.Errorf("%w: after-insert callback has type %T; expected func(%s, seedling.DBTX) or func(%s, seedling.DBTX) error", ErrInvalidOption, fn, rootType, rootType)
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

func validateResolvedOptions(os *optionSet, root bool) error {
	if os == nil {
		return nil
	}
	if !root {
		if os.ctx != nil {
			return fmt.Errorf("%w: with-context applies only to the root blueprint", ErrInvalidOption)
		}
		if len(os.afterInserts) > 0 {
			return fmt.Errorf("%w: after-insert applies only to the root blueprint", ErrInvalidOption)
		}
		if os.logFn != nil {
			return fmt.Errorf("%w: insert log applies only to the root blueprint", ErrInvalidOption)
		}
	}
	if len(os.seqs) > 0 || len(os.seqRefs) > 0 || len(os.seqUses) > 0 {
		return fmt.Errorf("%w: Seq, SeqRef, and SeqUse are only supported by InsertMany", ErrInvalidOption)
	}
	if len(os.traits) > 0 {
		// Traits are expanded before collection; a leftover means an option
		// stream bypassed optionExpander.
		return fmt.Errorf("%w: trait %q was not expanded", ErrInvalidOption, os.traits[0])
	}
	for name, refOpts := range os.refs {
		if err := validateResolvedOptions(collectOptions(refOpts), false); err != nil {
			return fmt.Errorf("validate options for ref %q: %w", name, err)
		}
	}
	return nil
}
