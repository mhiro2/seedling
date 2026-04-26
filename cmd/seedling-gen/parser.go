package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	createTableStartRE = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s(]+)\s*\(`)
	createEnumRE       = regexp.MustCompile(`(?is)CREATE\s+TYPE\s+([^\s]+)\s+AS\s+ENUM\s*\(`)
	inlineRefRE        = regexp.MustCompile(`(?i)REFERENCES\s+([^\s(]+)`)
	tablePKRE          = regexp.MustCompile(`(?i)(?:CONSTRAINT\s+[^\s]+\s+)?PRIMARY\s+KEY\s*\(([^)]+)\)`)
	tableFKRE          = regexp.MustCompile(`(?i)(?:CONSTRAINT\s+[^\s]+\s+)?FOREIGN\s+KEY\s*\(([^)]+)\)\s+REFERENCES\s+([^\s(]+)`)
)

// Column represents a parsed column from a CREATE TABLE statement.
type Column struct {
	Name       string
	SQLType    string
	GoName     string
	GoType     string
	IsPK       bool
	IsFK       bool
	FKRefTable string
	NotNull    bool
}

// ForeignKey represents a table-level or inline foreign key constraint.
type ForeignKey struct {
	Columns  []string
	RefTable string
	NotNull  bool
}

// Table represents a parsed CREATE TABLE statement.
type Table struct {
	Name        string
	GoName      string
	BlueprintID string
	Columns     []Column
	ForeignKeys []ForeignKey
}

// ParseSchema parses SQL schema text and returns a slice of Tables.
func ParseSchema(sql string) ([]Table, error) {
	return ParseSchemaWithDialect(sql, "auto")
}

// ParseSchemaWithDialect parses SQL schema text for the given dialect.
func ParseSchemaWithDialect(sql, dialect string) ([]Table, error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "auto", "postgres", "mysql", "sqlite":
	default:
		return nil, fmt.Errorf("unsupported dialect %q", dialect)
	}

	sql = stripSQLComments(sql)

	var tables []Table
	enumTypes := extractEnumTypes(sql)

	blocks, err := extractCreateTableBlocks(sql)
	if err != nil {
		return nil, err
	}
	for _, block := range blocks {
		tableName := normalizeIdent(block.Name)
		body := block.Body
		columns, foreignKeys := parseColumns(body, enumTypes)
		t := Table{
			Name:        tableName,
			GoName:      toGoStructName(tableName),
			BlueprintID: singularize(tableName),
			Columns:     columns,
			ForeignKeys: foreignKeys,
		}
		tables = append(tables, t)
	}

	return tables, nil
}

type createTableBlock struct {
	Name string
	Body string
}

func extractCreateTableBlocks(sql string) ([]createTableBlock, error) {
	var blocks []createTableBlock
	searchFrom := 0

	for {
		loc := createTableStartRE.FindStringSubmatchIndex(sql[searchFrom:])
		if loc == nil {
			return blocks, nil
		}

		matchStart := searchFrom + loc[0]
		bodyStart := searchFrom + loc[1]
		nameStart := searchFrom + loc[2]
		nameEnd := searchFrom + loc[3]

		tableName := sql[nameStart:nameEnd]
		bodyEnd := findMatchingParen(sql, bodyStart-1)
		if bodyEnd == -1 {
			return nil, fmt.Errorf("parse CREATE TABLE %s: unclosed parenthesis", tableName)
		}

		blocks = append(blocks, createTableBlock{
			Name: tableName,
			Body: sql[bodyStart:bodyEnd],
		})

		stmtEnd := strings.IndexByte(sql[bodyEnd:], ';')
		if stmtEnd == -1 {
			searchFrom = bodyEnd
		} else {
			searchFrom = bodyEnd + stmtEnd + 1
		}
		if searchFrom <= matchStart {
			searchFrom = bodyEnd + 1
		}
	}
}

func findMatchingParen(s string, openIdx int) int {
	depth := 0
	inSingle := false
	inDouble := false
	inBack := false

	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble && !inBack {
				// Doubled '' inside a single-quoted string is an escape, not a terminator.
				if inSingle && i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && !inBack {
				// Doubled "" inside a double-quoted identifier is an escape (ANSI SQL).
				if inDouble && i+1 < len(s) && s[i+1] == '"' {
					i++
					continue
				}
				inDouble = !inDouble
			}
		case '`':
			if !inSingle && !inDouble {
				// Doubled `` inside a backtick-quoted identifier is an escape (MySQL).
				if inBack && i+1 < len(s) && s[i+1] == '`' {
					i++
					continue
				}
				inBack = !inBack
			}
		case '(':
			if !inSingle && !inDouble && !inBack {
				depth++
			}
		case ')':
			if !inSingle && !inDouble && !inBack {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}

	return -1
}

func extractEnumTypes(sql string) map[string]struct{} {
	matches := createEnumRE.FindAllStringSubmatch(sql, -1)
	if len(matches) == 0 {
		return nil
	}

	enumTypes := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		enumTypes[normalizeIdent(match[1])] = struct{}{}
	}

	return enumTypes
}

