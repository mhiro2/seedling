package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// TimeMixin is defined next to the schemas, as the Ent documentation does.
// It is not a schema, although it has a Fields method.
type TimeMixin struct {
	mixin.Schema
}

// Fields declares the mixin fields.
func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").Default(time.Now),
	}
}
