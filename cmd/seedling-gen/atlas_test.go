package main

import (
	"bytes"
	"strings"
	"testing"
)

func mustParseAtlasHCL(t *testing.T, data string) []Table {
	t.Helper()
	tables, err := ParseAtlasHCL(data)
	if err != nil {
		t.Fatalf("ParseAtlasHCL error: %v", err)
	}
	return tables
}

func TestParseAtlasHCL_BasicTables(t *testing.T) {
	// Arrange
	hcl := `
table "companies" {
  schema = schema.public
  column "id" {
    type = int
    null = false
  }
  column "name" {
    type = varchar(255)
    null = false
  }
  primary_key {
    columns = [column.id]
  }
}

table "users" {
  schema = schema.public
  column "id" {
    type = int
    null = false
  }
  column "name" {
    type = varchar(255)
    null = false
  }
  column "company_id" {
    type = int
    null = false
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_company" {
    columns     = [column.company_id]
    ref_columns = [table.companies.column.id]
  }
}
`

	// Act
	tables := mustParseAtlasHCL(t, hcl)

	// Assert
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	if tables[0].Name != "companies" {
		t.Fatalf("expected table 'companies', got %q", tables[0].Name)
	}
	if tables[0].GoName != "Company" {
		t.Fatalf("expected GoName 'Company', got %q", tables[0].GoName)
	}
	if len(tables[0].Columns) != 2 {
		t.Fatalf("expected 2 columns on companies, got %d", len(tables[0].Columns))
	}

	if tables[1].Name != "users" {
		t.Fatalf("expected table 'users', got %q", tables[1].Name)
	}

	// Check primary key.
	var idCol Column
	for _, col := range tables[1].Columns {
		if col.Name == "id" {
			idCol = col
			break
		}
	}
	if !idCol.IsPK {
		t.Fatal("expected id to be primary key")
	}

	// Check foreign key.
	if len(tables[1].ForeignKeys) != 1 {
		t.Fatalf("expected 1 foreign key, got %d", len(tables[1].ForeignKeys))
	}
	fk := tables[1].ForeignKeys[0]
	if fk.RefTable != "companies" {
		t.Fatalf("expected ref table 'companies', got %q", fk.RefTable)
	}
	if len(fk.Columns) != 1 || fk.Columns[0] != "company_id" {
		t.Fatalf("expected FK column 'company_id', got %v", fk.Columns)
	}
}

func TestParseAtlasHCL_CompositePK(t *testing.T) {
	// Arrange
	hcl := `
table "article_tags" {
  column "article_id" {
    type = int
    null = false
  }
  column "tag_id" {
    type = int
    null = false
  }
  primary_key {
    columns = [column.article_id, column.tag_id]
  }
}
`

	// Act
	tables := mustParseAtlasHCL(t, hcl)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	pkCount := 0
	for _, col := range tables[0].Columns {
		if col.IsPK {
			pkCount++
		}
	}
	if pkCount != 2 {
		t.Fatalf("expected 2 PK columns, got %d", pkCount)
	}
}

func TestParseAtlasHCL_EmptyInput(t *testing.T) {
	// Act
	tables, err := ParseAtlasHCL("")
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tables != nil {
		t.Fatalf("expected nil tables, got %d", len(tables))
	}
}

func TestParseAtlasHCL_ColumnTypes(t *testing.T) {
	// Arrange
	hcl := `
table "items" {
  column "id" {
    type = bigint
  }
  column "name" {
    type = varchar(255)
  }
  column "active" {
    type = boolean
  }
  column "price" {
    type = numeric
  }
  column "created_at" {
    type = timestamp
  }
  primary_key {
    columns = [column.id]
  }
}
`

	// Act
	tables := mustParseAtlasHCL(t, hcl)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	tests := []struct {
		colName string
		goType  string
	}{
		{"id", "int64"},
		{"name", "string"},
		{"active", "bool"},
		{"price", "float64"},
		{"created_at", "time.Time"},
	}
	for _, tt := range tests {
		for _, col := range tables[0].Columns {
			if col.Name == tt.colName {
				if col.GoType != tt.goType {
					t.Errorf("column %q: expected GoType %q, got %q", tt.colName, tt.goType, col.GoType)
				}
				break
			}
		}
	}
}

