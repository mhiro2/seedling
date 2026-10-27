package seedling

import (
	"context"
	"fmt"
	"maps"
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
	uses         map[string]any                     // belongs-to relation name → existing value
	refs         map[string]*optionSet              // belongs-to relation name → nested options
	collections  map[string]*collectionTarget       // has-many/many-to-many relation name → element options
	elem         *elementScope                      // set when the options target one element of a collection
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
		bp:          bp,
		root:        root,
		uses:        make(map[string]any),
		refs:        make(map[string]*optionSet),
		collections: make(map[string]*collectionTarget),
		omits:       make(map[string]bool),
	}
}

// collectionTarget gathers the options given for a has-many or many-to-many
// relation. Its elements are resolved once the whole option tree is known,
// because the number of elements and the options of each one can come from
// any Ref, Use, or trait in the tree.
type collectionTarget struct {
	rel    relationDef
	bp     *blueprintDef // blueprint of each element
	joinBP *blueprintDef // join blueprint of a many-to-many relation; nil for has-many

	count    int  // element count from Count
	hasCount bool // whether Count was given
	use      any  // existing records from Use (many-to-many only)
	hasUse   bool // whether Use was given
	refs     []deferredRef

	atIndices []int // At indices seen while resolving elements
	elements  []*optionSet
	used      []any // existing record of each element when hasUse is set
}

// deferredRef holds the options of one Ref to a collection relation together
// with the traits that were expanding when it was written, so trait cycles are
// still detected when its options resolve per element.
type deferredRef struct {
	opts   []Option
	active []traitKey
}

