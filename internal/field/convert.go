package field

import (
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
)

// Converter turns a referenced parent field value into the value stored in a
// child FK field whose type the parent value cannot be assigned to, such as
// int64 into sql.NullInt64 or a named ID type.
type Converter struct {
	From reflect.Type
	To   reflect.Type
	// Fn receives a value of type From and returns a value of type To.
	Fn func(reflect.Value) reflect.Value
}

// CanBindType reports whether a source field type can be bound to a
// destination field type, through conv when it is non-nil and by direct
// assignment (or pointer wrapping) otherwise.
func CanBindType(source, destination reflect.Type, conv *Converter) bool {
	if conv == nil {
		return CanCopyType(source, destination)
	}
	return source.AssignableTo(conv.From) && CanCopyType(conv.To, destination)
}

// bindTypeError describes why CanBindType rejected a binding.
func bindTypeError(dstName string, source, destination reflect.Type, conv *Converter) error {
	if conv == nil {
		return fmt.Errorf("set field %q: %w; convert it with Relation.Convert", dstName, errx.TypeMismatch(dstName, destination.String(), source.String()))
	}
	if !source.AssignableTo(conv.From) {
		return fmt.Errorf("convert into field %q: %w", dstName, errx.TypeMismatch(dstName, conv.From.String()+" (converter input)", source.String()))
	}
	return fmt.Errorf("convert into field %q: %w", dstName, errx.TypeMismatch(dstName, destination.String(), conv.To.String()+" (converter output)"))
}

// convert runs conv on value, first assigning value to a fresh From so that a
// named source type reaches a converter written for its unnamed form.
func (c *Converter) convert(value reflect.Value) reflect.Value {
	in := value
	if value.Type() != c.From {
		in = reflect.New(c.From).Elem()
		in.Set(value)
	}
	return c.Fn(in)
}
