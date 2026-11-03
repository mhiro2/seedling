package main

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	alterTableRE  = regexp.MustCompile(`(?is)^\s*ALTER\s+(?:ONLINE\s+|IGNORE\s+)?TABLE\s+(IF\s+EXISTS\s+)?(?:ONLY\s+)?([^\s(,]+)`)
	dropTableRE   = regexp.MustCompile(`(?is)^\s*DROP\s+(?:TEMPORARY\s+)?TABLE\s+(IF\s+EXISTS\s+)?`)
	renameTableRE = regexp.MustCompile(`(?is)^\s*RENAME\s+TABLE\s+`)
	dropIndexRE   = regexp.MustCompile(`(?is)^\s*DROP\s+INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+EXISTS\s+)?`)
	alterIndexRE  = regexp.MustCompile(`(?is)^\s*ALTER\s+INDEX\s+(?:IF\s+EXISTS\s+)?([^\s]+)\s+RENAME\s+TO\s+([^\s]+)\s*$`)
	alterTypeRE   = regexp.MustCompile(`(?is)^\s*ALTER\s+TYPE\s+([^\s]+)\s+`)
	dropTypeRE    = regexp.MustCompile(`(?is)^\s*DROP\s+TYPE\s+(?:IF\s+EXISTS\s+)?`)

	trailingDropOptionsRE = regexp.MustCompile(`(?is)(?:\s+(?:CASCADE|RESTRICT|PURGE))+\s*$`)
	indexOnTableRE        = regexp.MustCompile(`(?is)\s+ON\s+([^\s]+).*$`)
	renamePairRE          = regexp.MustCompile(`(?is)^\s*([^\s]+)\s+TO\s+([^\s]+)\s*$`)
	columnPositionRE      = regexp.MustCompile(`(?is)\s+(?:FIRST|AFTER\s+([^\s]+))\s*$`)
	columnTypeEndRE       = regexp.MustCompile(`(?is)\s+(?:USING|COLLATE)\s+`)

	addValueRE    = regexp.MustCompile(`(?is)^ADD\s+VALUE\s+(?:IF\s+NOT\s+EXISTS\s+)?(E?'[^']*')(?:\s+(BEFORE|AFTER)\s+(E?'[^']*'))?\s*$`)
	renameValueRE = regexp.MustCompile(`(?is)^RENAME\s+VALUE\s+(E?'[^']*')\s+TO\s+(E?'[^']*')\s*$`)
	renameTypeRE  = regexp.MustCompile(`(?is)^RENAME\s+TO\s+([^\s]+)\s*$`)
)

// ignoredAlterActions are ALTER TABLE actions that leave the columns, keys,
// and value constraints seedling-gen reads unchanged, keyed by their first
// keyword.
var ignoredAlterActions = map[string]bool{
	// PostgreSQL
	"OWNER": true, "SET": true, "RESET": true, "ENABLE": true, "DISABLE": true, "FORCE": true,
	"NO": true, "CLUSTER": true, "REPLICA": true, "INHERIT": true, "OF": true, "NOT": true,
	"ATTACH": true, "DETACH": true, "VALIDATE": true,
	// MySQL table options and maintenance
	"ENGINE": true, "AUTO_INCREMENT": true, "CHARACTER": true, "CHARSET": true, "DEFAULT": true,
	"COLLATE": true, "COMMENT": true, "CONVERT": true, "ALGORITHM": true, "LOCK": true,
	"ROW_FORMAT": true, "ORDER": true, "KEY_BLOCK_SIZE": true, "STATS_PERSISTENT": true,
	"STATS_AUTO_RECALC": true, "STATS_SAMPLE_PAGES": true, "PACK_KEYS": true, "CHECKSUM": true,
	"DELAY_KEY_WRITE": true, "AVG_ROW_LENGTH": true, "MAX_ROWS": true, "MIN_ROWS": true,
	"TABLESPACE": true, "DISCARD": true, "IMPORT": true, "TRUNCATE": true, "COALESCE": true,
	"REORGANIZE": true, "EXCHANGE": true, "ANALYZE": true, "OPTIMIZE": true, "REBUILD": true,
	"REPAIR": true, "REMOVE": true, "PARTITION": true, "WITH": true, "WITHOUT": true, "UPGRADE": true,
}

