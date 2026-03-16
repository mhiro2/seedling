package seedling

import (
	"context"
	"testing"
)

type extractContextKey struct{}

func TestExtractContext_FiltersWithContextOption(t *testing.T) {
	// Arrange
	ctx := context.WithValue(t.Context(), extractContextKey{}, "value")
	opts := []Option{
		Set("Name", "company"),
		WithContext(ctx),
		Omit("company"),
	}

	// Act
	gotCtx, filtered := extractContext(t.Context(), opts)

	// Assert
	if gotCtx != ctx {
		t.Fatalf("got %v, want %v", gotCtx, ctx)
	}
	if len(filtered) != 2 {
		t.Fatalf("got len %d, want %d", len(filtered), 2)
	}
	for _, opt := range filtered {
		_, ok := opt.(contextOption)
		if ok {
			t.Fatal("expected false")
		}
	}
}
