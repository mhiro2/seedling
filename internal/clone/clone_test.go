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
