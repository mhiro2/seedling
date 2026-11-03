package seedlingpgx_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingpgx"
)

type pgParent struct {
	ID    int64
	ID4   int32
	ID2   int16
	Code  string
	Token [16]byte
}

type pgChild struct {
	ID       int64
	ParentID pgtype.Int8
	Parent4  pgtype.Int4
	Parent2  pgtype.Int2
	Code     pgtype.Text
	Token    pgtype.UUID
}

func TestFKConverters_ConvertParentKeysIntoPgtypeFields(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[pgParent]{
		Name:    "pg_parent",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v pgParent) (pgParent, error) {
			v.ID, v.ID4, v.ID2, v.Code, v.Token = 0, 4, 2, "c", [16]byte{1}
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[pgChild]{
		Name:    "pg_child",
		PKField: "ID",
		Relations: []seedling.Relation{{
			Name:         "parent",
			Kind:         seedling.BelongsTo,
			LocalFields:  []string{"ParentID", "Parent4", "Parent2", "Code", "Token"},
			RefFields:    []string{"ID", "ID4", "ID2", "Code", "Token"},
			RefBlueprint: "pg_parent",
			Convert: map[string]seedling.FKConverter{
				"ParentID": seedlingpgx.Int8FK(),
				"Parent4":  seedlingpgx.Int4FK(),
				"Parent2":  seedlingpgx.Int2FK(),
				"Code":     seedlingpgx.TextFK(),
				"Token":    seedlingpgx.UUIDFK(),
			},
		}},
		Insert: func(_ context.Context, _ seedling.DBTX, v pgChild) (pgChild, error) {
			return v, nil
		},
	})

	// Act
	result, err := seedling.NewSession[pgChild](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	want := pgChild{
		ParentID: pgtype.Int8{Int64: 0, Valid: true},
		Parent4:  pgtype.Int4{Int32: 4, Valid: true},
		Parent2:  pgtype.Int2{Int16: 2, Valid: true},
		Code:     pgtype.Text{String: "c", Valid: true},
		Token:    pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
	}
	if got := result.Root(); got != want {
		t.Fatalf("child = %+v, want %+v", got, want)
	}
}
