package seedling_test

import (
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func FuzzSet_InvalidFieldName(f *testing.F) {
	// Seed corpus
	f.Add("")
	f.Add("NonExistent")
	f.Add("id")
	f.Add("ID\x00")
	f.Add("Name\nInjection")
	f.Add(strings.Repeat("A", 10000))

	f.Fuzz(func(t *testing.T, fieldName string) {
		// Arrange
		reg := seedlingtest.NewRegistry()
		seedlingtest.RegisterBasic(t, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

		// Act & Assert -- must not panic
		_, _ = seedling.NewSession[Company](reg).BuildE(seedling.Set(fieldName, "value"))
	})
}

func FuzzUse_InvalidRelationName(f *testing.F) {
	// Seed corpus
	f.Add("")
	f.Add("nonexistent")
	f.Add("company\x00")
	f.Add(strings.Repeat("X", 10000))

	f.Fuzz(func(t *testing.T, relName string) {
		// Arrange
		reg := seedlingtest.NewRegistry()
		seedlingtest.RegisterBasic(t, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

		// Act & Assert -- must not panic
		_, _ = seedling.NewSession[Task](reg).BuildE(
			seedling.Use(relName, Company{ID: 1, Name: "test"}),
		)
	})
}
