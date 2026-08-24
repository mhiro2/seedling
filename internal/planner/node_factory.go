package planner

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/clone"
	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

func newBlueprintNode(bp *BlueprintDef, nodeID string, opts *OptionSet) (*graph.Node, error) {
	value := bp.Defaults()
	if value == nil {
		return nil, fmt.Errorf("%w: blueprint %q Defaults returned nil", errx.ErrInvalidOption, bp.Name)
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, fmt.Errorf("%w: blueprint %q Defaults returned nil %s", errx.ErrInvalidOption, bp.Name, rv.Type())
	}
	if rv.Type() != bp.ModelType {
		return nil, fmt.Errorf("%w: blueprint %q Defaults returned %s but expected %s", errx.ErrInvalidOption, bp.Name, rv.Type(), bp.ModelType)
	}

	node := newGraphNode(bp, nodeID, value, false)
	if err := applyOpts(node, opts); err != nil {
		return nil, err
	}
	return node, nil
}

func newProvidedNode(bp *BlueprintDef, nodeID string, value any) *graph.Node {
	return newGraphNode(bp, nodeID, clone.Value(value), true)
}

func newGraphNode(bp *BlueprintDef, nodeID string, value any, isProvided bool) *graph.Node {
	pkFields := pkFieldsForBlueprint(bp)
	return &graph.Node{
		ID:            nodeID,
		BlueprintName: bp.Name,
		Table:         bp.Table,
		Value:         value,
		IsProvided:    isProvided,
		PKField:       firstField(pkFields),
		PKFields:      pkFields,
	}
}

func (e *expander) providedBelongsToNode(rel RelationDef, nodeID nodeIdentity, opts *OptionSet) (*graph.Node, bool, error) {
	useVal, ok := usedRelationValue(opts, rel.Name)
	if !ok {
		return nil, false, nil
	}
	if useVal == nil {
		return nil, false, fmt.Errorf("%w: Use(%q) value must not be nil", errx.ErrInvalidOption, rel.Name)
	}
	rvUse := reflect.ValueOf(useVal)
	if rvUse.Kind() == reflect.Pointer && rvUse.IsNil() {
		return nil, false, fmt.Errorf("%w: Use(%q) value must not be nil", errx.ErrInvalidOption, rel.Name)
	}

	parentBP, err := e.reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return nil, false, fmt.Errorf("lookup blueprint %q for use %q: %w", rel.RefBlueprint, rel.Name, err)
	}

	// Normalize the provided value to the blueprint's exact model type.
	useVal = normalizeUseValue(parentBP.ModelType, useVal)

	usedNodeID := relationNodeIdentity(nodeID, rel.Name, 0, 1)
	usedNode := newProvidedNode(parentBP, usedNodeID.String(), useVal)
	e.graph.AddNode(usedNode)
	e.visited[usedNodeID] = usedNode

	return usedNode, true, nil
}

// normalizeUseValue converts between a struct and its direct pointer so a
// provided node always stores the blueprint's exact model type. Validation
// rejects all other type combinations before this function is called.
func normalizeUseValue(modelType reflect.Type, val any) any {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer && modelType.Kind() != reflect.Pointer {
		return rv.Elem().Interface()
	}
	if rv.Kind() != reflect.Pointer && modelType.Kind() == reflect.Pointer {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		return ptr.Interface()
	}
	return val
}
