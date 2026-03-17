package faker

import (
	"fmt"
	"math/rand/v2"
)

// localeData holds locale-specific data and formatting functions.
// Nil format functions fall back to English defaults.
type localeData struct {
	firstNames     []string
	lastNames      []string
	cities         []string
	streets        []string
	streetSuffixes []string

	// Romanized names for Email generation in non-Latin locales.
	// When nil, firstNames/lastNames are used directly.
	romanizedFirstNames []string
	romanizedLastNames  []string

	formatName    func(first, last string) string
	formatPhone   func(rng *rand.Rand) string
	formatZipCode func(rng *rand.Rand) string
	formatAddress func(rng *rand.Rand, street, suffix string) string
	formatEmail   func(first, last, domain string) string
}

var locales = map[string]*localeData{}

func registerLocale(name string, ld *localeData) {
	if _, ok := locales[name]; ok {
		panic(fmt.Sprintf("faker: locale %q already registered", name))
	}
	locales[name] = ld
}
