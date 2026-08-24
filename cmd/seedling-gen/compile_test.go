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
				Name:        "companies",
				GoName:      "Company",
				BlueprintID: "company",
				Columns: []Column{
					{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "spotify_url", GoName: "SpotifyURL", GoType: "string", NotNull: true},
				},
			},
			{
				Name:        "users",
				GoName:      "User",
				BlueprintID: "user",
				Columns: []Column{
					{Name: "id", GoName: "ID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "profile_name", GoName: "ProfileName", GoType: "string", NotNull: true},
					{Name: "created_at", GoName: "CreatedAt", GoType: "time.Time", NotNull: true},
					{Name: "company_spotify_url", GoName: "CompanySpotifyURL", GoType: "string", NotNull: true, IsFK: true},
				},
				ForeignKeys: []ForeignKey{
					{Columns: []string{"company_spotify_url"}, RefTable: "companies", RefColumns: []string{"spotify_url"}, NotNull: true},
				},
			},
			{
				Name:        "memberships",
				GoName:      "Membership",
				BlueprintID: "membership",
				Columns: []Column{
					{Name: "organization_id", GoName: "OrganizationID", GoType: "int64", IsPK: true, NotNull: true},
					{Name: "user_id", GoName: "UserID", GoType: "int64", IsPK: true, NotNull: true},
				},
			},
		}

		sqlcInfo, err := ParseSqlcDir("testsqlc")
		if err != nil {
			t.Fatalf("ParseSqlcDir: %v", err)
		}

		var buf strings.Builder
		if err := GenerateSqlc(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testsqlc", tables, sqlcInfo); err != nil {
			t.Fatalf("GenerateSqlc: %v", err)
		}
		ensureCompilesAndRuns(t, "sqlc", buf.String(), sqlcLifecycleTest)
	})

	t.Run("gorm", func(t *testing.T) {
		models, err := ParseGormDir("testmodels")
		if err != nil {
			t.Fatalf("ParseGormDir: %v", err)
		}

		var buf strings.Builder
		if err := GenerateGorm(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testmodels", models); err != nil {
			t.Fatalf("GenerateGorm: %v", err)
		}
		ensureCompilesAndRuns(t, "gorm", buf.String(), gormLifecycleTest)
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
		schemas, err := ParseEntSchemaDir(filepath.Join("testdata", "ent", "schema"))
		if err != nil {
			t.Fatalf("ParseEntSchemaDir: %v", err)
		}
		schemas, err = ResolveEntSchemas(
			filepath.Join("testdata", "ent", "schema"),
			"github.com/mhiro2/seedling/cmd/seedling-gen/testent",
			schemas,
		)
		if err != nil {
			t.Fatalf("ResolveEntSchemas: %v", err)
		}

		var buf strings.Builder
		if err := GenerateEnt(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testent", schemas); err != nil {
			t.Fatalf("GenerateEnt: %v", err)
		}
		ensureCompilesAndRuns(t, "ent", buf.String(), entLifecycleTest)
	})
}

func ensureCompiles(t *testing.T, name, src string) {
	t.Helper()
	ensureCompilesAndRuns(t, name, src, "")
}

