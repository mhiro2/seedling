package main

import (
	"bytes"
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

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"ID", "i_d"},
		{"Name", "name"},
		{"CompanyID", "company_i_d"},
		{"CreatedAt", "created_at"},
	}
	for _, tt := range tests {
		got := toSnakeCase(tt.input)
		if got != tt.want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
			if len(got) != len(tt.want) {
				t.Fatalf("got %d parts, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestRun_GormRequiresGormPkg(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{"-gorm", "/some/dir"}, &stdout, &stderr)

	// Assert
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "-gorm-pkg is required") {
		t.Fatalf("expected gorm-pkg required error, got: %s", stderr.String())
	}
}
