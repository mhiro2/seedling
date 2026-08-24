package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const actualEntFixtureImportPath = "github.com/mhiro2/seedling/cmd/seedling-gen/testdata/ent_actual/ent"

func TestActualEntFixtureLifecycle(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "ent_actual")
	schemaDir := filepath.Join(fixtureDir, "ent", "schema")
	schemas, err := ParseEntSchemaDir(schemaDir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}
	schemas, err = ResolveEntSchemas(schemaDir, actualEntFixtureImportPath, schemas)
	if err != nil {
		t.Fatalf("ResolveEntSchemas: %v", err)
	}

	var generated strings.Builder
	if err := GenerateEnt(&generated, "compile", actualEntFixtureImportPath, schemas); err != nil {
		t.Fatalf("GenerateEnt: %v", err)
	}
	ensureActualEntCompilesAndRuns(t, fixtureDir, generated.String(), actualEntLifecycleTest)
}

func ensureActualEntCompilesAndRuns(t *testing.T, fixtureDir, src, testSrc string) {
	t.Helper()
	requireGoToolchain(t)
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write generated Ent blueprint: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint_test.go"), []byte(testSrc), 0o600); err != nil {
		t.Fatalf("write generated Ent lifecycle test: %v", err)
	}

	root := moduleRoot(t)
	actualFixtureDir, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatalf("resolve actual Ent fixture dir: %v", err)
	}
	gomod := fmt.Sprintf(`module compiletest

go 1.26.0

require (
	github.com/mhiro2/seedling v0.0.0
	github.com/mhiro2/seedling/cmd/seedling-gen/testdata/ent_actual v0.0.0
	modernc.org/sqlite v1.50.1
)

replace github.com/mhiro2/seedling => %s
replace github.com/mhiro2/seedling/cmd/seedling-gen/testdata/ent_actual => %s
`, filepath.ToSlash(root), filepath.ToSlash(actualFixtureDir))
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatalf("write actual Ent fixture go.mod: %v", err)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test actual Ent fixture: %v\n%s", err, output)
	}
}

const actualEntLifecycleTest = `package compile

import (
	"database/sql"
	"testing"

	"github.com/mhiro2/seedling"
	actualent "github.com/mhiro2/seedling/cmd/seedling-gen/testdata/ent_actual/ent"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestGeneratedActualEntBlueprintLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", "file:seedling_ent_actual?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	driver := entsql.OpenDB(dialect.SQLite, db)
	client := actualent.NewClient(actualent.Driver(driver))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Ent client: %v", err)
		}
	})
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create Ent schema: %v", err)
	}

	reg := NewRegistry()
	result, err := seedling.NewSession[*actualent.User](reg).InsertOneE(t.Context(), client)
	if err != nil {
		t.Fatalf("insert generated Ent blueprint: %v", err)
	}
	root := result.Root()
	if root == nil || root.ID == 0 {
		t.Fatalf("inserted root = %#v, want a persisted user", root)
	}
	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("company node not found")
	}
	company := companyNode.Value().(*actualent.Company)
	if root.CompanyUUID == nil || *root.CompanyUUID != company.ID {
		t.Fatalf("user company UUID = %v, company ID = %d", root.CompanyUUID, company.ID)
	}
	if root.Tenant == "" {
		t.Fatal("required mixin field was not populated")
	}
	if root.OidcURL == "" {
		t.Fatal("generated OidcURL field was not populated")
	}
	if users, err := client.User.Query().Count(t.Context()); err != nil || users != 1 {
		t.Fatalf("persisted users = %d, err = %v; want 1", users, err)
	}
	if companies, err := client.Company.Query().Count(t.Context()); err != nil || companies != 1 {
		t.Fatalf("persisted companies = %d, err = %v; want 1", companies, err)
	}

	if err := result.CleanupE(t.Context(), client); err != nil {
		t.Fatalf("cleanup generated Ent blueprint: %v", err)
	}
	if users, err := client.User.Query().Count(t.Context()); err != nil || users != 0 {
		t.Fatalf("remaining users = %d, err = %v; want 0", users, err)
	}
	if companies, err := client.Company.Query().Count(t.Context()); err != nil || companies != 0 {
		t.Fatalf("remaining companies = %d, err = %v; want 0", companies, err)
	}
}
`
