package clone

import "reflect"

// Value returns a deep copy of v.
// If v is a pointer to a struct, it dereferences and copies the struct, returning a new pointer.
// If v is nil, it returns nil.
func Value(v any) any {
	if v == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(v)).Interface()
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