// ignoredAlterColumnActions are ALTER COLUMN actions that leave a column's
// type and nullability unchanged, keyed by their first keyword.
var ignoredAlterColumnActions = map[string]bool{
	"SET DEFAULT": true, "DROP DEFAULT": true, "SET STATISTICS": true, "SET STORAGE": true,
	"SET COMPRESSION": true, "ADD GENERATED": true, "SET GENERATED": true, "RESTART": true,
	"DROP IDENTITY": true, "DROP EXPRESSION": true, "SET (": true, "RESET (": true,
	"SET VISIBLE": true, "SET INVISIBLE": true, "SET INCREMENT": true, "SET START": true,
	"SET MAXVALUE": true, "SET MINVALUE": true, "SET NO": true, "SET CYCLE": true,
}

// apply applies one statement to the schema. Statements that do not define or
// change tables, such as views, functions, and data, are ignored.
func (s *schemaState) apply(stmt sqlStatement) error {
	switch {
	case alterTableRE.MatchString(stmt.Masked):
		return s.alterTable(stmt.sqlText)
	case dropTableRE.MatchString(stmt.Masked):
		return s.dropTables(stmt.sqlText)
	case renameTableRE.MatchString(stmt.Masked):
		return s.renameTables(stmt.sqlText)
	case dropIndexRE.MatchString(stmt.Masked):
		s.dropIndexes(stmt.sqlText)
		return nil
	case alterIndexRE.MatchString(stmt.Masked):
		loc := alterIndexRE.FindStringSubmatchIndex(stmt.Masked)
		s.renameIndex(normalizeIdent(stmt.Text[loc[2]:loc[3]]), normalizeIdent(stmt.Text[loc[4]:loc[5]]))
		return nil
	}
	if err := s.createTables(stmt); err != nil {
		return err
	}
	s.createUniqueIndexes(stmt.sqlText)
	return nil
}

// createUniqueIndexes marks the columns of single-column CREATE UNIQUE INDEX
// statements as unique.
func (s *schemaState) createUniqueIndexes(stmt sqlText) {
	for _, loc := range createUniqueIndexRE.FindAllStringSubmatchIndex(stmt.Masked, -1) {
		t := s.table(stmt.Text[loc[4]:loc[5]])
		if t == nil {
			continue
		}
		name := ""
		if loc[2] >= 0 {
			name = normalizeIdent(stmt.Text[loc[2]:loc[3]])
		}
		t.uniques = append(t.uniques, &keyConstraint{
			names:   constraintNames(name),
			columns: indexColumnNames(stmt.Text[loc[6]:loc[7]]),
		})
	}
}

// dropIndexes removes the unique indexes a DROP INDEX statement names. Other
// indexes are not tracked, so an unknown name is not an error.
func (s *schemaState) dropIndexes(stmt sqlText) {
	loc := dropIndexRE.FindStringIndex(stmt.Masked)
	rest := stmt.slice(loc[1], len(stmt.Masked))
	tables := s.tables
	if on := indexOnTableRE.FindStringSubmatchIndex(rest.Masked); on != nil {
		if t := s.table(rest.Text[on[2]:on[3]]); t != nil {
			tables = []*tableState{t}
		}
		rest = rest.slice(0, on[0])
	} else if end := trailingDropOptionsRE.FindStringIndex(rest.Masked); end != nil {
		rest = rest.slice(0, end[0])
	}
	for _, name := range splitIdentifierList(rest.Text) {
		for _, t := range tables {
			t.uniques = slices.DeleteFunc(t.uniques, func(c *keyConstraint) bool { return slices.Contains(c.names, name) })
		}
	}
}

