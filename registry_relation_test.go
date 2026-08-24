package seedling_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mhiro2/seedling"
)

type relationNameModel struct {
	ID int
}

func TestRegister_RelationNamesMustBePresentAndUnique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		relations []seedling.Relation
		wantErr   error
	}{
		{
			name:      "empty",
			relations: []seedling.Relation{{Kind: seedling.BelongsTo, RefBlueprint: "parent"}},
			wantErr:   seedling.ErrInvalidOption,
		},
		{
			name: "duplicate",
			relations: []seedling.Relation{
				{Name: "parent", Kind: seedling.BelongsTo, RefBlueprint: "parent"},
				{Name: "parent", Kind: seedling.BelongsTo, RefBlueprint: "other_parent"},
			},
			wantErr: seedling.ErrInvalidOption,
		},
		{
			name:      "path separator is allowed",
			relations: []seedling.Relation{{Name: "parent.company", Kind: seedling.BelongsTo, RefBlueprint: "company"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := seedling.RegisterTo(seedling.NewRegistry(), seedling.Blueprint[relationNameModel]{
				Name:      "relation_name_model",
				PKField:   "ID",
				Relations: tt.relations,
				Insert: func(context.Context, seedling.DBTX, relationNameModel) (relationNameModel, error) {
					return relationNameModel{}, nil
				},
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}
