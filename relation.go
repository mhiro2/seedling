package seedling

import (
	"reflect"

	"github.com/mhiro2/seedling/internal/field"
)

// Relation describes a dependency between two blueprints.
type Relation struct {
	// Name is the non-empty identifier used with Use() and Ref() (e.g.
	// "company", "assignee"). It must be unique within the blueprint.
	Name string

	// Kind is the type of relationship (BelongsTo, HasMany, or ManyToMany).
	Kind RelationKind

	// LocalField is the single-column form of LocalFields.
	// For BelongsTo: the FK field on the child struct (e.g. "CompanyID").
	// For HasMany: the FK field on the referenced (child) blueprint's struct
	// that points back to this parent (e.g. "CompanyID" on User).
	// For ManyToMany: the FK field on the join-table struct pointing to the
	// current (parent) blueprint.
	LocalField string

	// LocalFields is the multi-column form of LocalField.
	// Use this for composite foreign keys.
	LocalFields []string

	// RefField is the single-column form of RefFields.
	// For BelongsTo, it identifies the field on the referenced blueprint whose
	// value is copied into LocalField. When omitted, the referenced blueprint's
	// primary key field is used.
	RefField string

	// RefFields is the multi-column form of RefField. Its order must match
	// LocalFields. When omitted, the referenced blueprint's primary key fields
	// are used.
	RefFields []string

	// RefBlueprint is the name of the related blueprint.
	// For BelongsTo: the parent blueprint.
	// For HasMany: the child blueprint to auto-create.
	// For ManyToMany: the related blueprint to auto-create.
	RefBlueprint string

	// ThroughBlueprint is required for ManyToMany and names the join-table
	// blueprint that should be auto-created between the current blueprint and
	// RefBlueprint.
	ThroughBlueprint string

	// RemoteField is the single-column form of RemoteFields.
	// Only used for ManyToMany, where it identifies the FK field on the
	// join-table struct pointing to the related blueprint.
	RemoteField string

	// RemoteFields is the multi-column form of RemoteField.
	// Only used for ManyToMany.
	RemoteFields []string

	// Optional disables default automatic expansion for this relation.
	// When false (the zero value), the relation is required and the planner
	// will automatically expand it. Set to true when you want to keep a
	// relation nullable by default; Ref, Use, or a true When predicate can
	// still enable expansion explicitly.
	Optional bool

	// Count specifies how many related records to create for HasMany and
	// ManyToMany relations. The zero value (including when Count is left
	// unset) creates no related records, so set it explicitly (e.g. Count: 1)
	// to expand the relation. A negative Count is rejected when the blueprint
	// is registered. Tests override it per call with [Count] inside [Ref].
	Count int

	// When is an optional predicate that dynamically controls whether this
	// relation should be expanded. The function receives the current record
	// value (the owning blueprint's struct or pointer-to-struct) and returns
	// true to expand the relation or false to skip it. When nil, the relation
	// uses the standard Optional logic.
	//
	// This allows conditional relation expansion based on the record's
	// field values at plan time:
	//
	//	When: func(v any) bool {
	//	    return v.(Task).Status == "assigned"
	//	},
	When func(v any) bool

	// Convert maps an FK field name to the converter that turns the referenced
	// parent value into that field's type. Use it when the parent value cannot
	// be assigned to the FK field directly, such as an int64 key stored in a
	// sql.NullInt64 or pgtype.Int8 column, or in a named ID type. A field
	// without a converter must accept the parent value by assignment, or as a
	// pointer to it.
	//
	// Keys name the fields that receive the parent value: LocalFields for
	// BelongsTo and HasMany, and LocalFields or RemoteFields for ManyToMany.
	// Converters run after the parent is inserted, on the key it returned, so
	// a zero key still becomes a valid (non-NULL) value:
	//
	//	Convert: map[string]seedling.FKConverter{
	//	    "ManagerID": seedling.ConvertFK(func(id int64) sql.NullInt64 {
	//	        return sql.NullInt64{Int64: id, Valid: true}
	//	    }),
	//	},
	//
	// A has-many child created through a matching BelongsTo of its own takes
	// its FK from that relation, including that relation's converters.
	Convert map[string]FKConverter
}

// FKConverter converts a referenced parent value into the value stored in a
// child FK field. Build one with [ConvertFK] and attach it to a relation with
// [Relation.Convert]. The zero value is invalid.
type FKConverter struct {
	conv *field.Converter
}

// ConvertFK returns an [FKConverter] that passes the parent value through fn.
// The parent field must be assignable to From, and To must be assignable to
// the FK field (or to the element of a pointer FK field). Both are checked
// before any record is inserted.
//
//	seedling.ConvertFK(func(id int64) UserID { return UserID(id) })
func ConvertFK[From, To any](fn func(From) To) FKConverter {
	if fn == nil {
		return FKConverter{}
	}
	return FKConverter{conv: &field.Converter{
		From: reflect.TypeFor[From](),
		To:   reflect.TypeFor[To](),
		Fn: func(v reflect.Value) reflect.Value {
			var in From
			reflect.ValueOf(&in).Elem().Set(v)
			out := fn(in)
			return reflect.ValueOf(&out).Elem()
		},
	}}
}

// WhenFunc creates a type-safe predicate for use in [Relation.When].
// It wraps a typed function so callers do not need a manual type assertion:
//
//	When: seedling.WhenFunc(func(t Task) bool {
//	    return t.Status == "assigned"
//	}),
func WhenFunc[T any](fn func(T) bool) func(any) bool {
	return func(v any) bool {
		t, ok := v.(T)
		if !ok {
			return false
		}
		return fn(t)
	}
}

// BelongsToRelation builds a BelongsTo relation with explicit semantics.
func BelongsToRelation(name, refBlueprint string, optional bool, localFields ...string) Relation {
	return Relation{
		Name:         name,
		Kind:         BelongsTo,
		LocalField:   firstField(localFields),
		LocalFields:  cloneStrings(localFields),
		RefBlueprint: refBlueprint,
		Optional:     optional,
	}
}

// HasManyRelation builds a HasMany relation with explicit semantics.
func HasManyRelation(name, refBlueprint string, optional bool, count int, localFields ...string) Relation {
	return Relation{
		Name:         name,
		Kind:         HasMany,
		LocalField:   firstField(localFields),
		LocalFields:  cloneStrings(localFields),
		RefBlueprint: refBlueprint,
		Optional:     optional,
		Count:        count,
	}
}

// ManyToManyRelation builds a ManyToMany relation with explicit semantics.
func ManyToManyRelation(name, throughBlueprint, refBlueprint string, optional bool, count int, localFields, remoteFields []string) Relation {
	return Relation{
		Name:             name,
		Kind:             ManyToMany,
		LocalField:       firstField(localFields),
		LocalFields:      cloneStrings(localFields),
		RefBlueprint:     refBlueprint,
		ThroughBlueprint: throughBlueprint,
		RemoteField:      firstField(remoteFields),
		RemoteFields:     cloneStrings(remoteFields),
		Optional:         optional,
		Count:            count,
	}
}
