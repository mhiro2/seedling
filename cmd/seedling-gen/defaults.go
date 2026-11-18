package main

import (
	"go/token"
	"slices"
	"strconv"
	"strings"
)

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
// value, a valid UUID or JSON document, text its column type accepts, a string
// within its maximum length, and
// a value drawn from the scope for a unique string, number, enum, or time
// column, so that every record of a test gets its own. Unique booleans and
// JSON documents keep their fixed default.
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
		return enumDefaultExpr(model, field, fieldType, label, scopeVar), nil
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
	if fieldType == "string" {
		if expr, imports, ok := textTypeDefaultExpr(field, label, scopeVar); ok {
			return expr, imports
		}
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
		if field.Unique {
			// One second apart per record keeps unique timestamps distinct
			// at any column precision down to seconds.
			return scopeVar + ".Now().Add(time.Duration(" + scopeVar + ".Seq(" + strconv.Quote(label) + ")) * time.Second)", []string{`"time"`}
		}
		return scopeVar + ".Now()", nil
	default:
		return "", nil
	}
}

// textTypeDefaultExpr returns the default of a string field whose column type
// accepts only certain text, such as the NUMERIC and INET columns sqlc maps to
// string, and reports whether the type is one of them. A unique column gets a
// distinct value from the scope's sequence. The expression is empty, leaving
// the field unset, where no text suits every database: a BIT column needs
// exactly its declared number of bits in PostgreSQL but reads text as bytes
// in MySQL, and a range has one empty value only.
func textTypeDefaultExpr(field normalizedField, label, scopeVar string) (string, []string, bool) {
	seq := scopeVar + ".Seq(" + strconv.Quote(label) + ")"
	now := scopeVar + ".Now()"
	// pick returns value, or for a unique column the sequence number written
	// with uniqueFormat.
	pick := func(value, uniqueFormat string) (string, []string, bool) {
		if !field.Unique {
			return strconv.Quote(value), nil, true
		}
		return "fmt.Sprintf(" + strconv.Quote(uniqueFormat) + ", " + seq + ")", []string{`"fmt"`}, true
	}
	// clock formats the scope's clock with layout, one second apart per
	// record of a unique column.
	clock := func(layout string) (string, []string, bool) {
		if !field.Unique {
			return now + ".Format(" + strconv.Quote(layout) + ")", nil, true
		}
		return now + ".Add(time.Duration(" + seq + ") * time.Second).Format(" + strconv.Quote(layout) + ")", []string{`"time"`}, true
	}

	switch field.SQLType {
	case "NUMERIC", "DECIMAL", "DEC", "FIXED", "MONEY", "REAL", "FLOAT", "FLOAT4", "FLOAT8", "DOUBLE":
		return pick("1", "%d")
	case "INET":
		return pick("192.0.2.1", "2001:db8::%x")
	case "CIDR":
		return pick("192.0.2.0/24", "2001:db8:%x::/48")
	case "MACADDR":
		return pick("08:00:2b:01:02:03", "08002b%06x")
	case "MACADDR8":
		return pick("08:00:2b:01:02:03:04:05", "08002b0000%06x")
	case "INTERVAL":
		return pick("1 day", "%d seconds")
	case "XML":
		return pick("<seed/>", `<seed n="%d"/>`)
	case "YEAR":
		if !field.Unique {
			return `"2000"`, nil, true
		}
		return "fmt.Sprint(1900 + " + seq + ")", []string{`"fmt"`}, true
	case "TIME", "TIMETZ":
		if !field.Unique {
			return `"12:00:00"`, nil, true
		}
		return "time.Time{}.Add(time.Duration(" + seq + `) * time.Second).Format("15:04:05")`, []string{`"time"`}, true
	case "DATE":
		if !field.Unique {
			return now + `.Format("2006-01-02")`, nil, true
		}
		return now + ".AddDate(0, 0, " + seq + `).Format("2006-01-02")`, nil, true
	case "DATETIME", "TIMESTAMP":
		return clock("2006-01-02 15:04:05")
	case "TIMESTAMPTZ":
		return clock("2006-01-02 15:04:05-07:00")
	case "INT4RANGE", "INT8RANGE", "NUMRANGE", "TSRANGE", "TSTZRANGE", "DATERANGE":
		if field.Unique {
			return "", nil, true
		}
		return `"empty"`, nil, true
	case "BIT", "VARBIT":
		return "", nil, true
	}
	return "", nil, false
}

// enumDefaultExpr returns an allowed value as a default of fieldType: a string
// literal for a string field, or a conversion to a named string type declared
// in the model's package, such as the enum types sqlc generates. A unique
// column cycles through the allowed values so consecutive records differ. sqlc
// wraps a nullable enum in a Null<Type> struct, which is left unset.
func enumDefaultExpr(model normalizedModel, field normalizedField, fieldType, label, scopeVar string) string {
	value := strconv.Quote(field.EnumValues[0])
	if field.Unique && len(field.EnumValues) > 1 {
		quoted := make([]string, len(field.EnumValues))
		for i, v := range field.EnumValues {
			quoted[i] = strconv.Quote(v)
		}
		value = "[]string{" + strings.Join(quoted, ", ") + "}[(" + scopeVar + ".Seq(" + strconv.Quote(label) + ")-1)%" + strconv.Itoa(len(quoted)) + "]"
	}
	if fieldType == "string" {
		return value
	}
	qualifier, _, ok := strings.Cut(strings.TrimPrefix(model.TypeExpr, "*"), ".")
	if !ok || !token.IsIdentifier(fieldType) || predeclaredTypes[fieldType] || strings.HasPrefix(fieldType, "Null") {
		return ""
	}
	return qualifier + "." + fieldType + "(" + value + ")"
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
// bytes (zero for no limit).
func uniqueStringExpr(label string, maxLength int, scopeVar string) (string, []string) {
	if maxLength > 0 {
		return scopeVar + ".UniqueN(" + strconv.Quote(label) + ", " + strconv.Itoa(maxLength) + ")", nil
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
