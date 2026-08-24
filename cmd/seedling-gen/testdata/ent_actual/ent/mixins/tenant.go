// Package mixins contains fixture mixins that are not visible to the schema AST parser.
package mixins

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// Tenant adds a required tenant field to an Ent schema.
type Tenant struct {
	mixin.Schema
}

// Fields declares the mixin fields.
func (Tenant) Fields() []ent.Field {
	return []ent.Field{
		field.String("tenant"),
	}
}
