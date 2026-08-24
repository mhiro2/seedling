package main

import (
	"bytes"
	"go/parser"
	"go/token"
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
	}{
		{
			name: "invalid field name",
			model: normalizedModel{
				TypeExpr: "User",
				Fields:   []normalizedField{{GoName: "ID; var x = 1", GoType: "int64"}},
			},
		},
		{
			name: "type expression breakout",
			model: normalizedModel{
				TypeExpr: "User; func init() {}",
			},
		},
		{
			name: "zero-value expression breakout",
			model: normalizedModel{
				TypeExpr:      "User",
				ZeroValueExpr: "User{}; evil()",
			},
		},
		{
			name:        "invalid struct name when emitting structs",
			emitStructs: true,
			model: normalizedModel{
				StructName: "1Bad",
				TypeExpr:   "User",
			},
		},
		{
			name:        "invalid field type when emitting structs",
			emitStructs: true,
			model: normalizedModel{
				StructName: "User",
				TypeExpr:   "User",
				Fields:     []normalizedField{{GoName: "ID", GoType: "int64) struct{}; var _ = func("}},
			},
		},
		{
			name: "duplicate relation names",
			model: normalizedModel{
				TypeExpr: "User",
				Relations: []normalizedRelation{
					{Name: "company", LocalFields: []string{"CompanyID"}},
					{Name: "company", LocalFields: []string{"BillingCompanyID"}},
				},
			},
		},
		{
			name: "empty relation name",
			model: normalizedModel{
				TypeExpr: "User",
				Relations: []normalizedRelation{
					{LocalFields: []string{"CompanyID"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := validateNormalizedModels([]normalizedModel{tt.model}, tt.emitStructs)

			// Assert
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
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

func TestValidateNormalizedModels_AcceptsValidModels(t *testing.T) {
	models := []normalizedModel{
		{
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