func (s *schemaState) renameIndex(from, to string) {
	for _, t := range s.tables {
		for _, c := range t.uniques {
			renameConstraintName(&c.names, from, to)
		}
	}
}

// dropTables applies DROP TABLE.
func (s *schemaState) dropTables(stmt sqlText) error {
	loc := dropTableRE.FindStringSubmatchIndex(stmt.Masked)
	ifExists := loc[2] >= 0
	rest := stmt.slice(loc[1], len(stmt.Masked))
	if end := trailingDropOptionsRE.FindStringIndex(rest.Masked); end != nil {
		rest = rest.slice(0, end[0])
	}
	for _, name := range splitIdentifierList(rest.Text) {
		t := s.byName[name]
		if t == nil {
			if ifExists {
				continue
			}
			return fmt.Errorf("DROP TABLE %s: %w", name, errTableNotDefined)
		}
		s.tables = slices.DeleteFunc(s.tables, func(other *tableState) bool { return other == t })
		delete(s.byName, name)
		// Dropping a referenced table drops the foreign keys pointing at it,
		// as CASCADE does.
		for _, other := range s.tables {
			other.foreignKeys = slices.DeleteFunc(other.foreignKeys, func(fk *foreignKeyConstraint) bool { return fk.refTable == name })
		}
	}
	return nil
}

// renameTables applies MySQL's RENAME TABLE a TO b[, c TO d].
func (s *schemaState) renameTables(stmt sqlText) error {
	loc := renameTableRE.FindStringIndex(stmt.Masked)
	for _, pair := range splitSQLItems(stmt.slice(loc[1], len(stmt.Masked))) {
		m := renamePairRE.FindStringSubmatchIndex(pair.Masked)
		if m == nil {
			return fmt.Errorf("RENAME TABLE: cannot read %q", strings.TrimSpace(pair.Text))
		}
		from := normalizeIdent(pair.Text[m[2]:m[3]])
		t := s.byName[from]
		if t == nil {
			return fmt.Errorf("RENAME TABLE %s: %w", from, errTableNotDefined)
		}
		if err := s.renameTable(t, normalizeIdent(pair.Text[m[4]:m[5]])); err != nil {
			return fmt.Errorf("RENAME TABLE %s: %w", from, err)
		}
	}
	return nil
}

var errTableNotDefined = errors.New("table is not defined by an earlier CREATE TABLE")

func (s *schemaState) renameTable(t *tableState, to string) error {
	if _, exists := s.byName[to]; exists {
		return fmt.Errorf("table %s is already defined", to)
	}
	from := t.name
	delete(s.byName, from)
	t.name = to
	s.byName[to] = t
	for _, other := range s.tables {
		for _, fk := range other.foreignKeys {
			if fk.refTable == from {
				fk.refTable = to
			}
		}
	}
	return nil
}

// alterTable applies each action of an ALTER TABLE statement.
func (s *schemaState) alterTable(stmt sqlText) error {
	loc := alterTableRE.FindStringSubmatchIndex(stmt.Masked)
	ifExists := loc[2] >= 0
	name := normalizeIdent(stmt.Text[loc[4]:loc[5]])
	actions := splitSQLItems(stmt.slice(loc[1], len(stmt.Masked)))

	t := s.byName[name]
	if t == nil {
		if ifExists {
			return nil
		}
		for _, action := range actions {
			if !ignoredAlterAction(trimSQLText(action)) {
				return fmt.Errorf("ALTER TABLE %s: %w", name, errTableNotDefined)
			}
		}
		return nil
	}
	for _, action := range actions {
		action = trimSQLText(action)
		if action.Masked == "" {
			continue
		}
		if err := s.alterTableAction(t, action); err != nil {
			return fmt.Errorf("ALTER TABLE %s: %w", name, err)
		}
	}
	return nil
}

func ignoredAlterAction(action sqlText) bool {
	word := firstWord(action)
	return word == "" || ignoredAlterActions[word]
}

