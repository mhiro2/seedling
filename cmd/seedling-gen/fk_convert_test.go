package main

import (
	"strings"
	"testing"
)

func TestFKConverterExpr(t *testing.T) {
	imports := map[string]string{
		"sql":    sqlImportPath,
		"pgtype": pgtypeImportPath,
		"uuid":   uuidImportPath,
		"pg":     pgtypeImportPath,
	}
	tests := []struct {
		name       string
		parentType string
		childType  string
		wantExpr   string
		wantImport string
	}{
		{
			name:       "sql.NullInt64",
			parentType: "int64",
			childType:  "sql.NullInt64",
			wantExpr:   "seedling.ConvertFK(func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} })",
			wantImport: `"database/sql"`,
		},
		{
			name:       "integer widening",
			parentType: "int32",
			childType:  "sql.NullInt64",
			wantExpr:   "seedling.ConvertFK(func(v int32) sql.NullInt64 { return sql.NullInt64{Int64: int64(v), Valid: true} })",
			wantImport: `"database/sql"`,
		},
		{
			name:       "sql.Null generic",
			parentType: "string",
			childType:  "sql.Null[string]",
			wantExpr:   "seedling.ConvertFK(func(v string) sql.Null[string] { return sql.Null[string]{V: v, Valid: true} })",
			wantImport: `"database/sql"`,
		},
		{
			name:       "pgtype.Int8",
			parentType: "int64",
			childType:  "pgtype.Int8",
			wantExpr:   "seedling.ConvertFK(func(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} })",
			wantImport: `"github.com/jackc/pgx/v5/pgtype"`,
		},
		{
			name:       "aliased pgtype",
			parentType: "string",
			childType:  "pg.Text",
			wantExpr:   "seedling.ConvertFK(func(v string) pg.Text { return pg.Text{String: v, Valid: true} })",
			wantImport: `pg "github.com/jackc/pgx/v5/pgtype"`,
		},
		{
			name:       "pgtype.UUID from uuid.UUID",
			parentType: "uuid.UUID",
			childType:  "pgtype.UUID",
			wantExpr:   "seedling.ConvertFK(func(v [16]byte) pgtype.UUID { return pgtype.UUID{Bytes: v, Valid: true} })",
			wantImport: `"github.com/jackc/pgx/v5/pgtype"`,
		},
		{
			name:       "uuid.NullUUID",
			parentType: "uuid.UUID",
			childType:  "uuid.NullUUID",
			wantExpr:   "seedling.ConvertFK(func(v [16]byte) uuid.NullUUID { return uuid.NullUUID{UUID: uuid.UUID(v), Valid: true} })",
			wantImport: `"github.com/google/uuid"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			expr, gotImports, ok := fkConverterExpr(tt.parentType, imports, tt.childType, imports)

			// Assert
			if !ok {
				t.Fatal("fkConverterExpr found no conversion")
			}
			if expr != tt.wantExpr {
				t.Fatalf("expr = %s\nwant   %s", expr, tt.wantExpr)
			}
			if len(gotImports) != 1 || gotImports[0] != tt.wantImport {
				t.Fatalf("imports = %v, want [%s]", gotImports, tt.wantImport)
			}
		})
	}
}

func TestFKConverterExpr_SkipsUnknownConversions(t *testing.T) {
	imports := map[string]string{"sql": sqlImportPath, "pgtype": pgtypeImportPath, "ids": "example.com/ids"}
	tests := []struct {
		name       string
		parentType string
		childType  string
	}{
		{name: "string into integer wrapper", parentType: "string", childType: "sql.NullInt64"},
		{name: "unknown package", parentType: "int64", childType: "ids.UserID"},
		{name: "unimported qualifier", parentType: "int64", childType: "null.Int"},
		{name: "model-package type argument", parentType: "Status", childType: "sql.Null[Status]"},
		{name: "unqualified named type", parentType: "int64", childType: "UserID"},
		{name: "non-uuid into uuid wrapper", parentType: "string", childType: "pgtype.UUID"},
		{name: "narrowing integer", parentType: "int64", childType: "sql.NullInt16"},
		{name: "unsigned into same-width signed", parentType: "uint64", childType: "sql.NullInt64"},
		{name: "platform unsigned into int64", parentType: "uint", childType: "pgtype.Int8"},
		{name: "signed into unsigned", parentType: "int8", childType: "sql.NullByte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, _, ok := fkConverterExpr(tt.parentType, imports, tt.childType, imports)

			// Assert
			if ok {
				t.Fatal("fkConverterExpr found a conversion, want none")
			}
		})
	}
}

func TestWithFKConverters_ResolvesQualifiersPerModelFile(t *testing.T) {
	// Arrange
	parent := normalizedModel{
		BlueprintID: "team",
		PKFields:    []string{"ID"},
		Fields:      []normalizedField{{GoName: "ID", GoType: "int64"}},
	}
	child := func(imports map[string]string) normalizedModel {
		return normalizedModel{
			BlueprintID:   "player",
			Fields:        []normalizedField{{GoName: "TeamID", GoType: "sql.NullInt64"}},
			Relations:     []normalizedRelation{{Name: "team", LocalField: "TeamID", LocalFields: []string{"TeamID"}, RefBlueprint: "team"}},
			SourceImports: imports,
		}
	}

	// Act
	models := withFKConverters([]normalizedModel{
		parent,
		child(map[string]string{"sql": sqlImportPath}),
	})
	shadowed := withFKConverters([]normalizedModel{
		{BlueprintID: parent.BlueprintID, PKFields: parent.PKFields, Fields: parent.Fields, SourceImports: map[string]string{"sql": sqlImportPath}},
		child(map[string]string{"sql": "example.com/fake/sql"}),
	})

	// Assert
	if got := models[1].Relations[0].Converters; len(got) != 1 || got[0].Field != "TeamID" {
		t.Fatalf("converters = %+v, want one for TeamID", got)
	}
	if got := shadowed[1].Relations[0].Converters; len(got) != 0 {
		t.Fatalf("converters = %+v, want none when the child file binds sql to another package", got)
	}
}

func TestGenerateSqlc_ConvertsNullableForeignKeys(t *testing.T) {
	// Arrange
	tables, err := ParseSchema(`
CREATE TABLE teams (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL
);
CREATE TABLE players (
    id BIGINT PRIMARY KEY,
    team_id BIGINT REFERENCES teams(id)
);
CREATE TABLE managers (
    id BIGINT PRIMARY KEY,
    team_id BIGINT REFERENCES teams(id)
);`)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	info, err := ParseSqlcDir("testsqlcnull")
	if err != nil {
		t.Fatalf("ParseSqlcDir: %v", err)
	}

	// Act
	var buf strings.Builder
	if err := GenerateSqlc(&buf, "compile", "github.com/mhiro2/seedling/cmd/seedling-gen/testsqlcnull", tables, info); err != nil {
		t.Fatalf("GenerateSqlc: %v", err)
	}

	// Assert
	src := buf.String()
	for _, want := range []string{
		`"database/sql"`,
		`"TeamID": seedling.ConvertFK(func(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} })`,
		`"TeamID": seedling.ConvertFK(func(v int64) sql.Null[int64] { return sql.Null[int64]{V: v, Valid: true} })`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated code does not contain %s:\n%s", want, src)
		}
	}
	ensureCompilesAndRuns(t, "sqlc_nullable_fk", src, sqlcNullableFKTest)
}

const sqlcNullableFKTest = `package compile

import (
	"database/sql"
	"testing"

	"github.com/mhiro2/seedling"
	testsqlcnull "github.com/mhiro2/seedling/cmd/seedling-gen/testsqlcnull"
)

func TestGeneratedNullableForeignKeys(t *testing.T) {
	reg := seedling.NewRegistry()
	NewBlueprints().MustRegister(reg)
	recorder := &testsqlcnull.Recorder{}

	player := seedling.NewSession[testsqlcnull.Player](reg).InsertOne(t, recorder, seedling.Ref("team")).Root()
	if want := (sql.NullInt64{Int64: 1, Valid: true}); player.TeamID != want {
		t.Fatalf("player TeamID = %+v, want %+v", player.TeamID, want)
	}
	manager := seedling.NewSession[testsqlcnull.Manager](reg).InsertOne(t, recorder, seedling.Ref("team")).Root()
	if want := (sql.Null[int64]{V: 3, Valid: true}); manager.TeamID != want {
		t.Fatalf("manager.TeamID = %+v, want %+v", manager.TeamID, want)
	}
	orphan := seedling.NewSession[testsqlcnull.Player](reg).InsertOne(t, recorder).Root()
	if orphan.TeamID.Valid {
		t.Fatalf("player without a team has TeamID %+v, want NULL", orphan.TeamID)
	}
	teams := seedling.NewSession[testsqlcnull.Team](reg).InsertOne(t, recorder, seedling.Ref("players", seedling.Count(2)))
	for _, node := range teams.Nodes("player") {
		if got := node.Value().(testsqlcnull.Player).TeamID; got != (sql.NullInt64{Int64: teams.Root().ID, Valid: true}) {
			t.Fatalf("has-many player TeamID = %+v, want team %d", got, teams.Root().ID)
		}
	}
}
`

func TestGenerateGorm_ConvertsNullableForeignKeys(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "models.go", `package models

import (
	"database/sql"

	"gorm.io/gorm"
)

type Company struct {
	ID   int32 `+"`"+`gorm:"primaryKey"`+"`"+`
	Name string
}

type User struct {
	gorm.Model
	Name      string
	CompanyID sql.NullInt64
	Company   Company `+"`"+`gorm:"foreignKey:CompanyID"`+"`"+`
}
`)
	models, err := ParseGormDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	var buf strings.Builder
	if err := GenerateGorm(&buf, "fixtures", "example.com/app/models", models); err != nil {
		t.Fatalf("GenerateGorm: %v", err)
	}

	// Assert
	src := buf.String()
	for _, want := range []string{
		`"database/sql"`,
		`"CompanyID": seedling.ConvertFK(func(v int32) sql.NullInt64 { return sql.NullInt64{Int64: int64(v), Valid: true} })`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated code does not contain %s:\n%s", want, src)
		}
	}
}
