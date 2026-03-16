package main

import (
	"fmt"
	"go/format"
	"io"
	"strings"
	"text/template"
)

const codeTemplate = `package {{.Package}}

import (
	"context"
{{- if .NeedsTime}}
	"time"
{{- end}}

	"github.com/mhiro2/seedling"
)
{{- range .Tables}}

type {{.GoName}} struct {
{{- range .Columns}}
	{{.GoName}} {{.GoType}}
{{- end}}
}
{{- end}}

func RegisterBlueprints() {
{{- range $i, $t := .Tables}}
{{- if $i}}
{{end}}
	seedling.MustRegister(seedling.Blueprint[{{$t.GoName}}]{
		Name:  "{{$t.BlueprintID}}",
		Table: "{{$t.Name}}",
{{- if isCompositePK $t}}
		PKFields: []string{ {{- range $i, $field := pkFields $t}}{{if $i}}, {{end}}"{{$field}}"{{end}} },
{{- else}}
		PKField: "{{pkField $t}}",
{{- end}}
		Defaults: func() {{$t.GoName}} {
			return {{$t.GoName}}{}
		},
{{- if hasRelations $t}}
		Relations: []seedling.Relation{
{{- range relations $t}}
			{Name: "{{.Name}}", Kind: seedling.BelongsTo, {{- if .Composite}} LocalFields: []string{ {{- range $i, $field := .LocalFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} }, {{- else}} LocalField: "{{.LocalField}}", {{- end}} RefBlueprint: "{{.RefBlueprint}}"{{- if .Optional}}, Optional: true{{- end}}},
{{- end}}
		},
{{- end}}
		Insert: func(ctx context.Context, db seedling.DBTX, v {{$t.GoName}}) ({{$t.GoName}}, error) {
			// TODO: implement
			return v, nil
		},
	})
{{- end}}
}
`

type relationInfo struct {
	Name         string
	LocalField   string
	LocalFields  []string
	Composite    bool
	RefBlueprint string
	Optional     bool
}

type templateData struct {
	Package   string
	NeedsTime bool
	Tables    []Table
}

