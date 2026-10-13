package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	atlasTableRE  = regexp.MustCompile(`(?m)^table\s+"([^"]+)"\s*\{`)
	atlasColumnRE = regexp.MustCompile(`(?m)^\s*column\s+"([^"]+)"\s*\{`)
	// Attribute names must not be preceded by an identifier character, so one
	// attribute never matches inside a longer one, such as columns inside
	// ref_columns or null inside allow-null. HCL identifiers may contain '-',
	// which \b would treat as a boundary.
	atlasTypeRE   = regexp.MustCompile(`(?mi)(?:^|[^\w-])type\s*=\s*(\S+)`)
	atlasNullRE   = regexp.MustCompile(`(?mi)(?:^|[^\w-])null\s*=\s*(true|false)`)
	atlasPKRE     = regexp.MustCompile(`(?mi)(?:^|[^\w-])primary_key\s*\{[^}]*?(?:^|[^\w-])columns\s*=\s*\[([^\]]+)\]`)
	atlasFK_RE    = regexp.MustCompile(`(?mis)(?:^|[^\w-])foreign_key\s+"([^"]*)"\s*\{([^}]+)\}`)
	atlasFKColsRE = regexp.MustCompile(`(?mi)(?:^|[^\w-])columns\s*=\s*\[([^\]]+)\]`)
	atlasFKRefRE  = regexp.MustCompile(`(?mi)(?:^|[^\w-])ref_columns\s*=\s*\[([^\]]+)\]`)
)

