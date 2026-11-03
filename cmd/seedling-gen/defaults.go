package main

import (
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// uniqueSuffixReserve is the room a Scope.Unique suffix ("-" + sequence +
// "-" + namespace) takes in a length-limited column: up to five sequence
// digits and the six characters of a TestScope namespace.
const uniqueSuffixReserve = 13

// scopeVarCandidates are the names tried, in order, for the Scope parameter of
// generated Defaults; the first one no import binds is used.
var scopeVarCandidates = []string{"scope", "seedlingScope", "genScope"}

// chooseScopeVar returns a parameter name for the Scope that Defaults
// receive, avoiding the names the generated file imports packages under so a
// default never refers to the parameter where it means a package.
func chooseScopeVar(imports []string) string {
	taken := make(map[string]bool, len(imports))
	for _, spec := range imports {
		taken[importName(spec)] = true
	}
	for _, name := range scopeVarCandidates {
		if !taken[name] {
			return name
		}
	}
	return scopeVarCandidates[len(scopeVarCandidates)-1] + "_"
}

// buildDefaultLiteral renders the value generated Defaults return for model,
// drawing scope-dependent values from scopeVar.
func buildDefaultLiteral(model normalizedModel, scopeVar string) string {
	prefix, ok := strings.CutSuffix(model.ZeroValueExpr, "{}")
	if !ok {
		return model.ZeroValueExpr
	}

	// Promoted fields are grouped under their embedded struct, which takes the
	// position of its first defaulted field.
	type literalEntry struct {
		embedded *embeddedStruct
		parts    []string
	}
	var entries []*literalEntry
	embeddedEntries := make(map[embeddedStruct]*literalEntry)
	for _, field := range model.Fields {
		expr, _ := defaultFieldExpr(model, field, scopeVar)
		if expr == "" {
			continue
		}
		assignment := field.GoName + ": " + expr
		if field.Embedded == nil {
			entries = append(entries, &literalEntry{parts: []string{assignment}})
			continue
		}
		entry, ok := embeddedEntries[*field.Embedded]
		if !ok {
			entry = &literalEntry{embedded: field.Embedded}
			embeddedEntries[*field.Embedded] = entry
			entries = append(entries, entry)
		}
		entry.parts = append(entry.parts, assignment)
	}
	if len(entries) == 0 {
		return model.ZeroValueExpr
	}

	assignments := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.embedded == nil {
			assignments = append(assignments, entry.parts[0])
			continue
		}
		assignments = append(assignments, entry.embedded.FieldName+": "+entry.embedded.TypeExpr+"{"+strings.Join(entry.parts, ", ")+"}")
	}
	return prefix + "{" + strings.Join(assignments, ", ") + "}"
}

// normalizedDefaultImports returns the imports the rendered defaults of models
// need.
func normalizedDefaultImports(models []normalizedModel) []string {
	var imports []string
	for _, model := range models {
		for _, field := range model.Fields {
			_, fieldImports := defaultFieldExpr(model, field, "")
			imports = append(imports, fieldImports...)
		}
	}
	return uniqueStrings(imports)
}

// normalizedModelsNeedTimeImport reports whether emitted model structs declare
// a time.Time field. Generated defaults never name the time package, since
// time-based defaults come from the scope's clock.
func normalizedModelsNeedTimeImport(models []normalizedModel, emitStructs bool) bool {
	if !emitStructs {
		return false
	}
	return slices.ContainsFunc(models, func(model normalizedModel) bool {
		return slices.ContainsFunc(model.Fields, func(field normalizedField) bool {
			return field.GoType == "time.Time"
		})
	})
}

