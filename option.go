package seedling

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
)

// Option configures how a record is built and inserted.
type Option interface {
	applyOption(o *optionSet)
}

// optionSet collects all options for a single node.
type optionSet struct {
	sets         map[string]any                     // field name → value
	uses         map[string]any                     // relation name → existing value
	refs         map[string][]Option                // relation name → nested options
	omits        map[string]bool                    // relation name → true
	whens        map[string]func(any) (bool, error) // relation name → dynamic expansion predicate
	withFns      []withFn                           // typed root mutators
	ctx          context.Context                    // optional context for insert operations
	seqs         map[string]any                     // field name → func(int) T (sequence generators)
	seqRefs      map[string]any                     // relation name → func(int) []Option
	seqUses      map[string]any                     // relation name → func(int) T
	afterInserts []any                              // func(T, DBTX) closures run after insert
	genFns       []generateFn                       // typed rand-driven mutators
	rand         *rand.Rand                         // optional RNG for Generate options
	traits       []string                           // deferred blueprint trait names
	logFn        func(InsertLog)                    // optional insert log callback
	only         map[string]bool                    // Only option: nil = insert all, non-nil = insert only specified relations
}

type withFn func(value any) (any, error)

type generateFn func(r *rand.Rand, value any) (any, error)

func newOptionSet() *optionSet {
	return &optionSet{
		sets:    make(map[string]any),
		uses:    make(map[string]any),
		refs:    make(map[string][]Option),
		omits:   make(map[string]bool),
		seqs:    make(map[string]any),
		seqRefs: make(map[string]any),
		seqUses: make(map[string]any),
	}
}

func collectOptions(opts []Option) *optionSet {
	os := newOptionSet()
	for _, o := range opts {
		o.applyOption(os)
	}
	return os
}

// Set overrides a struct field value by its Go field name.
//
//	seedling.Set("Title", "urgent task")
func Set(field string, value any) Option {
	return setOption{field: field, value: value}
}

type setOption struct {
	field string
	value any
}

func (s setOption) applyOption(o *optionSet) {
	o.sets[s.field] = s.value
}

// Use provides an existing record for a direct relation, skipping auto-creation.
//
//	seedling.Use("company", existingCompany)
func Use(name string, value any) Option {
	return useOption{name: name, value: value}
}

type useOption struct {
	name  string
	value any
}

func (u useOption) applyOption(o *optionSet) {
	o.uses[u.name] = u.value
}

// Ref applies nested options to a specific relation's blueprint.
// It also enables expansion for optional relations.
//
// Options that only apply to the root record — [WithContext], [AfterInsert],
// [AfterInsertE], and [WithInsertLog] — cannot appear under Ref (including
// inside traits resolved for that relation) and return [ErrInvalidOption].
//
//	seedling.Ref("project", seedling.Set("Name", "renewal"))
func Ref(name string, opts ...Option) Option {
	return refOption{name: name, opts: opts}
}

type refOption struct {
	name string
	opts []Option
}

