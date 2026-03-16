package planner

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/clone"
	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

func newBlueprintNode(bp *BlueprintDef, nodeID string, opts *OptionSet) (*graph.Node, error) {
	node := newGraphNode(bp, nodeID, bp.Defaults(), false)
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

func validateUseValueType(relation string, expectedType reflect.Type, useVal any) error {
	useType := reflect.TypeOf(useVal)
	if useType == expectedType {
		return nil
	}
	if useType != nil && useType.Kind() == reflect.Pointer && useType.Elem() == expectedType {
		return nil
	}
	if expectedType != nil && expectedType.Kind() == reflect.Pointer && expectedType.Elem() == useType {
		return nil
	}

	gotName := "<nil>"
	if useType != nil {
		gotName = useType.String()
	}
	expectedName := "<nil>"
	if expectedType != nil {
		expectedName = expectedType.String()
	}

	return fmt.Errorf("validate use %q: %w", relation, errx.UseTypeMismatch(relation, expectedName, gotName))
}

func (e *expander) providedBelongsToNode(rel RelationDef, nodeID string, opts *OptionSet) (*graph.Node, bool, error) {
	useVal, ok := usedRelationValue(opts, rel.Name)
	if !ok {
		return nil, false, nil
	}
	if useVal == nil {
		return nil, false, fmt.Errorf("%w: Use(%q) value must not be nil", errx.ErrInvalidOption, rel.Name)
	}

	parentBP, err := e.reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return nil, false, fmt.Errorf("lookup blueprint %q for use %q: %w", rel.RefBlueprint, rel.Name, err)
	}
	if err := validateUseValueType(rel.Name, parentBP.ModelType, useVal); err != nil {
		return nil, false, err
	}

	// Normalize pointer to value type when blueprint expects a value type.
	useVal = normalizeUseValue(parentBP.ModelType, useVal)

	usedNode := newProvidedNode(parentBP, relationNodeID(nodeID, rel.Name, 0, 1), useVal)
	e.graph.AddNode(usedNode)
	e.visited[usedNode.ID] = usedNode

	return usedNode, true, nil
}

// normalizeUseValue dereferences a pointer value when the blueprint's model
// type is a non-pointer struct. This ensures that provided nodes always store
// values consistent with the blueprint definition, so NodeAs[T] works
// regardless of whether the caller passed T or *T to Use.
func normalizeUseValue(modelType reflect.Type, val any) any {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer && modelType.Kind() != reflect.Pointer {
		return rv.Elem().Interface()
	}
	return val
}
