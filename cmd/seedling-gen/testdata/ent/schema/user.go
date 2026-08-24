package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("nickname").Optional().Nillable(),
		field.Int("company_uuid").Nillable(),
		field.String("oidc_url"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).
			Ref("users").
			Unique().
			Required().
			Field("company_uuid"),
	}
}
