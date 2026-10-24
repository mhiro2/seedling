package clone_test

import (
	"reflect"
	"testing"
	"time"

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
	// Arrange: the elements are references, so copying the array by value alone
	// would still share their pointees and backing arrays.
	type S struct {
		Ptrs   [2]*sample
		Slices [2][]int
	}
	orig := S{
		Ptrs:   [2]*sample{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
		Slices: [2][]int{{1, 2}, {3, 4}},
	}

	// Act
	cp := clone.Value(orig).(S)

	// Assert
	if !reflect.DeepEqual(cp, orig) {
		t.Fatalf("got %+v, want %+v", cp, orig)
	}
	for i := range orig.Ptrs {
		if cp.Ptrs[i] == orig.Ptrs[i] {
			t.Fatalf("Ptrs[%d] still points at the original element", i)
		}
	}

	cp.Ptrs[0].Name = "mutated"
	cp.Slices[1][0] = 99
	if orig.Ptrs[0].Name != "a" {
		t.Fatalf("original pointer element was mutated: got %v, want %v", orig.Ptrs[0].Name, "a")
	}
	if !reflect.DeepEqual(orig.Slices, [2][]int{{1, 2}, {3, 4}}) {
		t.Fatalf("original slice element was mutated: got %v", orig.Slices)
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

type unexportedRefs struct {
	Name  string
	count int
	items []int
}

// TestValue_UnexportedReferenceFieldsAreShared characterizes the documented
// limitation: unexported fields are copied as-is, so scalars keep their value
// while reference fields are shallow-copied and keep sharing their backing with
// the original. Deep-copying them would require unsafe and would also corrupt
// value types whose internals are unexported pointers (e.g. time.Time's
// location); keep isolated mutable state in exported fields instead.
func TestValue_UnexportedReferenceFieldsAreShared(t *testing.T) {
	// Arrange
	orig := unexportedRefs{Name: "root", count: 7, items: []int{1, 2, 3}}

	// Act
	cp := clone.Value(orig).(unexportedRefs)

	// Assert: every field keeps its value, exported or not.
	if cp.Name != "root" || cp.count != 7 {
		t.Fatalf("got Name=%q count=%d, want Name=%q count=%d", cp.Name, cp.count, "root", 7)
	}

	// Assert: backing is shared (documented behavior, not deep-copied).
	cp.items[0] = 99
	if orig.items[0] != 99 {
		t.Fatalf("expected shared backing for unexported field, got %v", orig.items)
	}
}

type withTime struct {
	When time.Time
}

// TestValue_TimeFieldKeepsLocation guards against deep-copying the unexported
// internals of value types: a cloned time.Time must remain Equal and keep the
// same Location as the original.
func TestValue_TimeFieldKeepsLocation(t *testing.T) {
	// Arrange
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	orig := withTime{When: time.Date(2026, 6, 4, 12, 0, 0, 0, loc)}

	// Act
	cp := clone.Value(orig).(withTime)

	// Assert
	if !cp.When.Equal(orig.When) {
		t.Fatalf("cloned time not equal: got %v, want %v", cp.When, orig.When)
	}
	if cp.When.Location() != orig.When.Location() {
		t.Fatalf("cloned time lost its Location identity: got %v, want %v", cp.When.Location(), orig.When.Location())
	}
}

type selfRef struct {
	Name string
	Next *selfRef
}

func TestValue_SelfReferentialPointer(t *testing.T) {
	// Arrange: a node whose pointer points back to itself.
	orig := &selfRef{Name: "a"}
	orig.Next = orig

	// Act
	cp := clone.Value(orig).(*selfRef)
	cp.Name = "b"

	// Assert
	if orig.Name != "a" {
		t.Fatalf("original was mutated: got %v, want %v", orig.Name, "a")
	}
	if cp == orig {
		t.Fatal("clone returned same pointer")
	}
	if cp.Next != cp {
		t.Fatal("clone did not preserve self-reference identity")
	}
}

func TestValue_MutuallyReferentialPointers(t *testing.T) {
	// Arrange: two nodes that point at each other.
	a := &selfRef{Name: "a"}
	b := &selfRef{Name: "b"}
	a.Next = b
	b.Next = a

	// Act
	cp := clone.Value(a).(*selfRef)

	// Assert
	if cp.Next == b {
		t.Fatal("clone shared the original mutual pointer")
	}
	if cp.Next.Next != cp {
		t.Fatal("clone did not preserve the mutual cycle")
	}
}

func TestValue_CyclicSliceElement(t *testing.T) {
	// Arrange: a struct reachable from its own slice via a pointer element.
	type ring struct {
		Name  string
		Peers []*ring
	}
	orig := &ring{Name: "root"}
	orig.Peers = []*ring{orig}

	// Act
	cp := clone.Value(orig).(*ring)

	// Assert
	if len(cp.Peers) != 1 {
		t.Fatalf("got %d peers, want 1", len(cp.Peers))
	}
	if cp.Peers[0] != cp {
		t.Fatal("clone did not preserve the cycle through the slice element")
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

	// Assert: the copy carries every value before it diverges.
	if !reflect.DeepEqual(cp, orig) {
		t.Fatalf("got %+v, want %+v", cp, orig)
	}

	cp.Name = "changed"
	cp.Values[0] = 9
	cp.Meta["a"] = "changed"
	cp.Child.Name = "mutated"
	cp.Any.(*sample).Name = "iface-mutated"
	if orig.Name != "root" {
		t.Fatalf("got %v, want %v", orig.Name, "root")
	}
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

func TestValue_SelfReferentialMap(t *testing.T) {
	// Arrange: a map that contains itself as a value.
	orig := map[string]any{"name": "root"}
	orig["self"] = orig

	// Act
	cp := clone.Value(orig).(map[string]any)

	// Assert
	if cp["name"] != "root" {
		t.Fatalf("got name %v, want root", cp["name"])
	}
	self, ok := cp["self"].(map[string]any)
	if !ok {
		t.Fatalf("self entry has type %T, want map", cp["self"])
	}
	if reflect.ValueOf(self).Pointer() != reflect.ValueOf(cp).Pointer() {
		t.Fatal("clone did not preserve the map self-reference")
	}
	if reflect.ValueOf(self).Pointer() == reflect.ValueOf(orig).Pointer() {
		t.Fatal("clone still points at the original map")
	}
}

func TestValue_SelfReferentialSlice(t *testing.T) {
	// Arrange: a slice whose first element is the slice itself.
	orig := make([]any, 2)
	orig[0] = orig
	orig[1] = "tail"

	// Act
	cp := clone.Value(orig).([]any)

	// Assert
	if cp[1] != "tail" {
		t.Fatalf("got tail %v, want tail", cp[1])
	}
	self, ok := cp[0].([]any)
	if !ok {
		t.Fatalf("first element has type %T, want slice", cp[0])
	}
	if reflect.ValueOf(self).Pointer() != reflect.ValueOf(cp).Pointer() {
		t.Fatal("clone did not preserve the slice self-reference")
	}
	if reflect.ValueOf(self).Pointer() == reflect.ValueOf(orig).Pointer() {
		t.Fatal("clone still points at the original slice")
	}
}

type namedInts []int

func TestValue_AliasedSlicesWithDistinctTypes(t *testing.T) {
	// Arrange: two fields of different slice types share one backing array.
	type holder struct {
		Plain []int
		Named namedInts
	}
	base := []int{1, 2, 3}
	orig := holder{Plain: base, Named: namedInts(base)}

	// Act
	cp := clone.Value(orig).(holder)

	// Assert
	if len(cp.Plain) != 3 || len(cp.Named) != 3 || cp.Named[2] != 3 {
		t.Fatalf("got %+v, want both slices copied", cp)
	}
	if reflect.ValueOf(cp.Plain).Pointer() == reflect.ValueOf(base).Pointer() {
		t.Fatal("clone still shares the original backing array")
	}
}
