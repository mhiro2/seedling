package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const entFixtureImportPath = "github.com/mhiro2/seedling/cmd/seedling-gen/testent"

func TestResolveEntSchemasUsesGeneratedFieldNames(t *testing.T) {
	schemaDir := filepath.Join("testdata", "ent", "schema")
	schemas, err := ParseEntSchemaDir(schemaDir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}

	resolved, err := ResolveEntSchemas(schemaDir, entFixtureImportPath, schemas)
	if err != nil {
		t.Fatalf("ResolveEntSchemas: %v", err)
	}

	user := findEntSchema(t, resolved, "User")
	wantNames := map[string]string{
		"company_uuid": "CompanyUUID",
		"oidc_url":     "OIDCURL",
	}
	for fieldName, want := range wantNames {
		field := findEntField(t, user.Fields, fieldName)
		if field.GoName != want {
			t.Errorf("field %q GoName = %q, want %q", fieldName, field.GoName, want)
		}
	}

	original := findEntSchema(t, schemas, "User")
	if got := findEntField(t, original.Fields, "oidc_url").GoName; got != "" {
		t.Fatalf("input schema was mutated: oidc_url GoName = %q", got)
	}
}

func TestResolveEntSchemasRejectsMissingGeneratedType(t *testing.T) {
	_, err := ResolveEntSchemas(
		filepath.Join("testdata", "ent", "schema"),
		entFixtureImportPath,
		[]EntSchema{{Name: "Account"}},
	)
	if err == nil || !strings.Contains(err.Error(), "generated type testent.Account is absent") {
		t.Fatalf("expected missing generated type error, got %v", err)
	}
}

func TestResolveEntSchemasRejectsStaleGeneratedField(t *testing.T) {
	schemaDir := filepath.Join("testdata", "ent", "schema")
	schemas, err := ParseEntSchemaDir(schemaDir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}
	for i := range schemas {
		if schemas[i].Name != "User" {
			continue
		}
		schemas[i].Fields[0].Name = "renamed_name"
		schemas[i].Fields[0].JSONName = "renamed_name"
	}

	_, err = ResolveEntSchemas(
		schemaDir,
		entFixtureImportPath,
		schemas,
	)
	if err == nil || !strings.Contains(err.Error(), `field "renamed_name" with JSON name "renamed_name" is absent`) {
		t.Fatalf("expected stale generated field error, got %v", err)
	}
}

func TestResolveEntSchemasUsesGeneratedGoTypeAndIncludesMixinFields(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

import (
	"context"
	"database/sql"
)

type User struct {
	ID        int            `+"`json:\"id,omitempty\"`"+`
	CreatedAt int64          `+"`json:\"created_at,omitempty\"`"+`
	Name      sql.NullString `+"`json:\"name,omitempty\"`"+`
}

type UserCreate struct{}

func (*UserCreate) SetName(sql.NullString) {}
func (*UserCreate) SetCreatedAt(int64) {}
func (*UserCreate) Save(context.Context) (*User, error) { return &User{}, nil }

type UserClient struct{}

func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int) *UserDelete { return &UserDelete{} }

type UserDelete struct{}

func (*UserDelete) Exec(context.Context) error { return nil }
`)

	resolved, err := ResolveEntSchemas(schemaDir, importPath, []EntSchema{{
		Name: "User",
		Fields: []EntField{{
			Name:         "name",
			JSONName:     "name",
			Type:         "String",
			GoType:       "string",
			CustomGoType: true,
		}},
	}})
	if err != nil {
		t.Fatalf("ResolveEntSchemas: %v", err)
	}
	field := resolved[0].Fields[0]
	if field.GoName != "Name" || field.GoType != "sql.NullString" {
		t.Fatalf("resolved field = %+v, want Name/sql.NullString", field)
	}
	mixinField := findEntField(t, resolved[0].Fields, "created_at")
	if mixinField.GoName != "CreatedAt" || mixinField.GoType != "int64" || !mixinField.FromMixin {
		t.Fatalf("resolved mixin field = %+v, want generated CreatedAt/int64", mixinField)
	}

	var output bytes.Buffer
	if err := GenerateEnt(&output, "blueprints", importPath, resolved); err != nil {
		t.Fatalf("GenerateEnt: %v", err)
	}
	if strings.Contains(output.String(), `Name: "user-name"`) {
		t.Fatalf("generated defaults assign a string to sql.NullString:\n%s", output.String())
	}
	if !strings.Contains(output.String(), `CreatedAt: 1`) || !strings.Contains(output.String(), `builder.SetCreatedAt(v.CreatedAt)`) {
		t.Fatalf("generated output does not populate the required mixin field:\n%s", output.String())
	}
}

