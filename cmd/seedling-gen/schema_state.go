package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// schemaSource is one schema input, such as a single migration file. Name is
// used in error positions and may be empty.
type schemaSource struct {
	Name string
	SQL  string
}

// ParseSchema parses SQL schema text and returns a slice of Tables.
func ParseSchema(sql string) ([]Table, error) {
	return ParseSchemaWithDialect(sql, "auto")
}

// ParseSchemaWithDialect parses SQL schema text for the given dialect.
// The dialect selects the lexical rules for string literals: PostgreSQL
// dollar quotes and E'...' escape strings, and MySQL backslash escapes.
func ParseSchemaWithDialect(sql, dialect string) ([]Table, error) {
	return parseSchemaSources([]schemaSource{{SQL: sql}}, dialect)
}

// parseSchemaSources applies the statements of every source in order and
// returns the tables as they stand after the last one, so a migration
// directory yields the schema its migrations produce: ALTER TABLE, DROP, and
// RENAME statements change the tables earlier CREATE TABLE statements defined.
// A statement that would change the schema in a way the parser cannot follow
// is reported with its position instead of being skipped.
func parseSchemaSources(sources []schemaSource, dialect string) ([]Table, error) {
	d, err := newSQLDialect(dialect)
	if err != nil {
		return nil, err
	}

	var statements []sqlStatement
	for _, src := range sources {
		text, masked := d.mask(stripRollbackStatements(src.SQL))
		statements = append(statements, splitStatements(src.Name, text, masked)...)
	}

	s := &schemaState{
		dialect:   d,
		enumTypes: resolveEnumTypes(statements),
		byName:    make(map[string]*tableState),
	}
	for _, stmt := range statements {
		if err := s.apply(stmt); err != nil {
			return nil, fmt.Errorf("%s: %w", stmt.position(), err)
		}
	}
	return s.build(), nil
}

// rollbackMarkers start the down section of a migration file in the formats
// sqlc reads: goose, sql-migrate, tern, and dbmate. Like sqlc, everything
// after the first marker is ignored.
var rollbackMarkers = []string{
	"-- +goose down",
	"-- +migrate down",
	"---- create above / drop below ----",
	"-- migrate:down",
}

// stripRollbackStatements drops the down section of a migration file, so only
// the statements that bring the schema forward are applied.
func stripRollbackStatements(sql string) string {
	offset := 0
	for line := range strings.SplitAfterSeq(sql, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		for _, marker := range rollbackMarkers {
			if strings.HasPrefix(lower, marker) {
				return sql[:offset]
			}
		}
		offset += len(line)
	}
	return sql
}

// sqlStatement is one statement of a schema source, split at a top-level
// semicolon.
type sqlStatement struct {
	sqlText
	source string
	line   int
}

func (s sqlStatement) position() string {
	if s.source == "" {
		return "line " + strconv.Itoa(s.line)
	}
	return s.source + ":" + strconv.Itoa(s.line)
}

// splitStatements splits a masked schema source at its semicolons, which are
// all structural once string literals, quoted identifiers, and comments are
// masked. Each statement records the line it starts on.
func splitStatements(source, text, masked string) []sqlStatement {
	var statements []sqlStatement
	line, start := 1, 0
	emit := func(end int) {
		body := sqlText{Text: text[start:end], Masked: masked[start:end]}
		leading := len(body.Masked) - len(strings.TrimLeft(body.Masked, " \t\r\n"))
		if leading < len(body.Masked) {
			statements = append(statements, sqlStatement{
				sqlText: body,
				source:  source,
				line:    line + strings.Count(body.Masked[:leading], "\n"),
			})
		}
		line += strings.Count(body.Masked, "\n")
	}
	for i := 0; i < len(masked); i++ {
		if masked[i] == ';' {
			emit(i)
			line += strings.Count(masked[i:i+1], "\n")
			start = i + 1
		}
	}
	emit(len(masked))
	return statements
}

