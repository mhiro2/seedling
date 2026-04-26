package seedling

import "context"

// Blueprint describes how to create and insert a model of type T.
type Blueprint[T any] struct {
	// Name is the unique identifier for this blueprint (e.g. "task", "company").
	// Used for relation references (RefBlueprint) and Result.Node() lookups.
	Name string

	// Table is the database table name, used for debug output only.
	Table string

	// PKField is the Go struct field name of the primary key (e.g. "ID").
	// The executor reads this field from parent records to populate FK fields
	// on child records.
	PKField string

	// PKFields is the multi-column form of PKField for composite primary keys.
	// When set, its values are used in the given order.
	PKFields []string

	// Defaults returns a new instance of T with default field values.
	// This function is called once per record creation to avoid shared mutable state.
	// Always return a fresh value — do not return a package-level variable.
	//
	// The returned value's dynamic type must equal T. Pointer-typed Defaults
	// (e.g. returning *T from Blueprint[T]) are rejected at registration time
	// because [When] / [Validate] type-assertions assume the value type matches
	// the type parameter T exactly.
	//
	//	Defaults: func() User {
	//	    return User{Name: "test-user", Role: "member"}
	//	}
	Defaults func() T

	// Relations defines the dependencies of this blueprint.
	// Relations can point to parents via BelongsTo or auto-create children via HasMany.
	Relations []Relation

	// Traits defines named option presets that can be applied by name.
	// When a trait is defined here, callers can use BlueprintTrait("name")
	// without re-specifying the options each time.
	//
	//	Traits: map[string][]seedling.Option{
	//	    "admin": {seedling.Set("Role", "admin"), seedling.Set("Active", true)},
	//	}
	Traits map[string][]Option

	// Insert performs the actual database insertion and returns the inserted
	// record with the auto-generated primary key populated.
	// The returned value must have PKField set so that child records can
	// reference it via their FK fields. The callback is responsible for
	// handling the concrete DBTX type it expects.
	//
	//	Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
	//	    return queries.InsertUser(ctx, db.(*sql.DB), v)
	//	}
	Insert func(ctx context.Context, db DBTX, v T) (T, error)

	// Delete performs the actual database deletion for a previously inserted record.
	// This is optional and only required when using [Result.Cleanup] or [Result.CleanupE].
	// The value passed to Delete is the fully populated record as returned by Insert,
	// so primary key fields are available for constructing the DELETE query.
	//
	//	Delete: func(ctx context.Context, db seedling.DBTX, v User) error {
	//	    return queries.DeleteUser(ctx, db.(*sql.DB), v.ID)
	//	}
	Delete func(ctx context.Context, db DBTX, v T) error
}
