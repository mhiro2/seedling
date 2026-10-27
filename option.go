package seedling

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"

	"github.com/mhiro2/seedling/internal/planner"
)

// Option configures how a record is built and inserted.
type Option interface {
	// applyOption resolves the option into the node's option set. Structural
	// options (traits, sequences, Ref) resolve their nested options through r,
	// and options limited to the root record reject any other placement.
	applyOption(r *optionResolver, o *optionSet) error
}

// Set overrides a struct field value by its Go field name.
// Sets apply in the order they are written, so a later Set wins over an
// earlier one, including when one replaces an embedded struct and the other
// sets a field promoted from it.
//
//	seedling.Set("Title", "urgent task")
func Set(field string, value any) Option {
	return setOption{field: field, value: value}
}

type setOption struct {
	field string
	value any
}

func (s setOption) applyOption(_ *optionResolver, o *optionSet) error {
	o.sets = append(o.sets, planner.FieldSet{Field: s.field, Value: s.value})
	return nil
}

// Use provides an existing record for a direct relation, skipping auto-creation.
//
//	seedling.Use("company", existingCompany)
//
// For a many-to-many relation, Use takes a slice of existing related records
// and creates only the join records that link them, one per record:
//
//	seedling.Use("roles", []Role{admin, viewer})
//
// [Through] options inside a [Ref] to the same relation still configure those
// join records. Has-many relations do not accept Use, since attaching an
// existing child would require updating its foreign key.
func Use(name string, value any) Option {
	return useOption{name: name, value: value}
}

type useOption struct {
	name  string
	value any
}

func (u useOption) applyOption(r *optionResolver, o *optionSet) error {
	if rel, ok := collectionRelation(o, u.name); ok && rel.kind == ManyToMany {
		t, err := r.collectionTarget(o, rel)
		if err != nil {
			return err
		}
		t.use, t.hasUse = u.value, true
		return nil
	}
	o.uses[u.name] = u.value
	return nil
}

// Ref applies nested options to a specific relation's blueprint.
// It also enables expansion for optional relations.
//
// Options that only apply to the root record — [WithContext], [AfterInsert],
// [AfterInsertE], [WithInsertLog], and [Only] — cannot appear under Ref
// (including inside traits resolved for that relation) and return
// [ErrInvalidOption].
//
//	seedling.Ref("project", seedling.Set("Name", "renewal"))
//
// For a has-many or many-to-many relation, the options apply to every related
// record. [Count] replaces the relation's [Relation.Count] for this call, and
// [At], [Each], and [Through] narrow options to particular records or to their
// join records:
//
//	seedling.Ref("items",
//	    seedling.Count(3),
//	    seedling.Set("Status", "open"),
//	    seedling.At(1, seedling.Set("Status", "cancelled")),
//	)
func Ref(name string, opts ...Option) Option {
	return refOption{name: name, opts: opts}
}

type refOption struct {
	name string
	opts []Option
}

func (r refOption) applyOption(res *optionResolver, o *optionSet) error {
	if rel, ok := collectionRelation(o, r.name); ok {
		t, err := res.collectionTarget(o, rel)
		if err == nil {
			err = t.addRef(res, r.opts)
		}
		if err != nil {
			return fmt.Errorf("resolve options for ref %q: %w", r.name, err)
		}
		return nil
	}
	if err := res.resolve(r.opts, res.refTarget(o, r.name)); err != nil {
		return fmt.Errorf("resolve options for ref %q: %w", r.name, err)
	}
	return nil
}

// Count sets how many records a has-many or many-to-many relation creates,
// replacing the relation's [Relation.Count]. It must be passed directly to the
// [Ref] of that relation; Count(0) creates none.
//
//	seedling.Ref("items", seedling.Count(21))
func Count(n int) Option {
	return countOption{n: n}
}

type countOption struct {
	n int
}

func (countOption) applyOption(*optionResolver, *optionSet) error {
	return fmt.Errorf("%w: Count must be passed directly to Ref of a has_many or many_to_many relation", ErrInvalidOption)
}

// At applies options only to the related record at the 0-based index of a
// has-many or many-to-many relation. It is valid inside the [Ref] of such a
// relation, and like every option it applies in written order, so options
// written after it override it for that record.
//
//	seedling.Ref("items", seedling.Count(3), seedling.At(1, seedling.Set("Status", "cancelled")))
func At(index int, opts ...Option) Option {
	return atOption{index: index, opts: opts}
}

