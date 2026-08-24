package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const actualGormFixtureImportPath = "github.com/mhiro2/seedling/cmd/seedling-gen/testdata/gorm_actual/model"

func TestActualGormFixtureLifecycle(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "gorm_actual")
	models, err := ParseGormDir(filepath.Join(fixtureDir, "model"))
	if err != nil {
		t.Fatalf("ParseGormDir: %v", err)
	}

	var generated strings.Builder
	if err := GenerateGorm(&generated, "compile", actualGormFixtureImportPath, models); err != nil {
		t.Fatalf("GenerateGorm: %v", err)
	}
	ensureActualGormCompilesAndRuns(t, fixtureDir, generated.String(), actualGormLifecycleTest)
}

func ensureActualGormCompilesAndRuns(t *testing.T, fixtureDir, src, testSrc string) {
	t.Helper()
	requireGoToolchain(t)
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write generated GORM blueprint: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint_test.go"), []byte(testSrc), 0o600); err != nil {
		t.Fatalf("write generated GORM lifecycle test: %v", err)
	}
	schema, err := os.ReadFile(filepath.Join(fixtureDir, "schema.sql"))
	if err != nil {
		t.Fatalf("read actual GORM fixture schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "schema.sql"), schema, 0o600); err != nil {
		t.Fatalf("write actual GORM fixture schema: %v", err)
	}

	root := moduleRoot(t)
	actualFixtureDir, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatalf("resolve actual GORM fixture dir: %v", err)
	}
	gomod := fmt.Sprintf(`module compiletest

go 1.26.0

require (
	github.com/glebarez/sqlite v1.11.0
	github.com/mhiro2/seedling v0.0.0
	github.com/mhiro2/seedling/cmd/seedling-gen/testdata/gorm_actual v0.0.0
	gorm.io/gorm v1.25.11
)

replace github.com/mhiro2/seedling => %s
replace github.com/mhiro2/seedling/cmd/seedling-gen/testdata/gorm_actual => %s
`, filepath.ToSlash(root), filepath.ToSlash(actualFixtureDir))
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write actual GORM fixture go.mod: %v", err)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test actual GORM fixture: %v\n%s", err, output)
	}
}

const actualGormLifecycleTest = `package compile

import (
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mhiro2/seedling"
	actualgorm "github.com/mhiro2/seedling/cmd/seedling-gen/testdata/gorm_actual/model"
	"gorm.io/gorm"
)

func TestGeneratedActualGormBlueprintLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:seedling_gorm_actual?mode=memory&cache=shared&_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read fixture schema: %v", err)
	}
	for statement := range strings.SplitSeq(string(schema), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("apply fixture schema: %v", err)
		}
	}

	reg := NewRegistry()
	result, err := seedling.NewSession[actualgorm.User](reg).InsertOneE(t.Context(), db)
	if err != nil {
		t.Fatalf("insert generated GORM blueprint: %v", err)
	}
	root := result.Root()
	if root.ID == nil || *root.ID == 0 {
		t.Fatalf("inserted root ID = %v, want a generated value", root.ID)
	}
	countryNode, ok := result.Node("country")
	if !ok {
		t.Fatal("country node not found")
	}
	country := countryNode.Value().(actualgorm.Country)
	if country.ID == nil || *country.ID == 0 {
		t.Fatalf("inserted country ID = %v, want a generated value", country.ID)
	}
	if root.CountryCode != country.Code || country.Code == "" {
		t.Fatalf("user country code = %q, country code = %q", root.CountryCode, country.Code)
	}
	if root.DisplayName == "" {
		t.Fatal("generated display name was not populated")
	}
	assertActualGormCount(t, db, &actualgorm.User{}, 1)
	assertActualGormCount(t, db, &actualgorm.Country{}, 1)

	if err := result.CleanupE(t.Context(), db); err != nil {
		t.Fatalf("cleanup generated GORM blueprint: %v", err)
	}
	assertActualGormCount(t, db, &actualgorm.User{}, 0)
	assertActualGormCount(t, db, &actualgorm.Country{}, 0)

	membershipResult, err := seedling.NewSession[actualgorm.Membership](reg).InsertOneE(
		t.Context(),
		db,
		seedling.Set("OrganizationID", int32(7)),
		seedling.Set("UserID", int32(9)),
	)
	if err != nil {
		t.Fatalf("insert generated composite GORM blueprint: %v", err)
	}
	membership := membershipResult.Root()
	if membership.OrganizationID != 7 || membership.UserID != 9 {
		t.Fatalf("inserted membership = %+v, want organization 7 and user 9", membership)
	}
	assertActualGormCount(t, db, &actualgorm.Membership{}, 1)
	if err := membershipResult.CleanupE(t.Context(), db); err != nil {
		t.Fatalf("cleanup generated composite GORM blueprint: %v", err)
	}
	assertActualGormCount(t, db, &actualgorm.Membership{}, 0)
}

func assertActualGormCount(t *testing.T, db *gorm.DB, model any, want int64) {
	t.Helper()
	var got int64
	if err := db.Model(model).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	if got != want {
		t.Fatalf("count %T = %d, want %d", model, got, want)
	}
}
`
