package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SqlcModel represents a struct type from sqlc-generated models.go.
type SqlcModel struct {
	Name   string
	Fields []SqlcField
}

// SqlcField represents a field in a sqlc-generated struct.
type SqlcField struct {
	Name string
	Type string
}

// SqlcQuery represents an insert/create query function from sqlc-generated code.
type SqlcQuery struct {
	Name        string      // function name (e.g., "InsertUser")
	ReturnType  string      // return model type (e.g., "User")
	ParamType   string      // params type name (e.g., "InsertUserParams"); empty if single arg
	ParamFields []SqlcField // fields of the params struct; empty if single arg
}

// SqlcDeleteQuery represents a delete query function from sqlc-generated code.
type SqlcDeleteQuery struct {
	Name      string // function name (e.g., "DeleteUser")
	ParamType string // params type name or empty for single arg
	ArgName   string // single arg name (e.g., "id")
	ArgType   string // single arg type (e.g., "int64")
}

// SqlcInfo holds information extracted from sqlc-generated Go files.
type SqlcInfo struct {
	Package       string
	Models        []SqlcModel
	Queries       []SqlcQuery
	DeleteQueries []SqlcDeleteQuery
}

// ParseSqlcDir parses sqlc-generated Go files in the given directory.
func ParseSqlcDir(dir string) (*SqlcInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read sqlc dir: %w", err)
	}

	fset := token.NewFileSet()
	var files []*ast.File
	var pkgName string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		files = append(files, f)
		if pkgName == "" {
			pkgName = f.Name.Name
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files found in %s", dir)
	}

	info := &SqlcInfo{
		Package: pkgName,
	}

	structTypes := make(map[string]*ast.StructType)

	for _, file := range files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if st, ok := typeSpec.Type.(*ast.StructType); ok {
					structTypes[typeSpec.Name.Name] = st
				}
			}
		}
	}

	// Extract models: structs that are not Params, Queries, or DBTX.
	// Iterate struct names in sorted order to keep generator output byte-stable.
	names := make([]string, 0, len(structTypes))
	for name := range structTypes {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		if strings.HasSuffix(name, "Params") || name == "Queries" {
			continue
		}
		model := SqlcModel{Name: name}
		for _, field := range structTypes[name].Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			model.Fields = append(model.Fields, SqlcField{
				Name: field.Names[0].Name,
				Type: exprToString(field.Type),
			})
		}
		info.Models = append(info.Models, model)
	}

	// Extract query functions: methods on *Queries.
	for _, file := range files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil {
				continue
			}
			if !isQueriesReceiver(funcDecl.Recv) {
				continue
			}

			name := funcDecl.Name.Name
			lowerName := strings.ToLower(name)

			switch {
			case strings.HasPrefix(lowerName, "insert") || strings.HasPrefix(lowerName, "create"):
				q := parseInsertQuery(funcDecl, structTypes)
				if q != nil {
					info.Queries = append(info.Queries, *q)
				}
			case strings.HasPrefix(lowerName, "delete"):
				dq := parseDeleteQuery(funcDecl)
				if dq != nil {
					info.DeleteQueries = append(info.DeleteQueries, *dq)
				}
			}
		}
	}

	return info, nil
}

func parseInsertQuery(funcDecl *ast.FuncDecl, structTypes map[string]*ast.StructType) *SqlcQuery {
	q := &SqlcQuery{Name: funcDecl.Name.Name}

	// Get return type (first return value).
	if funcDecl.Type.Results != nil && len(funcDecl.Type.Results.List) > 0 {
		q.ReturnType = exprToString(funcDecl.Type.Results.List[0].Type)
	}

	// Get param type (second parameter after ctx).
	params := funcDecl.Type.Params
	if params == nil || len(params.List) < 2 {
		return q
	}

	paramExpr := params.List[1].Type
	paramType := exprToString(paramExpr)
	q.ParamType = paramType

	if st, ok := structTypes[paramType]; ok {
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			q.ParamFields = append(q.ParamFields, SqlcField{
				Name: field.Names[0].Name,
				Type: exprToString(field.Type),
			})
		}
	}

	return q
}

func parseDeleteQuery(funcDecl *ast.FuncDecl) *SqlcDeleteQuery {
	dq := &SqlcDeleteQuery{Name: funcDecl.Name.Name}

	params := funcDecl.Type.Params
	if params == nil || len(params.List) < 2 {
		return dq
	}

	paramField := params.List[1]
	paramType := exprToString(paramField.Type)

	if len(paramField.Names) > 0 {
		dq.ArgName = paramField.Names[0].Name
		dq.ArgType = paramType
	} else {
		dq.ParamType = paramType
	}

	return dq
}

func isQueriesReceiver(recv *ast.FieldList) bool {
	if len(recv.List) != 1 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "Queries"
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + exprToString(t.Elt)
		}
		return "[" + exprToString(t.Len) + "]" + exprToString(t.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.BasicLit:
		return t.Value
	default:
		return "any"
	}
}

// FindQueryForTable finds the sqlc insert/create query function for the given table.
func (info *SqlcInfo) FindQueryForTable(table Table) *SqlcQuery {
	for i, q := range info.Queries {
		if q.ReturnType == table.GoName {
			return &info.Queries[i]
		}
	}
	return nil
}

// FindDeleteQueryForTable finds the sqlc delete query function for the given table.
func (info *SqlcInfo) FindDeleteQueryForTable(table Table) *SqlcDeleteQuery {
	for i, dq := range info.DeleteQueries {
		name := strings.ToLower(dq.Name)
		target := strings.ToLower(table.GoName)
		if strings.Contains(name, target) {
			return &info.DeleteQueries[i]
		}
	}
	return nil
}

// FindModelForTable finds the sqlc model struct for the given table.
func (info *SqlcInfo) FindModelForTable(table Table) *SqlcModel {
	for i, m := range info.Models {
		if m.Name == table.GoName {
			return &info.Models[i]
		}
	}
	return nil
}