func TestResolveEntSchemasRejectsStaleGeneratedFieldType(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

import "context"

type User struct {
	ID   int `+"`json:\"id,omitempty\"`"+`
	Name int `+"`json:\"name,omitempty\"`"+`
}

type UserCreate struct{}

func (*UserCreate) SetName(int) {}
func (*UserCreate) Save(context.Context) (*User, error) { return &User{}, nil }

type UserClient struct{}

func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int) *UserDelete { return &UserDelete{} }

type UserDelete struct{}

func (*UserDelete) Exec(context.Context) error { return nil }
`)

	_, err := ResolveEntSchemas(schemaDir, importPath, []EntSchema{{
		Name: "User",
		Fields: []EntField{{
			Name:     "name",
			JSONName: "name",
			Type:     "String",
			GoType:   "string",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "generated type is int, incompatible with schema field.String") {
		t.Fatalf("expected stale generated field type error, got %v", err)
	}
}

func TestResolveEntSchemasDistinguishesSensitiveFieldsFromMixins(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

import "context"

type User struct {
	ID          int    `+"`json:\"id,omitempty\"`"+`
	MixinSecret string `+"`json:\"-\"`"+`
	Secret      string `+"`json:\"-\"`"+`
}

type UserCreate struct{}

func (*UserCreate) SetMixinSecret(string) {}
func (*UserCreate) SetSecret(string) {}
func (*UserCreate) Save(context.Context) (*User, error) { return &User{}, nil }

type UserClient struct{}

func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int) *UserDelete { return &UserDelete{} }

type UserDelete struct{}

func (*UserDelete) Exec(context.Context) error { return nil }
`)

	resolved, err := ResolveEntSchemas(schemaDir, importPath, []EntSchema{{
		Name: "User",
		Fields: []EntField{{
			Name:     "secret",
			JSONName: "-",
			Type:     "String",
			GoType:   "string",
		}},
	}})
	if err != nil {
		t.Fatalf("ResolveEntSchemas: %v", err)
	}
	if got := findEntField(t, resolved[0].Fields, "secret").GoName; got != "Secret" {
		t.Fatalf("sensitive field GoName = %q, want Secret", got)
	}
	if got := findEntField(t, resolved[0].Fields, "mixin_secret"); got.GoName != "MixinSecret" || !got.FromMixin {
		t.Fatalf("mixin sensitive field = %+v", got)
	}
}

func TestResolveEntSchemasMapsForeignKeySuppliedByMixin(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

import "context"

type Company struct { ID int `+"`json:\"id,omitempty\"`"+` }
type CompanyCreate struct{}
func (*CompanyCreate) Save(context.Context) (*Company, error) { return &Company{}, nil }
type CompanyClient struct{}
func (*CompanyClient) Create() *CompanyCreate { return &CompanyCreate{} }
func (*CompanyClient) DeleteOneID(int) *CompanyDelete { return &CompanyDelete{} }
type CompanyDelete struct{}
func (*CompanyDelete) Exec(context.Context) error { return nil }

type User struct {
	ID        int `+"`json:\"id,omitempty\"`"+`
	CompanyID int `+"`json:\"company_id,omitempty\"`"+`
}
type UserCreate struct{}
func (*UserCreate) SetCompanyID(int) {}
func (*UserCreate) Save(context.Context) (*User, error) { return &User{}, nil }
type UserClient struct{}
func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int) *UserDelete { return &UserDelete{} }
type UserDelete struct{}
func (*UserDelete) Exec(context.Context) error { return nil }
`)
	writeFile(t, schemaDir, "schemas.go", `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

type Company struct { ent.Schema }
type User struct { ent.Schema }

func (User) Mixin() []ent.Mixin { return nil }
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("company", Company.Type).Unique().Field("company_id"),
	}
}
`)

	schemas, err := ParseEntSchemaDir(schemaDir)
	if err != nil {
		t.Fatalf("ParseEntSchemaDir: %v", err)
	}
	resolved, err := ResolveEntSchemas(schemaDir, importPath, schemas)
	if err != nil {
		t.Fatalf("ResolveEntSchemas: %v", err)
	}
	user := findEntSchema(t, resolved, "User")
	field := findEntField(t, user.Fields, "company_id")
	if field.GoName != "CompanyID" || !field.FromMixin {
		t.Fatalf("resolved mixin foreign key = %+v", field)
	}

	var output bytes.Buffer
	if err := GenerateEnt(&output, "blueprints", importPath, resolved); err != nil {
		t.Fatalf("GenerateEnt: %v", err)
	}
	for _, want := range []string{
		`LocalField: "CompanyID"`,
		`if !reflect.ValueOf(v.CompanyID).IsZero() {`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("generated output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestResolveEntSchemasRejectsSetterSignatureMismatch(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

type User struct {
	ID      int    `+"`json:\"id,omitempty\"`"+`
	OIDCURL string `+"`json:\"oidc_url,omitempty\"`"+`
}

type UserCreate struct{}

func (*UserCreate) SetOIDCURL(int) {}

type UserClient struct{}

func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int)     {}
`)

	_, err := ResolveEntSchemas(
		schemaDir,
		importPath,
		[]EntSchema{{Name: "User", Fields: []EntField{{Name: "oidc_url"}}}},
	)
	if err == nil || !strings.Contains(err.Error(), "parameter 0 has type int, want string") {
		t.Fatalf("expected setter signature mismatch error, got %v", err)
	}
}

func TestResolveEntSchemasUsesScalarSetterForNillableEdgeField(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

import "context"

type User struct {
	ID          int  `+"`json:\"id,omitempty\"`"+`
	CompanyUUID *int `+"`json:\"company_uuid,omitempty\"`"+`
}

type UserCreate struct{}

func (*UserCreate) SetCompanyUUID(int) {}
func (*UserCreate) Save(context.Context) (*User, error) { return &User{}, nil }

type UserClient struct{}

func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int) *UserDelete { return &UserDelete{} }

type UserDelete struct{}

func (*UserDelete) Exec(context.Context) error { return nil }
`)

	resolved, err := ResolveEntSchemas(schemaDir, importPath, []EntSchema{{
		Name: "User",
		Fields: []EntField{{
			Name:     "company_uuid",
			JSONName: "company_uuid",
			Type:     "Int",
			GoType:   "*int",
			Optional: true,
			Nillable: true,
		}},
	}})
	if err != nil {
		t.Fatalf("ResolveEntSchemas: %v", err)
	}
	field := resolved[0].Fields[0]
	if field.SetterName != "SetCompanyUUID" || !field.SetterDeref {
		t.Fatalf("resolved setter = %+v, want dereferenced SetCompanyUUID", field)
	}

	var output bytes.Buffer
	if err := GenerateEnt(&output, "blueprints", importPath, resolved); err != nil {
		t.Fatalf("GenerateEnt: %v", err)
	}
	for _, want := range []string{
		`if v.CompanyUUID != nil {`,
		`builder.SetCompanyUUID(*v.CompanyUUID)`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("generated output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestResolveEntSchemasRejectsSaveSignatureMismatch(t *testing.T) {
	schemaDir, importPath := writeEntSignatureModule(t, `package generated

import "context"

type User struct {
	ID   int    `+"`json:\"id,omitempty\"`"+`
	Name string `+"`json:\"name,omitempty\"`"+`
}

type UserCreate struct{}

func (*UserCreate) SetName(string) {}
func (*UserCreate) Save(context.Context) (User, error) { return User{}, nil }

type UserClient struct{}

func (*UserClient) Create() *UserCreate { return &UserCreate{} }
func (*UserClient) DeleteOneID(int) *UserDelete { return &UserDelete{} }

type UserDelete struct{}

func (*UserDelete) Exec(context.Context) error { return nil }
`)

	_, err := ResolveEntSchemas(schemaDir, importPath, []EntSchema{{
		Name: "User",
		Fields: []EntField{{
			Name:     "name",
			JSONName: "name",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "UserCreate.Save result 0 has type") {
		t.Fatalf("expected Save signature mismatch error, got %v", err)
	}
}

func TestResolveEntSchemasReportsPackageLoadErrors(t *testing.T) {
	schemaDir, _ := writeEntSignatureModule(t, "package generated\n")

	_, err := ResolveEntSchemas(schemaDir, "example.invalid/missing/ent", []EntSchema{{Name: "User"}})
	if err == nil || !strings.Contains(err.Error(), `load generated Ent package "example.invalid/missing/ent"`) {
		t.Fatalf("expected package load error, got %v", err)
	}
}

func writeEntSignatureModule(t *testing.T, generatedSource string) (string, string) {
	t.Helper()
	root := t.TempDir()
	schemaDir := filepath.Join(root, "schema")
	generatedDir := filepath.Join(root, "generated")
	for _, dir := range []string{schemaDir, generatedDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/entfixture\n\ngo 1.26.0\n"), 0o600); err != nil {
		t.Fatalf("write fixture go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "ent.go"), []byte(generatedSource), 0o600); err != nil {
		t.Fatalf("write generated fixture: %v", err)
	}
	return schemaDir, "example.com/entfixture/generated"
}

func findEntSchema(t *testing.T, schemas []EntSchema, name string) EntSchema {
	t.Helper()
	for _, schema := range schemas {
		if schema.Name == name {
			return schema
		}
	}
	t.Fatalf("Ent schema %q not found", name)
	return EntSchema{}
}

func findEntField(t *testing.T, fields []EntField, name string) EntField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("Ent field %q not found", name)
	return EntField{}
}