// trimSQLText trims surrounding whitespace from both forms of text.
func trimSQLText(text sqlText) sqlText {
	leading := len(text.Masked) - len(strings.TrimLeft(text.Masked, " \t\r\n"))
	trailing := len(strings.TrimRight(text.Masked, " \t\r\n"))
	if trailing < leading {
		return sqlText{}
	}
	return text.slice(leading, trailing)
}

// skipWords drops the leading words of text that match words in order,
// case-insensitively, and reports how many matched.
func skipWords(text sqlText, words ...string) (sqlText, int) {
	matched := 0
	for _, word := range words {
		fields := strings.Fields(text.Masked)
		if len(fields) == 0 || !strings.EqualFold(fields[0], word) {
			break
		}
		at := strings.Index(text.Masked, fields[0]) + len(fields[0])
		text = trimSQLText(text.slice(at, len(text.Masked)))
		matched++
	}
	return text, matched
}

// optionalWords drops the given leading phrase when every word of it is
// present, and reports whether it was.
func optionalWords(text sqlText, words ...string) (sqlText, bool) {
	rest, n := skipWords(text, words...)
	if n == len(words) {
		return rest, true
	}
	return text, false
}

// firstWord returns the leading keyword of text in upper case, ending at
// whitespace, =, or (, as in ENGINE=InnoDB.
func firstWord(text sqlText) string {
	fields := strings.Fields(strings.ToUpper(text.Masked))
	if len(fields) == 0 {
		return ""
	}
	word, _, _ := strings.Cut(fields[0], "=")
	word, _, _ = strings.Cut(word, "(")
	return word
}

func (s *schemaState) alterTableAction(t *tableState, action sqlText) error {
	word := firstWord(action)
	rest, _ := skipWords(action, word)
	switch word {
	case "ADD":
		return s.alterAdd(t, rest)
	case "DROP":
		return s.alterDrop(t, rest)
	case "RENAME":
		return s.alterRename(t, rest)
	case "ALTER":
		return s.alterColumn(t, rest)
	case "MODIFY":
		rest, _ = optionalWords(rest, "COLUMN")
		return s.modifyColumn(t, "", rest)
	case "CHANGE":
		rest, _ = optionalWords(rest, "COLUMN")
		old, def, ok := splitLeadingIdent(rest.Text)
		if !ok {
			return fmt.Errorf("cannot read %q", action.Text)
		}
		return s.modifyColumn(t, normalizeIdent(old), rest.slice(len(rest.Text)-len(def), len(rest.Text)))
	}
	if ignoredAlterActions[word] {
		return nil
	}
	return fmt.Errorf("unsupported action %q; pass the schema dumped from the migrated database instead", strings.TrimSpace(action.Text))
}

func (s *schemaState) alterAdd(t *tableState, def sqlText) error {
	if rest, ok := optionalWords(def, "COLUMN"); ok {
		return s.addColumns(t, rest)
	}
	switch firstWord(def) {
	case "FULLTEXT", "SPATIAL", "EXCLUDE", "PARTITION", "PERIOD":
		return nil
	}
	if isTableConstraint(def.Masked) {
		if strings.Contains(strings.ToUpper(def.Masked), "USING INDEX") {
			return fmt.Errorf("unsupported constraint %q; constraints built from an existing index are not read", def.Text)
		}
		t.addConstraint(def)
		t.addChecks(def, s.dialect.backslashEscapes)
		return nil
	}
	return s.addColumns(t, def)
}

// addColumns applies ADD COLUMN, including MySQL's parenthesized list and
// FIRST / AFTER positions.
func (s *schemaState) addColumns(t *tableState, def sqlText) error {
	def, ifNotExists := optionalWords(def, "IF", "NOT", "EXISTS")
	if strings.HasPrefix(def.Masked, "(") {
		end := findMatchingParen(def.Masked, 0)
		if end == -1 {
			return fmt.Errorf("unclosed parenthesis in %q", def.Text)
		}
		for _, item := range splitSQLItems(def.slice(1, end)) {
			if err := s.addColumn(t, trimSQLText(item), ifNotExists); err != nil {
				return err
			}
		}
		return nil
	}
	return s.addColumn(t, def, ifNotExists)
}

