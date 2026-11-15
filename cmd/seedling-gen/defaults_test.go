package main

import (
	"strings"
	"testing"
)

func TestBuildDefaultLiteral_FillsSupportedScalarFields(t *testing.T) {
	// Arrange
	model := normalizedModel{
		ZeroValueExpr: "User{}",
		BlueprintID:   "user",
		Fields: []normalizedField{
			{GoName: "ID", GoType: "int64", IsPK: true},
			{GoName: "Name", GoType: "string"},
			{GoName: "Active", GoType: "bool"},
			{GoName: "Score", GoType: "float64"},
			{GoName: "Blob", GoType: "[]byte"},
			{GoName: "CreatedAt", GoType: "time.Time"},
			{GoName: "CompanyID", GoType: "int64", IsRelationFK: true},
			{GoName: "Token", GoType: "uuid.UUID"},
		},
	}

	// Act
	got := buildDefaultLiteral(model, "scope")

	// Assert
	tests := []struct {
		name    string
		substr  string
		missing bool
	}{
		{name: "string field", substr: `Name: "user-name"`},
		{name: "bool field", substr: `Active: true`},
		{name: "float field", substr: `Score: 1`},
		{name: "byte slice field", substr: `Blob: []byte("user-blob")`},
		{name: "time field", substr: `CreatedAt: scope.Now()`},
		{name: "primary key skipped", substr: `ID:`, missing: true},
		{name: "relation foreign key skipped", substr: `CompanyID:`, missing: true},
		{name: "unsupported type skipped", substr: `Token:`, missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contains := strings.Contains(got, tt.substr)
			if tt.missing && contains {
				t.Fatalf("got %q, expected substring %q to be absent", got, tt.substr)
			}
			if !tt.missing && !contains {
				t.Fatalf("got %q, expected substring %q", got, tt.substr)
			}
		})
	}
}

func TestBuildDefaultLiteral_FallsBackToZeroValueWhenNothingIsFillable(t *testing.T) {
	// Arrange
	model := normalizedModel{
		ZeroValueExpr: "&ent.Company{}",
		BlueprintID:   "company",
		Fields: []normalizedField{
			{GoName: "ID", GoType: "int64", IsPK: true},
			{GoName: "Token", GoType: "uuid.UUID"},
		},
	}

	// Act
	got := buildDefaultLiteral(model, "scope")

	// Assert
	if got != "&ent.Company{}" {
		t.Fatalf("got %q, want %q", got, "&ent.Company{}")
	}
}

