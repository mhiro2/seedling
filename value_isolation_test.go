package seedling_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/mhiro2/seedling"
)

type isolatedRecord struct {
	ID    int
	Tags  []string
	Attrs map[string]string
	Score *int
}

func TestInsertMany_SetValuesAreIndependentAcrossRecords(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	nextID := 0
	seedling.MustRegisterTo(reg, seedling.Blueprint[isolatedRecord]{
		Name:    "isolated_record",
		Table:   "isolated_records",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v isolatedRecord) (isolatedRecord, error) {
			nextID++
			v.ID = nextID
			// Each insert mutates the reference values it was handed.
			v.Tags[0] = "inserted-" + strconv.Itoa(nextID)
			v.Attrs["by"] = strconv.Itoa(nextID)
			*v.Score = nextID * 10
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	tags := []string{"original"}
	attrs := map[string]string{"by": "caller"}
	score := 7

	// Act
	records := insertMany[isolatedRecord](t, nil, 2,
		seedling.Set("Tags", tags),
		seedling.Set("Attrs", attrs),
		seedling.Set("Score", &score),
	)

	// Assert
	if tags[0] != "original" || attrs["by"] != "caller" || score != 7 {
		t.Fatalf("caller values changed: tags=%v attrs=%v score=%d", tags, attrs, score)
	}
	if got := records[0].Tags[0]; got != "inserted-1" {
		t.Fatalf("records[0].Tags[0] = %q, want %q", got, "inserted-1")
	}
	if got := records[0].Attrs["by"]; got != "1" {
		t.Fatalf("records[0].Attrs[by] = %q, want %q", got, "1")
	}
	if got := *records[0].Score; got != 10 {
		t.Fatalf("*records[0].Score = %d, want %d", got, 10)
	}
	if got := records[1].Tags[0]; got != "inserted-2" {
		t.Fatalf("records[1].Tags[0] = %q, want %q", got, "inserted-2")
	}
	if records[0].Score == records[1].Score {
		t.Fatal("records share the same Score pointer")
	}
}
