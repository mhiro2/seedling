package seedling

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/mhiro2/seedling/internal/debug"
	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/graph"
	"github.com/mhiro2/seedling/internal/planner"
)

// Scenario plans records of any blueprint into one dependency graph and
// inserts them together. Records added with [Add] never share nodes
// implicitly: every record expands its own relations, unless [Link] binds a
// belongs-to relation to a record added earlier. That makes sharing explicit,
// such as one company reached through both a task's project and its assignee:
//
//	s := seedling.NewScenario(reg)
//	company := seedling.Add[Company](s)
//	project := seedling.Add[Project](s, seedling.Link("company", company))
//	assignee := seedling.Add[User](s, seedling.Link("company", company))
//	task := seedling.Add[Task](s,
//	    seedling.Link("project", project),
//	    seedling.Link("assignee", assignee),
//	)
//
//	result := s.Insert(t, db)
//	got := task.Get(result)
//
// A Scenario is not safe for concurrent use.
type Scenario struct {
	registry *Registry
	roots    []scenarioRoot
}

type scenarioRoot struct {
	rootType reflect.Type
	opts     []Option
}

// NewScenario returns an empty scenario backed by reg. If reg is nil, the
// package default registry is used.
func NewScenario(reg *Registry) *Scenario {
	return &Scenario{registry: resolveRegistry(reg)}
}

// Node identifies a record added to a [Scenario]. It is the handle that
// [Link] shares the record through and that [Node.Get] reads the inserted
// record back with, so records of the same blueprint are told apart by role
// rather than by lookup order.
type Node[T any] struct {
	scenario *Scenario
	index    int
}

// Add adds a record of type T to the scenario and returns its handle. Options
// are resolved when the scenario is built, as they are for [InsertOne], except
// that [WithContext] and [WithInsertLog] are rejected; pass the context to
// [Scenario.InsertE] instead.
func Add[T any](s *Scenario, opts ...Option) Node[T] {
	// Options resolve when the scenario is built, so keep a copy the caller
	// cannot change afterwards.
	s.roots = append(s.roots, scenarioRoot{rootType: reflect.TypeFor[T](), opts: cloneOptions(opts)})
	return Node[T]{scenario: s, index: len(s.roots) - 1}
}

// Link binds a belongs-to relation to a record added to the same [Scenario]
// before the record being configured, instead of creating a new parent. Link
// may also appear under [Ref], so a parent created for a record can still
// point at a shared one:
//
//	seedling.Add[Task](s, seedling.Ref("project", seedling.Link("company", company)))
//
// Link is valid only in options passed to [Add].
func Link[T any](name string, node Node[T]) Option {
	return linkOption{name: name, scenario: node.scenario, index: node.index}
}

type linkOption struct {
	name     string
	scenario *Scenario
	index    int
}

func (l linkOption) applyOption(r *optionResolver, o *optionSet) error {
	switch {
	case r.scenario == nil:
		return fmt.Errorf("%w: Link(%q) applies only to records added to a Scenario", ErrInvalidOption, l.name)
	case l.scenario == nil:
		return fmt.Errorf("%w: Link(%q) needs a Node returned by Add", ErrInvalidOption, l.name)
	case l.scenario != r.scenario:
		return fmt.Errorf("%w: Link(%q) refers to a record of another Scenario", ErrInvalidOption, l.name)
	case l.index >= r.scenarioIndex:
		return fmt.Errorf("%w: Link(%q) must refer to a record added before this one", ErrInvalidOption, l.name)
	}
	// Check the target here rather than during expansion, so a Link under a
	// relation that a When predicate skips is still validated.
	if err := l.checkTarget(r, o); err != nil {
		return err
	}
	o.links[l.name] = l.index
	return nil
}

