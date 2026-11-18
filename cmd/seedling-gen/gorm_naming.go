package main

import "strings"

// The helpers in this file reproduce the default naming of
// gorm.io/gorm/schema.NamingStrategy (v1.25), together with the plural rules in
// inflection.go it relies on, so parsed GORM models report the same column and
// table names GORM itself would use.
//
// The naming rules are adapted from code released under the MIT License:
//   - gorm.io/gorm: Copyright (c) 2013-NOW Jinzhu <wosmvp@gmail.com>

// gormInitialismReplacer title-cases the initialisms GORM collapses into a
// single word before it splits a Go name, so CompanyID becomes company_id and
// APIKey becomes api_key.
var gormInitialismReplacer = func() *strings.Replacer {
	initialisms := []string{
		"API", "ASCII", "CPU", "CSS", "DNS", "EOF", "GUID", "HTML", "HTTP", "HTTPS",
		"ID", "IP", "JSON", "LHS", "QPS", "RAM", "RHS", "RPC", "SLA", "SMTP", "SSH",
		"TLS", "TTL", "UID", "UI", "UUID", "URI", "URL", "UTF8", "VM", "XML", "XSRF", "XSS",
	}
	pairs := make([]string, 0, 2*len(initialisms))
	for _, initialism := range initialisms {
		pairs = append(pairs, initialism, initialism[:1]+strings.ToLower(initialism[1:]))
	}
	return strings.NewReplacer(pairs...)
}()

// toSnakeCase converts a Go identifier to snake_case the way GORM derives a
// column name from a field name: known initialisms stay one word, and a run of
// upper-case letters is kept together (HTTPServer -> http_server).
func toSnakeCase(name string) string {
	if name == "" {
		return ""
	}

	value := gormInitialismReplacer.Replace(name)
	var b strings.Builder
	var lastUpper, nextUpper, nextDigit bool
	curUpper := isASCIIUpper(value[0])

	for i, r := range value[:len(value)-1] {
		nextUpper = isASCIIUpper(value[i+1])
		nextDigit = value[i+1] >= '0' && value[i+1] <= '9'

		if curUpper {
			if !lastUpper || (!nextUpper && !nextDigit) {
				if i > 0 && value[i-1] != '_' && value[i+1] != '_' {
					b.WriteByte('_')
				}
			}
			b.WriteRune(r + 'a' - 'A')
		} else {
			b.WriteRune(r)
		}

		lastUpper = curUpper
		curUpper = nextUpper
	}

	last := value[len(value)-1]
	if curUpper {
		if !lastUpper && len(value) > 1 {
			b.WriteByte('_')
		}
		b.WriteByte(last + 'a' - 'A')
	} else {
		b.WriteByte(last)
	}
	return b.String()
}

func isASCIIUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

// gormTableName returns the table GORM uses for a model type without a
// TableName method: the pluralized snake_case type name.
func gormTableName(typeName string) string {
	return pluralize(toSnakeCase(typeName))
}