type atOption struct {
	index int
	opts  []Option
}

func (a atOption) applyOption(r *optionResolver, o *optionSet) error {
	if o.elem == nil {
		return elementOnlyError("At")
	}
	if a.index < 0 {
		return fmt.Errorf("%w: At index must be >= 0, got %d", ErrInvalidOption, a.index)
	}
	o.elem.target.atIndices = append(o.elem.target.atIndices, a.index)
	if a.index != o.elem.index {
		return nil
	}
	return r.resolve(a.opts, o)
}

// Each applies the options fn returns for each related record of a has-many or
// many-to-many relation. fn receives the record's 0-based index. It is valid
// inside the [Ref] of such a relation.
//
//	seedling.Ref("items", seedling.Count(3), seedling.Each(func(i int) []seedling.Option {
//	    return []seedling.Option{seedling.Set("Position", i+1)}
//	}))
func Each(fn func(i int) []Option) Option {
	return eachOption{fn: fn}
}

type eachOption struct {
	fn func(int) []Option
}

func (e eachOption) applyOption(r *optionResolver, o *optionSet) error {
	if o.elem == nil {
		return elementOnlyError("Each")
	}
	if e.fn == nil {
		return fmt.Errorf("%w: Each function must not be nil", ErrInvalidOption)
	}
	return r.resolve(e.fn(o.elem.index), o)
}

// Through applies options to the join record that links a many-to-many related
// record to its owner. It is valid inside the [Ref] of a many-to-many relation,
// including inside [At] to configure a single join record.
//
//	seedling.Ref("roles", seedling.At(0, seedling.Through(seedling.Set("ExpiresAt", deadline))))
func Through(opts ...Option) Option {
	return throughOption{opts: opts}
}

type throughOption struct {
	opts []Option
}

func (th throughOption) applyOption(r *optionResolver, o *optionSet) error {
	if o.elem == nil || o.elem.through == nil {
		return fmt.Errorf("%w: Through applies only inside Ref of a many_to_many relation", ErrInvalidOption)
	}
	o.elem.throughUsed = true
	if err := r.resolve(th.opts, o.elem.through); err != nil {
		return fmt.Errorf("resolve through options: %w", err)
	}
	return nil
}

func elementOnlyError(option string) error {
	return fmt.Errorf("%w: %s applies only inside Ref of a has_many or many_to_many relation", ErrInvalidOption, option)
}

// Omit prevents auto-creation of an optional relation.
//
//	seedling.Omit("assignee")
func Omit(name string) Option {
	return omitOption{name: name}
}

type omitOption struct {
	name string
}

func (om omitOption) applyOption(_ *optionResolver, o *optionSet) error {
	o.omits[om.name] = true
	return nil
}

// When conditionally expands a relation based on the current record's state.
// The predicate receives the owning record after defaults and Set/With options
// are applied. If it returns true, the relation is expanded, including optional
// relations. If it returns false, the relation is skipped regardless of the
// blueprint's Required flag.
//
// This provides dynamic, insert-time control over relation expansion:
//
//	seedling.InsertOne[Task](t, db,
//	    seedling.Set("Status", "assigned"),
//	    seedling.When("assignee", func(t Task) bool {
//	        return t.Status == "assigned"
//	    }),
//	)
func When[T any](name string, fn func(T) bool) Option {
	return whenOption{name: name, fn: func(value any) (bool, error) {
		v, ok := value.(T)
		if !ok {
			return false, fmt.Errorf("%w: When(%q) expects %s but got %T", ErrTypeMismatch, name, typeName[T](), value)
		}
		return fn(v), nil
	}}
}

type whenOption struct {
	name string
	fn   func(any) (bool, error)
}

func (w whenOption) applyOption(_ *optionResolver, o *optionSet) error {
	if o.whens == nil {
		o.whens = make(map[string]func(any) (bool, error))
	}
	o.whens[w.name] = w.fn
	return nil
}

// With applies a type-safe modification function to the root struct.
// With functions run after Generate and Set options, whatever order they are
// written in, and in written order among themselves.
//
//	seedling.With(func(t *Task) { t.Title = "urgent" })
func With[T any](fn func(*T)) Option {
	return withOption[T]{fn: fn}
}