// schemaState is the schema built so far while statements are applied.
type schemaState struct {
	dialect   sqlDialect
	enumTypes map[string][]string
	tables    []*tableState
	byName    map[string]*tableState
}

// tableState holds a table's columns and constraints separately, so a later
// statement can drop or rename one constraint without losing the others. The
// flags on Column that constraints imply are derived from them in build.
type tableState struct {
	name         string
	columns      []Column // declared NOT NULL and type only; EnumValues holds the type's values
	primaryKey   *keyConstraint
	foreignKeys  []*foreignKeyConstraint
	uniques      []*keyConstraint
	checks       []*checkConstraint
	withoutRowID bool
	unnamedFKs   int // counts unnamed foreign keys for MySQL's generated names
}

// keyConstraint is a primary key or unique constraint. names lists every
// name a later statement may refer to it by: the declared one, or the names
// the databases generate for an unnamed constraint.
type keyConstraint struct {
	names   []string
	columns []string
}

type foreignKeyConstraint struct {
	keyConstraint
	refTable   string
	refColumns []string
	inline     bool // declared with REFERENCES on the column itself
}

// checkConstraint is a CHECK constraint. column and values are set for the
// CHECK (column IN (...)) shape, the only one that restricts values.
type checkConstraint struct {
	names  []string
	column string
	values []string
}

func (s *schemaState) table(name string) *tableState {
	return s.byName[normalizeIdent(name)]
}

// build derives the parsed tables from the current state.
func (s *schemaState) build() []Table {
	tables := make([]Table, 0, len(s.tables))
	for _, t := range s.tables {
		tables = append(tables, t.build(s.dialect))
	}
	return tables
}

func (t *tableState) build(d sqlDialect) Table {
	columns := slices.Clone(t.columns)
	columnIndex := make(map[string]int, len(columns))
	for i, col := range columns {
		columnIndex[col.Name] = i
		columns[i].EnumValues = slices.Clone(col.EnumValues)
	}

	if t.primaryKey != nil {
		for _, name := range t.primaryKey.columns {
			if idx, ok := columnIndex[name]; ok {
				columns[idx].IsPK = true
			}
		}
	}
	for _, check := range t.checks {
		if idx, ok := columnIndex[check.column]; ok && check.values != nil {
			restrictEnum(&columns[idx], check.values)
		}
	}
	for _, unique := range t.uniques {
		markUniqueColumns(columns, columnIndex, unique.columns)
	}
	// Nullability is settled only once every constraint is known, because a
	// table-level PRIMARY KEY can make a column NOT NULL after its foreign key
	// was read.
	markPrimaryKeysNotNull(columns, t.withoutRowID, d)

	foreignKeys := make([]ForeignKey, 0, len(t.foreignKeys))
	for _, fk := range t.foreignKeys {
		for _, name := range fk.columns {
			idx, ok := columnIndex[name]
			if !ok {
				continue
			}
			columns[idx].IsFK = true
			columns[idx].FKRefTable = fk.refTable
			if fk.inline {
				columns[idx].FKRefColumns = slices.Clone(fk.refColumns)
			}
		}
		foreignKeys = append(foreignKeys, ForeignKey{
			Columns:    slices.Clone(fk.columns),
			RefTable:   fk.refTable,
			RefColumns: slices.Clone(fk.refColumns),
			NotNull:    columnsNotNull(columns, columnIndex, fk.columns),
		})
	}

	return Table{
		Name:        t.name,
		GoName:      toGoStructName(t.name),
		BlueprintID: singularize(t.name),
		Columns:     columns,
		ForeignKeys: foreignKeys,
	}
}

