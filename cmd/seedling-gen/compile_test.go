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

	t.Run("sql tables without a primary key", func(t *testing.T) {
		// Arrange
		tables, err := ParseSchema(`
CREATE TABLE legacy_users (
    id BIGINT NOT NULL,
    name TEXT NOT NULL
);

CREATE TABLE audit_logs (
    message TEXT NOT NULL
);

CREATE TABLE configs ();
`)
		if err != nil {
			t.Fatalf("ParseSchema: %v", err)
		}

		// Act
		var buf strings.Builder
		if err := Generate(&buf, "compile", tables); err != nil {
			t.Fatalf("Generate: %v", err)
		}

		// Assert
		ensureCompilesAndRuns(t, "sql_without_pk", buf.String(), sqlWithoutPKLifecycleTest)
	})

	t.Run("sql regenerated after a schema change keeps hand-written code", func(t *testing.T) {
		// The hand-written file customizes Insert, Defaults, and Traits once.
		// Adding a column and a relation and regenerating must leave it
		// compiling and behaving the same, with no repair.
		for _, schema := range []string{regenerationSchemaBefore, regenerationSchemaAfter} {
			// Arrange
			tables, err := ParseSchema(schema)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}

			// Act
			var buf strings.Builder
			if err := Generate(&buf, "compile", tables); err != nil {
				t.Fatalf("Generate: %v", err)
			}

			// Assert
			ensurePackageRuns(t, "sql_regeneration", map[string]string{
				"blueprints_gen.go":  buf.String(),
				"blueprints.go":      regenerationHandWritten,
				"blueprints_test.go": regenerationLifecycleTest,
			})
		}
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

	t.Run("gorm time key only", func(t *testing.T) {
		// Arrange: a time.Time key is never defaulted, so nothing in the
		// generated file references the time package.
		models := []GormModel{
			{
				Name:  "Company",
				Table: "companies",
				Fields: []GormField{
					{Name: "CreatedAt", Type: "time.Time", IsPK: true},
					{Name: "Name", Type: "string"},
				},
			},
		}

		// Act
		var buf strings.Builder
		if err := GenerateGorm(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testmodels", models); err != nil {
			t.Fatalf("GenerateGorm: %v", err)
		}

		// Assert
		ensureCompiles(t, "gorm_time_key_only", buf.String())
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
	files := map[string]string{"blueprint.go": src}
	if testSrc != "" {
		files["blueprint_test.go"] = testSrc
	}
	ensurePackageRuns(t, name, files)
}

// ensurePackageRuns writes files into a temporary module that resolves
// seedling to this checkout, then builds and tests it.
func ensurePackageRuns(t *testing.T, name string, files map[string]string) {
	t.Helper()
	requireGoToolchain(t)
	tmpDir := t.TempDir()
	for file, src := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte(src), 0o600); err != nil {
			t.Fatalf("write %s file %s: %v", name, file, err)
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

const sqlWithoutPKLifecycleTest = `package compile

import (
	"context"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
)

func insertAsIs[T any](_ context.Context, _ seedling.DBTX, v T) (T, error) {
	return v, nil
}

func validateAndInsert[T any](t *testing.T, reg *seedling.Registry) {
	t.Helper()
	plan, err := seedling.NewSession[T](reg).BuildE()
	if err != nil {
		t.Fatalf("build %T plan: %v", *new(T), err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("validate %T plan: %v", *new(T), err)
	}
	if _, err := seedling.NewSession[T](reg).InsertOneE(t.Context(), nil); err != nil {
		t.Fatalf("insert %T: %v", *new(T), err)
	}
}

func TestGeneratedBlueprintsRequireInsert(t *testing.T) {
	err := NewBlueprints().Register(seedling.NewRegistry())
	if err == nil {
		t.Fatal("registering blueprints without Insert succeeded")
	}
	for _, name := range []string{"legacy_user", "audit_log", "config"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("registration error does not report blueprint %q: %v", name, err)
		}
	}
}

func TestGeneratedBlueprintsWithoutPrimaryKey(t *testing.T) {
	blueprints := NewBlueprints()
	blueprints.LegacyUser.Insert = insertAsIs[LegacyUser]
	blueprints.AuditLog.Insert = insertAsIs[AuditLog]
	blueprints.Config.Insert = insertAsIs[Config]
	reg := seedling.NewRegistry()
	blueprints.MustRegister(reg)

	validateAndInsert[LegacyUser](t, reg)
	validateAndInsert[AuditLog](t, reg)
	validateAndInsert[Config](t, reg)
}
`

const regenerationSchemaBefore = `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    name TEXT NOT NULL
);
`

const regenerationSchemaAfter = `
CREATE TABLE companies (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    parent_id INTEGER REFERENCES companies(id)
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    name TEXT NOT NULL,
    nickname TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
`

const regenerationHandWritten = `package compile

import (
	"context"
	"sync/atomic"

	"github.com/mhiro2/seedling"
)

var lastID atomic.Int64

func insertWithID[T any](setID func(*T, int)) func(context.Context, seedling.DBTX, T) (T, error) {
	return func(_ context.Context, _ seedling.DBTX, v T) (T, error) {
		setID(&v, int(lastID.Add(1)))
		return v, nil
	}
}

func NewRegistry() *seedling.Registry {
	blueprints := NewBlueprints()

	blueprints.Company.Insert = insertWithID(func(c *Company, id int) { c.ID = id })
	blueprints.User.Insert = insertWithID(func(u *User, id int) { u.ID = id })

	generatedUserDefaults := blueprints.User.Defaults
	blueprints.User.Defaults = func() User {
		user := generatedUserDefaults()
		user.Name = "alice"
		return user
	}
	blueprints.User.Traits = map[string][]seedling.Option{
		"bob": {seedling.Set("Name", "bob")},
	}

	reg := seedling.NewRegistry()
	blueprints.MustRegister(reg)
	return reg
}
`

const regenerationLifecycleTest = `package compile

import (
	"testing"

	"github.com/mhiro2/seedling"
)

func TestHandWrittenCustomizations(t *testing.T) {
	reg := NewRegistry()

	alice := seedling.NewSession[User](reg).InsertOne(t, nil).Root()
	if alice.ID == 0 || alice.CompanyID == 0 || alice.Name != "alice" {
		t.Fatalf("default user = %+v, want inserted alice with a company", alice)
	}

	bob := seedling.NewSession[User](reg).InsertOne(t, nil, seedling.BlueprintTrait("bob")).Root()
	if bob.ID == 0 || bob.Name != "bob" {
		t.Fatalf("trait user = %+v, want inserted bob", bob)
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
	reg := seedling.NewRegistry()
	NewBlueprints().MustRegister(reg)
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
	reg := seedling.NewRegistry()
	NewBlueprints().MustRegister(reg)
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
