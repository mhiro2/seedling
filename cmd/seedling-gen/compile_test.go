package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratorOutputsCompile(t *testing.T) {
	t.Run("sql", func(t *testing.T) {
		tables := []Table{
			{
				Name:        "companies",
				GoName:      "Company",
				BlueprintID: "company",
				Columns: []Column{
					{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "name", GoName: "Name", GoType: "string", NotNull: true},
				},
			},
			{
				Name:        "users",
				GoName:      "User",
				BlueprintID: "user",
				Columns: []Column{
					{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "name", GoName: "Name", GoType: "string", NotNull: true},
					{Name: "company_id", GoName: "CompanyID", GoType: "int64", NotNull: true, IsFK: true},
					{Name: "created_at", GoName: "CreatedAt", GoType: "time.Time", NotNull: true},
				},
				ForeignKeys: []ForeignKey{
					{Columns: []string{"company_id"}, RefTable: "companies", NotNull: true},
				},
			},
		}

		var buf strings.Builder
		if err := Generate(&buf, "compile", tables); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		ensureCompiles(t, "sql", buf.String())
	})

	t.Run("sql with go keyword identifiers", func(t *testing.T) {
		// Arrange
		tables := []Table{
			{
				Name:        "types",
				GoName:      "Type",
				BlueprintID: "type",
				Columns: []Column{
					{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "func", GoName: "Func", GoType: "string", NotNull: true},
				},
			},
		}

		// Act
		var buf strings.Builder
		if err := Generate(&buf, "compile", tables); err != nil {
			t.Fatalf("Generate: %v", err)
		}

		// Assert
		ensureCompiles(t, "sql_keywords", buf.String())
	})

	t.Run("sqlc", func(t *testing.T) {
		tables := []Table{
			{
				Name:        "users",
				GoName:      "User",
				BlueprintID: "user",
				Columns: []Column{
					{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "name", GoName: "Name", GoType: "string", NotNull: true},
					{Name: "company_id", GoName: "CompanyID", GoType: "int64", NotNull: true, IsFK: true},
				},
				ForeignKeys: []ForeignKey{
					{Columns: []string{"company_id"}, RefTable: "companies", NotNull: true},
				},
			},
		}

		sqlcInfo := &SqlcInfo{
			Package: "testsqlc",
			Models: []SqlcModel{
				{Name: "User", Fields: []SqlcField{
					{Name: "ID", Type: "int64"},
					{Name: "Name", Type: "string"},
					{Name: "CompanyID", Type: "int64"},
				}},
			},
			Queries: []SqlcQuery{
				{
					Name:        "InsertUser",
					ReturnType:  "User",
					ParamType:   "InsertUserParams",
					ParamFields: []SqlcField{{Name: "Name", Type: "string"}, {Name: "CompanyID", Type: "int64"}},
				},
			},
			DeleteQueries: []SqlcDeleteQuery{
				{Name: "DeleteUser", ArgName: "id", ArgType: "int64"},
			},
		}

		var buf strings.Builder
		if err := GenerateSqlc(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testsqlc", tables, sqlcInfo); err != nil {
			t.Fatalf("GenerateSqlc: %v", err)
		}
		ensureCompiles(t, "sqlc", buf.String())
	})

	t.Run("gorm", func(t *testing.T) {
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
					{Name: "CompanyID", Type: "uint", NotNull: true, IsFK: true},
					{Name: "Company", Type: "Company", Relation: &GormRelation{
						Kind: "BelongsTo", ForeignKey: "CompanyID", RefModel: "Company",
					}},
				},
			},
		}

		var buf strings.Builder
		if err := GenerateGorm(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testmodels", models); err != nil {
			t.Fatalf("GenerateGorm: %v", err)
		}
		ensureCompiles(t, "gorm", buf.String())
	})

	t.Run("gorm composite pk", func(t *testing.T) {
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
		var buf strings.Builder
		if err := GenerateGorm(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testmodels", models); err != nil {
			t.Fatalf("GenerateGorm: %v", err)
		}

		// Assert
		ensureCompiles(t, "gorm_composite_pk", buf.String())
	})

	t.Run("ent", func(t *testing.T) {
		schemas := []EntSchema{
			{Name: "Company"},
		}

		var buf strings.Builder
		if err := GenerateEnt(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testent", schemas); err != nil {
			t.Fatalf("GenerateEnt: %v", err)
		}
		ensureCompiles(t, "ent", buf.String())
	})
}

func ensureCompiles(t *testing.T, name, src string) {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write %s generated code: %v", name, err)
	}

	root := moduleRoot(t)

	gomod := fmt.Sprintf(`module compiletest

go 1.26

require (
	github.com/mhiro2/seedling v0.0.0
	gorm.io/gorm v0.0.0
)

replace github.com/mhiro2/seedling => %s
replace gorm.io/gorm => %s
`, filepath.ToSlash(root), filepath.ToSlash(filepath.Join(root, "third_party", "gorm")))

	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test compile %s: %v\n%s", name, err, output)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("determine working dir: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("unable to locate go.mod")
		}
		dir = parent
	}
}
