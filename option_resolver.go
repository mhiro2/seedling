package seedling

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"

	"github.com/mhiro2/seedling/internal/planner"
)

// optionSet holds the resolved options for a single node. Trait, sequence,
// and Ref options never appear here: they resolve into the plain options they
// stand for, and Ref options resolve into the child option set of their
// relation.
type optionSet struct {
	bp    *blueprintDef // blueprint the options target; nil when a Ref names an unresolvable relation
	bpErr error         // why bp could not be resolved
	root  bool          // whether the options target the root record

	sets         []planner.FieldSet                 // Set options in the order they were written
	uses         map[string]any                     // relation name → existing value
	refs         map[string]*optionSet              // relation name → nested options
	omits        map[string]bool                    // relation name → true
	whens        map[string]func(any) (bool, error) // relation name → dynamic expansion predicate
	withFns      []withFn                           // typed root mutators
	ctx          context.Context                    // optional context for insert operations
	afterInserts []any                              // func(T, DBTX) closures run after insert
	genFns       []generateFn                       // typed rand-driven mutators
	rand         *rand.Rand                         // optional RNG for Generate options
	logFn        func(InsertLog)                    // optional insert log callback
	only         map[string]bool                    // Only option: nil = insert all, non-nil = insert only specified relations
}

type withFn func(value any) (any, error)

type generateFn func(r *rand.Rand, value any) (any, error)

func newOptionSet(bp *blueprintDef, root bool) *optionSet {
	return &optionSet{
		bp:    bp,
		root:  root,
		uses:  make(map[string]any),
		refs:  make(map[string]*optionSet),
		omits: make(map[string]bool),
	}
}

// noSequenceIndex marks a resolution outside InsertMany, where Seq, SeqRef,
// and SeqUse are rejected.
const noSequenceIndex = -1

// optionResolver turns an option stream into resolved option sets in a single
// pass shared by every insert path. Options apply in the order they are
// written: a trait expands in place, a sequence option becomes the plain
// option it produces for the current record, and a Ref resolves its options
// against the blueprint of the relation it names. Placement rules, such as
// options limited to the root record, are enforced as each option resolves, so
// an option behaves the same whether it is written directly, inside a trait,
// or returned by SeqRef.
type optionResolver struct {
	registry *registry
	rootType reflect.Type
	seqIndex int        // 0-based record index of an InsertMany batch, or noSequenceIndex
	active   []traitKey // traits being expanded, across Ref boundaries
}

// traitKey identifies a trait on a specific blueprint while it is being expanded.
type traitKey struct {
	blueprint string
	trait     string
}

func (k traitKey) String() string { return k.blueprint + ":" + k.trait }

// resolveRootOptions resolves the root options of an insert. seqIndex is the
// 0-based record index of an InsertMany batch, or noSequenceIndex.
func resolveRootOptions(reg *Registry, rootType reflect.Type, opts []Option, seqIndex int) (*optionSet, error) {
	r := resolveRegistry(reg).reg
	def, err := r.lookupByType(rootType)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	root := newOptionSet(def, true)
	res := &optionResolver{registry: r, rootType: rootType, seqIndex: seqIndex}
	if err := res.resolve(opts, root); err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	return root, nil
}

// resolve applies opts to o in the order they are written.
func (r *optionResolver) resolve(opts []Option, o *optionSet) error {
	for _, opt := range opts {
		if err := opt.applyOption(r, o); err != nil {
			return err
		}
	}
	return nil
}

// resolveTrait expands the named trait of o's blueprint in place. A trait that
// (transitively) reaches itself, on the same blueprint or through a relation
// back to it, is reported instead of looping.
func (r *optionResolver) resolveTrait(name string, o *optionSet) error {
	if o.bp == nil {
		return o.bpErr
	}
	key := traitKey{blueprint: o.bp.name, trait: name}
	if slices.Contains(r.active, key) {
		chain := make([]string, 0, len(r.active)+1)
		for _, k := range append(slices.Clone(r.active), key) {
			chain = append(chain, k.String())
		}
		return fmt.Errorf("%w: trait %q on blueprint %q references itself via %s", ErrInvalidOption, name, o.bp.name, strings.Join(chain, " -> "))
	}
	traitOpts, ok := o.bp.traits[name]
	if !ok {
		return fmt.Errorf("%w: trait %q not defined on blueprint %q", ErrInvalidOption, name, o.bp.name)
	}
	r.active = append(r.active, key)
	defer func() { r.active = r.active[:len(r.active)-1] }()
	return r.resolve(traitOpts, o)
}

// refTarget returns the option set for relation name under o, creating it on
// first use so that every Ref to the same relation merges in written order.
// An unknown relation is left for the planner to report with its usual hints,
// unless a trait under it needs the blueprint.
func (r *optionResolver) refTarget(o *optionSet, name string) *optionSet {
	if child, ok := o.refs[name]; ok {
		return child
	}
	def, err := findRefBlueprint(o.bp, name, r.registry)
	child := newOptionSet(def, false)
	child.bpErr = err
	o.refs[name] = child
	return child
}

// sequenceIndex returns the record index a sequence option resolves for.
func (r *optionResolver) sequenceIndex() (int, error) {
	if r.seqIndex == noSequenceIndex {
		return 0, fmt.Errorf("%w: Seq, SeqRef, and SeqUse are only supported by InsertMany", ErrInvalidOption)
	}
	return r.seqIndex, nil
}

// addAfterInsert records an after-insert callback, rejecting one outside the
// root options, a nil callback that would panic when invoked, and one whose
// record type does not match the root blueprint type, e.g. AfterInsert[Post]
// passed to InsertOne[User].
func (o *optionSet) addAfterInsert(r *optionResolver, fn any) error {
	if !o.root {
		return rootOnlyError("after-insert")
	}
	fv := reflect.ValueOf(fn)
	if fv.IsNil() {
		return fmt.Errorf("%w: after-insert callback must not be nil", ErrInvalidOption)
	}
	ft := fv.Type()
	if ft.NumIn() != 2 || ft.In(0) != r.rootType || ft.In(1) != reflect.TypeFor[DBTX]() {
		return afterInsertTypeMismatch(r.rootType, fn)
	}
	o.afterInserts = append(o.afterInserts, fn)
	return nil
}

func rootOnlyError(option string) error {
	return fmt.Errorf("%w: %s applies only to the root blueprint", ErrInvalidOption, option)
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

// toOptionSet converts resolved options into the planner's option tree.
func toOptionSet(os *optionSet) *planner.OptionSet {
	if os == nil {
		return nil
	}

	refs := make(map[string]*planner.OptionSet, len(os.refs))
	for name, refOpts := range os.refs {
		refs[name] = toOptionSet(refOpts)
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
		GenFns:  genFns,
		Rand:    os.rand,
		Only:    os.only,
	}
}
