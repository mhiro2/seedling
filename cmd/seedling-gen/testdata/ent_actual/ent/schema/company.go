package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Company owns users in the generated-code fixture.
type Company struct {
	ent.Schema
}

// Fields declares the company fields.
func (Company) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

// Edges declares the inverse user edge.
func (Company) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
	}
}
