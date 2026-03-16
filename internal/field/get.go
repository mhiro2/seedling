package field

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
)

// GetField returns the value of the named exported field from a struct or pointer to struct.
func GetField(v any, name string) (any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: GetField requires a struct or pointer to struct", errx.ErrInvalidOption)
	}

	field := rv.FieldByName(name)
	if !field.IsValid() {
		return nil, fmt.Errorf("get field %q: %w", name, errx.FieldNotFoundWithHint(rv.Type().Name(), name, exportedFields(rv.Type())))
	}

	return field.Interface(), nil
}