func parseColumns(body string, enumTypes map[string]struct{}) ([]Column, []ForeignKey) {
	items := splitSQLItems(body)
	columns := make([]Column, 0, len(items))
	foreignKeys := make([]ForeignKey, 0, len(items))
	columnIndex := make(map[string]int, len(items))
	tableConstraints := make([]string, 0, len(items))

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		if isTableConstraint(item) {
			tableConstraints = append(tableConstraints, item)
			continue
		}

		col, ok := parseColumn(item, enumTypes)
		if !ok {
			continue
		}

		columnIndex[col.Name] = len(columns)
		columns = append(columns, col)
		if col.IsFK {
			foreignKeys = append(foreignKeys, ForeignKey{
				Columns:  []string{col.Name},
				RefTable: col.FKRefTable,
				NotNull:  col.NotNull,
			})
		}
	}

	for _, constraint := range tableConstraints {
		if fk, ok := applyTableConstraint(columns, columnIndex, constraint); ok {
			foreignKeys = append(foreignKeys, fk)
		}
	}

	return columns, foreignKeys
}

func parseColumn(item string, enumTypes map[string]struct{}) (Column, bool) {
	nameToken, rest, ok := splitLeadingIdent(item)
	if !ok {
		return Column{}, false
	}

	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return Column{}, false
	}

	colName := normalizeIdent(nameToken)
	sqlType := normalizeSQLType(parts[0])

	upper := strings.ToUpper(item)
	col := Column{
		Name:    colName,
		SQLType: sqlType,
		GoName:  toGoFieldName(colName),
		GoType:  sqlTypeToGoType(sqlType, enumTypes),
		IsPK:    strings.Contains(upper, "PRIMARY KEY"),
		NotNull: strings.Contains(upper, "NOT NULL"),
	}

	if refMatch := inlineRefRE.FindStringSubmatch(item); refMatch != nil {
		col.IsFK = true
		col.FKRefTable = normalizeIdent(refMatch[1])
	}

	return col, true
}

func applyTableConstraint(columns []Column, columnIndex map[string]int, constraint string) (ForeignKey, bool) {
	if match := tablePKRE.FindStringSubmatch(constraint); match != nil {
		for _, colName := range splitIdentifierList(match[1]) {
			if idx, ok := columnIndex[colName]; ok {
				columns[idx].IsPK = true
			}
		}
	}

	if match := tableFKRE.FindStringSubmatch(constraint); match != nil {
		refTable := normalizeIdent(match[2])
		cols := splitIdentifierList(match[1])
		notNull := true
		for _, colName := range cols {
			if idx, ok := columnIndex[colName]; ok {
				columns[idx].IsFK = true
				columns[idx].FKRefTable = refTable
				notNull = notNull && columns[idx].NotNull
			}
		}
		return ForeignKey{Columns: cols, RefTable: refTable, NotNull: notNull}, true
	}

	return ForeignKey{}, false
}

