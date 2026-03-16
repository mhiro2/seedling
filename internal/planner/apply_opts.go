package planner

import (
	"fmt"
	"math/rand/v2"
	"reflect"

	"github.com/mhiro2/seedling/internal/field"
	"github.com/mhiro2/seedling/internal/graph"
)

// applyOpts applies Set and With options to the root node's value.
func applyOpts(node *graph.Node, opts *OptionSet) error {
	if opts == nil {
		return nil
	}

	// Apply generator functions first so explicit Set/With options can override
	// generated values deterministically.
	rng := opts.Rand
	if rng == nil {
		//nolint:gosec // Generate uses deterministic pseudo-random data for reproducible fixtures.
		rng = rand.New(rand.NewPCG(1, 1))
	}
	for _, fn := range opts.GenFns {
		updated, err := fn(rng, node.Value)
		if err != nil {
			return err
		}
		node.Value = updated
	}

	// Apply Set options.
	for fieldName, value := range opts.Sets {
		// We need a pointer to set fields.
		ptr := toPointer(node.Value)
		if err := field.SetField(ptr, fieldName, value); err != nil {
			return fmt.Errorf("apply set %q: %w", fieldName, err)
		}
		node.Value = reflect.ValueOf(ptr).Elem().Interface()
		node.SetFields = append(node.SetFields, fieldName)
	}

	// Apply With functions.
	for _, fn := range opts.WithFns {
		updated, err := fn(node.Value)
		if err != nil {
			return err
		}
		node.Value = updated
	}

	return nil
}

// toPointer returns a pointer to v. If v is already a pointer, returns it as-is.
func toPointer(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return v
	}
	ptr := reflect.New(rv.Type())
	ptr.Elem().Set(rv)
	return ptr.Interface()
}
