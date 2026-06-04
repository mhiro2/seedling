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

	rt := rv.Type()
	entry, ok := lookupFieldIndex(rt, name)
	if !ok {
		return nil, fmt.Errorf("get field %q: %w", name, errx.FieldNotFoundWithHint(rt.Name(), name, exportedFields(rt)))
	}
	// An unexported field cannot be read via reflection (.Interface() panics), so
	// report it as not found, mirroring how Copy/SetField reject unexported
	// destinations instead of panicking.
	if !entry.Exported {
		return nil, fmt.Errorf("%w: field %q is unexported", errx.ErrFieldNotFound, name)
	}

	return rv.FieldByIndex(entry.Index).Interface(), nil
}
