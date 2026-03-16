package errx

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestBlueprintNotFound(t *testing.T) {
	// Act
	err := BlueprintNotFound("users")

	// Assert
	if !errors.Is(err, ErrBlueprintNotFound) {
		t.Fatalf("got %v, want %v", err, ErrBlueprintNotFound)
	}
	if !strings.Contains(err.Error(), "users") {
		t.Errorf("expected error to contain %q, got %v", "users", err)
	}
}

func TestRelationNotFound(t *testing.T) {
	// Act
	err := RelationNotFound("posts", "author")

	// Assert
	if !errors.Is(err, ErrRelationNotFound) {
		t.Fatalf("got %v, want %v", err, ErrRelationNotFound)
	}
	msg := err.Error()
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error to contain %q, got %v", "author", msg)
	}
	if !strings.Contains(msg, "posts") {
		t.Errorf("expected error to contain %q, got %v", "posts", msg)
	}
}

func TestFieldNotFound(t *testing.T) {
	// Act
	err := FieldNotFound("User", "email")

	// Assert
	if !errors.Is(err, ErrFieldNotFound) {
		t.Fatalf("got %v, want %v", err, ErrFieldNotFound)
	}
	msg := err.Error()
	if !strings.Contains(msg, "email") {
		t.Errorf("expected error to contain %q, got %v", "email", msg)
	}
	if !strings.Contains(msg, "User") {
		t.Errorf("expected error to contain %q, got %v", "User", msg)
	}
}

func TestTypeMismatch(t *testing.T) {
	// Act
	err := TypeMismatch("age", "int", "string")

	// Assert
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, ErrTypeMismatch)
	}
	msg := err.Error()
	if !strings.Contains(msg, "age") {
		t.Errorf("expected error to contain %q, got %v", "age", msg)
	}
	if !strings.Contains(msg, "int") {
		t.Errorf("expected error to contain %q, got %v", "int", msg)
	}
	if !strings.Contains(msg, "string") {
		t.Errorf("expected error to contain %q, got %v", "string", msg)
	}
}

func TestInsertFailed(t *testing.T) {
	// Arrange
	cause := fmt.Errorf("connection refused")

	// Act
	err := InsertFailed("users", cause)

	// Assert
	if !errors.Is(err, ErrInsertFailed) {
		t.Fatalf("got %v, want %v", err, ErrInsertFailed)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("got %v, want %v", err, cause)
	}
	msg := err.Error()
	if !strings.Contains(msg, "users") {
		t.Errorf("expected error to contain %q, got %v", "users", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("expected error to contain %q, got %v", "connection refused", msg)
	}

	var ife *InsertFailedError
	if !errors.As(err, &ife) {
		t.Fatal("errors.As should match *InsertFailedError")
	}
	if ife.Blueprint() != "users" {
		t.Fatalf("got %q, want %q", ife.Blueprint(), "users")
	}
}

func TestDeleteFailed(t *testing.T) {
	// Arrange
	cause := fmt.Errorf("permission denied")

	// Act
	err := DeleteFailed("posts", cause)

	// Assert
	if !errors.Is(err, ErrDeleteFailed) {
		t.Fatalf("got %v, want %v", err, ErrDeleteFailed)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("got %v, want %v", err, cause)
	}
	msg := err.Error()
	if !strings.Contains(msg, "posts") {
		t.Errorf("expected error to contain %q, got %v", "posts", msg)
	}
	if !strings.Contains(msg, "permission denied") {
		t.Errorf("expected error to contain %q, got %v", "permission denied", msg)
	}

	var dfe *DeleteFailedError
	if !errors.As(err, &dfe) {
		t.Fatal("errors.As should match *DeleteFailedError")
	}
	if dfe.Blueprint() != "posts" {
		t.Fatalf("got %q, want %q", dfe.Blueprint(), "posts")
	}
}

func TestDuplicateBlueprint(t *testing.T) {
	// Act
	err := DuplicateBlueprint("users")

	// Assert
	if !errors.Is(err, ErrDuplicateBlueprint) {
		t.Fatalf("got %v, want %v", err, ErrDuplicateBlueprint)
	}
	if !strings.Contains(err.Error(), "users") {
		t.Errorf("expected error to contain %q, got %v", "users", err)
	}
}

func TestCycleDetected(t *testing.T) {
	// Act
	err := CycleDetected([]string{"a", "b"})

	// Assert
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("got %v, want %v", err, ErrCycleDetected)
	}
	msg := err.Error()
	if !strings.Contains(msg, "a") {
		t.Errorf("expected error to contain %q, got %v", "a", msg)
	}
	if !strings.Contains(msg, "b") {
		t.Errorf("expected error to contain %q, got %v", "b", msg)
	}
}