// elementScope identifies the collection element an option set configures.
type elementScope struct {
	index       int
	target      *collectionTarget
	through     *optionSet // join record options; nil for has-many
	throughUsed bool       // whether any Through option was given
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
	if err := res.resolveCollections(root); err != nil {
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

// collectionRelation returns the has-many or many-to-many relation name of o's
// blueprint, if there is one.
func collectionRelation(o *optionSet, name string) (relationDef, bool) {
	if o.bp == nil {
		return relationDef{}, false
	}
	for _, rel := range o.bp.relations {
		if rel.name == name {
			return rel, rel.kind == HasMany || rel.kind == ManyToMany
		}
	}
	return relationDef{}, false
}

// collectionTarget returns the collection options of relation rel under o,
// creating them on first use so that every Ref and Use to the relation merges
// in written order.
func (r *optionResolver) collectionTarget(o *optionSet, rel relationDef) (*collectionTarget, error) {
	if t, ok := o.collections[rel.name]; ok {
		return t, nil
	}
	bp, err := r.registry.lookupByName(rel.refBlueprint)
	if err != nil {
		return nil, fmt.Errorf("resolve relation %q: %w", rel.name, err)
	}
	t := &collectionTarget{rel: rel, bp: bp}
	if rel.kind == ManyToMany && rel.throughBlueprint != "" {
		if t.joinBP, err = r.registry.lookupByName(rel.throughBlueprint); err != nil {
			return nil, fmt.Errorf("resolve relation %q: %w", rel.name, err)
		}
	}
	o.collections[rel.name] = t
	return t, nil
}

// addRef records the options of a Ref to the collection. Count applies to the
// relation itself, so it is taken out here; everything else resolves per
// element.
func (t *collectionTarget) addRef(r *optionResolver, opts []Option) error {
	rest := make([]Option, 0, len(opts))
	for _, opt := range opts {
		c, ok := opt.(countOption)
		if !ok {
			rest = append(rest, opt)
			continue
		}
		if c.n < 0 {
			return fmt.Errorf("%w: Count for relation %q must be >= 0, got %d", ErrInvalidOption, t.rel.name, c.n)
		}
		t.count, t.hasCount = c.n, true
	}
	t.refs = append(t.refs, deferredRef{opts: rest, active: slices.Clone(r.active)})
	return nil
}

// resolveCollections resolves the elements of every collection relation in
// the option tree rooted at o.
func (r *optionResolver) resolveCollections(o *optionSet) error {
	for _, name := range slices.Sorted(maps.Keys(o.refs)) {
		if err := r.resolveCollections(o.refs[name]); err != nil {
			return fmt.Errorf("resolve options for ref %q: %w", name, err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(o.collections)) {
		if err := r.resolveElements(o.collections[name]); err != nil {
			return fmt.Errorf("resolve options for ref %q: %w", name, err)
		}
	}
	return nil
}

// resolveElements decides the number of elements of a collection relation and
// resolves the options of each one. Every Ref option applies to each element in
// written order, with At and Each narrowing options to particular elements.
func (r *optionResolver) resolveElements(t *collectionTarget) error {
	count := t.rel.count
	if t.hasCount {
		count = t.count
	}
	if t.hasUse {
		used, err := useElements(t.rel.name, t.use)
		if err != nil {
			return err
		}
		if t.hasCount && t.count != len(used) {
			return fmt.Errorf("%w: relation %q has Count(%d) but Use provides %d records", ErrInvalidOption, t.rel.name, t.count, len(used))
		}
		count = len(used)
		t.used = used
	}
	if count == 0 && slices.ContainsFunc(t.refs, func(ref deferredRef) bool { return len(ref.opts) > 0 }) {
		return fmt.Errorf("%w: relation %q creates no records, so its options apply to nothing; pass Count(n) to Ref", ErrInvalidOption, t.rel.name)
	}

	saved := r.active
	defer func() { r.active = saved }()
	t.elements = make([]*optionSet, count)
	for i := range count {
		elem := newOptionSet(t.bp, false)
		elem.elem = &elementScope{index: i, target: t}
		if t.joinBP != nil {
			elem.elem.through = newOptionSet(t.joinBP, false)
		}
		for _, ref := range t.refs {
			r.active = ref.active
			if err := r.resolve(ref.opts, elem); err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
		}
		r.active = saved
		if err := r.resolveCollections(elem); err != nil {
			return fmt.Errorf("element %d: %w", i, err)
		}
		if elem.elem.through != nil {
			if err := r.resolveCollections(elem.elem.through); err != nil {
				return fmt.Errorf("element %d through: %w", i, err)
			}
		}
		t.elements[i] = elem
	}

	for _, index := range t.atIndices {
		if index >= count {
			return fmt.Errorf("%w: At(%d) is out of range for relation %q with %d records", ErrInvalidOption, index, t.rel.name, count)
		}
	}
	return nil
}

// useElements splits the value given to Use for a many-to-many relation into
// its existing records.
func useElements(relation string, value any) ([]any, error) {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil, fmt.Errorf("%w: Use(%q) on a many_to_many relation expects a slice of existing records, got %T", ErrInvalidOption, relation, value)
	}
	used := make([]any, rv.Len())
	for i := range used {
		elem := rv.Index(i)
		if !elem.IsValid() || (elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Interface) && elem.IsNil() {
			return nil, fmt.Errorf("%w: Use(%q) element %d must not be nil", ErrInvalidOption, relation, i)
		}
		used[i] = elem.Interface()
	}
	return used, nil
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
	var collections map[string][]planner.ElementOptions
	if len(os.collections) > 0 {
		collections = make(map[string][]planner.ElementOptions, len(os.collections))
		for name, t := range os.collections {
			elements := make([]planner.ElementOptions, len(t.elements))
			for i, elem := range t.elements {
				elements[i].Opts = toOptionSet(elem)
				if elem.elem.throughUsed {
					elements[i].Through = toOptionSet(elem.elem.through)
				}
				if t.hasUse {
					elements[i].Use = t.used[i]
				}
			}
			collections[name] = elements
		}
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
		Sets:        os.sets,
		Uses:        os.uses,
		Refs:        refs,
		Collections: collections,
		Omits:       os.omits,
		Whens:       os.whens,
		WithFns:     withFns,
		GenFns:      genFns,
		Rand:        os.rand,
		Only:        os.only,
	}
}
