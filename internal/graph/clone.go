package graph

import "reflect"

// Clone returns a structurally independent copy of the graph.
func (g *Graph) Clone() *Graph {
	if g == nil {
		return nil
	}

	cloned := New()
	nodeMap := make(map[*Node]*Node, len(g.nodes))

	for _, node := range g.nodes {
		copied := &Node{
			ID:            node.ID,
			BlueprintName: node.BlueprintName,
			Table:         node.Table,
			Value:         cloneValue(node.Value),
			IsProvided:    node.IsProvided,
			PKField:       node.PKField,
			PKFields:      append([]string(nil), node.PKFields...),
			SetFields:     append([]string(nil), node.SetFields...),
		}
		cloned.nodes[copied.ID] = copied
		nodeMap[node] = copied
		if g.root == node {
			cloned.root = copied
		}
	}

	for _, node := range g.nodes {
		parent := nodeMap[node]
		for _, edge := range node.dependents {
			copied := &Edge{
				Parent:     parent,
				Child:      nodeMap[edge.Child],
				LocalField: edge.LocalField,
				Bindings:   append([]FieldBinding(nil), edge.Bindings...),
			}
			parent.dependents = append(parent.dependents, copied)
			copied.Child.dependencies = append(copied.Child.dependencies, copied)
		}
	}

	return cloned
}

func cloneValue(value any) any {
	if value == nil {
		return nil
	}

	return cloneReflectValue(reflect.ValueOf(value)).Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	//nolint:exhaustive // only deep-copyable kinds need special handling; everything else falls through to default
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := reflect.New(value.Type().Elem())
		copied.Elem().Set(cloneReflectValue(value.Elem()))
		return copied
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := cloneReflectValue(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(copied)
		return wrapped
	case reflect.Struct:
		copied := reflect.New(value.Type()).Elem()
		copied.Set(value)
		for i := 0; i < value.NumField(); i++ {
			field := copied.Field(i)
			if !field.CanSet() {
				continue
			}
			field.Set(cloneReflectValue(value.Field(i)))
		}
		return copied
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			copied.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return copied
	case reflect.Array:
		copied := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			copied.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return copied
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			copied.SetMapIndex(cloneReflectValue(iter.Key()), cloneReflectValue(iter.Value()))
		}
		return copied
	default:
		return value
	}
}