// markPrimaryKeysNotNull applies the NOT NULL that a primary key implies. In
// SQLite a key column may hold NULL unless it is the INTEGER PRIMARY KEY rowid
// alias (not declared DESC inline) or the table is WITHOUT ROWID; every other
// dialect follows standard SQL.
func markPrimaryKeysNotNull(columns []Column, withoutRowID bool, d sqlDialect) {
	var keys []int
	for i, col := range columns {
		if col.IsPK {
			keys = append(keys, i)
		}
	}
	if d.sqliteKeys && !withoutRowID {
		if len(keys) == 1 && columns[keys[0]].SQLType == "INTEGER" && !columns[keys[0]].descendingKey {
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

func (t *tableState) columnIndex(name string) int {
	return slices.IndexFunc(t.columns, func(col Column) bool { return col.Name == name })
}

// generatedName returns the name PostgreSQL gives an unnamed constraint, such
// as users_email_key for UNIQUE (email).
func (t *tableState) generatedName(columns []string, suffix string) string {
	parts := append([]string{t.name}, columns...)
	return strings.Join(append(parts, suffix), "_")
}

// constraintNameRE matches the CONSTRAINT name clause that may precede a
// constraint keyword.
var constraintNameRE = regexp.MustCompile(`(?i)\bCONSTRAINT\s+(\S+)\s*$`)

// declaredConstraintName returns the name a CONSTRAINT clause gives the
// constraint whose keyword starts at keyword in item, or "".
func declaredConstraintName(item sqlText, keyword int) string {
	loc := constraintNameRE.FindStringSubmatchIndex(item.Masked[:keyword])
	if loc == nil {
		return ""
	}
	return normalizeIdent(item.Text[loc[2]:loc[3]])
}

// inlineConstraintName returns the declared name of the inline constraint
// keyword matches in item, or "".
func inlineConstraintName(item sqlText, keyword *regexp.Regexp) string {
	loc := keyword.FindStringIndex(item.Masked)
	if loc == nil {
		return ""
	}
	return declaredConstraintName(item, loc[0])
}

func constraintNames(declared string, generated ...string) []string {
	if declared != "" {
		return []string{declared}
	}
	return generated
}

var (
	inlinePKRE     = regexp.MustCompile(`(?i)\bPRIMARY\s+KEY\b`)
	inlineUniqueRE = regexp.MustCompile(`(?i)\bUNIQUE\b`)
	checkRE        = regexp.MustCompile(`(?i)\bCHECK\s*\(`)
)

// defineColumn inserts a column definition at position, or replaces the
// column there, turning its inline PRIMARY KEY, UNIQUE, and REFERENCES
// constraints into table constraints. Inline CHECK constraints are left to
// addChecks.
func (t *tableState) defineColumn(item sqlText, enumTypes map[string][]string, backslash bool, position int, replace bool) bool {
	col, ok := parseColumn(item, enumTypes, backslash)
	if !ok {
		return false
	}
	inlinePK, refs, refTable, refColumns := col.IsPK, col.IsFK, col.FKRefTable, col.FKRefColumns
	col.IsPK, col.Unique, col.IsFK, col.FKRefTable, col.FKRefColumns = false, false, false, "", nil
	if replace {
		t.columns[position] = col
	} else {
		t.columns = slices.Insert(t.columns, position, col)
	}

	columns := []string{col.Name}
	if inlinePK {
		t.addPrimaryKey(inlineConstraintName(item, inlinePKRE), columns)
	} else if inlineUniqueRE.MatchString(item.Masked) {
		t.addUnique(inlineConstraintName(item, inlineUniqueRE), "", columns)
	}
	if refs {
		t.addForeignKey(inlineConstraintName(item, inlineRefRE), columns, refTable, refColumns, true)
	}
	return true
}

func (t *tableState) addPrimaryKey(declared string, columns []string) {
	if t.primaryKey == nil {
		t.primaryKey = &keyConstraint{names: constraintNames(declared, t.name+"_pkey", "primary")}
	}
	for _, col := range columns {
		if !slices.Contains(t.primaryKey.columns, col) {
			t.primaryKey.columns = append(t.primaryKey.columns, col)
		}
	}
}

// addUnique records a unique constraint. index is the MySQL index name of a
// UNIQUE KEY name (...) clause, which names the constraint as well.
func (t *tableState) addUnique(declared, index string, columns []string) {
	if declared == "" {
		declared = index
	}
	names := constraintNames(declared, t.generatedName(columns, "key"))
	if declared == "" && len(columns) > 0 {
		names = append(names, columns[0]) // MySQL names an unnamed unique index after its first column
	}
	t.uniques = append(t.uniques, &keyConstraint{names: names, columns: columns})
}

func (t *tableState) addForeignKey(declared string, columns []string, refTable string, refColumns []string, inline bool) {
	names := []string{declared}
	if declared == "" {
		t.unnamedFKs++
		names = []string{t.generatedName(columns, "fkey"), t.name + "_ibfk_" + strconv.Itoa(t.unnamedFKs)}
	}
	t.foreignKeys = append(t.foreignKeys, &foreignKeyConstraint{
		keyConstraint: keyConstraint{names: names, columns: columns},
		refTable:      refTable,
		refColumns:    refColumns,
		inline:        inline,
	})
}

// addChecks records every CHECK constraint in text. A CHECK (column IN (...))
// also restricts the values of that column.
func (t *tableState) addChecks(text sqlText, backslash bool) {
	for _, loc := range checkRE.FindAllStringIndex(text.Masked, -1) {
		check := &checkConstraint{}
		declared := declaredConstraintName(text, loc[0])
		end := findMatchingParen(text.Masked, loc[1]-1)
		if end == -1 {
			end = len(text.Masked)
		} else {
			end++
		}
		if in := checkInRE.FindStringSubmatchIndex(text.Masked[loc[0]:end]); in != nil && in[0] == 0 {
			body := text.slice(loc[0], end)
			column := normalizeIdent(body.Text[in[2]:in[3]])
			if t.columnIndex(column) >= 0 {
				if values, ok := parseStringLiterals(body.Text[in[4]:in[5]], backslash); ok {
					check.column, check.values = column, values
				}
			}
		}
		generated := []string{t.name + "_check"}
		if check.column != "" {
			generated = []string{t.generatedName([]string{check.column}, "check"), t.name + "_check"}
		}
		check.names = constraintNames(declared, generated...)
		t.checks = append(t.checks, check)
	}
}

var tableUniqueKeyRE = regexp.MustCompile(`(?i)^UNIQUE(?:\s+(?:KEY|INDEX))?\s+([^\s(]+)\s*\(`)

// addConstraint records a table constraint, as written in CREATE TABLE or
// after ADD in ALTER TABLE.
func (t *tableState) addConstraint(constraint sqlText) {
	// Constraint keywords are matched in the masked text, so a CHECK body like
	// 'PRIMARY KEY (id)' is not mistaken for a real constraint. Captured
	// identifiers are sliced from the original to keep their exact spelling.
	leading := len(constraint.Masked) - len(strings.TrimLeft(constraint.Masked, " \t\r\n"))
	trimmed := constraint.slice(leading, len(constraint.Masked))
	declared := ""
	if loc := leadingConstraintNameRE.FindStringSubmatchIndex(trimmed.Masked); loc != nil {
		declared = normalizeIdent(trimmed.Text[loc[2]:loc[3]])
		trimmed = trimmed.slice(loc[1], len(trimmed.Masked))
	}

	if loc := uniqueColumnsRE.FindStringSubmatchIndex(trimmed.Masked); loc != nil {
		index := ""
		if key := tableUniqueKeyRE.FindStringSubmatchIndex(trimmed.Masked); key != nil {
			index = normalizeIdent(trimmed.Text[key[2]:key[3]])
		}
		t.addUnique(declared, index, indexColumnNames(trimmed.Text[loc[2]:loc[3]]))
	}
	if loc := tablePKRE.FindStringSubmatchIndex(trimmed.Masked); loc != nil {
		t.addPrimaryKey(declared, splitIdentifierList(trimmed.Text[loc[2]:loc[3]]))
	}
	if loc := tableFKRE.FindStringSubmatchIndex(trimmed.Masked); loc != nil {
		var refColumns []string
		if loc[6] >= 0 {
			refColumns = splitIdentifierList(trimmed.Text[loc[6]:loc[7]])
		}
		t.addForeignKey(declared, splitIdentifierList(trimmed.Text[loc[2]:loc[3]]), normalizeIdent(trimmed.Text[loc[4]:loc[5]]), refColumns, false)
	}
}

var leadingConstraintNameRE = regexp.MustCompile(`(?i)^CONSTRAINT\s+(\S+)\s+`)

// createTables applies every CREATE TABLE in stmt.
func (s *schemaState) createTables(stmt sqlStatement) error {
	for _, block := range extractCreateTableBlocks(stmt.sqlText) {
		if block.Unclosed {
			return fmt.Errorf("parse CREATE TABLE %s: unclosed parenthesis", block.Name)
		}
		name := normalizeIdent(block.Name)
		if _, exists := s.byName[name]; exists {
			if block.IfNotExists {
				continue
			}
			return fmt.Errorf("CREATE TABLE %s: table is already defined; drop or rename it first", name)
		}
		t := &tableState{name: name, withoutRowID: block.WithoutRowID}
		items := splitSQLItems(block.Body)
		var constraints []sqlText
		for _, item := range items {
			if strings.TrimSpace(item.Masked) == "" {
				continue
			}
			if isTableConstraint(item.Masked) {
				constraints = append(constraints, item)
				continue
			}
			t.defineColumn(item, s.enumTypes, s.dialect.backslashEscapes, len(t.columns), false)
		}
		for _, constraint := range constraints {
			t.addConstraint(constraint)
		}
		// Checks are read once every column exists, so a check may refer to a
		// column defined after it.
		for _, item := range items {
			t.addChecks(item, s.dialect.backslashEscapes)
		}
		s.tables = append(s.tables, t)
		s.byName[name] = t
	}
	return nil
}

type createTableBlock struct {
	Name         string
	Body         sqlText
	WithoutRowID bool // SQLite WITHOUT ROWID table option
	IfNotExists  bool
	Unclosed     bool
}

var (
	withoutRowIDRE  = regexp.MustCompile(`(?i)\bWITHOUT\s+ROWID\b`)
	descendingKeyRE = regexp.MustCompile(`(?i)\bPRIMARY\s+KEY\s+DESC\b`)
	ifNotExistsRE   = regexp.MustCompile(`(?i)IF\s+NOT\s+EXISTS`)
)

// extractCreateTableBlocks finds the CREATE TABLE statements in stmt. A
// statement normally holds one, but a missing semicolon must not hide the
// next table.
func extractCreateTableBlocks(stmt sqlText) []createTableBlock {
	var blocks []createTableBlock
	searchFrom := 0
	for {
		loc := createTableStartRE.FindStringSubmatchIndex(stmt.Masked[searchFrom:])
		if loc == nil {
			return blocks
		}
		matchStart := searchFrom + loc[0]
		bodyStart := searchFrom + loc[1]
		name := stmt.Text[searchFrom+loc[2] : searchFrom+loc[3]]
		ifNotExists := ifNotExistsRE.MatchString(stmt.Masked[matchStart:bodyStart])
		bodyEnd := findMatchingParen(stmt.Masked, bodyStart-1)
		if bodyEnd == -1 {
			return append(blocks, createTableBlock{Name: name, Unclosed: true})
		}
		options := stmt.Masked[bodyEnd+1:]
		if next := createTableStartRE.FindStringIndex(options); next != nil {
			options = options[:next[0]]
		}
		blocks = append(blocks, createTableBlock{
			Name:         name,
			Body:         stmt.slice(bodyStart, bodyEnd),
			WithoutRowID: withoutRowIDRE.MatchString(options),
			IfNotExists:  ifNotExists,
		})
		searchFrom = bodyEnd + 1
	}
}