func TestNormalizedModelsNeedTimeImport(t *testing.T) {
	tests := []struct {
		name        string
		models      []normalizedModel
		emitStructs bool
		want        bool
	}{
		{
			name: "without time fields",
			models: []normalizedModel{
				{Fields: []normalizedField{{GoName: "Name", GoType: "string"}}},
			},
			want: false,
		},
		{
			name: "time default drawn from the scope",
			models: []normalizedModel{
				{Fields: []normalizedField{{GoName: "CreatedAt", GoType: "time.Time"}}},
			},
			want: false,
		},
		{
			name: "time key on existing model type",
			models: []normalizedModel{
				{Fields: []normalizedField{
					{GoName: "ID", GoType: "time.Time", IsPK: true},
					{GoName: "ParentID", GoType: "time.Time", IsRelationFK: true},
				}},
			},
			want: false,
		},
		{
			name: "time key on emitted struct",
			models: []normalizedModel{
				{Fields: []normalizedField{{GoName: "ID", GoType: "time.Time", IsPK: true}}},
			},
			emitStructs: true,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := normalizedModelsNeedTimeImport(tt.models, tt.emitStructs)

			// Assert
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultFieldExpr_RespectsColumnConstraints(t *testing.T) {
	sqlcModel := normalizedModel{
		BlueprintID:   "user",
		TypeExpr:      "db.User",
		SourceImports: map[string]string{"json": "encoding/json"},
	}
	tests := []struct {
		name        string
		field       normalizedField
		want        string
		wantImports []string
	}{
		{name: "enum string", field: normalizedField{GoName: "Status", GoType: "string", EnumValues: []string{"draft", "active"}}, want: `"draft"`},
		{name: "enum named type", field: normalizedField{GoName: "Status", GoType: "UserStatus", EnumValues: []string{"draft"}}, want: `db.UserStatus("draft")`},
		{name: "enum of unknown package", field: normalizedField{GoName: "Status", GoType: "other.Status", EnumValues: []string{"draft"}}, want: ``},
		{name: "unique string", field: normalizedField{GoName: "Email", GoType: "string", Unique: true}, want: `scope.Unique("user-email")`},
		{name: "unique length-limited string", field: normalizedField{GoName: "Email", GoType: "string", Unique: true, MaxLength: 16}, want: `scope.UniqueN("user-email", 16)`},
		{name: "unique enum", field: normalizedField{GoName: "Status", GoType: "string", Unique: true, EnumValues: []string{"a", "b"}}, want: `[]string{"a", "b"}[(scope.Seq("user-status")-1)%2]`},
		{name: "nullable sqlc enum", field: normalizedField{GoName: "Status", GoType: "NullUserStatus", EnumValues: []string{"draft"}}, want: ``},
		{name: "unique time", field: normalizedField{GoName: "At", GoType: "time.Time", Unique: true}, want: `scope.Now().Add(time.Duration(scope.Seq("user-at")) * time.Second)`, wantImports: []string{`"time"`}},
		{name: "unique bytes", field: normalizedField{GoName: "Token", GoType: "[]byte", Unique: true}, want: `[]byte(scope.Unique("user-token"))`},
		{name: "unique int", field: normalizedField{GoName: "Number", GoType: "int", Unique: true}, want: `scope.Seq("user-number")`},
		{name: "unique int64", field: normalizedField{GoName: "Number", GoType: "int64", Unique: true}, want: `int64(scope.Seq("user-number"))`},
		{name: "length-limited string", field: normalizedField{GoName: "Nickname", GoType: "string", MaxLength: 6}, want: `"user-n"`},
		{name: "uuid string", field: normalizedField{GoName: "Token", GoType: "string", SQLType: "UUID"}, want: `scope.UUID()`},
		{name: "uuid typed", field: normalizedField{GoName: "Token", GoType: "uuid.UUID", SQLType: "UUID"}, want: ``},
		{name: "json string", field: normalizedField{GoName: "Meta", GoType: "string", SQLType: "JSONB"}, want: `"{}"`},
		{name: "json bytes", field: normalizedField{GoName: "Meta", GoType: "[]byte", SQLType: "JSON"}, want: `[]byte("{}")`},
		{name: "json raw message", field: normalizedField{GoName: "Meta", GoType: "json.RawMessage", SQLType: "JSONB"}, want: `json.RawMessage("{}")`, wantImports: []string{`"encoding/json"`}},
		{name: "numeric string", field: normalizedField{GoName: "Amount", GoType: "string", SQLType: "NUMERIC"}, want: `"1"`},
		{name: "unique decimal string", field: normalizedField{GoName: "Amount", GoType: "string", SQLType: "DECIMAL", Unique: true}, want: `fmt.Sprintf("%d", scope.Seq("user-amount"))`, wantImports: []string{`"fmt"`}},
		{name: "money string", field: normalizedField{GoName: "Price", GoType: "string", SQLType: "MONEY"}, want: `"1"`},
		{name: "inet string", field: normalizedField{GoName: "IP", GoType: "string", SQLType: "INET"}, want: `"192.0.2.1"`},
		{name: "unique inet string", field: normalizedField{GoName: "IP", GoType: "string", SQLType: "INET", Unique: true}, want: `fmt.Sprintf("2001:db8::%x", scope.Seq("user-ip"))`, wantImports: []string{`"fmt"`}},
		{name: "cidr string", field: normalizedField{GoName: "Network", GoType: "string", SQLType: "CIDR"}, want: `"192.0.2.0/24"`},
		{name: "macaddr string", field: normalizedField{GoName: "Mac", GoType: "string", SQLType: "MACADDR"}, want: `"08:00:2b:01:02:03"`},
		{name: "unique macaddr string", field: normalizedField{GoName: "Mac", GoType: "string", SQLType: "MACADDR", Unique: true}, want: `fmt.Sprintf("08002b%06x", scope.Seq("user-mac"))`, wantImports: []string{`"fmt"`}},
		{name: "time string", field: normalizedField{GoName: "OpensAt", GoType: "string", SQLType: "TIME"}, want: `"12:00:00"`},
		{name: "unique time string", field: normalizedField{GoName: "OpensAt", GoType: "string", SQLType: "TIME", Unique: true}, want: `time.Time{}.Add(time.Duration(scope.Seq("user-opens_at")) * time.Second).Format("15:04:05")`, wantImports: []string{`"time"`}},
		{name: "date string", field: normalizedField{GoName: "Birthday", GoType: "string", SQLType: "DATE"}, want: `scope.Now().Format("2006-01-02")`},
		{name: "timestamptz string", field: normalizedField{GoName: "SeenAt", GoType: "string", SQLType: "TIMESTAMPTZ"}, want: `scope.Now().Format("2006-01-02 15:04:05-07:00")`},
		{name: "interval string", field: normalizedField{GoName: "Period", GoType: "string", SQLType: "INTERVAL"}, want: `"1 day"`},
		{name: "year string", field: normalizedField{GoName: "Vintage", GoType: "string", SQLType: "YEAR", Unique: true}, want: `fmt.Sprint(1900 + scope.Seq("user-vintage"))`, wantImports: []string{`"fmt"`}},
		{name: "xml string", field: normalizedField{GoName: "Doc", GoType: "string", SQLType: "XML"}, want: `"<seed/>"`},
		{name: "range string", field: normalizedField{GoName: "During", GoType: "string", SQLType: "TSRANGE"}, want: `"empty"`},
		{name: "bit string", field: normalizedField{GoName: "Flags", GoType: "string", SQLType: "BIT"}, want: ``},
		{name: "numeric float", field: normalizedField{GoName: "Amount", GoType: "float64", SQLType: "NUMERIC"}, want: `1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, imports := defaultFieldExpr(sqlcModel, tt.field, "scope")

			// Assert
			if got != tt.want {
				t.Fatalf("expr = %s, want %s", got, tt.want)
			}
			if strings.Join(imports, ",") != strings.Join(tt.wantImports, ",") {
				t.Fatalf("imports = %v, want %v", imports, tt.wantImports)
			}
		})
	}
}

func TestChooseScopeVar_AvoidsImportedNames(t *testing.T) {
	if got := chooseScopeVar([]string{`"github.com/mhiro2/seedling"`}); got != "scope" {
		t.Fatalf("got %q, want scope", got)
	}
	if got := chooseScopeVar([]string{`"example.com/scope"`}); got != "seedlingScope" {
		t.Fatalf("got %q, want seedlingScope", got)
	}
}
