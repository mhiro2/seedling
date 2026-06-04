package clone

import "reflect"

// Value returns a deep copy of v.
// If v is a pointer to a struct, it dereferences and copies the struct, returning a new pointer.
// If v is nil, it returns nil.
//
// Cyclic references (e.g. a self-referential pointer or two structs that point
// at each other) are handled: each pointer is cloned at most once and shared
// addresses map to a single clone, preserving identity instead of recursing
// forever.
func Value(v any) any {
	if v == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(v), make(map[uintptr]reflect.Value)).Interface()
}

// cloneReflectValue deep-copies value. visited maps the address of an
// already-cloned pointer to its clone so that cycles terminate and aliased
// pointers stay aliased in the copy. Cycles can only form through pointers
// (interfaces, slices, and maps reach back via a pointer element), so recording
// pointers alone is sufficient to break every cycle.
func cloneReflectValue(value reflect.Value, visited map[uintptr]reflect.Value) reflect.Value {
	//nolint:exhaustive // only deep-copyable kinds need special handling; everything else falls through to default
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		ptr := value.Pointer()
		if existing, ok := visited[ptr]; ok {
			return existing
		}

		copied := reflect.New(value.Type().Elem())
		// Record before recursing so a pointer that reaches itself resolves to
		// this clone instead of recursing forever.
		visited[ptr] = copied
		copied.Elem().Set(cloneReflectValue(value.Elem(), visited))
		return copied
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := cloneReflectValue(value.Elem(), visited)
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
			field.Set(cloneReflectValue(value.Field(i), visited))
		}
		return copied
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			copied.Index(i).Set(cloneReflectValue(value.Index(i), visited))
		}
		return copied
	case reflect.Array:
		copied := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			copied.Index(i).Set(cloneReflectValue(value.Index(i), visited))
		}
		return copied
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		copied := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			copied.SetMapIndex(cloneReflectValue(iter.Key(), visited), cloneReflectValue(iter.Value(), visited))
		}
		return copied
	default:
		return value
	}
}