type withOption[T any] struct {
	fn func(*T)
}

func (w withOption[T]) applyOption(_ *optionResolver, o *optionSet) error {
	o.withFns = append(o.withFns, func(value any) (any, error) {
		return applyTypedMutation("With", value, func(target *T) error {
			w.fn(target)
			return nil
		})
	})
	return nil
}

// WithContext sets the context used for insert operations. It applies wherever
// it appears among the root options, including inside a [BlueprintTrait] or
// [InlineTrait], and overrides the ctx passed to InsertOneE, InsertManyE, or
// Plan.InsertE; WithContext(nil) runs with context.Background(). If not
// specified, testing-based APIs use t.Context() and error-returning APIs use
// the ctx they were given.
//
//	seedling.InsertOne[Task](t, db, seedling.WithContext(ctx))
func WithContext(ctx context.Context) Option {
	return contextOption{ctx: ctx}
}

type contextOption struct {
	ctx context.Context
}

func (c contextOption) applyOption(_ *optionResolver, o *optionSet) error {
	if !o.root {
		return rootOnlyError("with-context")
	}
	// A nil context still counts as given, so it overrides the caller's ctx
	// the same way any other WithContext does.
	o.ctx = c.ctx
	if o.ctx == nil {
		o.ctx = context.Background()
	}
	return nil
}

// Seq sets a field value based on the index when used with InsertMany.
// The function receives a 0-based index and returns the value for that field.
//
//	seedling.InsertMany[User](t, db, 3,
//	    seedling.Seq("Name", func(i int) string { return fmt.Sprintf("user-%d", i) }),
//	)
func Seq[V any](field string, fn func(i int) V) Option {
	return seqOption{field: field, fn: func(i int) any { return fn(i) }}
}

type seqOption struct {
	field string
	fn    func(int) any
}

func (s seqOption) applyOption(r *optionResolver, o *optionSet) error {
	i, err := r.sequenceIndex()
	if err != nil {
		return err
	}
	return setOption{field: s.field, value: s.fn(i)}.applyOption(r, o)
}

// SeqRef generates per-record Ref options when used with InsertMany.
// The function receives a 0-based index and returns the nested options for that relation.
//
//	seedling.InsertMany[Task](t, db, 3,
//	    seedling.SeqRef("project", func(i int) []seedling.Option {
//	        return []seedling.Option{seedling.Set("Name", fmt.Sprintf("proj-%d", i))}
//	    }),
//	)
func SeqRef(name string, fn func(i int) []Option) Option {
	return seqRefOption{name: name, fn: fn}
}

type seqRefOption struct {
	name string
	fn   func(int) []Option
}

func (s seqRefOption) applyOption(r *optionResolver, o *optionSet) error {
	i, err := r.sequenceIndex()
	if err != nil {
		return err
	}
	return refOption{name: s.name, opts: s.fn(i)}.applyOption(r, o)
}

// SeqUse provides per-record existing records for a relation when used with InsertMany.
//
//	companies := seedling.InsertMany[Company](t, db, 3)
//	seedling.InsertMany[Task](t, db, 3,
//	    seedling.SeqUse("company", func(i int) Company { return companies[i] }),
//	)
func SeqUse[V any](name string, fn func(i int) V) Option {
	return seqUseOption{name: name, fn: func(i int) any { return fn(i) }}
}

type seqUseOption struct {
	name string
	fn   func(int) any
}

func (s seqUseOption) applyOption(r *optionResolver, o *optionSet) error {
	i, err := r.sequenceIndex()
	if err != nil {
		return err
	}
	return useOption{name: s.name, value: s.fn(i)}.applyOption(r, o)
}

// AfterInsert registers a callback that runs after the root record is inserted.
// The callback receives the inserted root record and the database handle.
// This is useful for post-insert side effects like password hashing or
// populating join tables.
//
//	seedling.AfterInsert(func(u User, db seedling.DBTX) {
//	    // hash password, insert into join table, etc.
//	})
func AfterInsert[T any](fn func(t T, db DBTX)) Option {
	return afterInsertOption[T]{fn: fn}
}

type afterInsertOption[T any] struct {
	fn func(T, DBTX)
}

