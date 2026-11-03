package main

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// checkInRE matches CHECK (column IN (...)), the one CHECK shape whose
	// valid values can be read without solving the expression.
	checkInRE = regexp.MustCompile(`(?i)CHECK\s*\(\s*([^\s()]+)\s+IN\s*\(([^()]*)\)\s*\)`)
	// uniqueColumnsRE matches a table-level UNIQUE (columns) constraint.
	uniqueColumnsRE = regexp.MustCompile(`(?i)^(?:CONSTRAINT\s+\S+\s+)?UNIQUE(?:\s+(?:KEY|INDEX)(?:\s+[^\s(]+)?)?\s*\(([^()]+)\)`)
	// createUniqueIndexRE matches CREATE UNIQUE INDEX ... ON table (columns).
	createUniqueIndexRE = regexp.MustCompile(`(?is)CREATE\s+UNIQUE\s+INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(?:[^\s(]+\s+)?ON\s+(?:ONLY\s+)?([^\s(]+)\s*(?:USING\s+\w+\s*)?\(([^()]*)\)`)
	// columnUniqueRE matches an inline UNIQUE column constraint.
	columnUniqueRE = regexp.MustCompile(`(?i)\bUNIQUE\b`)
	// typeArgsRE matches a column type and its parenthesized arguments, such
	// as VARCHAR(255), CHARACTER VARYING(32), or ENUM('a', 'b').
	typeArgsRE = regexp.MustCompile(`(?i)^\s*([\w.]+(?:\s+VARYING)?)\s*\(([^()]*)\)`)
)

// lengthTypes are the string types whose single argument is a maximum length.
var lengthTypes = map[string]bool{
	"VARCHAR": true, "CHAR": true, "CHARACTER": true, "NVARCHAR": true, "NCHAR": true,
	"CHARACTER VARYING": true, "VARCHAR2": true,
}

// parseStringLiterals reads a comma-separated list of single-quoted SQL string
// literals, such as 'draft', 'it”s'. It reports false for anything else, so
// a list of expressions is never mistaken for enumerated values.
func parseStringLiterals(list string) ([]string, bool) {
	var values []string
	rest := strings.TrimSpace(list)
	for rest != "" {
		if rest[0] != '\'' {
			return nil, false
		}
		var value strings.Builder
		i := 1
		for ; i < len(rest); i++ {
			if rest[i] != '\'' {
				value.WriteByte(rest[i])
				continue
			}
			if i+1 < len(rest) && rest[i+1] == '\'' {
				value.WriteByte('\'')
				i++
				continue
			}
			break
		}
		if i >= len(rest) {
			return nil, false
		}
		values = append(values, value.String())
		rest = strings.TrimSpace(rest[i+1:])
		if rest == "" {
			break
		}
		if rest[0] != ',' {
			return nil, false
		}
		rest = strings.TrimSpace(rest[1:])
	}
	return values, len(values) > 0
}

// applyColumnTypeArgs reads the maximum length or the enumerated values from
// the arguments of a column type, given the text after the column name.
func applyColumnTypeArgs(col *Column, afterName sqlText) {
	loc := typeArgsRE.FindStringSubmatchIndex(afterName.Masked)
	if loc == nil {
		return
	}
	typeName := strings.ToUpper(strings.Join(strings.Fields(afterName.Text[loc[2]:loc[3]]), " "))
	args := afterName.Text[loc[4]:loc[5]]
	switch {
	case lengthTypes[typeName]:
		if n, err := strconv.Atoi(strings.TrimSpace(args)); err == nil && n > 0 {
			col.MaxLength = n
		}
	case typeName == "ENUM":
		if values, ok := parseStringLiterals(args); ok {
			col.EnumValues = values
		}
	}
}

// applyCheckIn records the values of CHECK (column IN (...)) constraints in
// text on the columns they restrict.
func applyCheckIn(columns []Column, columnIndex map[string]int, text sqlText) {
	for _, loc := range checkInRE.FindAllStringSubmatchIndex(text.Masked, -1) {
		idx, ok := columnIndex[normalizeIdent(text.Text[loc[2]:loc[3]])]
		if !ok {
			continue
		}
		if values, ok := parseStringLiterals(text.Text[loc[4]:loc[5]]); ok {
			columns[idx].EnumValues = values
		}
	}
}

// markUniqueColumns marks the single column of a unique constraint as unique.
// A composite unique constraint does not make any one column unique, so it is
// ignored.
func markUniqueColumns(columns []Column, columnIndex map[string]int, names []string) {
	if len(names) != 1 {
		return
	}
	if idx, ok := columnIndex[names[0]]; ok {
		columns[idx].Unique = true
	}
}

// applyUniqueIndexes marks the columns of single-column CREATE UNIQUE INDEX
// statements as unique.
func applyUniqueIndexes(tables []Table, sql, masked string) {
	tableIndex := make(map[string]int, len(tables))
	for i, table := range tables {
		tableIndex[table.Name] = i
	}
	for _, loc := range createUniqueIndexRE.FindAllStringSubmatchIndex(masked, -1) {
		ti, ok := tableIndex[normalizeIdent(sql[loc[2]:loc[3]])]
		if !ok {
			continue
		}
		table := &tables[ti]
		columnIndex := make(map[string]int, len(table.Columns))
		for i, col := range table.Columns {
			columnIndex[col.Name] = i
		}
		markUniqueColumns(table.Columns, columnIndex, splitIdentifierList(sql[loc[4]:loc[5]]))
	}
}
