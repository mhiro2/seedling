package field

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
)

// SetField sets the named exported field on a pointer to a struct.
func SetField(ptr any, name string, value any) error {
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("%w: SetField requires a pointer to struct", errx.ErrInvalidOption)
	}

	elem := rv.Elem()
	rt := elem.Type()
	entry, ok := lookupFieldIndex(rt, name)
	if !ok {
		return fmt.Errorf("set field %q: %w", name, errx.FieldNotFoundWithHint(rt.Name(), name, exportedFields(rt)))
	}

	field, err := fieldByIndexAlloc(elem, entry.Index)
	if err != nil {
		return fmt.Errorf("set field %q: %w", name, err)
	}
	if !field.CanSet() {
		return fmt.Errorf("%w: field %q is unexported", errx.ErrFieldNotFound, name)
	}

	// Handle nil value: allow for pointer/interface fields, reject for others.
	if value == nil {
		//nolint:exhaustive // only nillable kinds are relevant here; others fall through to default
		switch field.Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			field.Set(reflect.Zero(field.Type()))
			return nil
		default:
			return fmt.Errorf("set field %q: %w", name, errx.TypeMismatch(name, field.Type().String(), "<nil>"))
		}
	}

	val := reflect.ValueOf(value)
	if !val.Type().AssignableTo(field.Type()) {
		return fmt.Errorf("set field %q: %w", name, errx.TypeMismatch(name, field.Type().String(), val.Type().String()))
	}

	field.Set(val)
	return nil
}

// fieldByIndexAlloc walks index like reflect.Value.FieldByIndex but allocates
// nil embedded pointers along the way instead of panicking, so a promoted field
// can be assigned on a freshly zeroed struct. An embedded pointer that is nil
// and cannot be set (unexported) is reported as an error.
func fieldByIndexAlloc(v reflect.Value, index []int) (reflect.Value, error) {
	for depth, i := range index {
		if depth > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, fmt.Errorf("%w: embedded pointer %s is nil and cannot be allocated", errx.ErrFieldNotFound, v.Type())
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, nil
}

// checkFieldPath reports whether fieldByIndexAlloc could reach index on v and
// yield a settable field, without mutating v. Up to the first nil embedded
// pointer the walk follows the value; from there it follows types alone,
// because every deeper embedded pointer is nil as well and can be allocated
// only if its field is exported.
func checkFieldPath(v reflect.Value, index []int) error {
	for depth, i := range index {
		if depth > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return checkAllocPath(v, index[depth:])
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	if !v.CanSet() {
		return fmt.Errorf("%w: field is unexported", errx.ErrFieldNotFound)
	}
	return nil
}

func checkAllocPath(nilPtr reflect.Value, rest []int) error {
	if !nilPtr.CanSet() {
		return fmt.Errorf("%w: embedded pointer %s is nil and cannot be allocated", errx.ErrFieldNotFound, nilPtr.Type())
	}
	t := nilPtr.Type()
	for depth, i := range rest {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		sf := t.Field(i)
		if depth < len(rest)-1 && sf.Type.Kind() == reflect.Pointer && !sf.IsExported() {
			return fmt.Errorf("%w: embedded pointer %s is nil and cannot be allocated", errx.ErrFieldNotFound, sf.Type)
		}
		t = sf.Type
	}
	return nil
}
