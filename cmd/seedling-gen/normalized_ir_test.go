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