func (s *schemaState) addColumn(t *tableState, def sqlText, ifNotExists bool) error {
	position := len(t.columns)
	if loc := columnPositionRE.FindStringSubmatchIndex(def.Masked); loc != nil {
		position = 0
		if loc[2] >= 0 {
			after := normalizeIdent(def.Text[loc[2]:loc[3]])
			idx := t.columnIndex(after)
			if idx < 0 {
				return fmt.Errorf("ADD COLUMN ... AFTER %s: column is not defined", after)
			}
			position = idx + 1
		}
		def = def.slice(0, loc[0])
	}
	name, _, ok := splitLeadingIdent(def.Text)
	if !ok {
		return fmt.Errorf("cannot read column definition %q", def.Text)
	}
	if t.columnIndex(normalizeIdent(name)) >= 0 {
		if ifNotExists {
			return nil
		}
		return fmt.Errorf("ADD COLUMN %s: column is already defined", normalizeIdent(name))
	}
	if !t.defineColumn(def, s.enumTypes, s.dialect.backslashEscapes, position, false) {
		return fmt.Errorf("cannot read column definition %q", def.Text)
	}
	t.addChecks(def, s.dialect.backslashEscapes)
	return nil
}

// modifyColumn applies MySQL's MODIFY and CHANGE, which replace a column's
// whole definition. CHANGE passes the old name, as it may rename the column.
func (s *schemaState) modifyColumn(t *tableState, old string, def sqlText) error {
	newName, _, ok := splitLeadingIdent(def.Text)
	if !ok {
		return fmt.Errorf("cannot read column definition %q", def.Text)
	}
	if old == "" {
		old = normalizeIdent(newName)
	}
	idx := t.columnIndex(old)
	if idx < 0 {
		return fmt.Errorf("column %s is not defined", old)
	}
	if renamed := normalizeIdent(newName); renamed != old {
		if err := s.renameColumn(t, old, renamed); err != nil {
			return err
		}
	}
	if loc := columnPositionRE.FindStringIndex(def.Masked); loc != nil {
		def = def.slice(0, loc[0])
	}
	if !t.defineColumn(def, s.enumTypes, s.dialect.backslashEscapes, idx, true) {
		return fmt.Errorf("cannot read column definition %q", def.Text)
	}
	t.addChecks(def, s.dialect.backslashEscapes)
	return nil
}

func (s *schemaState) alterDrop(t *tableState, rest sqlText) error {
	switch firstWord(rest) {
	case "CONSTRAINT", "FOREIGN", "CHECK":
		rest, _ = skipWords(rest, "CONSTRAINT")
		rest, _ = skipWords(rest, "FOREIGN", "KEY")
		rest, _ = skipWords(rest, "CHECK")
		rest, ifExists := optionalWords(rest, "IF", "EXISTS")
		name, _, ok := splitLeadingIdent(rest.Text)
		if !ok {
			return errors.New("DROP CONSTRAINT needs a name")
		}
		if t.dropConstraint(normalizeIdent(name)) || ifExists {
			return nil
		}
		return fmt.Errorf("DROP CONSTRAINT %s: constraint is not defined; give it a name with CONSTRAINT where it is created", normalizeIdent(name))
	case "PRIMARY":
		t.primaryKey = nil
		return nil
	case "INDEX", "KEY":
		rest, _ = skipWords(rest, firstWord(rest))
		if name, _, ok := splitLeadingIdent(rest.Text); ok {
			name := normalizeIdent(name)
			t.uniques = slices.DeleteFunc(t.uniques, func(c *keyConstraint) bool { return slices.Contains(c.names, name) })
		}
		return nil
	case "PARTITION", "PERIOD":
		return nil
	}
	rest, _ = optionalWords(rest, "COLUMN")
	rest, ifExists := optionalWords(rest, "IF", "EXISTS")
	name, _, ok := splitLeadingIdent(rest.Text)
	if !ok {
		return errors.New("DROP COLUMN needs a name")
	}
	column := normalizeIdent(name)
	if t.columnIndex(column) < 0 {
		if ifExists {
			return nil
		}
		return fmt.Errorf("DROP COLUMN %s: column is not defined", column)
	}
	s.dropColumn(t, column)
	return nil
}

