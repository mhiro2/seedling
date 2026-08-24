package field

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/mhiro2/seedling/internal/errx"
)

var (
	exportedFieldCache sync.Map
	fieldIndexCache    sync.Map
)

// fieldIndexKey identifies a (struct type, field name) pair.
type fieldIndexKey struct {
	Type reflect.Type
	Name string
}

// fieldIndexEntry caches the resolved field index path and metadata for a
// given struct type / field name pair. Only successful lookups are stored to
// keep the cache bounded under adversarial inputs (e.g. fuzz tests).
type fieldIndexEntry struct {
	Index    []int
	Type     reflect.Type
	Exported bool
}

// lookupFieldIndex resolves a field index path for the given type and name,
// caching the result. The cache stores only successful lookups so that
// pathological miss patterns (random field names) cannot grow it unbounded.
func lookupFieldIndex(rt reflect.Type, name string) (fieldIndexEntry, bool) {
	key := fieldIndexKey{Type: rt, Name: name}
	if v, ok := fieldIndexCache.Load(key); ok {
		return v.(fieldIndexEntry), true
	}
	f, ok := rt.FieldByName(name)
	if !ok {
		return fieldIndexEntry{}, false
	}
	entry := fieldIndexEntry{
		Index:    append([]int(nil), f.Index...),
		Type:     f.Type,
		Exported: f.IsExported(),
	}
	fieldIndexCache.Store(key, entry)
	return entry, true
}

// Exists reports whether the struct type has an exported field with the given name.
func Exists(v any, name string) bool {
	rt := reflect.TypeOf(v)
	if rt == nil {
		return false
	}
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return false
	}
	entry, ok := lookupFieldIndex(rt, name)
	return ok && entry.Exported
}

// CanCopyType reports whether Copy can assign a source field type to a
// destination field type. In addition to direct assignment, a value can be
// wrapped in a newly allocated destination pointer.
func CanCopyType(source, destination reflect.Type) bool {
	return source.AssignableTo(destination) ||
		destination.Kind() == reflect.Pointer && source.AssignableTo(destination.Elem())
}

// Copy reads srcName from src (struct or *struct) and assigns the value to
// dstName on dstPtr (must be *struct). It avoids the boxing round-trip that
// using GetField + SetField would incur, and reuses the cached field index
// from lookupFieldIndex so the hot path performs no FieldByName lookups after
// the first call for a given (type, name) pair.
func Copy(src any, srcName string, dstPtr any, dstName string) error {
	srcVal := reflect.ValueOf(src)
	if srcVal.Kind() == reflect.Pointer {
		srcVal = srcVal.Elem()
	}
	if srcVal.Kind() != reflect.Struct {
		return fmt.Errorf("%w: source must be a struct or pointer to struct", errx.ErrInvalidOption)
	}
	srcType := srcVal.Type()
	srcEntry, ok := lookupFieldIndex(srcType, srcName)
	if !ok {
		return fmt.Errorf("get field %q: %w", srcName, errx.FieldNotFoundWithHint(srcType.Name(), srcName, exportedFields(srcType)))
	}
	if !srcEntry.Exported {
		return fmt.Errorf("%w: field %q is unexported", errx.ErrFieldNotFound, srcName)
	}

	dstRV := reflect.ValueOf(dstPtr)
	if dstRV.Kind() != reflect.Pointer || dstRV.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("%w: destination must be a pointer to struct", errx.ErrInvalidOption)
	}
	dstElem := dstRV.Elem()
	dstType := dstElem.Type()
	dstEntry, ok := lookupFieldIndex(dstType, dstName)
	if !ok {
		return fmt.Errorf("set field %q: %w", dstName, errx.FieldNotFoundWithHint(dstType.Name(), dstName, exportedFields(dstType)))
	}
	if !dstEntry.Exported {
		return fmt.Errorf("%w: field %q is unexported", errx.ErrFieldNotFound, dstName)
	}

	srcField, err := srcVal.FieldByIndexErr(srcEntry.Index)
	if err != nil {
		return fmt.Errorf("get field %q: %w: %w", srcName, errx.ErrInvalidOption, err)
	}
	if !srcField.CanInterface() {
		return fmt.Errorf("%w: field %q is unexported", errx.ErrFieldNotFound, srcName)
	}
	dstField, err := dstElem.FieldByIndexErr(dstEntry.Index)
	if err != nil {
		return fmt.Errorf("set field %q: %w: %w", dstName, errx.ErrInvalidOption, err)
	}

	if !dstField.CanSet() {
		return fmt.Errorf("%w: field %q is unexported", errx.ErrFieldNotFound, dstName)
	}
	if dstField.Kind() == reflect.Pointer && srcField.Type().AssignableTo(dstField.Type().Elem()) {
		wrapped := reflect.New(dstField.Type().Elem())
		wrapped.Elem().Set(srcField)
		dstField.Set(wrapped)
		return nil
	}
	if !CanCopyType(srcField.Type(), dstField.Type()) {
		return fmt.Errorf("set field %q: %w", dstName, errx.TypeMismatch(dstName, dstField.Type().String(), srcField.Type().String()))
	}

	dstField.Set(srcField)
	return nil
}

// exportedFields returns the sorted names of all exported fields on a struct type.
func exportedFields(rt reflect.Type) []string {
	if cached, ok := exportedFieldCache.Load(rt); ok {
		return cached.([]string)
	}

	var names []string
	for f := range rt.Fields() {
		if f.IsExported() {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	exportedFieldCache.Store(rt, names)
	return names
}
