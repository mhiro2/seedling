package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestGenerateNormalizedCode_QuotesStringPositions feeds a relation whose
// RefBlueprint tries to break out of its string literal (the demonstrated
// codegen injection) and verifies the value is escaped into a single Go string
// literal rather than smuggled in as code.
func TestGenerateNormalizedCode_QuotesStringPositions(t *testing.T) {
	// Arrange
	const injected = `x",Optional:true,RefBlueprint:"y`
	models := []normalizedModel{
		{
			FieldName:     "User",
			StructName:    "User",
			TypeExpr:      "User",
			ZeroValueExpr: "User{}",
			BlueprintID:   "user",
			TableName:     `users",Table:"evil`,
			PKFields:      []string{"ID"},
			Fields: []normalizedField{
				{GoName: "ID", GoType: "int64", IsPK: true},
			},
			Relations: []normalizedRelation{
				{
					Name:         "company",
					LocalField:   "CompanyID",
					LocalFields:  []string{"CompanyID"},
					RefBlueprint: injected,
				},
			},
			InsertHook: &normalizedMutationHook{Body: "return v, nil"},
		},
	}

	// Act
	var buf bytes.Buffer
	err := generateNormalizedCode(&buf, "test", "mypkg", []string{`seedling "github.com/mhiro2/seedling"`}, models, false)
	// Assert
	if err != nil {
		t.Fatalf("generateNormalizedCode error: %v", err)
	}

	output := buf.String()
	if _, perr := parser.ParseFile(token.NewFileSet(), "out.go", output, parser.AllErrors); perr != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", perr, output)
	}
	if !strings.Contains(output, strconv.Quote(injected)) {
		t.Fatalf("RefBlueprint was not safely quoted, got:\n%s", output)
	}
	if !strings.Contains(output, strconv.Quote(`users",Table:"evil`)) {
		t.Fatalf("Table was not safely quoted, got:\n%s", output)
	}
}

