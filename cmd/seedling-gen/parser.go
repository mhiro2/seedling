package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	createTableStartRE = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s(]+)\s*\(`)
	createEnumRE       = regexp.MustCompile(`(?is)CREATE\s+TYPE\s+([^\s]+)\s+AS\s+ENUM\s*\(`)
	inlineRefRE        = regexp.MustCompile(`(?i)REFERENCES\s+([^\s(]+)(?:\s*\(([^)]+)\))?`)
	tablePKRE          = regexp.MustCompile(`(?i)(?:CONSTRAINT\s+[^\s]+\s+)?PRIMARY\s+KEY\s*\(([^)]+)\)`)
	tableFKRE          = regexp.MustCompile(`(?i)(?:CONSTRAINT\s+[^\s]+\s+)?FOREIGN\s+KEY\s*\(([^)]+)\)\s+REFERENCES\s+([^\s(]+)(?:\s*\(([^)]+)\))?`)
)

// Column represents a parsed column from a CREATE TABLE statement.
type Column struct {
	Name         string
	SQLType      string
	GoName       string
	GoType       string
	IsPK         bool
	IsFK         bool
	FKRefTable   string
	FKRefColumns []string
	NotNull      bool

	// EnumValues lists the values the column accepts, read from an ENUM
	// type or a CHECK (column IN (...)) constraint.
	EnumValues []string
	// MaxLength is the declared maximum length of a string column, or zero.
	MaxLength int
	// Unique marks a column that a single-column unique constraint or index
	// covers.
	Unique bool

	// descendingKey marks an inline PRIMARY KEY DESC, which in SQLite is not an
	// alias for the rowid even on an INTEGER column.
	descendingKey bool
}

// ForeignKey represents a table-level or inline foreign key constraint.
type ForeignKey struct {
	Columns    []string
	RefTable   string
	RefColumns []string
	NotNull    bool
}

// Table represents a parsed CREATE TABLE statement.
type Table struct {
	Name        string
	GoName      string
	BlueprintID string
	Columns     []Column
	ForeignKeys []ForeignKey
}

// sqlText pairs a slice of the schema, with comments blanked, and the same
// slice of its masked form. Structure and keywords are found in Masked, where
// string literals and quoted identifiers can no longer look like SQL. Names
// and types are read from Text at the same offsets.
type sqlText struct {
	Text   string
	Masked string
}

func (t sqlText) slice(from, to int) sqlText {
	return sqlText{Text: t.Text[from:to], Masked: t.Masked[from:to]}
}

// findMatchingParen returns the index of the parenthesis closing the one at
// openIdx. masked comes from sqlDialect.mask, so every parenthesis is structural.
func findMatchingParen(masked string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(masked); i++ {
		switch masked[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseColumn(item sqlText, enumTypes map[string][]string, backslash bool) (Column, bool) {
	nameToken, rest, ok := splitLeadingIdent(item.Text)
	if !ok {
		return Column{}, false
	}

	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return Column{}, false
	}

	colName := normalizeIdent(nameToken)
	sqlType := normalizeSQLType(parts[0])

	// Constraints are detected in the masked text, so a DEFAULT value like
	// 'this is NOT NULL really' or $$see REFERENCES x$$ is not mistaken for a
	// real constraint. A REFERENCES target is sliced from the original.
	upper := strings.ToUpper(item.Masked)
	col := Column{
		Name:    colName,
		SQLType: sqlType,
		GoName:  toGoFieldName(colName),
		GoType:  sqlTypeToGoType(sqlType, enumTypes),
		IsPK:    strings.Contains(upper, "PRIMARY KEY"),
		NotNull: strings.Contains(upper, "NOT NULL"),

		descendingKey: descendingKeyRE.MatchString(upper),
	}
	if values, ok := enumTypes[strings.ToLower(sqlType)]; ok {
		col.EnumValues = values
	}
	if afterName := strings.Index(item.Text, nameToken) + len(nameToken); afterName >= len(nameToken) {
		applyColumnTypeArgs(&col, item.slice(afterName, len(item.Text)), backslash)
	}
	col.Unique = !col.IsPK && columnUniqueRE.MatchString(item.Masked)

	if loc := inlineRefRE.FindStringSubmatchIndex(item.Masked); loc != nil {
		col.IsFK = true
		col.FKRefTable = normalizeIdent(item.Text[loc[2]:loc[3]])
		if loc[4] >= 0 {
			col.FKRefColumns = splitIdentifierList(item.Text[loc[4]:loc[5]])
		}
	}

	return col, true
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

// splitSQLItems splits a CREATE TABLE body at its top-level commas.
func splitSQLItems(body sqlText) []sqlText {
	var items []sqlText
	depth, start := 0, 0
	for i := 0; i < len(body.Masked); i++ {
		switch body.Masked[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				items = append(items, body.slice(start, i))
				start = i + 1
			}
		}
	}
	if start < len(body.Masked) {
		items = append(items, body.slice(start, len(body.Masked)))
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

func sqlTypeToGoType(sqlType string, enumTypes ...map[string][]string) string {
	var knownEnums map[string][]string
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

// sqlDialect holds the rules that differ between SQL dialects. Lexically,
// PostgreSQL recognizes dollar-quoted strings, E'...' strings with backslash
// escapes, and nested block comments; MySQL treats a backslash inside a string
// literal as an escape and # as a line comment; SQLite uses only the standard
// quoting. auto follows PostgreSQL and also reads a # that starts a line as a
// MySQL comment. sqliteKeys selects SQLite's primary key semantics, under
// which a key column may hold NULL.
type sqlDialect struct {
	dollarQuotes          bool
	backslashEscapes      bool
	escapeStringConst     bool
	nestedComments        bool
	hashComments          bool // # starts a comment anywhere
	lineStartHashComments bool // # starts a comment only as the first token of a line
	sqliteKeys            bool
}

func newSQLDialect(dialect string) (sqlDialect, error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "auto":
		return sqlDialect{dollarQuotes: true, escapeStringConst: true, nestedComments: true, lineStartHashComments: true}, nil
	case "postgres":
		return sqlDialect{dollarQuotes: true, escapeStringConst: true, nestedComments: true}, nil
	case "mysql":
		return sqlDialect{backslashEscapes: true, hashComments: true}, nil
	case "sqlite":
		return sqlDialect{sqliteKeys: true}, nil
	default:
		return sqlDialect{}, fmt.Errorf("unsupported dialect %q", dialect)
	}
}

// mask returns two offset-preserving copies of sql. In text, comments become
// spaces. In masked, everything that is not SQL structure is neutralized as
// well: the contents of string literals become spaces, and the contents of
// quoted identifiers ("..." and `...`) become underscores, so they still read
// as one identifier token. Keywords, parentheses, commas, and semicolons left
// in masked are all structural; names and types are read from text.
func (l sqlDialect) mask(sql string) (text, masked string) {
	t := []byte(sql)
	b := []byte(sql)
	fill := func(from, to int, c byte) {
		for i := from; i < to; i++ {
			if b[i] != '\n' {
				b[i] = c
			}
		}
	}
	comment := func(from, to int) {
		fill(from, to, ' ')
		for i := from; i < to; i++ {
			if t[i] != '\n' {
				t[i] = ' '
			}
		}
	}

	for i := 0; i < len(b); {
		switch c := b[i]; {
		case c == '-' && i+1 < len(b) && b[i+1] == '-',
			c == '#' && (l.hashComments || l.lineStartHashComments && startsLine(sql, i)):
			end := indexByteFrom(sql, '\n', i)
			comment(i, end)
			i = end
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := blockCommentEnd(sql, i, l.nestedComments)
			comment(i, end)
			i = end
		case c == '\'':
			backslash := l.backslashEscapes || l.escapeStringConst && isEscapeStringPrefix(sql, i)
			end := quotedEnd(sql, i, '\'', backslash)
			fill(i+1, max(end-1, i+1), ' ')
			i = end
		case c == '"' || c == '`':
			end := quotedEnd(sql, i, c, false)
			fill(i+1, max(end-1, i+1), '_')
			i = end
		case c == '$' && l.dollarQuotes && (i == 0 || !isSQLIdentByte(b[i-1])):
			tag, ok := dollarQuoteTag(sql, i)
			if !ok {
				i++
				continue
			}
			bodyStart := i + len(tag)
			end := len(b)
			if j := strings.Index(sql[bodyStart:], tag); j != -1 {
				end = bodyStart + j
			}
			fill(bodyStart, end, ' ')
			i = min(end+len(tag), len(b))
		default:
			i++
		}
	}
	return string(t), string(b)
}

