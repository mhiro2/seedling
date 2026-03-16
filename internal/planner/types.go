package planner

import (
	"context"
	"math/rand/v2"
	"reflect"

	"github.com/mhiro2/seedling/internal/graph"
)

// RelationKind is the planner's canonical relation kind.
type RelationKind string

const (
	BelongsTo  RelationKind = "belongs_to"
	HasMany    RelationKind = "has_many"
	ManyToMany RelationKind = "many_to_many"
)

// BlueprintDef is the planner's view of a registered blueprint.
type BlueprintDef struct {
	Name      string
	Table     string
	PKFields  []string
	Relations []RelationDef
	Defaults  func() any
	Insert    func(ctx context.Context, db, v any) (any, error)
	Delete    func(ctx context.Context, db, v any) error
	ModelType reflect.Type
}

// RelationDef is the planner's view of a relation.
type RelationDef struct {
	Name             string
	Kind             RelationKind
	LocalFields      []string
	RefBlueprint     string
	ThroughBlueprint string
	RemoteFields     []string
	Required         bool
	Count            int            // For has_many/many_to_many: number of records to create (default 1)
	When             func(any) bool // Optional predicate: expand only when true
}

// Registry is the interface the planner uses to look up blueprints.
type Registry interface {
	LookupByName(name string) (*BlueprintDef, error)
	LookupByType(t reflect.Type) (*BlueprintDef, error)
}

type WithFn func(value any) (any, error)

type GenerateFn func(r *rand.Rand, value any) (any, error)

// OptionSet holds parsed options for a single node.
type OptionSet struct {
	Sets    map[string]any                     // field name → value
	Uses    map[string]any                     // relation name → existing value
	Refs    map[string]*OptionSet              // relation name → nested options
	Omits   map[string]bool                    // relation name → true
	Whens   map[string]func(any) (bool, error) // relation name → dynamic expansion predicate
	WithFns []WithFn                           // typed root mutators
	Seqs    map[string]any                     // field name → func(int) any (sequence generators)
	GenFns  []GenerateFn                       // typed rand-driven mutators
	Rand    *rand.Rand                         // RNG used by GenFns
	Only    map[string]bool                    // nil = expand all; non-nil = lazy mode (root-level relation filter)
}

// PlanResult is the output of the planner.
type PlanResult struct {
	Graph *graph.Graph
}

// PlanManyResult is the output of the batch planner.
type PlanManyResult struct {
	Graph   *graph.Graph
	RootIDs []string
}
