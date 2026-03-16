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

	field := rv.Elem().FieldByName(name)
	if !field.IsValid() {
		return fmt.Errorf("set field %q: %w", name, errx.FieldNotFoundWithHint(rv.Elem().Type().Name(), name, exportedFields(rv.Elem().Type())))
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
