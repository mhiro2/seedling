package main

import (
	"go/ast"
	"go/parser"
	"slices"
	"strconv"
)

// normalizedConverter is a generated FK converter for one relation field.
type normalizedConverter struct {
	Field string // FK field that receives the converted value
	Expr  string // seedling.ConvertFK expression
}

// nullWrapper describes a struct type that holds a nullable value next to a
// Valid flag, such as sql.NullInt64.
type nullWrapper struct {
	field string // field holding the value, e.g. "Int64"
	base  string // value type: a predeclared type, "[16]byte", or "uuid" for uuid.UUID
}

const (
	sqlImportPath    = "database/sql"
	pgtypeImportPath = "github.com/jackc/pgx/v5/pgtype"
	uuidImportPath   = "github.com/google/uuid"
)

// nullWrappers lists the nullable types, by import path and type name, that a
// referenced key can be converted into. Only key-like value types are listed.
var nullWrappers = map[string]map[string]nullWrapper{
	sqlImportPath: {
		"NullInt64":  {field: "Int64", base: "int64"},
		"NullInt32":  {field: "Int32", base: "int32"},
		"NullInt16":  {field: "Int16", base: "int16"},
		"NullByte":   {field: "Byte", base: "byte"},
		"NullString": {field: "String", base: "string"},
	},
	pgtypeImportPath: {
		"Int8": {field: "Int64", base: "int64"},
		"Int4": {field: "Int32", base: "int32"},
		"Int2": {field: "Int16", base: "int16"},
		"Text": {field: "String", base: "string"},
		"UUID": {field: "Bytes", base: "[16]byte"},
	},
	uuidImportPath: {
		"NullUUID": {field: "UUID", base: "uuid"},
	},
}

// integerType describes a predeclared integer type for lossless conversion
// checks. int and uint are taken as 64 bits wide.
type integerType struct {
	bits   int
	signed bool
}

var integerTypes = map[string]integerType{
	"int": {64, true}, "int8": {8, true}, "int16": {16, true}, "int32": {32, true}, "int64": {64, true},
	"uint": {64, false}, "uint8": {8, false}, "uint16": {16, false}, "uint32": {32, false}, "uint64": {64, false},
	"byte": {8, false},
}

// losslessIntegerConversion reports whether every value of the integer type
// from is representable in the integer type to, so converting a key can never
// make it reference a different row.
func losslessIntegerConversion(from, to string) bool {
	src, ok := integerTypes[from]
	if !ok {
		return false
	}
	dst, ok := integerTypes[to]
	if !ok {
		return false
	}
	switch {
	case src.signed == dst.signed:
		return src.bits <= dst.bits
	case dst.signed:
		return src.bits < dst.bits
	default:
		return false
	}
}

// withFKConverters attaches a converter to every belongs-to field whose type
// is a known nullable wrapper of the referenced field's type, such as an
// sql.NullInt64 FK pointing at an int64 key. Package qualifiers are resolved
// through each model's SourceImports, the imports of the file declaring it;
// the imports the converters need are added to the child model. A field the
// parent value can be assigned to directly, or as a pointer, needs no
// converter, and any other mismatch is left to the runtime check, which
// reports it before inserting.
func withFKConverters(models []normalizedModel) []normalizedModel {
	byID := make(map[string]normalizedModel, len(models))
	for _, model := range models {
		byID[model.BlueprintID] = model
	}

	for i := range models {
		model := &models[i]
		for j := range model.Relations {
			rel := &model.Relations[j]
			parent, ok := byID[rel.RefBlueprint]
			if rel.HasMany || !ok {
				continue
			}
			refFields := rel.RefFields
			if len(refFields) == 0 {
				refFields = parent.PKFields
			}
			if len(refFields) != len(rel.LocalFields) {
				continue
			}
			for k, local := range rel.LocalFields {
				childType, ok := normalizedFieldType(*model, local)
				if !ok {
					continue
				}
				parentType, ok := normalizedFieldType(parent, refFields[k])
				if !ok || parentType == childType || childType == "*"+parentType {
					continue
				}
				expr, exprImports, ok := fkConverterExpr(parentType, parent.SourceImports, childType, model.SourceImports)
				if !ok {
					continue
				}
				rel.Converters = append(rel.Converters, normalizedConverter{Field: local, Expr: expr})
				model.Imports = append(model.Imports, exprImports...)
			}
		}
	}
	return models
}