func TestValidateNormalizedModels_RejectsInvalidPositions(t *testing.T) {
	tests := []struct {
		name        string
		emitStructs bool
		model       normalizedModel
		want        string
	}{
		{
			name: "invalid field name",
			want: "invalid field name",
			model: normalizedModel{
				FieldName: "User",
				TypeExpr:  "User",
				Fields:    []normalizedField{{GoName: "ID; var x = 1", GoType: "int64"}},
			},
		},
		{
			name: "type expression breakout",
			want: "invalid type expression",
			model: normalizedModel{
				FieldName: "User",
				TypeExpr:  "User; func init() {}",
			},
		},
		{
			name: "zero-value expression breakout",
			want: "invalid zero-value expression",
			model: normalizedModel{
				FieldName:     "User",
				TypeExpr:      "User",
				ZeroValueExpr: "User{}; evil()",
			},
		},
		{
			name:        "invalid struct name when emitting structs",
			want:        "invalid struct name",
			emitStructs: true,
			model: normalizedModel{
				FieldName:  "User",
				StructName: "1Bad",
				TypeExpr:   "User",
			},
		},
		{
			name:        "invalid field type when emitting structs",
			want:        "invalid field type",
			emitStructs: true,
			model: normalizedModel{
				FieldName:  "User",
				StructName: "User",
				TypeExpr:   "User",
				Fields:     []normalizedField{{GoName: "ID", GoType: "int64) struct{}; var _ = func("}},
			},
		},
		{
			name: "invalid embedded field name",
			want: "invalid embedded field name",
			model: normalizedModel{
				FieldName: "User",
				TypeExpr:  "User",
				Fields: []normalizedField{{GoName: "CreatedAt", GoType: "time.Time", Embedded: &embeddedStruct{
					FieldName: "Model: evil()", TypeExpr: "gorm.Model",
				}}},
			},
		},
		{
			name: "embedded type breakout",
			want: "invalid embedded type",
			model: normalizedModel{
				FieldName: "User",
				TypeExpr:  "User",
				Fields: []normalizedField{{GoName: "CreatedAt", GoType: "time.Time", Embedded: &embeddedStruct{
					FieldName: "Model", TypeExpr: "gorm.Model{}; evil()",
				}}},
			},
		},
		{
			name: "duplicate relation names",
			want: "duplicate relation name",
			model: normalizedModel{
				FieldName: "User",
				TypeExpr:  "User",
				Relations: []normalizedRelation{
					{Name: "company", LocalFields: []string{"CompanyID"}},
					{Name: "company", LocalFields: []string{"BillingCompanyID"}},
				},
			},
		},
		{
			name: "empty relation name",
			want: "relation name must not be empty",
			model: normalizedModel{
				FieldName: "User",
				TypeExpr:  "User",
				Relations: []normalizedRelation{
					{LocalFields: []string{"CompanyID"}},
				},
			},
		},
		{
			name:  "invalid Blueprints field name",
			model: normalizedModel{FieldName: "user-model", TypeExpr: "User"},
			want:  "invalid Blueprints field name",
		},
		{
			name:  "Blueprints field named after a generated method",
			model: normalizedModel{FieldName: "Register", TypeExpr: "Register"},
			want:  "collides with a generated method",
		},
		{
			name:        "struct named after a generated declaration",
			emitStructs: true,
			model:       normalizedModel{FieldName: "Blueprints", StructName: "Blueprints", TypeExpr: "Blueprints"},
			want:        "collides with a generated declaration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := validateNormalizedModels([]normalizedModel{tt.model}, tt.emitStructs)

			// Assert
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestValidateNormalizedModels_RejectsDuplicateBlueprintsFieldNames(t *testing.T) {
	// Arrange
	models := []normalizedModel{
		{FieldName: "User", TypeExpr: "models.User"},
		{FieldName: "User", TypeExpr: "legacy.User"},
	}

	// Act
	err := validateNormalizedModels(models, false)

	// Assert
	if err == nil || !strings.Contains(err.Error(), `duplicate Blueprints field name "User"`) {
		t.Fatalf("validation error = %v, want duplicate Blueprints field name", err)
	}
}

func TestImportSpec_QuotesPathAndValidatesAlias(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		path    string
		want    string
		wantErr bool
	}{
		{name: "no alias", path: "example.com/m", want: `"example.com/m"`},
		{name: "valid alias", alias: "models", path: "example.com/m", want: `models "example.com/m"`},
		{name: "path with quote is escaped", alias: "m", path: `x";import _ "evil`, want: `m ` + strconv.Quote(`x";import _ "evil`)},
		{name: "invalid alias is rejected", alias: "go-models", path: "example.com/m", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := importSpec(tt.alias, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderImports_SeparatesStandardLibrary(t *testing.T) {
	// Arrange
	imports := []string{`"github.com/mhiro2/seedling"`, `"errors"`, `db "example.com/app/db"`, `"context"`}

	// Act
	var buf strings.Builder
	renderImports(&buf, imports)

	// Assert
	want := "import (\n\t\"errors\"\n\t\"context\"\n\n\t\"github.com/mhiro2/seedling\"\n\tdb \"example.com/app/db\"\n)\n\n"
	if got := buf.String(); got != want {
		t.Fatalf("renderImports =\n%s\nwant\n%s", got, want)
	}
}

func TestGenerateSqlc_RejectsPackageNamedLikeStandardImport(t *testing.T) {
	// Arrange: the generated Register uses the standard library errors package.
	tables := mustParseSchema(t, `CREATE TABLE tags (id SERIAL PRIMARY KEY);`)
	sqlcInfo := &SqlcInfo{
		Package: "errors",
		Models:  []SqlcModel{{Name: "Tag", Fields: []SqlcField{{Name: "ID", Type: "int32"}}}},
	}

	// Act
	var buf bytes.Buffer
	err := GenerateSqlc(&buf, "testutil", "example.com/app/errors", tables, sqlcInfo)

	// Assert
	if err == nil || !strings.Contains(err.Error(), `both import as "errors"`) {
		t.Fatalf("GenerateSqlc error = %v, want an import name collision", err)
	}
}

func TestValidateNormalizedModels_AcceptsValidModels(t *testing.T) {
	models := []normalizedModel{
		{
			FieldName:     "User",
			StructName:    "User",
			TypeExpr:      "User",
			ZeroValueExpr: "User{}",
			Fields: []normalizedField{
				{GoName: "ID", GoType: "int64"},
				{GoName: "CreatedAt", GoType: "time.Time"},
				{GoName: "Data", GoType: "[]byte"},
				{GoName: "Owner", GoType: "*ent.User"},
			},
		},
	}

	if err := validateNormalizedModels(models, true); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestNormalizeTableRelations_DistinguishesCompositeForeignKeysToSameTable(t *testing.T) {
	table := Table{
		Name: "routes",
		Columns: []Column{
			{Name: "billing_country_code", GoName: "BillingCountryCode"},
			{Name: "billing_region_code", GoName: "BillingRegionCode"},
			{Name: "shipping_country_code", GoName: "ShippingCountryCode"},
			{Name: "shipping_region_code", GoName: "ShippingRegionCode"},
		},
		ForeignKeys: []ForeignKey{
			{
				Columns:    []string{"billing_country_code", "billing_region_code"},
				RefTable:   "regions",
				RefColumns: []string{"country_code", "region_code"},
			},
			{
				Columns:    []string{"shipping_country_code", "shipping_region_code"},
				RefTable:   "regions",
				RefColumns: []string{"country_code", "region_code"},
			},
		},
	}

	relations := normalizeTableRelations(table)
	if len(relations) != 2 {
		t.Fatalf("relations = %d, want 2", len(relations))
	}
	if relations[0].Name != "billing" || relations[1].Name != "shipping" {
		t.Fatalf("relation names = [%s %s], want [billing shipping]", relations[0].Name, relations[1].Name)
	}
}

func TestNormalizeTableRelations_NamesUnqualifiedCompositeKeyAfterReferencedTable(t *testing.T) {
	// Recording referenced columns must not rename a relation that a caller
	// already addresses through Use()/Ref().
	table := Table{
		Name: "posts",
		Columns: []Column{
			{Name: "tenant_id", GoName: "TenantID"},
			{Name: "user_id", GoName: "UserID"},
		},
		ForeignKeys: []ForeignKey{
			{
				Columns:    []string{"tenant_id", "user_id"},
				RefTable:   "users",
				RefColumns: []string{"tenant_id", "id"},
			},
		},
	}

	relations := normalizeTableRelations(table)
	if len(relations) != 1 {
		t.Fatalf("relations = %d, want 1", len(relations))
	}
	if relations[0].Name != "user" {
		t.Fatalf("relation name = %q, want %q", relations[0].Name, "user")
	}
}

func TestNormalizeTableRelations_FallsBackToColumnsOnlyOnCollision(t *testing.T) {
	table := Table{
		Name: "posts",
		Columns: []Column{
			{Name: "tenant_id", GoName: "TenantID"},
			{Name: "user_id", GoName: "UserID"},
			{Name: "reviewer_tenant", GoName: "ReviewerTenant"},
			{Name: "reviewer_user", GoName: "ReviewerUser"},
		},
		ForeignKeys: []ForeignKey{
			{
				Columns:    []string{"tenant_id", "user_id"},
				RefTable:   "users",
				RefColumns: []string{"tenant_id", "id"},
			},
			{
				// Neither key strips its referenced column names, so both want
				// to be called "user" and both have to be disambiguated.
				Columns:    []string{"reviewer_tenant", "reviewer_user"},
				RefTable:   "users",
				RefColumns: []string{"tenant_id", "id"},
			},
		},
	}

	relations := normalizeTableRelations(table)
	if len(relations) != 2 {
		t.Fatalf("relations = %d, want 2", len(relations))
	}
	if relations[0].Name != "tenant_id_user_id" || relations[1].Name != "reviewer" {
		t.Fatalf("relation names = [%s %s], want [tenant_id_user_id reviewer]", relations[0].Name, relations[1].Name)
	}
}

func TestGenerate_EmitsNonPrimaryReferencedField(t *testing.T) {
	tables, err := ParseSchema(`
CREATE TABLE countries (
  id INT PRIMARY KEY,
  code TEXT UNIQUE NOT NULL
);
CREATE TABLE cities (
  id INT PRIMARY KEY,
  country_code TEXT NOT NULL REFERENCES countries(code)
);`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Generate(&buf, "testutil", tables); err != nil {
		t.Fatal(err)
	}
	if output := buf.String(); !strings.Contains(output, `LocalField: "CountryCode", RefField: "Code"`) {
		t.Fatalf("expected non-primary reference mapping, got:\n%s", output)
	}
}

func TestWithReverseRelations(t *testing.T) {
	// Arrange
	models := []normalizedModel{
		{BlueprintID: "user", Relations: []normalizedRelation{
			{Name: "team", LocalField: "TeamID", LocalFields: []string{"TeamID"}, RefBlueprint: "external", Optional: true},
		}},
		{BlueprintID: "project"},
		{BlueprintID: "task", Relations: []normalizedRelation{
			{Name: "project", LocalField: "ProjectID", LocalFields: []string{"ProjectID"}, RefField: "ID", RefFields: []string{"ID"}, RefBlueprint: "project"},
			{Name: "assignee", LocalField: "AssigneeID", LocalFields: []string{"AssigneeID"}, RefBlueprint: "user"},
			{Name: "owner", LocalField: "OwnerID", LocalFields: []string{"OwnerID"}, RefBlueprint: "external"},
		}},
	}

	// Act
	got := withReverseRelations(models)

	// Assert
	if len(models[1].Relations) != 0 {
		t.Fatalf("input models were modified: %+v", models[1].Relations)
	}
	project := got[1].Relations
	if len(project) != 1 {
		t.Fatalf("got project relations %+v, want one has-many", project)
	}
	if want := (normalizedRelation{HasMany: true, Name: "tasks", LocalField: "ProjectID", LocalFields: []string{"ProjectID"}, RefField: "ID", RefFields: []string{"ID"}, RefBlueprint: "task", Optional: true}); !reflect.DeepEqual(project[0], want) {
		t.Fatalf("got %+v, want %+v", project[0], want)
	}
	user := got[0].Relations
	if len(user) != 2 || user[1].Name != "assignee_tasks" || !user[1].HasMany || user[1].RefBlueprint != "task" {
		t.Fatalf("got user relations %+v, want the existing relation plus assignee_tasks", user)
	}
	if len(got[2].Relations) != 3 {
		t.Fatalf("got task relations %+v, want the belongs-to relations unchanged", got[2].Relations)
	}
}

func TestWithReverseRelations_SkipsTakenName(t *testing.T) {
	// Arrange
	models := []normalizedModel{
		{BlueprintID: "company", Relations: []normalizedRelation{
			{Name: "users", LocalField: "UsersID", LocalFields: []string{"UsersID"}, RefBlueprint: "user"},
		}},
		{BlueprintID: "user", Relations: []normalizedRelation{
			{Name: "company", LocalField: "CompanyID", LocalFields: []string{"CompanyID"}, RefBlueprint: "company"},
		}},
	}

	// Act
	got := withReverseRelations(models)

	// Assert
	if len(got[0].Relations) != 1 || got[0].Relations[0].HasMany {
		t.Fatalf("got company relations %+v, want only the existing users relation", got[0].Relations)
	}
}
