package main

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestParseGormDir_BasicModel(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

import "gorm.io/gorm"

type Company struct {
	gorm.Model
	Name string
}

type User struct {
	gorm.Model
	Name      string
	CompanyID uint
	Company   Company `+"`"+`gorm:"foreignKey:CompanyID"`+"`"+`
}
`)

	// Act
	models, err := ParseGormDir(dir)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(models) < 2 {
		t.Fatalf("expected at least 2 models, got %d", len(models))
	}

	var company, user *GormModel
	for i, m := range models {
		switch m.Name {
		case "Company":
			company = &models[i]
		case "User":
			user = &models[i]
		}
	}

	if company == nil {
		t.Fatal("Company model not found")
	}
	// gorm.Model adds ID, CreatedAt, UpdatedAt, DeletedAt + Name = 5 fields.
	if len(company.Fields) != 5 {
		t.Fatalf("expected 5 fields on Company, got %d", len(company.Fields))
	}

	if user == nil {
		t.Fatal("User model not found")
	}

	// Check that Company relation is detected on User.
	var hasCompanyRelation bool
	for _, f := range user.Fields {
		if f.Relation != nil && f.Relation.RefModel == "Company" {
			hasCompanyRelation = true
			break
		}
	}
	if !hasCompanyRelation {
		t.Fatal("expected Company relation on User")
	}
}

func TestParseGormDir_GormModelEmbed(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

import "gorm.io/gorm"

type Item struct {
	gorm.Model
	Label string
}
`)

	// Act
	models, err := ParseGormDir(dir)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	// Check ID field from gorm.Model embed.
	var hasID bool
	for _, f := range models[0].Fields {
		if f.Name == "ID" && f.IsPK {
			hasID = true
			break
		}
	}
	if !hasID {
		t.Fatal("expected ID primary key field from gorm.Model embed")
	}
}

func TestParseGormDir_TableNameMethod(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type Post struct {
	ID    uint
	Title string
}

func (Post) TableName() string {
	return "blog_posts"
}
`)

	// Act
	models, err := ParseGormDir(dir)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].Table != "blog_posts" {
		t.Fatalf("expected table %q, got %q", "blog_posts", models[0].Table)
	}
}

func TestParseGormDir_EmptyDir(t *testing.T) {
	// Act & Assert
	dir := t.TempDir()
	_, err := ParseGormDir(dir)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestParseGormDir_PrimaryKeyTag(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type Region struct {
	Code   string `+"`"+`gorm:"primaryKey"`+"`"+`
	Number int    `+"`"+`gorm:"primaryKey"`+"`"+`
	Name   string
}
`)

	// Act
	models, err := ParseGormDir(dir)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	var pkCount int
	for _, f := range models[0].Fields {
		if f.IsPK {
			pkCount++
		}
	}
	if pkCount != 2 {
		t.Fatalf("expected 2 primary key fields, got %d", pkCount)
	}
}

func TestGenerateGorm_BasicOutput(t *testing.T) {
	// Arrange
	models := []GormModel{
		{
			Name:  "Company",
			Table: "companies",
			Fields: []GormField{
				{Name: "ID", Type: "uint", IsPK: true},
				{Name: "Name", Type: "string"},
			},
		},
		{
			Name:  "User",
			Table: "users",
			Fields: []GormField{
				{Name: "ID", Type: "uint", IsPK: true},
				{Name: "Name", Type: "string"},
				{Name: "CompanyID", Type: "uint"},
				{Name: "Company", Type: "Company", Relation: &GormRelation{
					Kind: "BelongsTo", ForeignKey: "CompanyID", RefModel: "Company",
				}},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models)
	// Assert
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	checks := []struct {
		name   string
		substr string
	}{
		{"package", "package testutil"},
		{"seedling import", `"github.com/mhiro2/seedling"`},
		{"gorm import", `"gorm.io/gorm"`},
		{"models import", `models "github.com/myapp/models"`},
		{"company blueprint", "models.Company"},
		{"user blueprint", "models.User"},
		{"gorm create", "gorm.DB).WithContext(ctx).Create(&v)"},
		{"gorm delete", "gorm.DB).WithContext(ctx).Delete(&v)"},
		{"belongs to", "seedling.BelongsTo"},
		{"local field", `LocalField: "CompanyID"`},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(output, check.substr) {
				t.Fatalf("expected output to contain %q\n\nGot:\n%s", check.substr, output)
			}
		})
	}
}