func isTableConstraint(item string) bool {
	upper := strings.ToUpper(strings.TrimSpace(item))
	return strings.HasPrefix(upper, "PRIMARY KEY") ||
		strings.HasPrefix(upper, "UNIQUE") ||
		strings.HasPrefix(upper, "CHECK") ||
		strings.HasPrefix(upper, "FOREIGN KEY") ||
		strings.HasPrefix(upper, "CONSTRAINT") ||
		strings.HasPrefix(upper, "KEY ") ||
		strings.HasPrefix(upper, "INDEX ")
}

func splitSQLItems(body string) []string {
	var (
		items    []string
		current  strings.Builder
		depth    int
		inSingle bool
		inDouble bool
		inBack   bool
	)

	runes := []rune(body)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '\'':
			if !inDouble && !inBack {
				// Doubled '' inside a single-quoted string is an escape.
				if inSingle && i+1 < len(runes) && runes[i+1] == '\'' {
					current.WriteRune(r)
					current.WriteRune(runes[i+1])
					i++
					continue
				}
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && !inBack {
				// Doubled "" inside a double-quoted identifier is an escape.
				if inDouble && i+1 < len(runes) && runes[i+1] == '"' {
					current.WriteRune(r)
					current.WriteRune(runes[i+1])
					i++
					continue
				}
				inDouble = !inDouble
			}
		case '`':
			if !inSingle && !inDouble {
				// Doubled `` inside a backtick-quoted identifier is an escape.
				if inBack && i+1 < len(runes) && runes[i+1] == '`' {
					current.WriteRune(r)
					current.WriteRune(runes[i+1])
					i++
					continue
				}
				inBack = !inBack
			}
		case '(':
			if !inSingle && !inDouble && !inBack {
				depth++
			}
		case ')':
			if !inSingle && !inDouble && !inBack && depth > 0 {
				depth--
			}
		case ',':
			if !inSingle && !inDouble && !inBack && depth == 0 {
				items = append(items, current.String())
				current.Reset()
				continue
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		items = append(items, current.String())
	}

	return items
}

func splitLeadingIdent(s string) (ident, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}

	if s[0] == '"' || s[0] == '`' || s[0] == '[' {
		open := s[0]
		closing := byte('"')
		switch open {
		case '`':
			closing = '`'
		case '[':
			closing = ']'
		}
		// Walk past doubled "" / `` escapes inside ANSI / MySQL quoted identifiers.
		// Brackets ([ ]) don't have a documented escape form so we don't try to.
		for i := 1; i < len(s); i++ {
			if s[i] != closing {
				continue
			}
			if open != '[' && i+1 < len(s) && s[i+1] == closing {
				i++
				continue
			}
			return s[:i+1], strings.TrimSpace(s[i+1:]), true
		}
		return "", "", false
	}

	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", "", false
	}
	ident = parts[0]
	return ident, strings.TrimSpace(s[len(ident):]), true
}

func splitIdentifierList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := normalizeIdent(part)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func normalizeIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	parts := strings.Split(s, ".")
	s = parts[len(parts)-1]
	s = trimIdentifierQuotes(s)
	return strings.ToLower(s)
}

func normalizeSQLType(sqlType string) string {
	sqlType = strings.TrimSpace(sqlType)
	if idx := strings.Index(sqlType, "("); idx != -1 {
		sqlType = sqlType[:idx]
	}

	parts := strings.Split(sqlType, ".")
	sqlType = parts[len(parts)-1]
	sqlType = trimIdentifierQuotes(sqlType)

	return strings.ToUpper(sqlType)
}