// checkTarget reports a Link whose target record is not of the blueprint the
// relation refers to. Unknown relations and blueprints are left for the
// planner to report with its usual hints.
func (l linkOption) checkTarget(r *optionResolver, o *optionSet) error {
	if o.bp == nil {
		return nil
	}
	i := slices.IndexFunc(o.bp.relations, func(rel relationDef) bool { return rel.name == l.name })
	if i < 0 || o.bp.relations[i].kind != BelongsTo {
		return nil
	}
	// An unregistered target type is reported when its own record resolves.
	target, err := r.registry.lookupByType(l.scenario.roots[l.index].rootType)
	if err != nil {
		return nil
	}
	if want := o.bp.relations[i].refBlueprint; target.name != want {
		return fmt.Errorf("%w: Link(%q) expects a %q record but got %q", ErrTypeMismatch, l.name, want, target.name)
	}
	return nil
}

// Get returns the inserted record from a result of the node's scenario. It
// panics when the record is not part of the result, which happens only for a
// partial result or a result of another scenario.
func (n Node[T]) Get(result ScenarioResult) T {
	value, ok := n.Lookup(result)
	if !ok {
		panic(fmt.Sprintf("seedling: scenario record %d (%s) not found in result", n.index, reflect.TypeFor[T]()))
	}
	return value
}

// Lookup returns the inserted record from a result of the node's scenario. It
// reports false when the record is not part of the result, such as a record
// that did not complete in a partial result.
func (n Node[T]) Lookup(result ScenarioResult) (T, bool) {
	var zero T
	if n.scenario == nil || result.scenario != n.scenario || n.index >= len(result.rootIDs) {
		return zero, false
	}
	node, ok := result.nodes[result.rootIDs[n.index]]
	if !ok {
		return zero, false
	}
	value, ok := node.Value.(T)
	return value, ok
}

// ScenarioPlan is a built [Scenario] ready for insertion. Like [Plan], it can
// be executed multiple times, each run on a cloned graph.
type ScenarioPlan struct {
	scenario     *Scenario
	graph        *graph.Graph
	rootIDs      []string
	afterInserts [][]afterInsertFn // per root, in the order the records were added
}

// BuildE builds the scenario without inserting anything. Records added later
// are not part of the returned plan.
func (s *Scenario) BuildE() (*ScenarioPlan, error) {
	roots := make([]planner.Root, len(s.roots))
	afterInserts := make([][]afterInsertFn, len(s.roots))
	for i, root := range s.roots {
		resolved, err := resolveOptions(&optionResolver{
			registry:      s.registry.reg,
			rootType:      root.rootType,
			seqIndex:      noSequenceIndex,
			scenario:      s,
			scenarioIndex: i,
		}, root.opts)
		if err != nil {
			return nil, fmt.Errorf("record %d (%s): %w", i, root.rootType, err)
		}
		roots[i] = planner.Root{Type: root.rootType, Opts: toOptionSet(resolved)}
		afterInserts[i] = resolved.afterInserts
	}

	result, err := planner.PlanRoots(newRegistryAdapter(s.registry), roots)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	return &ScenarioPlan{
		scenario:     s,
		graph:        result.Graph,
		rootIDs:      result.RootIDs,
		afterInserts: afterInserts,
	}, nil
}

// Build builds the scenario without inserting anything. Fails the test on error.
func (s *Scenario) Build(tb testing.TB) *ScenarioPlan {
	tb.Helper()
	plan, err := s.BuildE()
	if err != nil {
		tb.Fatal(err)
	}
	return plan
}

// InsertE builds the scenario and inserts every record, returning an error on
// failure. If execution fails after inserting some records, the returned
// result contains them and can be passed to [ScenarioResult.CleanupE].
func (s *Scenario) InsertE(ctx context.Context, db DBTX) (ScenarioResult, error) {
	plan, err := s.BuildE()
	if err != nil {
		return ScenarioResult{}, err
	}
	return plan.InsertE(ctx, db)
}

