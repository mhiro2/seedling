package seedling

// Relation describes a dependency between two blueprints.
type Relation struct {
	// Name is the identifier used with Use() and Ref() (e.g. "company", "assignee").
	Name string

	// Kind is the type of relationship (BelongsTo, HasMany, or ManyToMany).
	Kind RelationKind

	// LocalField is the legacy single-column form of LocalFields.
	// For BelongsTo: the FK field on the child struct (e.g. "CompanyID").
	// For HasMany: the FK field on the referenced (child) blueprint's struct
	// that points back to this parent (e.g. "CompanyID" on User).
	// For ManyToMany: the FK field on the join-table struct pointing to the
	// current (parent) blueprint.
	LocalField string

	// LocalFields is the multi-column form of LocalField.
	// Use this when the related blueprint has a composite primary key.
	LocalFields []string

	// RefBlueprint is the name of the related blueprint.
	// For BelongsTo: the parent blueprint.
	// For HasMany: the child blueprint to auto-create.
	// For ManyToMany: the related blueprint to auto-create.
	RefBlueprint string

	// ThroughBlueprint is required for ManyToMany and names the join-table
	// blueprint that should be auto-created between the current blueprint and
	// RefBlueprint.
	ThroughBlueprint string

	// RemoteField is the legacy single-column form of RemoteFields.
	// Only used for ManyToMany, where it identifies the FK field on the
	// join-table struct pointing to the related blueprint.
	RemoteField string

	// RemoteFields is the multi-column form of RemoteField.
	// Only used for ManyToMany.
	RemoteFields []string

	// Optional disables automatic expansion for this relation.
	// When false (the zero value), the relation is required and the planner
	// will automatically expand it. Set to true when you want to keep a
	// relation nullable or handle it manually.
	Optional bool

	// Count specifies how many related records to create for HasMany and
	// ManyToMany relations. Defaults to 1 if not set.
	Count int

	// When is an optional predicate that dynamically controls whether this
	// relation should be expanded. The function receives the current record
	// value (the owning blueprint's struct) and returns true to expand the
	// relation or false to skip it. When nil, the relation uses the
	// standard Optional logic.
	//
	// This allows conditional relation expansion based on the record's
	// field values at plan time:
	//
	//	When: func(v any) bool {
	//	    return v.(Task).Status == "assigned"
	//	},
	When func(v any) bool
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
