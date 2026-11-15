package main

import (
	"regexp"
	"slices"
	"strings"
)

// The English inflection rules in this file reproduce github.com/jinzhu/inflection,
// which GORM pluralizes table names with and sqlc singularizes table names
// with to name its models, so the names seedling-gen derives match the ones
// those tools generate. Only the end of a name changes, so order_items becomes
// order_item and back.
//
// The rules are adapted from code released under the MIT License:
//   - github.com/jinzhu/inflection: Copyright (c) 2015 - Jinzhu

type inflectionRule struct {
	pattern *regexp.Regexp
	replace string
}

type inflectionPair struct {
	from, to string
}

var inflectionUncountable = []string{"equipment", "information", "rice", "money", "species", "series", "fish", "sheep", "jeans", "police"}

// inflectionIrregular pairs each irregular singular with its plural.
var inflectionIrregular = []inflectionPair{
	{"person", "people"},
	{"man", "men"},
	{"child", "children"},
	{"sex", "sexes"},
	{"move", "moves"},
	{"mombie", "mombies"},
}

// pluralRules and singularRules list the rules in the order they are tried;
// the first matching rule decides the result.
var (
	pluralRules = compileInflectionRules(true, []inflectionPair{
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
	})
	singularRules = compileInflectionRules(false, []inflectionPair{
		{"s$", ""},
		{"(ss)$", "${1}"},
		{"(n)ews$", "${1}ews"},
		{"([ti])a$", "${1}um"},
		{"((a)naly|(b)a|(d)iagno|(p)arenthe|(p)rogno|(s)ynop|(t)he)(sis|ses)$", "${1}sis"},
		{"(^analy)(sis|ses)$", "${1}sis"},
		{"([^f])ves$", "${1}fe"},
		{"(hive)s$", "${1}"},
		{"(tive)s$", "${1}"},
		{"([lr])ves$", "${1}f"},
		{"([^aeiouy]|qu)ies$", "${1}y"},
		{"(s)eries$", "${1}eries"},
		{"(m)ovies$", "${1}ovie"},
		{"(c)ookies$", "${1}ookie"},
		{"(x|ch|ss|sh)es$", "${1}"},
		{"^(m|l)ice$", "${1}ouse"},
		{"(bus)(es)?$", "${1}"},
		{"(o)es$", "${1}"},
		{"(shoe)s$", "${1}"},
		{"(cris|test)(is|es)$", "${1}is"},
		{"^(a)x[ie]s$", "${1}xis"},
		{"(octop|vir)(us|i)$", "${1}us"},
		{"(alias|status)(es)?$", "${1}"},
		{"^(ox)en", "${1}"},
		{"(vert|ind)ices$", "${1}ex"},
		{"(matr)ices$", "${1}ix"},
		{"(quiz)zes$", "${1}"},
		{"(database)s$", "${1}"},
	})
)

// compileInflectionRules orders the rules as jinzhu/inflection does:
// uncountable words, then irregular words, then the regular rules with later
// entries taking precedence over earlier ones. plural selects the direction
// of the irregular words.
func compileInflectionRules(plural bool, regular []inflectionPair) []inflectionRule {
	rules := make([]inflectionRule, 0, len(inflectionUncountable)+3*len(inflectionIrregular)+3*len(regular))
	for _, noun := range inflectionUncountable {
		rules = append(rules, inflectionRule{regexp.MustCompile("^(?i)(" + noun + ")$"), "${1}"})
	}
	for _, word := range inflectionIrregular {
		from, to := word.from, word.to
		if !plural {
			from, to = to, from
		}
		rules = append(rules,
			inflectionRule{regexp.MustCompile(strings.ToUpper(from) + "$"), strings.ToUpper(to)},
			inflectionRule{regexp.MustCompile(titleWord(from) + "$"), titleWord(to)},
			inflectionRule{regexp.MustCompile(from + "$"), to},
		)
	}
	for _, rule := range slices.Backward(regular) {
		rules = append(rules,
			inflectionRule{regexp.MustCompile(strings.ToUpper(rule.from)), strings.ToUpper(rule.to)},
			inflectionRule{regexp.MustCompile(rule.from), rule.to},
			inflectionRule{regexp.MustCompile("(?i)" + rule.from), rule.to},
		)
	}
	return rules
}

func titleWord(word string) string {
	return strings.ToUpper(word[:1]) + word[1:]
}

func inflect(rules []inflectionRule, name string) string {
	for _, rule := range rules {
		if rule.pattern.MatchString(name) {
			return rule.pattern.ReplaceAllString(name, rule.replace)
		}
	}
	return name
}

// pluralize returns the English plural of a name, as GORM derives a table
// name from a type name.
func pluralize(name string) string {
	return inflect(pluralRules, name)
}

// sqlcSingularExceptions are the names sqlc singularizes itself instead of
// leaving them to jinzhu/inflection, which gets them wrong.
var sqlcSingularExceptions = map[string]string{
	"campus":   "campus",
	"meta":     "meta",
	"calories": "calorie",
	"waves":    "wave",
	"metadata": "metadata",
}

// singularize returns the English singular of a table name, as sqlc derives
// a model name from a table name, so courses becomes course and statuses
// becomes status.
func singularize(name string) string {
	if singular, ok := sqlcSingularExceptions[strings.ToLower(name)]; ok {
		return singular
	}
	return inflect(singularRules, name)
}
