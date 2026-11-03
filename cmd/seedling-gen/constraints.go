package main

import (
	"regexp"
	"slices"
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
	createUniqueIndexRE = regexp.MustCompile(`(?is)CREATE\s+UNIQUE\s+INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(?:([^\s(]+)\s+)?ON\s+(?:ONLY\s+)?([^\s(]+)\s*(?:USING\s+\w+\s*)?\(([^()]*)\)`)
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
// literals, such as 'draft', 'it”s'. A doubled quote always escapes a quote;
// with backslash (MySQL), and in a PostgreSQL E'...' literal, a backslash
// escapes the next character as well. It reports false for anything else, so a
// list of expressions is never mistaken for enumerated values.
func parseStringLiterals(list string, backslash bool) ([]string, bool) {
	var values []string
	rest := strings.TrimSpace(list)
	for rest != "" {
		escapes := backslash
		if len(rest) > 1 && (rest[0] == 'E' || rest[0] == 'e') && rest[1] == '\'' {
			escapes = true
			rest = rest[1:]
		}
		if rest[0] != '\'' {
			return nil, false
		}
		value, n, ok := readStringLiteral(rest, escapes)
		if !ok {
			return nil, false
		}
		values = append(values, value)
		rest = strings.TrimSpace(rest[n:])
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

// readStringLiteral decodes the literal opening at s[0] and returns its value
// and length.
func readStringLiteral(s string, escapes bool) (string, int, bool) {
	var value strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && escapes && i+1 < len(s):
			i++
			value.WriteByte(unescapeByte(s[i]))
		case c != '\'':
			value.WriteByte(c)
		case i+1 < len(s) && s[i+1] == '\'':
			value.WriteByte('\'')
			i++
		default:
			return value.String(), i + 1, true
		}
	}
	return "", 0, false
}

// unescapeByte decodes the character after a backslash in a MySQL or
// PostgreSQL escape-string literal.
func unescapeByte(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case 'b':
		return '\b'
	case '0':
		return 0
	case 'Z':
		return 0x1a
	default:
		return c
	}
}

// restrictEnum narrows the values a column accepts to those in values as
// well, so a column under an enum type and several CHECK constraints keeps
// only the values every one of them allows.
func restrictEnum(col *Column, values []string) {
	if col.EnumValues == nil {
		col.EnumValues = values
		return
	}
	col.EnumValues = slices.DeleteFunc(slices.Clone(col.EnumValues), func(v string) bool {
		return !slices.Contains(values, v)
	})
}

// applyColumnTypeArgs reads the maximum length or the enumerated values from
// the arguments of a column type, given the text after the column name.
func applyColumnTypeArgs(col *Column, afterName sqlText, backslash bool) {
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
		if values, ok := parseStringLiterals(args, backslash); ok {
			restrictEnum(col, values)
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

// indexColumnNames returns the column names of an index column list, dropping
// what may follow each name, such as ASC, DESC, COLLATE, or an operator class.
func indexColumnNames(list string) []string {
	var names []string
	for item := range strings.SplitSeq(list, ",") {
		if fields := strings.Fields(item); len(fields) > 0 {
			names = append(names, normalizeIdent(fields[0]))
		}
	}
	return names
}
