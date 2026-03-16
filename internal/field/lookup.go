package field

import (
	"reflect"
	"sort"
	"sync"
)

var exportedFieldCache sync.Map

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
	f, ok := rt.FieldByName(name)
	return ok && f.IsExported()
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