func normalizedFieldType(model normalizedModel, name string) (string, bool) {
	i := slices.IndexFunc(model.Fields, func(f normalizedField) bool { return f.GoName == name })
	if i < 0 {
		return "", false
	}
	return model.Fields[i].GoType, true
}

// fkConverterExpr returns a seedling.ConvertFK expression that wraps a value
// of parentType into childType, with the imports it needs. Each type's package
// qualifiers resolve through the imports of the file declaring it.
func fkConverterExpr(parentType string, parentImports map[string]string, childType string, childImports map[string]string) (string, []string, bool) {
	qualifier, name, typeArg, ok := splitQualifiedType(childType)
	if !ok {
		return "", nil, false
	}
	path, ok := childImports[qualifier]
	if !ok {
		return "", nil, false
	}

	var wrapper nullWrapper
	if path == sqlImportPath && name == "Null" && predeclaredTypes[typeArg] {
		// sql.Null[T] holds any T in V; only predeclared T are named here,
		// since a model-package type would need qualifying.
		wrapper = nullWrapper{field: "V", base: typeArg}
	} else {
		wrapper, ok = nullWrappers[path][name]
		if !ok || typeArg != "" {
			return "", nil, false
		}
	}

	childImport, err := importSpecFor(qualifier, path)
	if err != nil {
		return "", nil, false
	}
	exprImports := []string{childImport}

	paramType, value := parentType, "v"
	switch {
	case wrapper.base == "uuid" || wrapper.base == "[16]byte":
		// A uuid.UUID or [16]byte key is assigned as is; the converter
		// takes it as [16]byte so the generated file needs no uuid import
		// unless the wrapper itself comes from that package.
		if !isUUIDType(parentType, parentImports) {
			return "", nil, false
		}
		paramType = "[16]byte"
		if wrapper.base == "uuid" {
			value = qualifier + ".UUID(v)"
		}
	case parentType == wrapper.base:
	case losslessIntegerConversion(parentType, wrapper.base):
		value = wrapper.base + "(v)"
	default:
		return "", nil, false
	}

	expr := "seedling.ConvertFK(func(v " + paramType + ") " + childType + " { return " +
		childType + "{" + wrapper.field + ": " + value + ", Valid: true} })"
	return expr, exprImports, true
}

// isUUIDType reports whether goType is a 16-byte UUID key: [16]byte or a
// github.com/google/uuid UUID.
func isUUIDType(goType string, imports map[string]string) bool {
	if goType == "[16]byte" {
		return true
	}
	qualifier, name, typeArg, ok := splitQualifiedType(goType)
	return ok && typeArg == "" && name == "UUID" && imports[qualifier] == uuidImportPath
}

// splitQualifiedType splits pkg.Name or pkg.Name[T] into its parts.
func splitQualifiedType(goType string) (qualifier, name, typeArg string, ok bool) {
	expr, err := parser.ParseExpr(goType)
	if err != nil {
		return "", "", "", false
	}
	if index, isIndex := expr.(*ast.IndexExpr); isIndex {
		typeArg = goType[index.Lbrack : index.Rbrack-1]
		expr = index.X
	}
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel {
		return "", "", "", false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", "", "", false
	}
	return pkg.Name, sel.Sel.Name, typeArg, true
}

// importSpecFor returns the import spec binding qualifier to path, aliased
// only when the qualifier differs from the package name the path implies.
func importSpecFor(qualifier, path string) (string, error) {
	if importPathPackageName(path) == qualifier {
		return strconv.Quote(path), nil
	}
	return importSpec(qualifier, path)
}