func ensureCompilesAndRuns(t *testing.T, name, src, testSrc string) {
	t.Helper()
	requireGoToolchain(t)
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write %s generated code: %v", name, err)
	}
	if testSrc != "" {
		if err := os.WriteFile(filepath.Join(tmpDir, "blueprint_test.go"), []byte(testSrc), 0o600); err != nil {
			t.Fatalf("write %s generated test: %v", name, err)
		}
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

const gormLifecycleTest = `package compile

import (
	"testing"

	"github.com/mhiro2/seedling"
	testmodels "github.com/mhiro2/seedling/cmd/seedling-gen/testmodels"
	"gorm.io/gorm"
)

func TestGeneratedGormBlueprintLifecycle(t *testing.T) {
	db := &gorm.DB{}
	reg := NewRegistry()
	result, err := seedling.NewSession[testmodels.User](reg).InsertOneE(t.Context(), db)
	if err != nil {
		t.Fatalf("insert generated GORM blueprint: %v", err)
	}
	if len(db.Created) != 2 {
		t.Fatalf("created records = %d, want 2", len(db.Created))
	}
	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("company node not found")
	}
	company := companyNode.Value().(testmodels.Company)
	if result.Root().CompanyID != company.ID || company.ID == 0 {
		t.Fatalf("user company ID = %d, company ID = %d", result.Root().CompanyID, company.ID)
	}
	if err := result.CleanupE(t.Context(), db); err != nil {
		t.Fatalf("cleanup generated GORM blueprint: %v", err)
	}
	if len(db.Deleted) != 2 {
		t.Fatalf("deleted records = %d, want 2", len(db.Deleted))
	}
}
`

const sqlcLifecycleTest = `package compile

import (
	"testing"

	"github.com/mhiro2/seedling"
	testsqlc "github.com/mhiro2/seedling/cmd/seedling-gen/testsqlc"
)

func TestGeneratedSQLCBlueprintLifecycle(t *testing.T) {
	recorder := &testsqlc.Recorder{}
	reg := NewRegistry()
	result, err := seedling.NewSession[testsqlc.User](reg).InsertOneE(t.Context(), recorder)
	if err != nil {
		t.Fatalf("insert generated sqlc blueprint: %v", err)
	}
	if len(recorder.InsertedCompanies) != 1 || len(recorder.InsertedUsers) != 1 {
		t.Fatalf("insert counts = companies:%d users:%d, want 1 each", len(recorder.InsertedCompanies), len(recorder.InsertedUsers))
	}
	root := result.Root()
	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("company node not found")
	}
	company := companyNode.Value().(testsqlc.Company)
	if root.CompanySpotifyUrl != company.SpotifyUrl || company.SpotifyUrl == "" {
		t.Fatalf("user company Spotify URL = %q, company Spotify URL = %q", root.CompanySpotifyUrl, company.SpotifyUrl)
	}
	if root.DisplayLabel == "" || recorder.InsertedUsers[0].DisplayLabel != root.DisplayLabel {
		t.Fatalf("inserted renamed profile field = %q, root = %q", recorder.InsertedUsers[0].DisplayLabel, root.DisplayLabel)
	}
	if root.CreatedAt.IsZero() || recorder.InsertedUsers[0].CreatedAt != root.CreatedAt {
		t.Fatalf("inserted remapped timestamp = %v, root = %v", recorder.InsertedUsers[0].CreatedAt, root.CreatedAt)
	}
	if err := result.CleanupE(t.Context(), recorder); err != nil {
		t.Fatalf("cleanup generated sqlc blueprint: %v", err)
	}
	if len(recorder.DeletedUserIDs) != 1 || len(recorder.DeletedCompanyIDs) != 1 {
		t.Fatalf("delete counts = companies:%d users:%d, want 1 each", len(recorder.DeletedCompanyIDs), len(recorder.DeletedUserIDs))
	}

	membershipResult, err := seedling.NewSession[testsqlc.Membership](reg).InsertOneE(
		t.Context(),
		recorder,
		seedling.Set("OrganizationID", int64(7)),
		seedling.Set("UserID", int64(9)),
	)
	if err != nil {
		t.Fatalf("insert generated composite sqlc blueprint: %v", err)
	}
	if len(recorder.InsertedMemberships) != 1 {
		t.Fatalf("composite inserts = %d, want 1", len(recorder.InsertedMemberships))
	}
	inserted := recorder.InsertedMemberships[0]
	if inserted.OrganizationID != 7 || inserted.UserID != 9 {
		t.Fatalf("inserted composite key = %+v, want organization=7 user=9", inserted)
	}
	if err := membershipResult.CleanupE(t.Context(), recorder); err != nil {
		t.Fatalf("cleanup generated composite sqlc blueprint: %v", err)
	}
	if len(recorder.DeletedMemberships) != 1 {
		t.Fatalf("composite deletes = %d, want 1", len(recorder.DeletedMemberships))
	}
	deleted := recorder.DeletedMemberships[0]
	if deleted.OrganizationID != 7 || deleted.UserID != 9 {
		t.Fatalf("deleted composite key = %+v, want organization=7 user=9", deleted)
	}
}
`

const entLifecycleTest = `package compile

import (
	"testing"

	"github.com/mhiro2/seedling"
	testent "github.com/mhiro2/seedling/cmd/seedling-gen/testent"
)

func TestGeneratedEntBlueprintLifecycle(t *testing.T) {
	companyClient := &testent.CompanyClient{}
	userClient := &testent.UserClient{}
	client := &testent.Client{Company: companyClient, User: userClient}
	reg := NewRegistry()
	result, err := seedling.NewSession[*testent.User](reg).InsertOneE(
		t.Context(),
		client,
	)
	if err != nil {
		t.Fatalf("insert generated ent blueprint: %v", err)
	}

	root := result.Root()
	if root == nil {
		t.Fatal("inserted root is nil")
	}
	if companyClient.InsertCount != 1 || userClient.InsertCount != 1 {
		t.Fatalf("insert counts = companies:%d users:%d, want 1 each", companyClient.InsertCount, userClient.InsertCount)
	}
	if root.CompanyUUID == nil || *root.CompanyUUID != companyClient.InsertedValue.ID {
		t.Fatalf("user company UUID = %v, company ID = %d", root.CompanyUUID, companyClient.InsertedValue.ID)
	}
	if userClient.InsertedValue.CompanyUUID == nil || *userClient.InsertedValue.CompanyUUID != companyClient.InsertedValue.ID {
		t.Fatalf("builder company UUID = %v, company ID = %d", userClient.InsertedValue.CompanyUUID, companyClient.InsertedValue.ID)
	}
	if root.OIDCURL == "" || userClient.InsertedValue.OIDCURL != root.OIDCURL {
		t.Fatalf("builder OIDC URL = %q, root OIDC URL = %q", userClient.InsertedValue.OIDCURL, root.OIDCURL)
	}
	if root.CreatedAt != 1 || userClient.InsertedValue.CreatedAt != root.CreatedAt {
		t.Fatalf("builder mixin CreatedAt = %d, root = %d, want 1", userClient.InsertedValue.CreatedAt, root.CreatedAt)
	}
	if root.ID == 0 {
		t.Fatal("inserted root ID is zero")
	}

	if err := result.CleanupE(t.Context(), client); err != nil {
		t.Fatalf("cleanup generated ent blueprint: %v", err)
	}
	if companyClient.DeleteCount != 1 || userClient.DeleteCount != 1 {
		t.Fatalf("delete counts = companies:%d users:%d, want 1 each", companyClient.DeleteCount, userClient.DeleteCount)
	}
	if userClient.DeletedID != root.ID {
		t.Fatalf("deleted user ID = %d, want %d", userClient.DeletedID, root.ID)
	}
}
`

// requireGoToolchain skips tests that build and run a temporary module. They
// shell out to the go toolchain and need its module cache populated, so they are
// not runnable in short mode or in an environment without the go binary.
func requireGoToolchain(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a temporary module with the go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain is unavailable: %v", err)
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