// dropColumn removes a column together with every constraint over it,
// including foreign keys of other tables that reference it.
func (s *schemaState) dropColumn(t *tableState, column string) {
	t.columns = slices.DeleteFunc(t.columns, func(col Column) bool { return col.Name == column })
	if t.primaryKey != nil && slices.Contains(t.primaryKey.columns, column) {
		t.primaryKey = nil
	}
	covers := func(c *keyConstraint) bool { return slices.Contains(c.columns, column) }
	t.uniques = slices.DeleteFunc(t.uniques, covers)
	t.foreignKeys = slices.DeleteFunc(t.foreignKeys, func(fk *foreignKeyConstraint) bool { return covers(&fk.keyConstraint) })
	t.checks = slices.DeleteFunc(t.checks, func(c *checkConstraint) bool { return c.column == column })
	for _, other := range s.tables {
		other.foreignKeys = slices.DeleteFunc(other.foreignKeys, func(fk *foreignKeyConstraint) bool {
			return fk.refTable == t.name && slices.Contains(fk.refColumns, column)
		})
	}
}

// dropConstraint removes the constraint with the given name and reports
// whether one existed.
func (t *tableState) dropConstraint(name string) bool {
	if t.primaryKey != nil && slices.Contains(t.primaryKey.names, name) {
		t.primaryKey = nil
		return true
	}
	named := func(c *keyConstraint) bool { return slices.Contains(c.names, name) }
	before := len(t.uniques) + len(t.foreignKeys) + len(t.checks)
	t.uniques = slices.DeleteFunc(t.uniques, named)
	t.foreignKeys = slices.DeleteFunc(t.foreignKeys, func(fk *foreignKeyConstraint) bool { return named(&fk.keyConstraint) })
	t.checks = slices.DeleteFunc(t.checks, func(c *checkConstraint) bool { return slices.Contains(c.names, name) })
	return len(t.uniques)+len(t.foreignKeys)+len(t.checks) < before
}

func (s *schemaState) alterRename(t *tableState, rest sqlText) error {
	switch firstWord(rest) {
	case "TO", "AS":
		rest, _ = skipWords(rest, firstWord(rest))
		return s.renameTable(t, normalizeIdent(strings.TrimSpace(rest.Text)))
	case "CONSTRAINT", "INDEX", "KEY":
		rest, _ = skipWords(rest, firstWord(rest))
		from, to, err := readRenamePair(rest)
		if err != nil {
			return err
		}
		t.renameConstraint(from, to)
		return nil
	}
	if len(strings.Fields(rest.Masked)) == 1 {
		return s.renameTable(t, normalizeIdent(strings.TrimSpace(rest.Text)))
	}
	rest, _ = optionalWords(rest, "COLUMN")
	from, to, err := readRenamePair(rest)
	if err != nil {
		return err
	}
	return s.renameColumn(t, from, to)
}

func readRenamePair(text sqlText) (from, to string, err error) {
	m := renamePairRE.FindStringSubmatchIndex(text.Masked)
	if m == nil {
		return "", "", fmt.Errorf("cannot read rename %q", text.Text)
	}
	return normalizeIdent(text.Text[m[2]:m[3]]), normalizeIdent(text.Text[m[4]:m[5]]), nil
}

