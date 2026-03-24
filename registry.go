package seedling

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/mhiro2/seedling/internal/errx"
)

type relationDef struct {
	name             string
	kind             RelationKind
	localFields      []string
	refBlueprint     string
	throughBlueprint string
	remoteFields     []string
	required         bool
	count            int
	when             func(any) bool
}

// blueprintDef is a type-erased wrapper around Blueprint[T].
type blueprintDef struct {
	name      string
	table     string
	pkFields  []string
	relations []relationDef
	traits    map[string][]Option
	defaults  func() any
	insert    func(ctx context.Context, db, v any) (any, error)
	delete    func(ctx context.Context, db, v any) error
	modelType reflect.Type
}

// Registry stores registered blueprints independently from the package default registry.
// A registry enforces a 1:1 mapping between a Go type and a blueprint name.
// If you need multiple blueprints for similar shapes, define distinct Go types.
type Registry struct {
	reg *registry
}

var defaultRegistry = NewRegistry()

type registry struct {
	mu     sync.RWMutex
	byName map[string]*blueprintDef
	byType map[reflect.Type]*blueprintDef
}

func newRegistry() *registry {
	return &registry{
		byName: make(map[string]*blueprintDef),
		byType: make(map[reflect.Type]*blueprintDef),
	}
}

// NewRegistry creates an isolated blueprint registry.
func NewRegistry() *Registry {
	return &Registry{reg: newRegistry()}
}

// Register registers a blueprint in the package default registry.
func Register[T any](bp Blueprint[T]) error {
	return RegisterTo(defaultRegistry, bp)
}

// MustRegister registers a blueprint in the package default registry and panics on error.
func MustRegister[T any](bp Blueprint[T]) {
	MustRegisterTo(defaultRegistry, bp)
}

// RegisterTo registers a blueprint in the provided registry.
// Each Go type can be registered at most once per registry.
func RegisterTo[T any](dst *Registry, bp Blueprint[T]) error {
	return registerTyped[T](resolveRegistry(dst).reg, bp)
}

// MustRegisterTo registers a blueprint in the provided registry and panics on error.
func MustRegisterTo[T any](dst *Registry, bp Blueprint[T]) {
	if err := RegisterTo(dst, bp); err != nil {
		panic(err)
	}
}

// ResetRegistry clears all blueprints from the package default registry. Intended for testing.
func ResetRegistry() {
	defaultRegistry.Reset()
}

// Reset clears all blueprints from the registry.
func (r *Registry) Reset() {
	reg := resolveRegistry(r).reg
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.byName = make(map[string]*blueprintDef)
	reg.byType = make(map[reflect.Type]*blueprintDef)
}

func registerTyped[T any](r *registry, bp Blueprint[T]) error {
	if bp.Name == "" {
		return fmt.Errorf("%w: blueprint Name must not be empty", errx.ErrInvalidOption)
	}
	if bp.Insert == nil {
		return fmt.Errorf("%w: blueprint %q must have an Insert function", errx.ErrInvalidOption, bp.Name)
	}

	modelType := reflect.TypeFor[T]()
	if modelType.Kind() == reflect.Interface {
		return fmt.Errorf("%w: blueprint %q uses interface type %s; register a concrete struct type", errx.ErrInvalidOption, bp.Name, modelType)
	}
	if modelType.Kind() == reflect.Pointer {
		return fmt.Errorf("%w: blueprint %q uses pointer type %s; use the struct type directly (e.g. Blueprint[User] instead of Blueprint[*User])", errx.ErrInvalidOption, bp.Name, modelType)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byName[bp.Name]; exists {
		return fmt.Errorf("register blueprint %q: %w", bp.Name, errx.DuplicateBlueprint(bp.Name))
	}

	if _, exists := r.byType[modelType]; exists {
		return fmt.Errorf("%w: Go type %v is already registered under a different blueprint name; define a distinct Go type to register another blueprint", errx.ErrDuplicateBlueprint, modelType)
	}

	var deleteFn func(ctx context.Context, db, v any) error
	if bp.Delete != nil {
		deleteFn = func(ctx context.Context, db, v any) error {
			return bp.Delete(ctx, db, v.(T))
		}
	}

	def := &blueprintDef{
		name:      bp.Name,
		table:     bp.Table,
		pkFields:  normalizeFields(bp.PKField, bp.PKFields),
		relations: normalizeRelations(bp.Relations),
		traits:    bp.Traits,
		modelType: modelType,
		defaults: func() any {
			if bp.Defaults != nil {
				return bp.Defaults()
			}
			var z T
			return z
		},
		insert: func(ctx context.Context, db, v any) (any, error) {
			return bp.Insert(ctx, db, v.(T))
		},
		delete: deleteFn,
	}

	r.byName[bp.Name] = def
	r.byType[modelType] = def
	return nil
}

func resolveRegistry(r *Registry) *Registry {
	if r == nil {
		return defaultRegistry
	}
	return r
}

func normalizeFields(single string, multi []string) []string {
	if len(multi) > 0 {
		out := make([]string, 0, len(multi))
		for _, field := range multi {
			if field == "" {
				continue
			}
			out = append(out, field)
		}
		if len(out) > 0 {
			return out
		}
	}
	if single == "" {
		return nil
	}
	return []string{single}
}

func normalizeRelations(rels []Relation) []relationDef {
	if len(rels) == 0 {
		return nil
	}

	out := make([]relationDef, len(rels))
	for i, rel := range rels {
		out[i] = relationDef{
			name:             rel.Name,
			kind:             rel.Kind,
			localFields:      normalizeFields(rel.LocalField, rel.LocalFields),
			refBlueprint:     rel.RefBlueprint,
			throughBlueprint: rel.ThroughBlueprint,
			remoteFields:     normalizeFields(rel.RemoteField, rel.RemoteFields),
			required:         relationRequired(rel),
			count:            rel.Count,
			when:             rel.When,
		}
	}
	return out
}

func relationRequired(rel Relation) bool {
	return !rel.Optional
}

func cloneStrings(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	out := make([]string, len(fields))
	copy(out, fields)
	return out
}

func firstField(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func (r *registry) lookupByName(name string) (*blueprintDef, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint %q: %w", name, errx.BlueprintNotFound(name))
	}
	return def, nil
}

func (r *registry) lookupByType(t reflect.Type) (*blueprintDef, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.byType[t]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint type %s: %w", t, errx.BlueprintNotFound(t.String()))
	}
	return def, nil
}
