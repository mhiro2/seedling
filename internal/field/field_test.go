package field_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/field"
)

type sample struct {
	ID   int
	Name string
}

func TestSetField_SetsExportedField(t *testing.T) {
	// Arrange
	var s sample

	// Act
	err := field.SetField(&s, "Name", "hello")
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "hello" {
		t.Fatalf("got %v, want %v", s.Name, "hello")
	}
}

func TestSetField_ErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		value     any
		wantErr   error
		checkErr  func(t *testing.T, err error)
	}{
		{
			name:      "field not found",
			fieldName: "Missing",
			value:     "x",
			wantErr:   errx.ErrFieldNotFound,
			checkErr: func(t *testing.T, err error) {
				t.Helper()
				msg := err.Error()
				if !strings.Contains(msg, "ID") {
					t.Fatalf("expected error to contain %q, got %v", "ID", msg)
				}
				if !strings.Contains(msg, "Name") {
					t.Fatalf("expected error to contain %q, got %v", "Name", msg)
				}
				if !strings.Contains(msg, "available fields") {
					t.Fatalf("expected error to contain %q, got %v", "available fields", msg)
				}
			},
		},
		{
			name:      "type mismatch",
			fieldName: "ID",
			value:     "string-value",
			wantErr:   errx.ErrTypeMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var s sample

			// Act
			err := field.SetField(&s, tt.fieldName, tt.value)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
			if tt.checkErr != nil {
				tt.checkErr(t, err)
			}
		})
	}
}

func TestGetField_SupportsStructAndPointer(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		fieldName string
		want      any
	}{
		{name: "struct value", input: sample{ID: 42, Name: "test"}, fieldName: "ID", want: 42},
		{name: "pointer value", input: &sample{Name: "ptr"}, fieldName: "Name", want: "ptr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := field.GetField(tt.input, tt.fieldName)
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetField_FieldNotFound(t *testing.T) {
	// Act
	_, err := field.GetField(sample{}, "Missing")

	// Assert
	if !errors.Is(err, errx.ErrFieldNotFound) {
		t.Fatalf("got %v, want %v", err, errx.ErrFieldNotFound)
	}
	msg := err.Error()
	if !strings.Contains(msg, "ID") {
		t.Fatalf("expected error to contain %q, got %v", "ID", msg)
	}
	if !strings.Contains(msg, "Name") {
		t.Fatalf("expected error to contain %q, got %v", "Name", msg)
	}
	if !strings.Contains(msg, "available fields") {
		t.Fatalf("expected error to contain %q, got %v", "available fields", msg)
	}
}

func TestExists_ReturnsExpectedResult(t *testing.T) {
	// Arrange
	s := sample{}
	tests := []struct {
		name      string
		input     any
		fieldName string
		want      bool
	}{
		{name: "struct field exists", input: s, fieldName: "ID", want: true},
		{name: "missing field", input: s, fieldName: "Missing", want: false},
		{name: "pointer field exists", input: &s, fieldName: "Name", want: true},
		{name: "nil input", input: nil, fieldName: "Name", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act & Assert
			if got := field.Exists(tt.input, tt.fieldName); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
