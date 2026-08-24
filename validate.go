package seedling

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/field"
	"github.com/mhiro2/seedling/internal/graph"
)

// Validate performs a dry-run of the plan, checking for type mismatches and
// constraint violations without inserting any records.
func (p *Plan[T]) Validate() error {
	return validatePlan(p.graph)
}

func validatePlan(g *graph.Graph) error {
	// 1. Check topological sort (cycle detection).
	nodes, err := g.TopoSort()
	if err != nil {
		return fmt.Errorf("sort validation graph: %w", err)
	}

	// 2. For each node, validate PKField exists on the Value struct.
	for _, node := range nodes {
		pkFields := node.PrimaryKeyFields()
		if len(pkFields) == 0 {
			continue // provided nodes may not have PKField
		}
		for _, pkField := range pkFields {
			if !field.Exists(node.Value, pkField) {
				return fmt.Errorf("validate plan: PKField %q not found on %s (node %s)",
					pkField, reflect.TypeOf(node.Value), node.ID)
			}
		}
	}

	// 3. For each edge, validate FK and referenced fields and their types.
	for _, node := range nodes {
		for _, edge := range node.Dependencies() {
			parent := edge.Parent
			child := edge.Child

			// Check type compatibility between the referenced parent field and child FK.
			parentType := reflect.TypeOf(parent.Value)
			childType := reflect.TypeOf(child.Value)

			if parentType.Kind() == reflect.Pointer {
				parentType = parentType.Elem()
			}
			if childType.Kind() == reflect.Pointer {
				childType = childType.Elem()
			}

			for _, binding := range edge.Bindings {
				if !field.Exists(child.Value, binding.ChildField) {
					return fmt.Errorf("validate plan: FK field %q not found on %s (node %s)",
						binding.ChildField, reflect.TypeOf(child.Value), child.ID)
				}

				refField, ok := parentType.FieldByName(binding.ParentField)
				if !ok {
					return fmt.Errorf("validate plan: referenced field %q not found on %s (node %s)",
						binding.ParentField, parentType, parent.ID)
				}

				fkField, ok := childType.FieldByName(binding.ChildField)
				if !ok {
					return fmt.Errorf("validate plan: FK field %q not found on %s (node %s)",
						binding.ChildField, childType, child.ID)
				}

				if !field.CanCopyType(refField.Type, fkField.Type) {
					return fmt.Errorf("validate plan: type mismatch: parent %s.%s (%s) is not assignable to child %s.%s (%s)",
						parent.BlueprintName, binding.ParentField, refField.Type,
						child.BlueprintName, binding.ChildField, fkField.Type)
				}
			}
		}
	}

	return nil
}
