package field_test

import (
	"strings"
	"testing"

	"github.com/mhiro2/seedling/internal/field"
)

type setFieldSample struct {
	ID    int
	Name  string
	Tags  []string
	Meta  any
	Alias *string
}

type lookupFieldSample struct {
	ID   int
	Name string
}

func FuzzSetField(f *testing.F) {
	// Seed corpus
	f.Add("", uint8(0), "")
	f.Add("ID", uint8(1), "7")
	f.Add("Name", uint8(2), "alice")
	f.Add("Tags", uint8(3), "tag")
	f.Add("Meta", uint8(0), "")
	f.Add("Alias", uint8(4), "nick")
	f.Add("private", uint8(2), "hidden")
	f.Add("Missing", uint8(2), "value")
	f.Add("ID\x00", uint8(2), "value")
	f.Add(strings.Repeat("A", 10000), uint8(2), strings.Repeat("B", 10000))

	f.Fuzz(func(t *testing.T, fieldName string, mode uint8, payload string) {
		// Arrange
		var sample setFieldSample

		var value any
		switch mode % 5 {
		case 0:
			value = nil
		case 1:
			value = len(payload)
		case 2:
			value = payload
		case 3:
			value = []string{payload}
		case 4:
			value = &payload
		}

		// Act & Assert
		_ = field.SetField(&sample, fieldName, value)
	})
}

func FuzzLookupField(f *testing.F) {
	// Seed corpus
	f.Add("", false, false, false)
	f.Add("ID", false, false, false)
	f.Add("Name", true, false, false)
	f.Add("Missing", false, false, false)
	f.Add("Name\x00", true, false, false)
	f.Add(strings.Repeat("X", 10000), false, false, false)
	f.Add("ID", false, true, false)
	f.Add("Name", false, false, true)

	f.Fuzz(func(t *testing.T, fieldName string, usePointer, useNil, useTypedNil bool) {
		// Arrange
		sample := lookupFieldSample{ID: 1, Name: "lookup"}
		var input any = sample

		if usePointer {
			input = &sample
		}
		if useNil {
			input = nil
		}
		if useTypedNil {
			var ptr *lookupFieldSample
			input = ptr
		}

		// Act & Assert
		_ = field.Exists(input, fieldName)
		_, _ = field.GetField(input, fieldName)
	})
}