// Insert builds the scenario and inserts every record. Fails the test on error.
func (s *Scenario) Insert(tb testing.TB, db DBTX) ScenarioResult {
	tb.Helper()
	result, err := s.InsertE(tb.Context(), db)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// InsertE inserts every record of the plan, returning an error on failure.
// AfterInsert callbacks run after all records are inserted, in the order the
// records were added.
func (p *ScenarioPlan) InsertE(ctx context.Context, db DBTX) (ScenarioResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reg := p.scenario.registry
	execResult, err := executor.Execute(ctx, db, p.graph.Clone(), newRegistryAdapter(reg), nil)
	if execResult == nil {
		return ScenarioResult{}, fmt.Errorf("%w: executor returned a nil result", ErrInvalidOption)
	}
	result := ScenarioResult{
		scenario:      p.scenario,
		rootIDs:       p.rootIDs,
		nodes:         execResult.Nodes,
		graph:         execResult.Graph,
		deleteFns:     snapshotDeleteFns(reg, execResult.Nodes),
		cleanupValues: execResult.Inserted,
	}
	if err != nil {
		return result, fmt.Errorf("execute plan: %w", err)
	}

	for i, callbacks := range p.afterInserts {
		root := execResult.Nodes[p.rootIDs[i]].Value
		for _, fn := range callbacks {
			if err := fn(root, db); err != nil {
				return result, fmt.Errorf("run after-insert callback: %w", err)
			}
		}
	}
	return result, nil
}

// Insert inserts every record of the plan. Fails the test on error.
func (p *ScenarioPlan) Insert(tb testing.TB, db DBTX) ScenarioResult {
	tb.Helper()
	result, err := p.InsertE(tb.Context(), db)
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// DebugString returns a human-readable tree representation of the plan.
func (p *ScenarioPlan) DebugString() string {
	return debug.TreeString(p.graph)
}

// DryRunString returns the planned INSERT execution order with FK assignments.
func (p *ScenarioPlan) DryRunString() string {
	return debug.DryRunString(p.graph)
}

// Validate performs a dry-run of the plan, checking for type mismatches and
// constraint violations without inserting any records.
func (p *ScenarioPlan) Validate() error {
	return validatePlan(p.graph)
}

// ScenarioResult holds every record inserted by a [Scenario]. Read a record
// added with [Add] through its [Node]; records created for relations are
// available through [ScenarioResult.Nodes] and [ScenarioResult.All].
type ScenarioResult struct {
	scenario      *Scenario
	rootIDs       []string
	nodes         map[string]executor.NodeResult
	graph         *graph.Graph
	deleteFns     map[string]deleteFn // blueprint name → delete function snapshot
	cleanupValues map[string]any      // node ID → as-inserted value snapshot
}

// Node returns a node by blueprint name. If several nodes share the name, the
// one with the lexicographically smallest node ID is returned; prefer
// [Node.Get] for records added to the scenario.
func (r ScenarioResult) Node(name string) (NodeResult, bool) {
	return lookupNodeResult(r.nodes, name)
}

// Nodes returns all nodes that match the given blueprint name, sorted by node ID.
func (r ScenarioResult) Nodes(name string) []NodeResult {
	return lookupNodeResults(r.nodes, name)
}

// All returns all nodes in the result as a map keyed by opaque node ID.
func (r ScenarioResult) All() map[string]NodeResult {
	return cloneNodeResults(r.nodes)
}

// DebugString returns a human-readable tree of the execution result.
func (r ScenarioResult) DebugString() string {
	return debugResultString(r.graph)
}

// Cleanup deletes all records that were inserted in reverse dependency order.
func (r ScenarioResult) Cleanup(tb testing.TB, db DBTX, opts ...CleanupOption) {
	tb.Helper()
	ctx, cancel := cleanupContext(tb, opts)
	defer cancel()

	if err := r.CleanupE(ctx, db); err != nil {
		tb.Fatal(err)
	}
}

// CleanupE deletes all records that were inserted in reverse dependency order.
// CleanupE is fail-fast: it stops at the first delete error and returns it.
func (r ScenarioResult) CleanupE(ctx context.Context, db DBTX) error {
	return cleanupResultGraph(ctx, r.graph, r.deleteFns, r.cleanupValues, db)
}