func Generate(w io.Writer, pkg string, tables []Table) error {
	funcMap := template.FuncMap{
		"pkField": func(t Table) string {
			fields := pkFields(t)
			return fields[0]
		},
		"pkFields": pkFields,
		"isCompositePK": func(t Table) bool {
			return len(pkFields(t)) > 1
		},
		"hasRelations": func(t Table) bool {
			return len(t.ForeignKeys) > 0
		},
		"relations": buildRelations,
	}

	needsTime := false
	for _, t := range tables {
		for _, c := range t.Columns {
			if c.GoType == "time.Time" {
				needsTime = true
			}
		}
	}

	data := templateData{
		Package:   pkg,
		NeedsTime: needsTime,
		Tables:    tables,
	}

	tmpl, err := template.New("code").Funcs(funcMap).Parse(codeTemplate)
	if err != nil {
		return fmt.Errorf("parse code template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute code template: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return fmt.Errorf("format generated code: %w", err)
	}

	_, err = w.Write(formatted)
	if err != nil {
		return fmt.Errorf("write generated code: %w", err)
	}
	return nil
}

const sqlcCodeTemplate = `package {{.Package}}

import (
	"context"

	"github.com/mhiro2/seedling"
	{{.SqlcPkgAlias}} "{{.SqlcImportPath}}"
)

func RegisterBlueprints() {
{{- range $i, $entry := .Entries}}
{{- if $i}}
{{end}}
	seedling.MustRegister(seedling.Blueprint[{{$.SqlcPkgAlias}}.{{$entry.GoName}}]{
		Name:  "{{$entry.BlueprintID}}",
		Table: "{{$entry.TableName}}",
{{- if $entry.CompositePK}}
		PKFields: []string{ {{- range $i, $field := $entry.PKFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} },
{{- else}}
		PKField: "{{$entry.PKField}}",
{{- end}}
		Defaults: func() {{$.SqlcPkgAlias}}.{{$entry.GoName}} {
			return {{$.SqlcPkgAlias}}.{{$entry.GoName}}{}
		},
{{- if $entry.HasRelations}}
		Relations: []seedling.Relation{
{{- range $entry.Relations}}
			{Name: "{{.Name}}", Kind: seedling.BelongsTo, {{- if .Composite}} LocalFields: []string{ {{- range $i, $field := .LocalFields}}{{if $i}}, {{end}}"{{$field}}"{{end}} }, {{- else}} LocalField: "{{.LocalField}}", {{- end}} RefBlueprint: "{{.RefBlueprint}}"{{- if .Optional}}, Optional: true{{- end}}},
{{- end}}
		},
{{- end}}
{{- if $entry.HasInsertQuery}}
		Insert: func(ctx context.Context, dbtx seedling.DBTX, v {{$.SqlcPkgAlias}}.{{$entry.GoName}}) ({{$.SqlcPkgAlias}}.{{$entry.GoName}}, error) {
			return {{$.SqlcPkgAlias}}.New(dbtx.({{$.SqlcPkgAlias}}.DBTX)).{{$entry.InsertQueryName}}(ctx, {{$.SqlcPkgAlias}}.{{$entry.InsertParamType}}{
{{- range $entry.InsertParamFields}}
				{{.Name}}: v.{{.Name}},
{{- end}}
			})
		},
{{- else}}
		Insert: func(ctx context.Context, dbtx seedling.DBTX, v {{$.SqlcPkgAlias}}.{{$entry.GoName}}) ({{$.SqlcPkgAlias}}.{{$entry.GoName}}, error) {
			// TODO: implement
			return v, nil
		},
{{- end}}
{{- if $entry.HasDeleteQuery}}
		Delete: func(ctx context.Context, dbtx seedling.DBTX, v {{$.SqlcPkgAlias}}.{{$entry.GoName}}) error {
{{- if $entry.DeleteArgName}}
			return {{$.SqlcPkgAlias}}.New(dbtx.({{$.SqlcPkgAlias}}.DBTX)).{{$entry.DeleteQueryName}}(ctx, v.{{$entry.DeleteArgField}})
{{- else}}
			return {{$.SqlcPkgAlias}}.New(dbtx.({{$.SqlcPkgAlias}}.DBTX)).{{$entry.DeleteQueryName}}(ctx, v)
{{- end}}
		},
{{- end}}
	})
{{- end}}
}
`

type sqlcEntry struct {
	GoName            string
	BlueprintID       string
	TableName         string
	PKField           string
	PKFields          []string
	CompositePK       bool
	HasRelations      bool
	Relations         []relationInfo
	HasInsertQuery    bool
	InsertQueryName   string
	InsertParamType   string
	InsertParamFields []SqlcField
	HasDeleteQuery    bool
	DeleteQueryName   string
	DeleteArgName     bool
	DeleteArgField    string
}

type sqlcTemplateData struct {
	Package        string
	SqlcPkgAlias   string
	SqlcImportPath string
	Entries        []sqlcEntry
}

// GenerateSqlc generates blueprint code that imports and uses sqlc-generated types.
func GenerateSqlc(w io.Writer, pkg, sqlcImportPath string, tables []Table, sqlcInfo *SqlcInfo) error {
	alias := sqlcInfo.Package

	entries := make([]sqlcEntry, 0, len(tables))
	for _, t := range tables {
		pks := pkFields(t)
		entry := sqlcEntry{
			GoName:      t.GoName,
			BlueprintID: t.BlueprintID,
			TableName:   t.Name,
			PKField:     pks[0],
			PKFields:    pks,
			CompositePK: len(pks) > 1,
		}

		// Relations from FK constraints.
		rels := buildRelations(t)
		entry.HasRelations = len(rels) > 0
		entry.Relations = rels

		// Match to sqlc insert query.
		if q := sqlcInfo.FindQueryForTable(t); q != nil && q.ParamType != "" {
			entry.HasInsertQuery = true
			entry.InsertQueryName = q.Name
			entry.InsertParamType = q.ParamType
			entry.InsertParamFields = q.ParamFields
		}

		// Match to sqlc delete query.
		if dq := sqlcInfo.FindDeleteQueryForTable(t); dq != nil {
			entry.HasDeleteQuery = true
			entry.DeleteQueryName = dq.Name
			if dq.ArgName != "" {
				entry.DeleteArgName = true
				entry.DeleteArgField = pkFieldForDeleteArg(dq.ArgName, pks)
			}
		}

		entries = append(entries, entry)
	}

	data := sqlcTemplateData{
		Package:        pkg,
		SqlcPkgAlias:   alias,
		SqlcImportPath: sqlcImportPath,
		Entries:        entries,
	}

	tmpl, err := template.New("sqlc").Parse(sqlcCodeTemplate)
	if err != nil {
		return fmt.Errorf("parse sqlc template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute sqlc template: %w", err)
	}

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return fmt.Errorf("format sqlc generated code: %w", err)
	}

	_, err = w.Write(formatted)
	if err != nil {
		return fmt.Errorf("write sqlc generated code: %w", err)
	}
	return nil
}

// pkFieldForDeleteArg maps a delete function's arg name (e.g., "id") to the model's PK field name (e.g., "ID").
func pkFieldForDeleteArg(argName string, pks []string) string {
	goName := toGoFieldName(argName)
	for _, pk := range pks {
		if pk == goName {
			return pk
		}
	}
	// Fallback to the first PK field.
	if len(pks) > 0 {
		return pks[0]
	}
	return "ID"
}

func buildRelations(t Table) []relationInfo {
	var rels []relationInfo
	for _, fk := range t.ForeignKeys {
		if len(fk.Columns) == 0 {
			continue
		}
		localFields := make([]string, 0, len(fk.Columns))
		for _, colName := range fk.Columns {
			for _, c := range t.Columns {
				if c.Name != colName {
					continue
				}
				localFields = append(localFields, c.GoName)
				break
			}
		}
		if len(localFields) == 0 {
			continue
		}

		name := singularize(fk.RefTable)
		if len(fk.Columns) == 1 {
			name = relationNameForColumn(fk.Columns[0], fk.RefTable)
		}
		rels = append(rels, relationInfo{
			Name:         name,
			LocalField:   localFields[0],
			LocalFields:  localFields,
			Composite:    len(localFields) > 1,
			RefBlueprint: singularize(fk.RefTable),
			Optional:     !fk.NotNull,
		})
	}
	return rels
}

func relationNameForColumn(columnName, refTable string) string {
	if name, ok := strings.CutSuffix(columnName, "_id"); ok {
		return name
	}
	return singularize(refTable)
}

func pkFields(t Table) []string {
	var fields []string
	for _, c := range t.Columns {
		if c.IsPK {
			fields = append(fields, c.GoName)
		}
	}
	if len(fields) == 0 {
		return []string{"ID"}
	}
	return fields
}