func TestFieldNotFoundWithHint(t *testing.T) {
	// Act
	err := FieldNotFoundWithHint("Task", "NonExistent", []string{"ID", "Name", "Title"})

	// Assert
	if !errors.Is(err, ErrFieldNotFound) {
		t.Fatalf("got %v, want %v", err, ErrFieldNotFound)
	}
	msg := err.Error()
	if !strings.Contains(msg, "available fields") {
		t.Errorf("expected error to contain %q, got %v", "available fields", msg)
	}
	if !strings.Contains(msg, "ID") {
		t.Errorf("expected error to contain %q, got %v", "ID", msg)
	}
	if !strings.Contains(msg, "Name") {
		t.Errorf("expected error to contain %q, got %v", "Name", msg)
	}
	if !strings.Contains(msg, "Title") {
		t.Errorf("expected error to contain %q, got %v", "Title", msg)
	}
}

func TestRelationNotFoundWithHint(t *testing.T) {
	// Act
	err := RelationNotFoundWithHint("task", "nonexistent", []string{"project", "assignee"})

	// Assert
	if !errors.Is(err, ErrRelationNotFound) {
		t.Fatalf("got %v, want %v", err, ErrRelationNotFound)
	}
	msg := err.Error()
	if !strings.Contains(msg, "available relations") {
		t.Errorf("expected error to contain %q, got %v", "available relations", msg)
	}
	if !strings.Contains(msg, "project") {
		t.Errorf("expected error to contain %q, got %v", "project", msg)
	}
	if !strings.Contains(msg, "assignee") {
		t.Errorf("expected error to contain %q, got %v", "assignee", msg)
	}
}

func TestDeleteNotDefined(t *testing.T) {
	// Act
	err := DeleteNotDefined("users")

	// Assert
	if !errors.Is(err, ErrDeleteNotDefined) {
		t.Fatalf("got %v, want %v", err, ErrDeleteNotDefined)
	}
	msg := err.Error()
	if !strings.Contains(msg, "users") {
		t.Errorf("expected error to contain %q, got %v", "users", msg)
	}
	if !strings.Contains(msg, "Delete") {
		t.Errorf("expected error to mention Delete function, got %v", msg)
	}
}

func TestUseAndRefConflict(t *testing.T) {
	// Act
	err := UseAndRefConflict("posts", "author")

	// Assert
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, ErrInvalidOption)
	}
	msg := err.Error()
	if !strings.Contains(msg, "posts") {
		t.Errorf("expected error to contain %q, got %v", "posts", msg)
	}
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error to contain %q, got %v", "author", msg)
	}
	if !strings.Contains(msg, "Use") || !strings.Contains(msg, "Ref") {
		t.Errorf("expected error to mention Use and Ref, got %v", msg)
	}
}

func TestOmitRequiredRelation(t *testing.T) {
	// Act
	err := OmitRequiredRelation("posts", "author")

	// Assert
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, ErrInvalidOption)
	}
	msg := err.Error()
	if !strings.Contains(msg, "posts") {
		t.Errorf("expected error to contain %q, got %v", "posts", msg)
	}
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error to contain %q, got %v", "author", msg)
	}
}

func TestSetOnFKField(t *testing.T) {
	// Act
	err := SetOnFKField("posts", "AuthorID", "author")

	// Assert
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, ErrInvalidOption)
	}
	msg := err.Error()
	if !strings.Contains(msg, "posts") {
		t.Errorf("expected error to contain %q, got %v", "posts", msg)
	}
	if !strings.Contains(msg, "AuthorID") {
		t.Errorf("expected error to contain %q, got %v", "AuthorID", msg)
	}
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error to contain %q, got %v", "author", msg)
	}
}

