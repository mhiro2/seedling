package clone_test

import (
	"reflect"
	"testing"

	"github.com/mhiro2/seedling/internal/clone"
)

type sample struct {
	ID   int
	Name string
}

type nestedSample struct {
	Name   string
	Values []int
	Meta   map[string]string
	Child  *sample
	Any    any
}

func TestValue_Struct(t *testing.T) {
	// Arrange
	orig := sample{ID: 1, Name: "a"}

	// Act
	cp := clone.Value(orig).(sample)
	cp.Name = "b"

	// Assert
	if orig.Name != "a" {
		t.Fatalf("original was mutated: got %v, want %v", orig.Name, "a")
	}
}

func TestValue_Pointer(t *testing.T) {
	// Arrange
	orig := &sample{ID: 1, Name: "a"}

	// Act
	cp := clone.Value(orig).(*sample)
	cp.Name = "b"

	// Assert
	if orig.Name != "a" {
		t.Fatalf("original was mutated via pointer clone: got %v, want %v", orig.Name, "a")
	}
	if orig == cp {
		t.Fatal("clone returned same pointer")
	}
}

func TestValue_Nil(t *testing.T) {
	// Act
	result := clone.Value(nil)

	// Assert
	if result != nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestValue_NilPointer(t *testing.T) {
	// Arrange
	var orig *sample

	// Act
	result := clone.Value(orig)

	// Assert
	if result.(*sample) != nil {
		t.Fatal("expected nil *sample from cloning nil pointer")
	}
}

func TestValue_NilSlice(t *testing.T) {
	// Arrange
	type S struct {
		Items []int
	}
	orig := S{Items: nil}

	// Act
	cp := clone.Value(orig).(S)

	// Assert
	if cp.Items != nil {
		t.Fatal("expected nil slice after cloning")
	}
}

func TestValue_NilMap(t *testing.T) {
	// Arrange
	type S struct {
		Meta map[string]string
	}
	orig := S{Meta: nil}

	// Act
	cp := clone.Value(orig).(S)

	// Assert
	if cp.Meta != nil {
		t.Fatal("expected nil map after cloning")
	}
}

func TestValue_Array(t *testing.T) {
	// Arrange
	type S struct {
		Arr [3]int
	}
	orig := S{Arr: [3]int{1, 2, 3}}

	// Act
	cp := clone.Value(orig).(S)
	cp.Arr[0] = 99

	// Assert
	if orig.Arr[0] != 1 {
		t.Fatal("original array was mutated")
	}
}

func TestValue_NilInterface(t *testing.T) {
	// Arrange
	type S struct {
		Iface any
	}
	orig := S{Iface: nil}

	// Act
	cp := clone.Value(orig).(S)

	// Assert
	if cp.Iface != nil {
		t.Fatal("expected nil interface after cloning")
	}
}

type withUnexported struct {
	Name     string
	internal int //nolint:unused // intentionally unexported to test clone behavior
}

func TestValue_UnexportedField(t *testing.T) {
	// Arrange
	orig := withUnexported{Name: "test"}

	// Act
	cp := clone.Value(orig).(withUnexported)

	// Assert
	if cp.Name != "test" {
		t.Fatalf("got %v, want %v", cp.Name, "test")
	}
}

func TestValue_DeepCopy(t *testing.T) {
	// Arrange
	orig := nestedSample{
		Name:   "root",
		Values: []int{1, 2, 3},
		Meta:   map[string]string{"a": "b"},
		Child:  &sample{ID: 1, Name: "child"},
		Any:    &sample{ID: 2, Name: "iface"},
	}

	// Act
	cp := clone.Value(orig).(nestedSample)
	cp.Values[0] = 9
	cp.Meta["a"] = "changed"
	cp.Child.Name = "mutated"
	cp.Any.(*sample).Name = "iface-mutated"

	// Assert
	if !reflect.DeepEqual(orig.Values, []int{1, 2, 3}) {
		t.Fatalf("got %v, want %v", orig.Values, []int{1, 2, 3})
	}
	if !reflect.DeepEqual(orig.Meta, map[string]string{"a": "b"}) {
		t.Fatalf("got %v, want %v", orig.Meta, map[string]string{"a": "b"})
	}
	if orig.Child.Name != "child" {
		t.Fatalf("got %v, want %v", orig.Child.Name, "child")
	}
	if orig.Any.(*sample).Name != "iface" {
		t.Fatalf("got %v, want %v", orig.Any.(*sample).Name, "iface")
	}
}