func TestGenerateGorm_CompositePK(t *testing.T) {
	// Arrange
	models := []GormModel{
		{
			Name:  "Membership",
			Table: "memberships",
			Fields: []GormField{
				{Name: "CompanyID", Type: "uint", IsPK: true},
				{Name: "UserID", Type: "uint", IsPK: true},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `PKFields: []string{"CompanyID", "UserID"}`) {
		t.Fatalf("expected composite PKFields, got:\n%s", buf.String())
	}
}

func TestGenerateGorm_UsesForeignKeyNullabilityAndReferences(t *testing.T) {
	models := []GormModel{
		{
			Name:  "Country",
			Table: "countries",
			Fields: []GormField{
				{Name: "ID", Type: "uint", IsPK: true},
				{Name: "Code", Type: "string", NotNull: true},
			},
		},
		{
			Name:  "City",
			Table: "cities",
			Fields: []GormField{
				{Name: "ID", Type: "uint", IsPK: true},
				{Name: "CountryCode", Type: "string", NotNull: true},
				{Name: "Country", Type: "Country", NotNull: false, Relation: &GormRelation{
					Kind: "BelongsTo", ForeignKey: "CountryCode", References: "Code", RefModel: "Country",
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, `LocalField: "CountryCode", RefField: "Code"`) {
		t.Fatalf("expected non-PK reference mapping, got:\n%s", output)
	}
	if strings.Contains(output, `RefBlueprint: "country", Optional: true`) {
		t.Fatalf("relation should be required from the not-null FK scalar, got:\n%s", output)
	}
}

func TestParseGormDir_RetainsReferencesTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type Country struct {
	ID uint
	Code string
}

type City struct {
	ID uint
	CountryCode string `+"`"+`gorm:"not null"`+"`"+`
	Country Country `+"`"+`gorm:"foreignKey:CountryCode;references:Code"`+"`"+`
}
`)

	models, err := ParseGormDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		if model.Name != "City" {
			continue
		}
		for _, field := range model.Fields {
			if field.Name == "Country" && field.Relation != nil {
				if field.Relation.References != "Code" {
					t.Fatalf("references = %q, want Code", field.Relation.References)
				}
				return
			}
		}
	}
	t.Fatal("Country relation not found")
}

func TestGenerateGorm_DefaultsAutofillSupportedFields(t *testing.T) {
	// Arrange
	models := []GormModel{
		{
			Name:  "Company",
			Table: "companies",
			Fields: []GormField{
				{Name: "ID", Type: "uint", IsPK: true},
				{Name: "Name", Type: "string"},
			},
		},
		{
			Name:  "User",
			Table: "users",
			Fields: []GormField{
				{Name: "ID", Type: "uint", IsPK: true},
				{Name: "Name", Type: "string"},
				{Name: "CreatedAt", Type: "time.Time"},
				{Name: "CompanyID", Type: "uint"},
				{Name: "Company", Type: "Company", Relation: &GormRelation{
					Kind: "BelongsTo", ForeignKey: "CompanyID", RefModel: "Company",
				}},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	if err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models); err != nil {
		t.Fatal(err)
	}

	// Assert
	output := buf.String()
	tests := []struct {
		name    string
		substr  string
		missing bool
	}{
		{name: "time import", substr: `"time"`},
		{name: "string default", substr: `Name: "user-name"`},
		{name: "time default", substr: `CreatedAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)`},
		{name: "relation key skipped", substr: `CompanyID: 1`, missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contains := strings.Contains(output, tt.substr)
			if tt.missing && contains {
				t.Fatalf("expected output not to contain %q\n\nGot:\n%s", tt.substr, output)
			}
			if !tt.missing && !contains {
				t.Fatalf("expected output to contain %q\n\nGot:\n%s", tt.substr, output)
			}
		})
	}
}

func TestGenerateGorm_InferredPrimaryKeyHasNoDefault(t *testing.T) {
	// Arrange: no field carries a primaryKey tag, so GORM treats ID as the key.
	models := []GormModel{
		{
			Name:  "User",
			Table: "users",
			Fields: []GormField{
				{Name: "ID", Type: "uint"},
				{Name: "Name", Type: "string"},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	if err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models); err != nil {
		t.Fatal(err)
	}

	// Assert
	output := buf.String()
	if !strings.Contains(output, `PKField: "ID"`) {
		t.Fatalf("expected inferred ID primary key\n\nGot:\n%s", output)
	}
	if !strings.Contains(output, `return models.User{Name: "user-name"}`) {
		t.Fatalf("expected defaults to leave the inferred primary key unset\n\nGot:\n%s", output)
	}
}

func TestGenerateGorm_NestsGormModelDefaults(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

import "gorm.io/gorm"

type Item struct {
	gorm.Model
	Label string
}
`)
	models, err := ParseGormDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	var buf bytes.Buffer
	if err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models); err != nil {
		t.Fatal(err)
	}

	// Assert
	output := buf.String()
	want := `return models.Item{Model: gorm.Model{CreatedAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)}, Label: "item-label"}`
	if !strings.Contains(output, want) {
		t.Fatalf("expected promoted defaults under gorm.Model\n\nGot:\n%s", output)
	}
}

func TestGenerateGorm_OmitsTimeImportForTimeKeysOnly(t *testing.T) {
	// Arrange: the only time.Time field is a primary key, which never receives
	// a generated default.
	models := []GormModel{
		{
			Name:  "Snapshot",
			Table: "snapshots",
			Fields: []GormField{
				{Name: "TakenAt", Type: "time.Time", IsPK: true},
				{Name: "Label", Type: "string"},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	if err := GenerateGorm(&buf, "testutil", "github.com/myapp/models", models); err != nil {
		t.Fatal(err)
	}

	// Assert
	if output := buf.String(); strings.Contains(output, `"time"`) {
		t.Fatalf("expected no time import\n\nGot:\n%s", output)
	}
}

func TestParseGormDir_DefaultNamesFollowGormNamingStrategy(t *testing.T) {
	// Arrange: without TableName methods or column tags, GORM's default
	// NamingStrategy keeps initialisms together and pluralizes table names
	// with English inflection rules.
	dir := t.TempDir()
	writeFile(t, dir, "models.go", "package models\n\n"+
		"type APIKey struct {\n"+
		"\tID         uint\n"+
		"\tCompanyID  uint\n"+
		"\tHTTPServer string\n"+
		"\tUTF8Name   string\n"+
		"\tLabel      string `gorm:\"column:display_label\"`\n"+
		"}\n\n"+
		"type Company struct{ ID uint }\n\n"+
		"type Person struct{ ID uint }\n\n"+
		"type OrderItem struct{ ID uint }\n")

	// Act
	models, err := ParseGormDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Assert: the expected names are what gorm.io/gorm/schema.NamingStrategy{}
	// returns from TableName and ColumnName for the same Go names.
	tables := make(map[string]string, len(models))
	for _, model := range models {
		tables[model.Name] = model.Table
	}
	wantTables := map[string]string{
		"APIKey":    "api_keys",
		"Company":   "companies",
		"Person":    "people",
		"OrderItem": "order_items",
	}
	if !maps.Equal(tables, wantTables) {
		t.Fatalf("tables = %v, want %v", tables, wantTables)
	}

	apiKey := models[slices.IndexFunc(models, func(m GormModel) bool { return m.Name == "APIKey" })]
	columns := make(map[string]string, len(apiKey.Fields))
	for _, field := range apiKey.Fields {
		columns[field.Name] = field.ColumnName
	}
	wantColumns := map[string]string{
		"ID":         "id",
		"CompanyID":  "company_id",
		"HTTPServer": "http_server",
		"UTF8Name":   "utf8_name",
		"Label":      "display_label",
	}
	if !maps.Equal(columns, wantColumns) {
		t.Fatalf("columns = %v, want %v", columns, wantColumns)
	}
}

func TestParseGormTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want map[string]string
	}{
		{
			name: "primary key",
			tag:  "primaryKey",
			want: map[string]string{"primaryKey": ""},
		},
		{
			name: "column override",
			tag:  "column:user_name",
			want: map[string]string{"column": "user_name"},
		},
		{
			name: "foreign key",
			tag:  "foreignKey:CompanyID",
			want: map[string]string{"foreignKey": "CompanyID"},
		},
		{
			name: "multiple",
			tag:  "primaryKey;column:id",
			want: map[string]string{"primaryKey": "", "column": "id"},
		},
		{
			name: "empty",
			tag:  "",
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := parseGormTag(tt.tag)

			// Assert
			if !maps.Equal(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseGormDir_DeterministicOrdering(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type Apple struct {
	ID uint `+"`"+`gorm:"primaryKey"`+"`"+`
}

type Zebra struct {
	ID uint `+"`"+`gorm:"primaryKey"`+"`"+`
}

type Mango struct {
	ID uint `+"`"+`gorm:"primaryKey"`+"`"+`
}

type Banana struct {
	ID uint `+"`"+`gorm:"primaryKey"`+"`"+`
}
`)

	// Act
	var first []string
	for i := range 8 {
		models, err := ParseGormDir(dir)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		names := make([]string, len(models))
		for j, m := range models {
			names[j] = m.Name
		}
		if i == 0 {
			first = names
			continue
		}
		// Assert
		if !slices.Equal(first, names) {
			t.Fatalf("iteration %d: model order is not deterministic\nfirst: %v\ngot:   %v", i, first, names)
		}
	}

	// Assert: explicit alphabetical order.
	want := []string{"Apple", "Banana", "Mango", "Zebra"}
	if !slices.Equal(first, want) {
		t.Fatalf("expected sorted model order %v, got %v", want, first)
	}
}

func TestGenerateGorm_DeterministicOutput(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

type Apple struct {
	ID   uint `+"`"+`gorm:"primaryKey"`+"`"+`
	Name string
}

type Zebra struct {
	ID   uint `+"`"+`gorm:"primaryKey"`+"`"+`
	Name string
}

type Mango struct {
	ID   uint `+"`"+`gorm:"primaryKey"`+"`"+`
	Name string
}
`)

	// Act
	var first string
	for i := range 8 {
		models, err := ParseGormDir(dir)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		var buf bytes.Buffer
		if err := GenerateGorm(&buf, "testutil", "example.com/models", models); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if i == 0 {
			first = buf.String()
			continue
		}
		// Assert
		if buf.String() != first {
			t.Fatalf("iteration %d: generated output is not byte-identical", i)
		}
	}
}

func TestRun_GormRequiresImportPath(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"gorm", "--dir", "/some/dir"}, &stdout, &stderr)

	// Assert
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--import-path is required") {
		t.Fatalf("expected import-path required error, got: %s", stderr.String())
	}
}