func TestUseOnNonBelongsTo(t *testing.T) {
	// Act
	err := UseOnNonBelongsTo("users", "posts", "has_many")

	// Assert
	if !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, ErrInvalidOption)
	}
	msg := err.Error()
	if !strings.Contains(msg, "users") {
		t.Errorf("expected error to contain %q, got %v", "users", msg)
	}
	if !strings.Contains(msg, "posts") {
		t.Errorf("expected error to contain %q, got %v", "posts", msg)
	}
	if !strings.Contains(msg, "has_many") {
		t.Errorf("expected error to contain %q, got %v", "has_many", msg)
	}
}

func TestUseTypeMismatch(t *testing.T) {
	// Act
	err := UseTypeMismatch("author", "User", "Post")

	// Assert
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, ErrTypeMismatch)
	}
	msg := err.Error()
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error to contain %q, got %v", "author", msg)
	}
	if !strings.Contains(msg, "User") {
		t.Errorf("expected error to contain %q, got %v", "User", msg)
	}
	if !strings.Contains(msg, "Post") {
		t.Errorf("expected error to contain %q, got %v", "Post", msg)
	}
}

func TestSentinelsAreDistinguishable(t *testing.T) {
	// Arrange
	tests := []struct {
		name     string
		err      error
		sentinel error
		others   []error
	}{
		{
			name:     "BlueprintNotFound",
			err:      BlueprintNotFound("x"),
			sentinel: ErrBlueprintNotFound,
			others:   []error{ErrRelationNotFound, ErrFieldNotFound, ErrTypeMismatch, ErrInsertFailed, ErrDuplicateBlueprint, ErrCycleDetected, ErrInvalidOption},
		},
		{
			name:     "RelationNotFound",
			err:      RelationNotFound("x", "y"),
			sentinel: ErrRelationNotFound,
			others:   []error{ErrBlueprintNotFound, ErrFieldNotFound, ErrTypeMismatch, ErrInsertFailed, ErrDuplicateBlueprint, ErrCycleDetected, ErrInvalidOption},
		},
		{
			name:     "FieldNotFound",
			err:      FieldNotFound("x", "y"),
			sentinel: ErrFieldNotFound,
			others:   []error{ErrBlueprintNotFound, ErrRelationNotFound, ErrTypeMismatch, ErrInsertFailed, ErrDuplicateBlueprint, ErrCycleDetected, ErrInvalidOption},
		},
		{
			name:     "TypeMismatch",
			err:      TypeMismatch("x", "y", "z"),
			sentinel: ErrTypeMismatch,
			others:   []error{ErrBlueprintNotFound, ErrRelationNotFound, ErrFieldNotFound, ErrInsertFailed, ErrDuplicateBlueprint, ErrCycleDetected, ErrInvalidOption},
		},
		{
			name:     "InsertFailed",
			err:      InsertFailed("x", fmt.Errorf("cause")),
			sentinel: ErrInsertFailed,
			others:   []error{ErrBlueprintNotFound, ErrRelationNotFound, ErrFieldNotFound, ErrTypeMismatch, ErrDuplicateBlueprint, ErrCycleDetected, ErrInvalidOption},
		},
		{
			name:     "DuplicateBlueprint",
			err:      DuplicateBlueprint("x"),
			sentinel: ErrDuplicateBlueprint,
			others:   []error{ErrBlueprintNotFound, ErrRelationNotFound, ErrFieldNotFound, ErrTypeMismatch, ErrInsertFailed, ErrCycleDetected, ErrInvalidOption},
		},
		{
			name:     "CycleDetected",
			err:      CycleDetected([]string{"a"}),
			sentinel: ErrCycleDetected,
			others:   []error{ErrBlueprintNotFound, ErrRelationNotFound, ErrFieldNotFound, ErrTypeMismatch, ErrInsertFailed, ErrDuplicateBlueprint, ErrInvalidOption},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act & Assert
			if !errors.Is(tt.err, tt.sentinel) {
				t.Fatalf("got %v, want %v", tt.err, tt.sentinel)
			}
			for _, other := range tt.others {
				if errors.Is(tt.err, other) {
					t.Errorf("expected %v not to match %v", tt.err, other)
				}
			}
		})
	}
}
