package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	atlasTableRE  = regexp.MustCompile(`(?m)^table\s+"([^"]+)"\s*\{`)
	atlasColumnRE = regexp.MustCompile(`(?m)^\s*column\s+"([^"]+)"\s*\{`)
	// Attribute names are anchored with \b so that one attribute never matches
	// inside a longer one, such as columns inside ref_columns.
	atlasTypeRE   = regexp.MustCompile(`(?mi)\btype\s*=\s*(\S+)`)
	atlasNullRE   = regexp.MustCompile(`(?mi)\bnull\s*=\s*(true|false)`)
	atlasPKRE     = regexp.MustCompile(`(?mi)\bprimary_key\s*\{[^}]*\bcolumns\s*=\s*\[([^\]]+)\]`)
	atlasFK_RE    = regexp.MustCompile(`(?mis)\bforeign_key\s+"([^"]*)"\s*\{([^}]+)\}`)
	atlasFKColsRE = regexp.MustCompile(`(?mi)\bcolumns\s*=\s*\[([^\]]+)\]`)
	atlasFKRefRE  = regexp.MustCompile(`(?mi)\bref_columns\s*=\s*\[([^\]]+)\]`)
)

// ParseAtlasHCL parses an Atlas HCL schema file and returns []Table.
func ParseAtlasHCL(data string) ([]Table, error) {
	data = stripHCLComments(data)
	tableBlocks, err := extractAtlasTableBlocks(data)
	if err != nil {
		return nil, err
	}
	if len(tableBlocks) == 0 {
		return nil, nil
	}

	var tables []Table
	for _, block := range tableBlocks {
		t, err := parseAtlasTable(block)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, nil
}

type atlasTableBlock struct {
	Name string
	Body string
}

func extractAtlasTableBlocks(data string) ([]atlasTableBlock, error) {
	var blocks []atlasTableBlock
	locs := atlasTableRE.FindAllStringSubmatchIndex(data, -1)
	for _, loc := range locs {
		tableName := data[loc[2]:loc[3]]
		braceStart := loc[1] - 1
		braceEnd := findAtlasMatchingBrace(data, braceStart)
		if braceEnd == -1 {
			return nil, fmt.Errorf("parse table %q: unclosed brace", tableName)
		}
		blocks = append(blocks, atlasTableBlock{
			Name: tableName,
			Body: data[braceStart+1 : braceEnd],
		})
	}
	return blocks, nil
}

func findAtlasMatchingBrace(s string, openIdx int) int {
	depth := 0
	inString := false
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

func parseAtlasTable(block atlasTableBlock) (Table, error) {
	tableName := block.Name
	t := Table{
		Name:        tableName,
		GoName:      toGoStructName(tableName),
		BlueprintID: singularize(tableName),
	}

	// Parse columns.
	columnBlocks, err := extractAtlasColumnBlocks(tableName, block.Body)
	if err != nil {
		return Table{}, err
	}
	columnIndex := make(map[string]int, len(columnBlocks))

	for _, cb := range columnBlocks {
		col := parseAtlasColumn(cb)
		columnIndex[col.Name] = len(t.Columns)
		t.Columns = append(t.Columns, col)
	}

	// Parse primary key.
	if match := atlasPKRE.FindStringSubmatch(block.Body); match != nil {
		for _, colRef := range splitAtlasColumnRefs(match[1]) {
			if idx, ok := columnIndex[colRef]; ok {
				t.Columns[idx].IsPK = true
			}
		}
	}

	// Parse foreign keys.
	for _, fkMatch := range atlasFK_RE.FindAllStringSubmatch(block.Body, -1) {
		fk, err := parseAtlasForeignKey(tableName, fkMatch[1], fkMatch[2], t.Columns, columnIndex)
		if err != nil {
			return Table{}, err
		}
		t.ForeignKeys = append(t.ForeignKeys, fk)
	}

	return t, nil
}

// parseAtlasForeignKey reads one foreign_key block and marks its local columns
// as foreign keys. A block whose columns or ref_columns are missing, or whose
// local columns are not declared on the table, is rejected rather than dropped,
// since generating without the relation would silently skip its parent.
func parseAtlasForeignKey(tableName, fkName, fkBody string, columns []Column, columnIndex map[string]int) (ForeignKey, error) {
	colsMatch := atlasFKColsRE.FindStringSubmatch(fkBody)
	refMatch := atlasFKRefRE.FindStringSubmatch(fkBody)
	if colsMatch == nil || refMatch == nil {
		return ForeignKey{}, fmt.Errorf("parse table %q foreign key %q: columns and ref_columns are required", tableName, fkName)
	}

	fkCols := splitAtlasColumnRefs(colsMatch[1])
	refTable := extractAtlasRefTable(refMatch[1])
	refColumns := extractAtlasRefColumns(refMatch[1])
	if len(fkCols) == 0 || refTable == "" {
		return ForeignKey{}, fmt.Errorf("parse table %q foreign key %q: columns and ref_columns must reference columns", tableName, fkName)
	}

	notNull := true
	for _, colName := range fkCols {
		idx, ok := columnIndex[colName]
		if !ok {
			return ForeignKey{}, fmt.Errorf("parse table %q foreign key %q: column %q is not defined on the table", tableName, fkName, colName)
		}
		columns[idx].IsFK = true
		columns[idx].FKRefTable = refTable
		notNull = notNull && columns[idx].NotNull
	}
	return ForeignKey{
		Columns:    fkCols,
		RefTable:   refTable,
		RefColumns: refColumns,
		NotNull:    notNull,
	}, nil
}

type atlasColumnBlock struct {
	Name string
	Body string
}

func extractAtlasColumnBlocks(tableName, tableBody string) ([]atlasColumnBlock, error) {
	var blocks []atlasColumnBlock
	locs := atlasColumnRE.FindAllStringSubmatchIndex(tableBody, -1)
	for _, loc := range locs {
		colName := tableBody[loc[2]:loc[3]]
		// Find the opening brace after "column "name" "
		braceIdx := strings.IndexByte(tableBody[loc[1]-1:], '{')
		if braceIdx == -1 {
			return nil, fmt.Errorf("parse table %q column %q: missing opening brace", tableName, colName)
		}
		braceStart := loc[1] - 1 + braceIdx
		braceEnd := findAtlasMatchingBrace(tableBody, braceStart)
		if braceEnd == -1 {
			return nil, fmt.Errorf("parse table %q column %q: unclosed brace", tableName, colName)
		}
		blocks = append(blocks, atlasColumnBlock{
			Name: colName,
			Body: tableBody[braceStart+1 : braceEnd],
		})
	}
	return blocks, nil
}

func parseAtlasColumn(cb atlasColumnBlock) Column {
	col := Column{
		Name:   cb.Name,
		GoName: toGoFieldName(cb.Name),
	}

	// Extract type.
	if m := atlasTypeRE.FindStringSubmatch(cb.Body); m != nil {
		sqlType := normalizeAtlasType(m[1])
		col.SQLType = sqlType
		col.GoType = sqlTypeToGoType(sqlType)
	}

	// Extract null.
	if m := atlasNullRE.FindStringSubmatch(cb.Body); m != nil {
		col.NotNull = m[1] == "false"
	} else {
		// Default: not null if no null attribute specified.
		col.NotNull = true
	}

	return col
}

func normalizeAtlasType(t string) string {
	// Atlas types can be like: int, varchar(255), bigint, serial, etc.
	t = strings.TrimSpace(t)
	if idx := strings.Index(t, "("); idx != -1 {
		t = t[:idx]
	}
	return strings.ToUpper(t)
}

// stripHCLComments removes HCL line comments (# and //) while preserving
// comment-like sequences inside double-quoted string literals.
func stripHCLComments(data string) string {
	var b strings.Builder
	b.Grow(len(data))

	i := 0
	inString := false
	for i < len(data) {
		ch := data[i]

		if inString {
			b.WriteByte(ch)
			if ch == '"' {
				inString = false
			}
			i++
			continue
		}

		if ch == '"' {
			inString = true
			b.WriteByte(ch)
			i++
			continue
		}

		// Line comment: # to end of line.
		if ch == '#' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				b.WriteByte('\n')
				i++
			}
			continue
		}

		// Line comment: // to end of line.
		if ch == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				b.WriteByte('\n')
				i++
			}
			continue
		}

		b.WriteByte(ch)
		i++
	}

	return b.String()
}

