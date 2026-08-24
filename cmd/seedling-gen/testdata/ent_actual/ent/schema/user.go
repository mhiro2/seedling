package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/mhiro2/seedling/cmd/seedling-gen/testdata/ent_actual/ent/mixins"
)

// User exercises nillable fields and an edge with an explicit FK field.
type User struct {
	ent.Schema
}

// Mixin adds a required field that is visible only in generated output.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.Tenant{},
	}
}

// Fields declares the user fields.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("nickname").Optional().Nillable(),
		field.Int("company_uuid").Nillable(),
		field.String("oidc_url"),
	}
}

// Edges declares a required company edge backed by company_uuid.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).
			Ref("users").
			Unique().
			Required().
			Field("company_uuid"),
	}
}
