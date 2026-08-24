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
	Name          string      // function name (e.g., "InsertUser")
	ReturnType    string      // return model type (e.g., "User")
	ReturnPointer bool        // whether the query returns a pointer to the model
	DBArgument    bool        // whether the method accepts a DBTX argument after context
	ParamType     string      // params struct type name (e.g., "InsertUserParams")
	ParamPointer  bool        // whether the query accepts a pointer to the params struct
	ParamFields   []SqlcField // fields of the params struct
	ArgName       string      // scalar argument name (e.g., "name")
	ArgType       string      // scalar argument type (e.g., "string")
}

// SqlcDeleteQuery represents a delete query function from sqlc-generated code.
type SqlcDeleteQuery struct {
	Name         string      // function name (e.g., "DeleteUser")
	DBArgument   bool        // whether the method accepts a DBTX argument after context
	ParamType    string      // params struct type name (e.g., "DeleteMembershipParams")
	ParamPointer bool        // whether the query accepts a pointer to the params struct
	ParamFields  []SqlcField // fields of the params struct
	ArgName      string      // scalar argument name (e.g., "id")
	ArgType      string      // scalar argument type (e.g., "int64")
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
				dq := parseDeleteQuery(funcDecl, structTypes)
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
		q.ReturnType, q.ReturnPointer = sqlcNamedType(funcDecl.Type.Results.List[0].Type)
	}

	paramField, dbArgument := sqlcMethodParam(funcDecl.Type.Params)
	q.DBArgument = dbArgument
	if paramField == nil {
		return q
	}

	paramExpr := paramField.Type
	paramType := exprToString(paramExpr)
	structType := paramType
	paramPointer := false
	if pointedType, ok := strings.CutPrefix(structType, "*"); ok {
		structType = pointedType
		paramPointer = true
	}

	if st, ok := structTypes[structType]; ok {
		q.ParamType = structType
		q.ParamPointer = paramPointer
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			q.ParamFields = append(q.ParamFields, SqlcField{
				Name: field.Names[0].Name,
				Type: exprToString(field.Type),
			})
		}
		return q
	}

	if len(paramField.Names) > 0 {
		q.ArgName = paramField.Names[0].Name
	}
	q.ArgType = paramType

	return q
}

func parseDeleteQuery(funcDecl *ast.FuncDecl, structTypes map[string]*ast.StructType) *SqlcDeleteQuery {
	dq := &SqlcDeleteQuery{Name: funcDecl.Name.Name}

	paramField, dbArgument := sqlcMethodParam(funcDecl.Type.Params)
	dq.DBArgument = dbArgument
	if paramField == nil {
		return dq
	}

	paramType := exprToString(paramField.Type)
	structType := paramType
	paramPointer := false
	if pointedType, ok := strings.CutPrefix(structType, "*"); ok {
		structType = pointedType
		paramPointer = true
	}
	if st, ok := structTypes[structType]; ok {
		dq.ParamType = structType
		dq.ParamPointer = paramPointer
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			dq.ParamFields = append(dq.ParamFields, SqlcField{
				Name: field.Names[0].Name,
				Type: exprToString(field.Type),
			})
		}
		return dq
	}

	if len(paramField.Names) > 0 {
		dq.ArgName = paramField.Names[0].Name
	}
	dq.ArgType = paramType

	return dq
}

func sqlcMethodParam(params *ast.FieldList) (*ast.Field, bool) {
	if params == nil || len(params.List) < 2 {
		return nil, false
	}

	index := 1
	dbArgument := exprToString(params.List[index].Type) == "DBTX"
	if dbArgument {
		index++
	}
	if index >= len(params.List) {
		return nil, dbArgument
	}
	return params.List[index], dbArgument
}

func sqlcNamedType(expr ast.Expr) (string, bool) {
	typeName := exprToString(expr)
	if pointedType, ok := strings.CutPrefix(typeName, "*"); ok {
		return pointedType, true
	}
	return typeName, false
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