func sqlTypeToGoType(sqlType string, enumTypes ...map[string]struct{}) string {
	var knownEnums map[string]struct{}
	if len(enumTypes) > 0 {
		knownEnums = enumTypes[0]
	}

	if _, ok := knownEnums[strings.ToLower(sqlType)]; ok {
		return "string"
	}

	switch sqlType {
	case "SERIAL", "INTEGER", "INT", "SMALLINT", "TINYINT", "INT2", "MEDIUMINT":
		return "int"
	case "BIGSERIAL", "BIGINT", "INT8":
		return "int64"
	case "TEXT", "VARCHAR", "CHAR", "CHARACTER", "UUID", "JSON", "JSONB", "ENUM":
		return "string"
	case "BLOB", "BYTEA":
		return "[]byte"
	case "BOOLEAN", "BOOL":
		return "bool"
	case "DATE", "DATETIME", "TIMESTAMP", "TIMESTAMPTZ":
		return "time.Time"
	case "NUMERIC", "DECIMAL", "REAL", "FLOAT", "DOUBLE":
		return "float64"
	default:
		return "string"
	}
}

// toGoFieldName converts a snake_case column name to CamelCase Go field name.
func toGoFieldName(name string) string {
	parts := strings.Split(name, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		upper := strings.ToUpper(p)
		if upper == "ID" || upper == "URL" || upper == "API" || upper == "HTTP" || upper == "SQL" {
			b.WriteString(upper)
		} else {
			b.WriteString(strings.ToUpper(p[:1]) + strings.ToLower(p[1:]))
		}
	}
	return b.String()
}

// toGoStructName converts a table name (plural, snake_case) to a CamelCase Go struct name (singular).
func toGoStructName(tableName string) string {
	singular := singularize(tableName)
	return toGoFieldName(singular)
}

// singularize does a simple singularization by stripping trailing "s".
// Handles "ies" -> "y" (companies -> company) and "ses" -> "s" (addresses -> address).
func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "ses") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

// stripSQLComments removes SQL line comments (--) and block comments (/* ... */)
// while preserving comment-like sequences inside string literals and quoted
// identifiers (single quotes, ANSI double quotes, MySQL backticks). Doubled
// quote characters are treated as in-string escapes per SQL conventions.
func stripSQLComments(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))

	i := 0
	inSingle := false
	inDouble := false
	inBack := false
	for i < len(sql) {
		ch := sql[i]

		if inSingle {
			b.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteByte(sql[i+1])
					i += 2
					continue
				}
				inSingle = false
			}
			i++
			continue
		}

		if inDouble {
			b.WriteByte(ch)
			if ch == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					b.WriteByte(sql[i+1])
					i += 2
					continue
				}
				inDouble = false
			}
			i++
			continue
		}

		if inBack {
			b.WriteByte(ch)
			if ch == '`' {
				if i+1 < len(sql) && sql[i+1] == '`' {
					b.WriteByte(sql[i+1])
					i += 2
					continue
				}
				inBack = false
			}
			i++
			continue
		}

		switch ch {
		case '\'':
			inSingle = true
			b.WriteByte(ch)
			i++
			continue
		case '"':
			inDouble = true
			b.WriteByte(ch)
			i++
			continue
		case '`':
			inBack = true
			b.WriteByte(ch)
			i++
			continue
		}

		// Line comment: -- to end of line.
		if ch == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			// Preserve the newline to keep line structure.
			if i < len(sql) {
				b.WriteByte('\n')
				i++
			}
			continue
		}

		// Block comment: /* to */.
		if ch == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i += 2
			for i+1 < len(sql) {
				if sql[i] == '*' && sql[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			// Replace with a space to avoid joining tokens.
			b.WriteByte(' ')
			continue
		}

		b.WriteByte(ch)
		i++
	}

	return b.String()
}

func trimIdentifierQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	switch {
	case s[0] == '"' && s[len(s)-1] == '"':
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	case s[0] == '`' && s[len(s)-1] == '`':
		return strings.ReplaceAll(s[1:len(s)-1], "``", "`")
	case s[0] == '[' && s[len(s)-1] == ']':
		return s[1 : len(s)-1]
	}
	return s
}