func TestSplitAtlasColumnRefs(t *testing.T) {
	// Arrange
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single",
			input: "column.id",
			want:  []string{"id"},
		},
		{
			name:  "multiple",
			input: "column.article_id, column.tag_id",
			want:  []string{"article_id", "tag_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := splitAtlasColumnRefs(tt.input)

			// Assert
			if len(got) != len(tt.want) {
				t.Fatalf("got %d refs, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

func TestExtractAtlasRefTable(t *testing.T) {
	// Arrange
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple ref",
			input: "table.companies.column.id",
			want:  "companies",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act & Assert
			got := extractAtlasRefTable(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateAtlas_ViaGenerate(t *testing.T) {
	// Arrange
	hcl := `
table "tags" {
  column "id" {
    type = serial
    null = false
  }
  column "label" {
    type = text
    null = false
  }
  primary_key {
    columns = [column.id]
  }
}
`
	tables := mustParseAtlasHCL(t, hcl)

	// Act
	var buf bytes.Buffer
	if err := Generate(&buf, "testutil", tables); err != nil {
		t.Fatal(err)
	}

	// Assert
	output := buf.String()
	if !strings.Contains(output, "package testutil") {
		t.Fatalf("expected package testutil, got:\n%s", output)
	}
	if !strings.Contains(output, `Name:    "tag"`) {
		t.Fatalf("expected blueprint name 'tag', got:\n%s", output)
	}
	if !strings.Contains(output, `Table:   "tags"`) {
		t.Fatalf("expected table 'tags', got:\n%s", output)
	}
}

func TestParseAtlasHCL_UnclosedBrace(t *testing.T) {
	tests := []struct {
		name string
		hcl  string
		want string
	}{
		{
			name: "table brace",
			hcl: `table "users" {
  column "id" {
    type = int
  }
`,
			want: `parse table "users": unclosed brace`,
		},
		{
			name: "column brace",
			hcl: `table "users" {
  column "id" {
    type = int
}
`,
			want: `unclosed brace`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := ParseAtlasHCL(tt.hcl)

			// Assert
			if err == nil {
				t.Fatal("expected error for unclosed brace")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestRun_AtlasMode(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeFile(t, dir, "atlas.hcl", `
table "items" {
  column "id" {
    type = serial
    null = false
  }
  column "name" {
    type = text
    null = false
  }
  primary_key {
    columns = [column.id]
  }
}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{
		"atlas",
		"-pkg", "testutil",
		dir + "/atlas.hcl",
	}, &stdout, &stderr)

	// Assert
	if exitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", exitCode, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "package testutil") {
		t.Fatalf("expected package testutil, got:\n%s", output)
	}
	if !strings.Contains(output, `Name:    "item"`) {
		t.Fatalf("expected blueprint name 'item', got:\n%s", output)
	}
}

func TestStripHCLComments(t *testing.T) {
	// Arrange
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "hash comment",
			input: "# comment\ntable \"t\" {",
			want:  "\ntable \"t\" {",
		},
		{
			name:  "double-slash comment",
			input: "// comment\ntable \"t\" {",
			want:  "\ntable \"t\" {",
		},
		{
			name:  "comment inside string",
			input: `column "name" { default = "# not a comment" }`,
			want:  `column "name" { default = "# not a comment" }`,
		},
		{
			name:  "no comments",
			input: `table "t" { column "id" { type = int } }`,
			want:  `table "t" { column "id" { type = int } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act & Assert
			got := stripHCLComments(tt.input)
			if got != tt.want {
				t.Fatalf("stripHCLComments(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseAtlasHCL_MultipleFKs(t *testing.T) {
	// Arrange
	hcl := `
table "reviews" {
  column "id" {
    type = serial
    null = false
  }
  column "author_id" {
    type = int
    null = false
  }
  column "reviewer_id" {
    type = int
    null = true
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_author" {
    columns     = [column.author_id]
    ref_columns = [table.users.column.id]
  }
  foreign_key "fk_reviewer" {
    columns     = [column.reviewer_id]
    ref_columns = [table.users.column.id]
  }
}
`
	// Act
	tables := mustParseAtlasHCL(t, hcl)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].ForeignKeys) != 2 {
		t.Fatalf("expected 2 FKs, got %d", len(tables[0].ForeignKeys))
	}
	for _, fk := range tables[0].ForeignKeys {
		if fk.RefTable != "users" {
			t.Fatalf("expected FK ref %q, got %q", "users", fk.RefTable)
		}
	}
	if !tables[0].ForeignKeys[0].NotNull {
		t.Fatal("expected author FK to be NOT NULL")
	}
	if tables[0].ForeignKeys[1].NotNull {
		t.Fatal("expected reviewer FK to be nullable")
	}
}

func TestParseAtlasHCL_CompositeFK(t *testing.T) {
	// Arrange
	hcl := `
table "regions" {
  column "country_code" {
    type = text
    null = false
  }
  column "region_code" {
    type = text
    null = false
  }
  primary_key {
    columns = [column.country_code, column.region_code]
  }
}

table "deployments" {
  column "id" {
    type = serial
    null = false
  }
  column "region_country" {
    type = text
    null = false
  }
  column "region_code" {
    type = text
    null = false
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_region" {
    columns     = [column.region_country, column.region_code]
    ref_columns = [table.regions.column.country_code, table.regions.column.region_code]
  }
}
`
	// Act
	tables := mustParseAtlasHCL(t, hcl)

	// Assert
	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	deployments := tables[1]
	if len(deployments.ForeignKeys) != 1 {
		t.Fatalf("expected 1 FK, got %d", len(deployments.ForeignKeys))
	}
	fk := deployments.ForeignKeys[0]
	if fk.RefTable != "regions" {
		t.Fatalf("expected FK ref %q, got %q", "regions", fk.RefTable)
	}
	if len(fk.Columns) != 2 {
		t.Fatalf("expected 2 FK columns, got %d", len(fk.Columns))
	}
	if fk.Columns[0] != "region_country" || fk.Columns[1] != "region_code" {
		t.Fatalf("unexpected FK columns: %v", fk.Columns)
	}
}

func TestParseAtlasHCL_SelfReferencingFK(t *testing.T) {
	// Arrange
	hcl := `
table "categories" {
  column "id" {
    type = serial
    null = false
  }
  column "parent_id" {
    type = int
    null = true
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "fk_parent" {
    columns     = [column.parent_id]
    ref_columns = [table.categories.column.id]
  }
}
`
	// Act
	tables := mustParseAtlasHCL(t, hcl)

	// Assert
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if len(tables[0].ForeignKeys) != 1 {
		t.Fatalf("expected 1 FK, got %d", len(tables[0].ForeignKeys))
	}
	fk := tables[0].ForeignKeys[0]
	if fk.RefTable != "categories" {
		t.Fatalf("expected self-ref FK to %q, got %q", "categories", fk.RefTable)
	}
	if fk.NotNull {
		t.Fatal("expected self-referencing FK to be nullable")
	}

	// Verify column-level FK info.
	var parentCol Column
	for _, col := range tables[0].Columns {
		if col.Name == "parent_id" {
			parentCol = col
			break
		}
	}
	if !parentCol.IsFK {
		t.Fatal("expected parent_id to be marked as FK")
	}
	if parentCol.FKRefTable != "categories" {
		t.Fatalf("expected FKRefTable %q, got %q", "categories", parentCol.FKRefTable)
	}
}

func TestParseAtlasHCL_WithComments(t *testing.T) {
	// Arrange
	tests := []struct {
		name     string
		hcl      string
		wantCols int
	}{
		{
			name: "hash comment",
			hcl: `
# Users table
table "users" {
  # Primary key
  column "id" {
    type = serial
    null = false
  }
  column "name" {
    type = text
    null = false
  }
  primary_key {
    columns = [column.id]
  }
}
`,
			wantCols: 2,
		},
		{
			name: "double-slash comment",
			hcl: `
table "users" {
  column "id" {
    type = serial
    null = false
  }
  // column "deprecated" {
  //   type = text
  // }
  column "name" {
    type = text
    null = false
  }
  primary_key {
    columns = [column.id]
  }
}
`,
			wantCols: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			tables := mustParseAtlasHCL(t, tt.hcl)

			// Assert
			if len(tables) != 1 {
				t.Fatalf("expected 1 table, got %d", len(tables))
			}
			if len(tables[0].Columns) != tt.wantCols {
				t.Fatalf("expected %d columns, got %d", tt.wantCols, len(tables[0].Columns))
			}
		})
	}
}
