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

// ParseSchema parses SQL schema text and returns a slice of Tables.
func ParseSchema(sql string) ([]Table, error) {
	return ParseSchemaWithDialect(sql, "auto")
}

// ParseSchemaWithDialect parses SQL schema text for the given dialect.
// The dialect selects the lexical rules for string literals: PostgreSQL
// dollar quotes and E'...' escape strings, and MySQL backslash escapes.
func ParseSchemaWithDialect(sql, dialect string) ([]Table, error) {
	d, err := newSQLDialect(dialect)
	if err != nil {
		return nil, err
	}
	sql, masked := d.mask(sql)

	var tables []Table
	enumTypes := extractEnumTypes(sql, masked)

	blocks, err := extractCreateTableBlocks(sql, masked)
	if err != nil {
		return nil, err
	}
	for _, block := range blocks {
		tableName := normalizeIdent(block.Name)
		columns, foreignKeys := parseColumns(block, enumTypes, d)
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

type createTableBlock struct {
	Name         string
	Body         sqlText
	WithoutRowID bool // SQLite WITHOUT ROWID table option
}

var withoutRowIDRE = regexp.MustCompile(`(?i)\bWITHOUT\s+ROWID\b`)

func extractCreateTableBlocks(sql, masked string) ([]createTableBlock, error) {
	var blocks []createTableBlock
	searchFrom := 0

	for {
		loc := createTableStartRE.FindStringSubmatchIndex(masked[searchFrom:])
		if loc == nil {
			return blocks, nil
		}

		matchStart := searchFrom + loc[0]
		bodyStart := searchFrom + loc[1]
		nameStart := searchFrom + loc[2]
		nameEnd := searchFrom + loc[3]

		tableName := sql[nameStart:nameEnd]
		bodyEnd := findMatchingParen(masked, bodyStart-1)
		if bodyEnd == -1 {
			return nil, fmt.Errorf("parse CREATE TABLE %s: unclosed parenthesis", tableName)
		}

		stmtEnd := strings.IndexByte(masked[bodyEnd:], ';')
		options := masked[bodyEnd+1:]
		if stmtEnd != -1 {
			options = masked[bodyEnd+1 : bodyEnd+stmtEnd]
		}
		blocks = append(blocks, createTableBlock{
			Name:         tableName,
			Body:         sqlText{Text: sql[bodyStart:bodyEnd], Masked: masked[bodyStart:bodyEnd]},
			WithoutRowID: withoutRowIDRE.MatchString(options),
		})

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

func extractEnumTypes(sql, masked string) map[string]struct{} {
	locs := createEnumRE.FindAllStringSubmatchIndex(masked, -1)
	if len(locs) == 0 {
		return nil
	}

	enumTypes := make(map[string]struct{}, len(locs))
	for _, loc := range locs {
		enumTypes[normalizeIdent(sql[loc[2]:loc[3]])] = struct{}{}
	}

	return enumTypes
}

func parseColumns(block createTableBlock, enumTypes map[string]struct{}, d sqlDialect) ([]Column, []ForeignKey) {
	items := splitSQLItems(block.Body)
	columns := make([]Column, 0, len(items))
	foreignKeys := make([]ForeignKey, 0, len(items))
	columnIndex := make(map[string]int, len(items))
	tableConstraints := make([]sqlText, 0, len(items))

	for _, item := range items {
		if strings.TrimSpace(item.Masked) == "" {
			continue
		}

		if isTableConstraint(item.Masked) {
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
				Columns:    []string{col.Name},
				RefTable:   col.FKRefTable,
				RefColumns: append([]string(nil), col.FKRefColumns...),
			})
		}
	}

	for _, constraint := range tableConstraints {
		if fk, ok := applyTableConstraint(columns, columnIndex, constraint); ok {
			foreignKeys = append(foreignKeys, fk)
		}
	}

	// Nullability is settled only once every constraint is known, because a
	// table-level PRIMARY KEY can make a column NOT NULL after its foreign key
	// was read.
	markPrimaryKeysNotNull(columns, block.WithoutRowID, d)
	for i := range foreignKeys {
		foreignKeys[i].NotNull = columnsNotNull(columns, columnIndex, foreignKeys[i].Columns)
	}

	return columns, foreignKeys
}

// markPrimaryKeysNotNull applies the NOT NULL that a primary key implies. In
// SQLite a key column may hold NULL unless it is the INTEGER PRIMARY KEY rowid
// alias or the table is WITHOUT ROWID; every other dialect follows standard SQL.
func markPrimaryKeysNotNull(columns []Column, withoutRowID bool, d sqlDialect) {
	var keys []int
	for i, col := range columns {
		if col.IsPK {
			keys = append(keys, i)
		}
	}
	if d.sqliteKeys && !withoutRowID {
		if len(keys) == 1 && columns[keys[0]].SQLType == "INTEGER" {
			columns[keys[0]].NotNull = true
		}
		return
	}
	for _, i := range keys {
		columns[i].NotNull = true
	}
}

// columnsNotNull reports whether every named column is NOT NULL, and so
// whether a foreign key over them always references a parent.
func columnsNotNull(columns []Column, columnIndex map[string]int, names []string) bool {
	for _, name := range names {
		if idx, ok := columnIndex[name]; ok && !columns[idx].NotNull {
			return false
		}
	}
	return true
}

func parseColumn(item sqlText, enumTypes map[string]struct{}) (Column, bool) {
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
	}

	if loc := inlineRefRE.FindStringSubmatchIndex(item.Masked); loc != nil {
		col.IsFK = true
		col.FKRefTable = normalizeIdent(item.Text[loc[2]:loc[3]])
		if loc[4] >= 0 {
			col.FKRefColumns = splitIdentifierList(item.Text[loc[4]:loc[5]])
		}
	}

	return col, true
}

func applyTableConstraint(columns []Column, columnIndex map[string]int, constraint sqlText) (ForeignKey, bool) {
	// Constraint keywords are matched in the masked text, so a CHECK body like
	// 'PRIMARY KEY (id)' is not mistaken for a real constraint. Captured
	// identifiers are sliced from the original to keep their exact spelling.
	if loc := tablePKRE.FindStringSubmatchIndex(constraint.Masked); loc != nil {
		for _, colName := range splitIdentifierList(constraint.Text[loc[2]:loc[3]]) {
			if idx, ok := columnIndex[colName]; ok {
				columns[idx].IsPK = true
			}
		}
	}

	if loc := tableFKRE.FindStringSubmatchIndex(constraint.Masked); loc != nil {
		refTable := normalizeIdent(constraint.Text[loc[4]:loc[5]])
		cols := splitIdentifierList(constraint.Text[loc[2]:loc[3]])
		var refColumns []string
		if loc[6] >= 0 {
			refColumns = splitIdentifierList(constraint.Text[loc[6]:loc[7]])
		}
		for _, colName := range cols {
			if idx, ok := columnIndex[colName]; ok {
				columns[idx].IsFK = true
				columns[idx].FKRefTable = refTable
			}
		}
		return ForeignKey{Columns: cols, RefTable: refTable, RefColumns: refColumns}, true
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

// sqlDialect holds the rules that differ between SQL dialects. Lexically,
// PostgreSQL (and auto) recognizes dollar-quoted strings and E'...' strings
// with backslash escapes, MySQL treats a backslash inside a string literal as
// an escape, and SQLite uses only the standard quoting. sqliteKeys selects
// SQLite's primary key semantics, under which a key column may hold NULL.
type sqlDialect struct {
	dollarQuotes      bool
	backslashEscapes  bool
	escapeStringConst bool
	sqliteKeys        bool
}

func newSQLDialect(dialect string) (sqlDialect, error) {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "", "auto", "postgres":
		return sqlDialect{dollarQuotes: true, escapeStringConst: true}, nil
	case "mysql":
		return sqlDialect{backslashEscapes: true}, nil
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
		case c == '-' && i+1 < len(b) && b[i+1] == '-':
			end := indexByteFrom(sql, '\n', i)
			comment(i, end)
			i = end
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := len(b)
			if j := strings.Index(sql[i+2:], "*/"); j != -1 {
				end = i + 2 + j + 2
			}
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