// splitAtlasColumnRefs splits "[column.id, column.name]" content into column names.
func splitAtlasColumnRefs(s string) []string {
	parts := strings.Split(s, ",")
	var names []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Handle "column.name" format.
		if after, ok := strings.CutPrefix(p, "column."); ok {
			names = append(names, strings.TrimSpace(after))
		} else if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// extractAtlasRefTable extracts the table name from ref_columns like
// "table.companies.column.id".
func extractAtlasRefTable(s string) string {
	parts := strings.Split(s, ",")
	if len(parts) == 0 {
		return ""
	}
	// Take the first reference and extract table name.
	ref := strings.TrimSpace(parts[0])
	// Format: table.tablename.column.colname
	segments := strings.Split(ref, ".")
	for i, seg := range segments {
		if seg == "table" && i+1 < len(segments) {
			return segments[i+1]
		}
	}
	return ""
}

// extractAtlasRefColumns extracts column names from Atlas references like
// "table.companies.column.code".
func extractAtlasRefColumns(s string) []string {
	parts := strings.Split(s, ",")
	columns := make([]string, 0, len(parts))
	for _, ref := range parts {
		segments := strings.Split(strings.TrimSpace(ref), ".")
		for i, segment := range segments {
			if segment == "column" && i+1 < len(segments) {
				columns = append(columns, segments[i+1])
				break
			}
		}
	}
	return columns
}
