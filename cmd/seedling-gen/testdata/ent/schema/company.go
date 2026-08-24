package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Company struct {
	ent.Schema
}

func (Company) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (Company) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
	}
}
