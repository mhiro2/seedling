package seedling

// InsertLog holds information about a single insert operation performed by
// the executor. Use [WithInsertLog] to receive these entries during execution.
type InsertLog struct {
	// Step is the 1-based position in the topological execution order.
	Step int

	// Blueprint is the blueprint name of the inserted record.
	Blueprint string

	// Table is the database table name.
	Table string

	// Provided is true when the record was supplied via [Use] (no INSERT executed).
	Provided bool

	// FKBindings describes the FK fields that were populated from parent PKs
	// before this record was inserted.
	FKBindings []FKBinding
}

// FKBinding describes a single FK assignment made before an insert.
type FKBinding struct {
	// ChildField is the FK field on the inserted record.
	ChildField string

	// ParentBlueprint is the parent blueprint name.
	ParentBlueprint string

	// ParentTable is the parent table name.
	ParentTable string

	// ParentField is the parent PK field name.
	ParentField string

	// Value is the PK value assigned to the FK field.
	Value any
}

// DBTX is an opaque database handle passed through to Blueprint Insert functions.
// seedling does not execute SQL directly; it delegates all database operations
// to user-provided Insert functions which can accept *sql.DB, *sql.Tx, pgx.Tx, etc.
// Callers and Insert callbacks must agree on the concrete handle type. seedling
// does not validate or convert db before invoking the callback.
type DBTX any

// RelationKind describes the type of relationship between blueprints.
type RelationKind string

const (
	// BelongsTo indicates a foreign key relationship where the child holds
	// a reference to the parent's primary key.
	BelongsTo RelationKind = "belongs_to"

	// HasMany indicates a one-to-many relationship where creating the parent
	// automatically creates N child records. The LocalField is the FK field
	// on the child blueprint that points back to the parent.
	HasMany RelationKind = "has_many"

	// ManyToMany indicates a relationship where creating the parent
	// automatically creates related records plus the join-table rows that link
	// them together. ThroughBlueprint identifies the join-table blueprint.
	ManyToMany RelationKind = "many_to_many"
)
