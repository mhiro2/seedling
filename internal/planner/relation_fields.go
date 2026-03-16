package planner

import (
	"fmt"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/graph"
)

func pkFieldsForBlueprint(bp *BlueprintDef) []string {
	return cloneFields(bp.PKFields)
}

func localFieldsForRelation(rel RelationDef) []string {
	return cloneFields(rel.LocalFields)
}

func remoteFieldsForRelation(rel RelationDef) []string {
	return cloneFields(rel.RemoteFields)
}

func buildBindings(blueprint string, rel RelationDef, parentFields, childFields []string) ([]graph.FieldBinding, error) {
	if len(parentFields) == 0 {
		return nil, fmt.Errorf("%w: relation %q on blueprint %q refers to a blueprint with no PK fields", errx.ErrInvalidOption, rel.Name, blueprint)
	}
	if len(childFields) == 0 {
		return nil, fmt.Errorf("%w: relation %q on blueprint %q is missing FK field mappings", errx.ErrInvalidOption, rel.Name, blueprint)
	}
	if len(parentFields) != len(childFields) {
		return nil, fmt.Errorf("%w: relation %q on blueprint %q maps %d FK fields to %d PK fields", errx.ErrInvalidOption, rel.Name, blueprint, len(childFields), len(parentFields))
	}

	bindings := make([]graph.FieldBinding, len(parentFields))
	for i := range parentFields {
		bindings[i] = graph.FieldBinding{
			ParentField: parentFields[i],
			ChildField:  childFields[i],
		}
	}
	return bindings, nil
}

func buildLocalBindings(blueprint string, rel RelationDef, parentFields []string) ([]graph.FieldBinding, error) {
	return buildBindings(blueprint, rel, parentFields, localFieldsForRelation(rel))
}

func buildRemoteBindings(blueprint string, rel RelationDef, parentFields []string) ([]graph.FieldBinding, error) {
	return buildBindings(blueprint, rel, parentFields, remoteFieldsForRelation(rel))
}

func sameFields(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneFields(fields []string) []string {
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
