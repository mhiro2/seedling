package clone

import "reflect"

// Value returns a deep copy of v.
// If v is a pointer to a struct, it dereferences and copies the struct, returning a new pointer.
// If v is nil, it returns nil.
//
// Cyclic references (a self-referential pointer, two structs that point at each
// other, a map or slice that contains itself) are handled: each pointer, map,
// and slice is cloned at most once and shared references map to a single clone,
// preserving identity instead of recursing forever.
//
// Only settable (exported) fields are deep-copied. An unexported reference-typed
// field (pointer, slice, map, ...) is shallow-copied and therefore keeps sharing
// its backing storage with the original; this also preserves the internal
// pointers of value types such as time.Time. Keep mutable state that must be
// isolated across clones in exported fields.
func Value(v any) any {
	if v == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(v), make(map[visitKey]reflect.Value)).Interface()
}

// visitKey identifies a reference already being cloned. Pointers and maps are
// identified by type and address; slices additionally carry their bounds
// because distinct slices can share one backing array. The type is part of the
// key so that a []T and a named slice type aliasing the same memory each get a
// clone of their own type.
type visitKey struct {
	typ reflect.Type
	ptr uintptr
	len int
	cap int
}

func keyFor(value reflect.Value) visitKey {
	key := visitKey{typ: value.Type(), ptr: value.Pointer()}
	if value.Kind() == reflect.Slice {
		key.len = value.Len()
		key.cap = value.Cap()
	}
	return key
}

// cloneReflectValue deep-copies value. visited maps every pointer, map, and
// slice already being cloned to its clone so that cycles terminate and aliased
// references stay aliased in the copy. Each of those kinds can reach itself
// without going through a pointer (m["self"] = m, s[0] = s), so all three are
// recorded before their contents are visited.
func cloneReflectValue(value reflect.Value, visited map[visitKey]reflect.Value) reflect.Value {
	//nolint:exhaustive // only deep-copyable kinds need special handling; everything else falls through to default
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}

		key := keyFor(value)
		if existing, ok := visited[key]; ok {
			return existing
		}

		copied := reflect.New(value.Type().Elem())
		// Record before recursing so a pointer that reaches itself resolves to
		// this clone instead of recursing forever.
		visited[key] = copied
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

		key := keyFor(value)
		if existing, ok := visited[key]; ok {
			return existing
		}

		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		visited[key] = copied
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

		key := keyFor(value)
		if existing, ok := visited[key]; ok {
			return existing
		}

		copied := reflect.MakeMapWithSize(value.Type(), value.Len())
		visited[key] = copied
		iter := value.MapRange()
		for iter.Next() {
			copied.SetMapIndex(cloneReflectValue(iter.Key(), visited), cloneReflectValue(iter.Value(), visited))
		}
		return copied
	default:
		return value
	}
}