func (r refOption) applyOption(o *optionSet) {
	o.refs[r.name] = append(o.refs[r.name], r.opts...)
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

func (om omitOption) applyOption(o *optionSet) {
	o.omits[om.name] = true
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

func (w whenOption) applyOption(o *optionSet) {
	if o.whens == nil {
		o.whens = make(map[string]func(any) (bool, error))
	}
	o.whens[w.name] = w.fn
}

// With applies a type-safe modification function to the root struct.
//
//	seedling.With(func(t *Task) { t.Title = "urgent" })
func With[T any](fn func(*T)) Option {
	return withOption[T]{fn: fn}
}

type withOption[T any] struct {
	fn func(*T)
}

func (w withOption[T]) applyOption(o *optionSet) {
	o.withFns = append(o.withFns, func(value any) (any, error) {
		return applyTypedMutation("With", value, func(target *T) error {
			w.fn(target)
			return nil
		})
	})
}

// WithContext sets the context used for insert operations.
// If not specified, testing-based APIs use t.Context() and error-returning APIs
// use the ctx passed to InsertOneE, InsertManyE, or Plan.InsertE.
//
//	seedling.InsertOne[Task](t, db, seedling.WithContext(ctx))
func WithContext(ctx context.Context) Option {
	return contextOption{ctx: ctx}
}

type contextOption struct {
	ctx context.Context
}

func (c contextOption) applyOption(o *optionSet) {
	o.ctx = c.ctx
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

func (s seqOption) applyOption(o *optionSet) {
	o.seqs[s.field] = s.fn
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

func (s seqRefOption) applyOption(o *optionSet) {
	o.seqRefs[s.name] = s.fn
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

func (s seqUseOption) applyOption(o *optionSet) {
	o.seqUses[s.name] = s.fn
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

func (a afterInsertOption[T]) applyOption(o *optionSet) {
	o.afterInserts = append(o.afterInserts, a.fn)
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

func (a afterInsertEOption[T]) applyOption(o *optionSet) {
	o.afterInserts = append(o.afterInserts, a.fn)
}

// BlueprintTrait applies a named trait defined on the target blueprint.
//
//	seedling.InsertOne[User](t, db, seedling.BlueprintTrait("admin"))
func BlueprintTrait(name string) Option {
	return blueprintTraitOption{name: name}
}

type blueprintTraitOption struct {
	name string
}

func (tr blueprintTraitOption) applyOption(o *optionSet) {
	o.traits = append(o.traits, tr.name)
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

func (tr inlineTraitOption) applyOption(o *optionSet) {
	for _, opt := range tr.opts {
		opt.applyOption(o)
	}
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

func (g generateOption[T]) applyOption(o *optionSet) {
	o.genFns = append(o.genFns, func(r *rand.Rand, value any) (any, error) {
		return applyTypedMutation("Generate", value, func(target *T) error {
			g.fn(r, target)
			return nil
		})
	})
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

func (g generateEOption[T]) applyOption(o *optionSet) {
	o.genFns = append(o.genFns, func(r *rand.Rand, value any) (any, error) {
		return applyTypedMutation("Generate", value, func(target *T) error {
			return g.fn(r, target)
		})
	})
}

// WithRand sets the RNG used by Generate options on the current node.
func WithRand(r *rand.Rand) Option {
	return randOption{rand: r}
}

type randOption struct {
	rand *rand.Rand
}

func (r randOption) applyOption(o *optionSet) {
	o.rand = r.rand
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
// FK bindings that were resolved from parent PK values.
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

func (l insertLogOption) applyOption(o *optionSet) {
	o.logFn = l.fn
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
func Only(relations ...string) Option {
	return onlyOption{relations: relations}
}

type onlyOption struct {
	relations []string
}

func (o onlyOption) applyOption(os *optionSet) {
	if os.only == nil {
		os.only = make(map[string]bool)
	}
	for _, r := range o.relations {
		os.only[r] = true
	}
}

// withFnOption wraps a pre-built withFn for internal use (e.g. reconstructOptions).
type withFnOption struct{ fn withFn }

func (w withFnOption) applyOption(o *optionSet) { o.withFns = append(o.withFns, w.fn) }

// rawAfterInsertOption wraps a pre-built afterInsert closure for internal use.
type rawAfterInsertOption struct{ fn any }

func (a rawAfterInsertOption) applyOption(o *optionSet) {
	o.afterInserts = append(o.afterInserts, a.fn)
}

// rawGenerateOption wraps a pre-built generateFn for internal use.
type rawGenerateOption struct{ fn generateFn }

func (g rawGenerateOption) applyOption(o *optionSet) { o.genFns = append(o.genFns, g.fn) }

func applyTypedMutation[T any](kind string, value any, fn func(*T) error) (any, error) {
	ptr := toOptionPointer(value)
	target, ok := ptr.(*T)
	if !ok {
		return nil, fmt.Errorf("%w: %s option expects *%s but got %T", ErrTypeMismatch, kind, typeName[T](), value)
	}
	if err := fn(target); err != nil {
		return nil, err
	}
	return reflect.ValueOf(ptr).Elem().Interface(), nil
}

func toOptionPointer(value any) any {
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		return value
	}
	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	return ptr.Interface()
}

func typeName[T any]() string {
	return reflect.TypeFor[T]().String()
}