func (t *tableState) renameConstraint(from, to string) {
	keys := slices.Clone(t.uniques)
	if t.primaryKey != nil {
		keys = append(keys, t.primaryKey)
	}
	for _, fk := range t.foreignKeys {
		keys = append(keys, &fk.keyConstraint)
	}
	for _, c := range keys {
		renameConstraintName(&c.names, from, to)
	}
	for _, c := range t.checks {
		renameConstraintName(&c.names, from, to)
	}
}

func renameConstraintName(names *[]string, from, to string) {
	if slices.Contains(*names, from) {
		*names = []string{to}
	}
}

// renameColumn renames a column in the table, its constraints, and the
// foreign keys of other tables that reference it.
func (s *schemaState) renameColumn(t *tableState, from, to string) error {
	idx := t.columnIndex(from)
	if idx < 0 {
		return fmt.Errorf("RENAME COLUMN %s: column is not defined", from)
	}
	if t.columnIndex(to) >= 0 {
		return fmt.Errorf("RENAME COLUMN %s: column %s is already defined", from, to)
	}
	t.columns[idx].Name = to
	t.columns[idx].GoName = toGoFieldName(to)
	rename := func(columns []string) {
		for i, col := range columns {
			if col == from {
				columns[i] = to
			}
		}
	}
	if t.primaryKey != nil {
		rename(t.primaryKey.columns)
	}
	for _, c := range t.uniques {
		rename(c.columns)
	}
	for _, fk := range t.foreignKeys {
		rename(fk.columns)
	}
	for _, c := range t.checks {
		if c.column == from {
			c.column = to
		}
	}
	for _, other := range s.tables {
		for _, fk := range other.foreignKeys {
			if fk.refTable == t.name {
				rename(fk.refColumns)
			}
		}
	}
	return nil
}

// alterColumn applies ALTER [COLUMN] name action.
func (s *schemaState) alterColumn(t *tableState, rest sqlText) error {
	switch firstWord(rest) {
	case "INDEX", "CONSTRAINT", "CHECK":
		return nil
	}
	rest, _ = optionalWords(rest, "COLUMN")
	name, _, ok := splitLeadingIdent(rest.Text)
	if !ok {
		return errors.New("ALTER COLUMN needs a name")
	}
	column := normalizeIdent(name)
	idx := t.columnIndex(column)
	if idx < 0 {
		return fmt.Errorf("ALTER COLUMN %s: column is not defined", column)
	}
	action := trimSQLText(rest.slice(strings.Index(rest.Text, name)+len(name), len(rest.Text)))
	words := strings.Fields(strings.ToUpper(action.Masked))
	phrase := strings.Join(words[:min(2, len(words))], " ")
	switch {
	case phrase == "SET NOT":
		t.columns[idx].NotNull = true
		return nil
	case phrase == "DROP NOT":
		t.columns[idx].NotNull = false
		return nil
	case len(words) > 0 && (words[0] == "TYPE" || phrase == "SET DATA"):
		typ, _ := optionalWords(action, "SET", "DATA")
		typ, _ = skipWords(typ, "TYPE")
		if end := columnTypeEndRE.FindStringIndex(typ.Masked); end != nil {
			typ = typ.slice(0, end[0])
		}
		return s.changeColumnType(t, idx, typ)
	case ignoredAlterColumnActions[phrase] || strings.HasPrefix(phrase, "SET (") || strings.HasPrefix(phrase, "RESET ("):
		return nil
	}
	return fmt.Errorf("ALTER COLUMN %s: unsupported action %q", column, strings.TrimSpace(action.Text))
}

// changeColumnType replaces the type of a column, keeping its name,
// nullability, and constraints.
func (s *schemaState) changeColumnType(t *tableState, idx int, typ sqlText) error {
	col := t.columns[idx]
	def := sqlText{Text: col.Name + " " + typ.Text, Masked: col.Name + " " + typ.Masked}
	changed, ok := parseColumn(def, s.enumTypes, s.dialect.backslashEscapes)
	if !ok {
		return fmt.Errorf("ALTER COLUMN %s: cannot read type %q", col.Name, typ.Text)
	}
	col.SQLType, col.GoType, col.MaxLength, col.EnumValues = changed.SQLType, changed.GoType, changed.MaxLength, changed.EnumValues
	t.columns[idx] = col
	return nil
}

