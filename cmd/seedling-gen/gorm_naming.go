package main

import (
	"regexp"
	"slices"
	"strings"
)

// The helpers in this file reproduce the default naming of
// gorm.io/gorm/schema.NamingStrategy (v1.25) and the github.com/jinzhu/inflection
// plural rules it relies on, so parsed GORM models report the same column and
// table names GORM itself would use.
//
// The naming and plural rules are adapted from code released under the MIT
// License:
//   - gorm.io/gorm: Copyright (c) 2013-NOW Jinzhu <wosmvp@gmail.com>
//   - github.com/jinzhu/inflection: Copyright (c) 2015 - Jinzhu

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

// pluralRules lists the inflection rules in the order they are tried; the
// first matching rule decides the plural.
var pluralRules = compilePluralRules()

type pluralRule struct {
	pattern *regexp.Regexp
	replace string
}

func compilePluralRules() []pluralRule {
	uncountable := []string{"equipment", "information", "rice", "money", "species", "series", "fish", "sheep", "jeans", "police"}
	irregular := []struct{ singular, plural string }{
		{"person", "people"},
		{"man", "men"},
		{"child", "children"},
		{"sex", "sexes"},
		{"move", "moves"},
		{"mombie", "mombies"},
	}
	// Later entries take precedence over earlier ones.
	regular := []struct{ find, replace string }{
		{"([a-z])$", "${1}s"},
		{"s$", "s"},
		{"^(ax|test)is$", "${1}es"},
		{"(octop|vir)us$", "${1}i"},
		{"(octop|vir)i$", "${1}i"},
		{"(alias|status)$", "${1}es"},
		{"(bu)s$", "${1}ses"},
		{"(buffal|tomat)o$", "${1}oes"},
		{"([ti])um$", "${1}a"},
		{"([ti])a$", "${1}a"},
		{"sis$", "ses"},
		{"(?:([^f])fe|([lr])f)$", "${1}${2}ves"},
		{"(hive)$", "${1}s"},
		{"([^aeiouy]|qu)y$", "${1}ies"},
		{"(x|ch|ss|sh)$", "${1}es"},
		{"(matr|vert|ind)(?:ix|ex)$", "${1}ices"},
		{"^(m|l)ouse$", "${1}ice"},
		{"^(m|l)ice$", "${1}ice"},
		{"^(ox)$", "${1}en"},
		{"^(oxen)$", "${1}"},
		{"(quiz)$", "${1}zes"},
	}

	rules := make([]pluralRule, 0, len(uncountable)+3*len(irregular)+3*len(regular))
	for _, noun := range uncountable {
		rules = append(rules, pluralRule{regexp.MustCompile("^(?i)(" + noun + ")$"), "${1}"})
	}
	for _, word := range irregular {
		rules = append(rules,
			pluralRule{regexp.MustCompile(strings.ToUpper(word.singular) + "$"), strings.ToUpper(word.plural)},
			pluralRule{regexp.MustCompile(titleWord(word.singular) + "$"), titleWord(word.plural)},
			pluralRule{regexp.MustCompile(word.singular + "$"), word.plural},
		)
	}
	for _, rule := range slices.Backward(regular) {
		rules = append(rules,
			pluralRule{regexp.MustCompile(strings.ToUpper(rule.find)), strings.ToUpper(rule.replace)},
			pluralRule{regexp.MustCompile(rule.find), rule.replace},
			pluralRule{regexp.MustCompile("(?i)" + rule.find), rule.replace},
		)
	}
	return rules
}

func titleWord(word string) string {
	return strings.ToUpper(word[:1]) + word[1:]
}

// pluralize returns the English plural of a name using the inflection rules
// GORM pluralizes table names with. Only the last word changes, so order_item
// becomes order_items.
func pluralize(name string) string {
	for _, rule := range pluralRules {
		if rule.pattern.MatchString(name) {
			return rule.pattern.ReplaceAllString(name, rule.replace)
		}
	}
	return name
}