// blockCommentEnd returns the index just past the block comment opening at i.
// With nested, an inner /* ... */ must close before the outer one does, as in
// PostgreSQL. An unterminated comment runs to the end of the input.
func blockCommentEnd(s string, i int, nested bool) int {
	depth := 0
	for j := i; j+1 < len(s); j++ {
		switch {
		case s[j] == '/' && s[j+1] == '*' && (nested || depth == 0):
			depth++
			j++
		case s[j] == '*' && s[j+1] == '/':
			depth--
			j++
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(s)
}

// startsLine reports whether only spaces or tabs precede i on its line.
func startsLine(s string, i int) bool {
	for j := i - 1; j >= 0 && s[j] != '\n'; j-- {
		if s[j] != ' ' && s[j] != '\t' && s[j] != '\r' {
			return false
		}
	}
	return true
}

func indexByteFrom(s string, c byte, from int) int {
	if j := strings.IndexByte(s[from:], c); j != -1 {
		return from + j
	}
	return len(s)
}

// quotedEnd returns the index just past the quote closing the one at open. A
// doubled quote is an escape; with backslash, so is a backslash before any
// character. An unterminated quote runs to the end of the input.
func quotedEnd(s string, open int, quote byte, backslash bool) int {
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if backslash {
				i++
			}
		case quote:
			if i+1 < len(s) && s[i+1] == quote {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(s)
}

// isEscapeStringPrefix reports whether the quote at i opens a PostgreSQL
// escape string constant such as E'it\'s'.
func isEscapeStringPrefix(s string, i int) bool {
	if i == 0 || (s[i-1] != 'E' && s[i-1] != 'e') {
		return false
	}
	return i == 1 || !isSQLIdentByte(s[i-2])
}

// dollarQuoteTag returns the opening delimiter of a PostgreSQL dollar-quoted
// string at i, such as $$ or $body$.
func dollarQuoteTag(s string, i int) (string, bool) {
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		if c == '$' {
			return s[i : j+1], true
		}
		if !isSQLIdentByte(c) || (j == i+1 && c >= '0' && c <= '9') {
			return "", false
		}
	}
	return "", false
}

func isSQLIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
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