func (a afterInsertOption[T]) applyOption(r *optionResolver, o *optionSet) error {
	return o.addAfterInsert(r, a.fn)
}

// AfterInsertE registers a callback that runs after the root record is inserted.
// Unlike AfterInsert, the callback can return an error to signal failure.
//
//	seedling.AfterInsertE(func(u User, db seedling.DBTX) error {
//	    _, err := db.(*sql.DB).Exec("INSERT INTO roles ...")
//	    return err
//	})
func AfterInsertE[T any](fn func(t T, db DBTX) error) Option {
	return afterInsertEOption[T]{fn: fn}
}

type afterInsertEOption[T any] struct {
	fn func(T, DBTX) error
}

func (a afterInsertEOption[T]) applyOption(r *optionResolver, o *optionSet) error {
	return o.addAfterInsert(r, a.fn)
}

// BlueprintTrait applies a named trait defined on the target blueprint.
//
//	seedling.InsertOne[User](t, db, seedling.BlueprintTrait("admin"))
//
// The trait expands in place, so its options take effect exactly where the
// trait is written: options listed after it override the trait, and the trait
// overrides options listed before it. A trait may reference other traits of
// the same blueprint, but a trait that reaches itself is rejected.
func BlueprintTrait(name string) Option {
	return blueprintTraitOption{name: name}
}

type blueprintTraitOption struct {
	name string
}

func (tr blueprintTraitOption) applyOption(r *optionResolver, o *optionSet) error {
	return r.resolveTrait(tr.name, o)
}

// InlineTrait creates a group of options that is expanded immediately.
// Use this when you want to compose reusable option bundles in Go code rather
// than reference a trait stored on the blueprint.
//
//	adminTrait := seedling.InlineTrait(seedling.Set("Role", "admin"))
//	seedling.InsertOne[User](t, db, adminTrait)
func InlineTrait(opts ...Option) Option {
	return inlineTraitOption{opts: opts}
}

type inlineTraitOption struct {
	opts []Option
}

func (tr inlineTraitOption) applyOption(r *optionResolver, o *optionSet) error {
	return r.resolve(tr.opts, o)
}

// Generate applies a rand-driven mutation function to the current node before
// Set/With options run. This is useful when integrating seedling with
// property-based tests.
//
//	seedling.Generate(func(r *rand.Rand, t *Task) {
//	    t.Title = fmt.Sprintf("task-%d", r.IntN(1000))
//	})
func Generate[T any](fn func(r *rand.Rand, t *T)) Option {
	return generateOption[T]{fn: fn}
}

type generateOption[T any] struct {
	fn func(*rand.Rand, *T)
}

func (g generateOption[T]) applyOption(_ *optionResolver, o *optionSet) error {
	o.genFns = append(o.genFns, func(r *rand.Rand, value any) (any, error) {
		return applyTypedMutation("Generate", value, func(target *T) error {
			g.fn(r, target)
			return nil
		})
	})
	return nil
}

// GenerateE applies a rand-driven mutation function that can return an error.
// Unlike Generate, the function can signal failure by returning a non-nil error.
//
//	seedling.GenerateE(func(r *rand.Rand, t *Task) error {
//	    name, err := randomName(r)
//	    if err != nil { return err }
//	    t.Title = name
//	    return nil
//	})
func GenerateE[T any](fn func(r *rand.Rand, t *T) error) Option {
	return generateEOption[T]{fn: fn}
}

type generateEOption[T any] struct {
	fn func(*rand.Rand, *T) error
}

func (g generateEOption[T]) applyOption(_ *optionResolver, o *optionSet) error {
	o.genFns = append(o.genFns, func(r *rand.Rand, value any) (any, error) {
		return applyTypedMutation("Generate", value, func(target *T) error {
			return g.fn(r, target)
		})
	})
	return nil
}

// WithRand sets the RNG used by Generate options on the current node.
func WithRand(r *rand.Rand) Option {
	return randOption{rand: r}
}

type randOption struct {
	rand *rand.Rand
}

func (r randOption) applyOption(_ *optionResolver, o *optionSet) error {
	o.rand = r.rand
	return nil
}

