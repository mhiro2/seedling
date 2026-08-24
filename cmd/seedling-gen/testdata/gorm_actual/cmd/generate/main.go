package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open("file:seedling_gorm_gen?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		panic(fmt.Errorf("open fixture database: %w", err))
	}
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		panic(fmt.Errorf("read fixture schema: %w", err))
	}
	for statement := range strings.SplitSeq(string(schema), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if err := db.Exec(statement).Error; err != nil {
			panic(fmt.Errorf("apply fixture schema: %w", err))
		}
	}

	generator := gen.NewGenerator(gen.Config{
		OutPath:           "./query",
		ModelPkgPath:      "./model",
		FieldNullable:     true,
		FieldSignable:     true,
		FieldWithIndexTag: true,
		Mode:              gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	generator.UseDB(db)

	country := generator.GenerateModel("countries")
	user := generator.GenerateModel(
		"users",
		gen.FieldRelate(field.BelongsTo, "Country", country, &field.RelateConfig{
			GORMTag: field.GormTag{
				"foreignKey": []string{"CountryCode"},
				"references": []string{"Code"},
			},
		}),
	)
	membership := generator.GenerateModel(
		"memberships",
		gen.FieldGORMTag("user_id", func(tag field.GormTag) field.GormTag {
			return tag.Set(field.TagKeyGormPrimaryKey)
		}),
	)
	generator.ApplyBasic(country, user, membership)
	generator.Execute()
}
