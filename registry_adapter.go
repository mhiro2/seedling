package seedling

import (
	"reflect"

	"github.com/mhiro2/seedling/internal/planner"
)

// registryAdapter adapts a Registry to the planner.Registry interface.
// It caches converted BlueprintDef values to avoid repeated allocations
// during a single planning/insert operation.
type registryAdapter struct {
	reg    *Registry
	byName map[string]*planner.BlueprintDef
	byType map[reflect.Type]*planner.BlueprintDef
}

func newRegistryAdapter(reg *Registry) *registryAdapter {
	return &registryAdapter{
		reg:    resolveRegistry(reg),
		byName: make(map[string]*planner.BlueprintDef),
		byType: make(map[reflect.Type]*planner.BlueprintDef),
	}
}

func (a *registryAdapter) LookupByName(name string) (*planner.BlueprintDef, error) {
	if cached, ok := a.byName[name]; ok {
		return cached, nil
	}
	def, err := a.reg.reg.lookupByName(name)
	if err != nil {
		return nil, err
	}
	bd := toBlueprintDef(def)
	a.byName[name] = bd
	return bd, nil
}

func (a *registryAdapter) LookupByType(t reflect.Type) (*planner.BlueprintDef, error) {
	if cached, ok := a.byType[t]; ok {
		return cached, nil
	}
	def, err := a.reg.reg.lookupByType(t)
	if err != nil {
		return nil, err
	}
	bd := toBlueprintDef(def)
	a.byType[t] = bd
	return bd, nil
}

func toBlueprintDef(def *blueprintDef) *planner.BlueprintDef {
	rels := make([]planner.RelationDef, len(def.relations))
	for i, r := range def.relations {
		rels[i] = planner.RelationDef{
			Name:             r.name,
			Kind:             planner.RelationKind(r.kind),
			LocalFields:      cloneStrings(r.localFields),
			RefBlueprint:     r.refBlueprint,
			ThroughBlueprint: r.throughBlueprint,
			RemoteFields:     cloneStrings(r.remoteFields),
			Required:         r.required,
			Count:            r.count,
			When:             r.when,
		}
	}
	return &planner.BlueprintDef{
		Name:      def.name,
		Table:     def.table,
		PKFields:  cloneStrings(def.pkFields),
		Relations: rels,
		Defaults:  def.defaults,
		Insert:    def.insert,
		Delete:    def.delete,
		ModelType: def.modelType,
	}
}