// resolveEnumTypes maps each enum type to its values as they stand after
// every statement, following CREATE TYPE ... AS ENUM, ALTER TYPE, and DROP
// TYPE in order. Columns refer to types by name, so a type renamed later is
// also reachable under its earlier names. A type whose value list cannot be
// read maps to nil, which still marks it as an enum.
func resolveEnumTypes(statements []sqlStatement) map[string][]string {
	enums := make(map[string][]string)
	renamed := make(map[string]string)
	for _, stmt := range statements {
		switch {
		case alterTypeRE.MatchString(stmt.Masked):
			loc := alterTypeRE.FindStringSubmatchIndex(stmt.Masked)
			alterEnumType(enums, renamed, normalizeIdent(stmt.Text[loc[2]:loc[3]]), trimSQLText(stmt.slice(loc[1], len(stmt.Masked))))
		case dropTypeRE.MatchString(stmt.Masked):
			loc := dropTypeRE.FindStringIndex(stmt.Masked)
			rest := stmt.slice(loc[1], len(stmt.Masked))
			if end := trailingDropOptionsRE.FindStringIndex(rest.Masked); end != nil {
				rest = rest.slice(0, end[0])
			}
			for _, name := range splitIdentifierList(rest.Text) {
				delete(enums, name)
			}
		default:
			for _, loc := range createEnumRE.FindAllStringSubmatchIndex(stmt.Masked, -1) {
				var values []string
				if end := findMatchingParen(stmt.Masked, loc[1]-1); end != -1 {
					values, _ = parseStringLiterals(stmt.Text[loc[1]:end], false)
				}
				enums[normalizeIdent(stmt.Text[loc[2]:loc[3]])] = values
			}
		}
	}
	for old := range renamed {
		name := old
		for seen := 0; seen <= len(renamed); seen++ {
			next, ok := renamed[name]
			if !ok {
				break
			}
			name = next
		}
		if values, ok := enums[name]; ok {
			if _, defined := enums[old]; !defined {
				enums[old] = values
			}
		}
	}
	if len(enums) == 0 {
		return nil
	}
	return enums
}

func alterEnumType(enums map[string][]string, renamed map[string]string, name string, action sqlText) {
	values, ok := enums[name]
	if !ok {
		return
	}
	literal := func(loc []int, i int) string {
		v, _ := parseStringLiterals(action.Text[loc[i]:loc[i+1]], false)
		if len(v) == 0 {
			return ""
		}
		return v[0]
	}
	switch {
	case addValueRE.MatchString(action.Masked):
		loc := addValueRE.FindStringSubmatchIndex(action.Masked)
		value := literal(loc, 2)
		if slices.Contains(values, value) {
			return
		}
		at := len(values)
		if loc[4] >= 0 {
			if i := slices.Index(values, literal(loc, 6)); i >= 0 {
				at = i
				if strings.EqualFold(action.Text[loc[4]:loc[5]], "AFTER") {
					at++
				}
			}
		}
		enums[name] = slices.Insert(slices.Clone(values), at, value)
	case renameValueRE.MatchString(action.Masked):
		loc := renameValueRE.FindStringSubmatchIndex(action.Masked)
		if i := slices.Index(values, literal(loc, 2)); i >= 0 {
			values = slices.Clone(values)
			values[i] = literal(loc, 4)
			enums[name] = values
		}
	case renameTypeRE.MatchString(action.Masked):
		loc := renameTypeRE.FindStringSubmatchIndex(action.Masked)
		to := normalizeIdent(action.Text[loc[2]:loc[3]])
		delete(enums, name)
		enums[to] = values
		renamed[name] = to
	}
}