// ParseAtlasHCL parses an Atlas HCL schema file and returns []Table.
func ParseAtlasHCL(data string) ([]Table, error) {
	tableBlocks, err := extractAtlasTableBlocks(data, maskHCL(data))
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

// atlasTableBlock holds a table block body twice: Body is the source text,
// used for block names, and Masked is the same text after maskHCL, used for
// everything else.
type atlasTableBlock struct {
	Name   string
	Body   string
	Masked string
}

// extractAtlasTableBlocks finds the table blocks in masked, the maskHCL form
// of data, so that braces and keywords inside strings and comments are never
// mistaken for structure. Names are read from data at the same offsets.
func extractAtlasTableBlocks(data, masked string) ([]atlasTableBlock, error) {
	var blocks []atlasTableBlock
	locs := atlasTableRE.FindAllStringSubmatchIndex(masked, -1)
	for _, loc := range locs {
		tableName := data[loc[2]:loc[3]]
		braceStart := loc[1] - 1
		braceEnd := findAtlasMatchingBrace(masked, braceStart)
		if braceEnd == -1 {
			return nil, fmt.Errorf("parse table %q: unclosed brace", tableName)
		}
		blocks = append(blocks, atlasTableBlock{
			Name:   tableName,
			Body:   data[braceStart+1 : braceEnd],
			Masked: masked[braceStart+1 : braceEnd],
		})
	}
	return blocks, nil
}

// findAtlasMatchingBrace returns the index of the brace closing the one at
// openIdx. masked must come from maskHCL, so every brace in it is structural.
func findAtlasMatchingBrace(masked string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(masked); i++ {
		switch masked[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
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
	columnBlocks, err := extractAtlasColumnBlocks(tableName, block)
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
	if match := atlasPKRE.FindStringSubmatch(block.Masked); match != nil {
		for _, colRef := range splitAtlasColumnRefs(match[1]) {
			if idx, ok := columnIndex[colRef]; ok {
				t.Columns[idx].IsPK = true
			}
		}
	}

	// Parse foreign keys.
	for _, loc := range atlasFK_RE.FindAllStringSubmatchIndex(block.Masked, -1) {
		fkName := block.Body[loc[2]:loc[3]]
		fk, err := parseAtlasForeignKey(tableName, fkName, block.Masked[loc[4]:loc[5]], t.Columns, columnIndex)
		if err != nil {
			return Table{}, err
		}
		t.ForeignKeys = append(t.ForeignKeys, fk)
	}

	return t, nil
}

// parseAtlasForeignKey reads one foreign_key block and marks its local columns
// as foreign keys. A block that cannot be resolved to one referenced column per
// local column, all on one table, is rejected rather than dropped, since
// generating without the relation would silently skip its parent.
func parseAtlasForeignKey(tableName, fkName, fkBody string, columns []Column, columnIndex map[string]int) (ForeignKey, error) {
	fail := func(format string, args ...any) (ForeignKey, error) {
		return ForeignKey{}, fmt.Errorf("parse table %q foreign key %q: %s", tableName, fkName, fmt.Sprintf(format, args...))
	}

	colsMatch := atlasFKColsRE.FindStringSubmatch(fkBody)
	refMatch := atlasFKRefRE.FindStringSubmatch(fkBody)
	if colsMatch == nil || refMatch == nil {
		return fail("columns and ref_columns are required")
	}

	fkCols := splitAtlasColumnRefs(colsMatch[1])
	refTable, refColumns, err := parseAtlasRefColumns(refMatch[1], tableName)
	if err != nil {
		return fail("%v", err)
	}
	if len(fkCols) == 0 || len(fkCols) != len(refColumns) {
		return fail("columns lists %d columns but ref_columns lists %d", len(fkCols), len(refColumns))
	}

	notNull := true
	for _, colName := range fkCols {
		idx, ok := columnIndex[colName]
		if !ok {
			return fail("column %q is not defined on the table", colName)
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

// atlasColumnBlock holds a column name and the maskHCL form of its body.
type atlasColumnBlock struct {
	Name   string
	Masked string
}

func extractAtlasColumnBlocks(tableName string, table atlasTableBlock) ([]atlasColumnBlock, error) {
	var blocks []atlasColumnBlock
	locs := atlasColumnRE.FindAllStringSubmatchIndex(table.Masked, -1)
	for _, loc := range locs {
		colName := table.Body[loc[2]:loc[3]]
		braceStart := loc[1] - 1
		braceEnd := findAtlasMatchingBrace(table.Masked, braceStart)
		if braceEnd == -1 {
			return nil, fmt.Errorf("parse table %q column %q: unclosed brace", tableName, colName)
		}
		blocks = append(blocks, atlasColumnBlock{
			Name:   colName,
			Masked: table.Masked[braceStart+1 : braceEnd],
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
	if m := atlasTypeRE.FindStringSubmatch(cb.Masked); m != nil {
		sqlType := normalizeAtlasType(m[1])
		col.SQLType = sqlType
		col.GoType = sqlTypeToGoType(sqlType)
	}

	// Extract null.
	if m := atlasNullRE.FindStringSubmatch(cb.Masked); m != nil {
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

// maskHCL returns data with every comment and the contents of every quoted
// string replaced by spaces. Offsets are preserved, so a match found in the
// masked text maps back to the same bytes of data. Patterns run on the masked
// text cannot match a keyword, brace, or comment marker that sits inside a
// string, and a string cannot hide structure behind an escaped quote (\").
// Quoted strings end at a newline, as HCL does not allow them to span lines;
// heredoc strings (<<EOT or <<-EOT) are masked up to their closing marker.
func maskHCL(data string) string {
	b := []byte(data)
	blank := func(from, to int) {
		for i := from; i < to; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}

	for i := 0; i < len(b); {
		switch {
		case b[i] == '"':
			start := i + 1
			i = start
			for i < len(b) && b[i] != '"' && b[i] != '\n' {
				if b[i] == '\\' && i+1 < len(b) && b[i+1] != '\n' {
					i++
				}
				i++
			}
			blank(start, i)
			if i < len(b) && b[i] == '"' {
				i++
			}
		case b[i] == '<' && i+1 < len(b) && b[i+1] == '<':
			start, end, ok := hclHeredocBody(data, i)
			if !ok {
				i += 2
				continue
			}
			blank(start, end)
			i = end
		case b[i] == '#', b[i] == '/' && i+1 < len(b) && b[i+1] == '/':
			start := i
			for i < len(b) && b[i] != '\n' {
				i++
			}
			blank(start, i)
		case b[i] == '/' && i+1 < len(b) && b[i+1] == '*':
			start := i
			end := strings.Index(data[i+2:], "*/")
			if end == -1 {
				i = len(b)
			} else {
				i += 2 + end + 2
			}
			blank(start, i)
		default:
			i++
		}
	}
	return string(b)
}

// hclHeredocBody reports the body of the heredoc that opens at i, from the
// line after <<MARKER (or <<-MARKER) up to the line holding only MARKER. A body
// whose closing marker is missing runs to the end of the input.
func hclHeredocBody(data string, i int) (start, end int, ok bool) {
	j := i + 2
	if j < len(data) && data[j] == '-' {
		j++
	}
	markerStart := j
	for j < len(data) && (data[j] == '_' || data[j] == '-' || unicode.IsLetter(rune(data[j])) || j > markerStart && unicode.IsDigit(rune(data[j]))) {
		j++
	}
	marker := data[markerStart:j]
	if marker == "" {
		return 0, 0, false
	}
	lineEnd := strings.IndexByte(data[j:], '\n')
	if lineEnd == -1 || strings.TrimSpace(data[j:j+lineEnd]) != "" {
		return 0, 0, false
	}

	start = j + lineEnd + 1
	for pos := start; pos < len(data); {
		next := strings.IndexByte(data[pos:], '\n')
		line := data[pos:]
		if next != -1 {
			line = data[pos : pos+next]
		}
		if strings.TrimSpace(line) == marker {
			return start, pos, true
		}
		if next == -1 {
			break
		}
		pos += next + 1
	}
	return start, len(data), true
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

// parseAtlasRefColumns resolves ref_columns content such as
// "table.companies.column.id, table.companies.column.code" into the referenced
// table and its columns. A bare "column.id" references selfTable. Every entry
// must name a column, and all of them the same table.
func parseAtlasRefColumns(s, selfTable string) (string, []string, error) {
	var table string
	var columns []string
	for ref := range strings.SplitSeq(s, ",") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		refTable, column := selfTable, ""
		segments := strings.Split(ref, ".")
		for i := 0; i+1 < len(segments); i += 2 {
			switch segments[i] {
			case "table":
				refTable = segments[i+1]
			case "column":
				column = segments[i+1]
			}
		}
		if column == "" {
			return "", nil, fmt.Errorf("ref_columns entry %q does not name a column", ref)
		}
		if table != "" && refTable != table {
			return "", nil, fmt.Errorf("ref_columns references both %q and %q", table, refTable)
		}
		table = refTable
		columns = append(columns, column)
	}
	return table, columns, nil
}