// WithSeed is a convenience wrapper around WithRand(rand.New(rand.NewPCG(seed, seed^goldenGamma))).
func WithSeed(seed uint64) Option {
	//nolint:gosec // WithSeed intentionally creates a deterministic pseudo-random generator.
	return WithRand(rand.New(rand.NewPCG(seed, seed^goldenGamma)))
}

// goldenGamma is the golden-ratio-derived constant (⌊2^64 / φ⌋). XOR-ing the
// user seed with this value produces a well-distributed second seed for PCG,
// avoiding correlation when both PCG seeds are identical.
const goldenGamma uint64 = 0x9e3779b97f4a7c15

// WithInsertLog registers a callback that is invoked for each step in the
// execution plan, including both inserted and provided (skipped) nodes.
// The callback receives an [InsertLog] describing the operation, including
// FK bindings that were resolved from referenced parent field values.
//
// This is useful for debugging the dependency resolution order and
// understanding which FK values were assigned:
//
//	seedling.InsertOne[Task](t, db,
//	    seedling.WithInsertLog(func(log seedling.InsertLog) {
//	        t.Logf("Step %d: %s (table: %s)", log.Step, log.Blueprint, log.Table)
//	        for _, fk := range log.FKBindings {
//	            t.Logf("  SET %s = %v (from %s.%s)", fk.ChildField, fk.Value, fk.ParentTable, fk.ParentField)
//	        }
//	    }),
//	)
func WithInsertLog(fn func(InsertLog)) Option {
	return insertLogOption{fn: fn}
}

type insertLogOption struct {
	fn func(InsertLog)
}

func (l insertLogOption) applyOption(_ *optionResolver, o *optionSet) error {
	if !o.root {
		return rootOnlyError("insert log")
	}
	o.logFn = l.fn
	return nil
}

// Only restricts the planner to build only the root node and the specified
// relation subtrees. Relations not listed are never expanded, so the resulting
// graph contains only the necessary nodes. [Plan.DebugString] and
// [Plan.DryRunString] reflect this lazily built subgraph.
//
// With no arguments, Only builds only the root node:
//
//	seedling.InsertOne[Task](t, db, seedling.Only())
//
// With arguments, the named relations and their transitive dependencies are
// also included:
//
//	seedling.InsertOne[Task](t, db, seedling.Only("project"))
//
// Only is an explicit minimal-graph escape hatch: relations not listed are never
// expanded, even when they are Required. Their FK fields are left at the zero
// value, so the caller is responsible for any NOT NULL FK constraints on the
// excluded relations. Combining Only with an explicit [Use], [Ref], or [When] on
// a relation outside the set is reported as an error, since that configuration
// would otherwise be silently discarded.
func Only(relations ...string) Option {
	return onlyOption{relations: relations}
}

type onlyOption struct {
	relations []string
}

func (o onlyOption) applyOption(_ *optionResolver, os *optionSet) error {
	if !os.root {
		return rootOnlyError("only")
	}
	if os.only == nil {
		os.only = make(map[string]bool)
	}
	for _, r := range o.relations {
		os.only[r] = true
	}
	return nil
}

func applyTypedMutation[T any](kind string, value any, fn func(*T) error) (any, error) {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		return nil, fmt.Errorf("%w: %s option expects non-nil *%s but got %T", ErrTypeMismatch, kind, typeName[T](), value)
	}

	targetType := reflect.TypeFor[T]()
	if rv.Type() == targetType {
		target := new(T)
		*target = value.(T)
		if err := fn(target); err != nil {
			return nil, err
		}
		updated := reflect.ValueOf(*target)
		if updated.Kind() == reflect.Pointer && updated.IsNil() {
			return nil, fmt.Errorf("%w: %s option returned nil %s", ErrInvalidOption, kind, targetType)
		}
		return *target, nil
	}

	if rv.Kind() == reflect.Pointer && rv.Type().Elem() == targetType {
		target, ok := value.(*T)
		if !ok {
			return nil, fmt.Errorf("%w: %s option expects *%s but got %T", ErrTypeMismatch, kind, typeName[T](), value)
		}
		if err := fn(target); err != nil {
			return nil, err
		}
		return value, nil
	}

	return nil, fmt.Errorf("%w: %s option expects *%s but got %T", ErrTypeMismatch, kind, typeName[T](), value)
}

func typeName[T any]() string {
	return reflect.TypeFor[T]().String()
}