// defaultFieldExpr returns the generated default for one field and the
// imports it needs, or "" when the field is left to its zero value: keys,
// relation fields, and types the generator cannot construct. Defaults respect
// the column's constraints where the schema states them: the first enumerated
// value, a valid UUID or JSON document, a string within its maximum length, and
// a value drawn from the scope for a unique column, so that every record of a
// test gets its own.
func defaultFieldExpr(model normalizedModel, field normalizedField, scopeVar string) (string, []string) {
	if field.IsPK || field.IsRelationFK {
		return "", nil
	}

	label := model.BlueprintID + "-" + toSnakeCase(field.GoName)
	fieldType := field.GoType
	if field.DefaultType != "" {
		fieldType = field.DefaultType
	}

	if len(field.EnumValues) > 0 {
		return enumDefaultExpr(model, fieldType, field.EnumValues[0]), nil
	}
	switch field.SQLType {
	case "UUID":
		if fieldType == "string" {
			return scopeVar + ".UUID()", nil
		}
		return "", nil
	case "JSON", "JSONB":
		return jsonDefaultExpr(model, fieldType)
	}

	switch fieldType {
	case "string":
		if field.Unique {
			return uniqueStringExpr(label, field.MaxLength, scopeVar)
		}
		return strconv.Quote(truncateLabel(label, field.MaxLength)), nil
	case "[]byte":
		if field.Unique {
			expr, imports := uniqueStringExpr(label, field.MaxLength, scopeVar)
			return "[]byte(" + expr + ")", imports
		}
		return "[]byte(" + strconv.Quote(truncateLabel(label, field.MaxLength)) + ")", nil
	case "bool":
		return "true", nil
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		if !field.Unique {
			return "1", nil
		}
		seq := scopeVar + ".Seq(" + strconv.Quote(label) + ")"
		if fieldType == "int" {
			return seq, nil
		}
		return fieldType + "(" + seq + ")", nil
	case "time.Time":
		return scopeVar + ".Now()", nil
	default:
		return "", nil
	}
}

// enumDefaultExpr returns value as a default of fieldType: a string literal
// for a string field, or a conversion to a named string type declared in the
// model's package, such as the enum types sqlc generates.
func enumDefaultExpr(model normalizedModel, fieldType, value string) string {
	literal := strconv.Quote(value)
	if fieldType == "string" {
		return literal
	}
	qualifier, _, ok := strings.Cut(strings.TrimPrefix(model.TypeExpr, "*"), ".")
	if !ok || !token.IsIdentifier(fieldType) || predeclaredTypes[fieldType] {
		return ""
	}
	return qualifier + "." + fieldType + "(" + literal + ")"
}

// jsonDefaultExpr returns an empty JSON object as a default of fieldType.
func jsonDefaultExpr(model normalizedModel, fieldType string) (string, []string) {
	const empty = `"{}"`
	switch fieldType {
	case "string":
		return empty, nil
	case "[]byte":
		return "[]byte(" + empty + ")", nil
	}
	qualifier, name, typeArg, ok := splitQualifiedType(fieldType)
	if !ok || typeArg != "" || name != "RawMessage" || model.SourceImports[qualifier] != "encoding/json" {
		return "", nil
	}
	return "json.RawMessage(" + empty + ")", []string{`"encoding/json"`}
}

// uniqueStringExpr draws a unique string for a column of at most maxLength
// characters (zero for no limit). The label is shortened to leave room for
// the scope's suffix; a column too short for any label gets the bare sequence
// number.
func uniqueStringExpr(label string, maxLength int, scopeVar string) (string, []string) {
	switch {
	case maxLength == 0 || maxLength >= len(label)+uniqueSuffixReserve:
	case maxLength > uniqueSuffixReserve:
		label = label[:maxLength-uniqueSuffixReserve]
	default:
		return "strconv.Itoa(" + scopeVar + ".Seq(" + strconv.Quote(label) + "))", []string{`"strconv"`}
	}
	return scopeVar + ".Unique(" + strconv.Quote(label) + ")", nil
}

// truncateLabel shortens label to maxLength bytes; zero means no limit.
// Labels are built from ASCII identifiers, so a byte cut never splits a rune.
func truncateLabel(label string, maxLength int) string {
	if maxLength > 0 && len(label) > maxLength {
		return label[:maxLength]
	}
	return label
}
