package main

import (
	"bytes"
	"strings"
	"testing"
)

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
	tables := ParseAtlasHCL(hcl)
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
	tables := ParseAtlasHCL(hcl)
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
	tables := ParseAtlasHCL("")
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
	tables := ParseAtlasHCL(hcl)
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
	tables := ParseAtlasHCL(hcl)

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
		"-atlas", dir + "/atlas.hcl",
		"-pkg", "testutil",
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

func TestRun_MutualExclusivity(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Act
	exitCode := run([]string{
		"-gorm", "/a",
		"-atlas", "/b",
	}, &stdout, &stderr)

	// Assert
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "only one adapter flag") {
		t.Fatalf("expected mutual exclusivity error, got: %s", stderr.String())
	}
}
