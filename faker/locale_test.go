package faker

import (
	"math/rand/v2"
	"net/mail"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// localeFormat describes the output every locale-dependent Faker method must
// produce for one locale.
type localeFormat struct {
	locale    string
	script    []*unicode.RangeTable // scripts allowed in FirstName, LastName and City
	lastFirst bool                  // Name is LastFirst with no space instead of "First Last"
	phone     *regexp.Regexp
	zipCode   *regexp.Regexp
	address   *regexp.Regexp
}

var localeFormats = []localeFormat{
	{
		locale:  "en",
		script:  []*unicode.RangeTable{unicode.Latin},
		phone:   regexp.MustCompile(`^\+1-\d{3}-\d{3}-\d{4}$`),
		zipCode: regexp.MustCompile(`^\d{5}$`),
		address: regexp.MustCompile(`^[1-9]\d{0,3} \p{Lu}\p{Ll}+ (St|Ave|Blvd|Dr|Ln|Rd|Way|Ct|Pl|Cir)$`),
	},
	{
		locale:    "ja",
		script:    []*unicode.RangeTable{unicode.Han, unicode.Hiragana, unicode.Katakana},
		lastFirst: true,
		phone:     regexp.MustCompile(`^\+81-\d{2}-\d{4}-\d{4}$`),
		zipCode:   regexp.MustCompile(`^\d{3}-\d{4}$`),
		address:   regexp.MustCompile(`^\p{Han}+[1-9]丁目[1-9]\d?-[1-9]\d?$`),
	},
	{
		locale:    "zh",
		script:    []*unicode.RangeTable{unicode.Han},
		lastFirst: true,
		phone:     regexp.MustCompile(`^\+86-1\d{2}-\d{4}-\d{4}$`),
		zipCode:   regexp.MustCompile(`^\d{6}$`),
		address:   regexp.MustCompile(`^\p{Han}+(路|街|大道|巷)[1-9]\d{0,2}号$`),
	},
	{
		locale:    "ko",
		script:    []*unicode.RangeTable{unicode.Hangul},
		lastFirst: true,
		phone:     regexp.MustCompile(`^\+82-10-\d{4}-\d{4}$`),
		zipCode:   regexp.MustCompile(`^\d{5}$`),
		address:   regexp.MustCompile(`^\p{Hangul}+(로|길|대로) [1-9]\d{0,2}$`),
	},
	{
		locale:  "de",
		script:  []*unicode.RangeTable{unicode.Latin},
		phone:   regexp.MustCompile(`^\+49-\d{3}-\d{7}$`),
		zipCode: regexp.MustCompile(`^[1-9]\d{4}$`),
		address: regexp.MustCompile(`^\p{Lu}\p{Ll}+(straße|weg|gasse|allee|platz|ring) [1-9]\d{0,2}$`),
	},
	{
		locale:  "fr",
		script:  []*unicode.RangeTable{unicode.Latin},
		phone:   regexp.MustCompile(`^\+33-\d-\d{2}-\d{2}-\d{2}-\d{2}$`),
		zipCode: regexp.MustCompile(`^\d{5}$`),
		address: regexp.MustCompile(`^[1-9]\d{0,2} (rue|avenue|boulevard|place|allée|impasse) \p{Lu}[\p{L} ]*\p{Ll}$`),
	},
}

func TestNewWithLocale_AllLocales(t *testing.T) {
	if len(localeFormats) != len(locales) {
		t.Fatalf("localeFormats covers %d locales, want all %d registered", len(localeFormats), len(locales))
	}
	username := regexp.MustCompile(`^[^\s\d]+[1-9]\d{3}$`)
	for _, tt := range localeFormats {
		t.Run(tt.locale, func(t *testing.T) {
			f := NewWithLocale(rand.New(rand.NewPCG(42, 42)), tt.locale)
			for range 50 {
				for method, v := range map[string]string{
					"FirstName": f.FirstName(),
					"LastName":  f.LastName(),
					"City":      f.City(),
				} {
					if !inScript(v, tt.script) {
						t.Fatalf("%s() = %q, want a non-empty name in the locale script", method, v)
					}
				}
				for method, c := range map[string]struct {
					re *regexp.Regexp
					v  string
				}{
					"Username": {username, f.Username()},
					"Phone":    {tt.phone, f.Phone()},
					"ZipCode":  {tt.zipCode, f.ZipCode()},
					"Address":  {tt.address, f.Address()},
				} {
					if !c.re.MatchString(c.v) {
						t.Fatalf("%s() = %q, want match for %s", method, c.v, c.re)
					}
				}
				assertEmail(t, f.Email())
			}
		})
	}
}

// inScript reports whether s is non-empty, has no surrounding spaces, and
// consists of letters from tables, allowing inner spaces and hyphens as in
// "New York" or "Saint-Étienne".
func inScript(s string, tables []*unicode.RangeTable) bool {
	if s == "" || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if r != ' ' && r != '-' && !unicode.IsOneOf(tables, r) {
			return false
		}
	}
	return true
}

// assertEmail checks that email is a bare ASCII "first.last@domain" address,
// which every locale must produce by romanizing names where needed.
func assertEmail(t *testing.T, email string) {
	t.Helper()
	addr, err := mail.ParseAddress(email)
	if err != nil {
		t.Fatalf("Email() = %q does not parse: %v", email, err)
	}
	if addr.Name != "" || addr.Address != email {
		t.Fatalf("Email() = %q is not a bare address", email)
	}
	for _, r := range email {
		if r > unicode.MaxASCII {
			t.Fatalf("Email() = %q contains non-ASCII", email)
		}
	}
	local, domain, _ := strings.Cut(email, "@")
	first, last, ok := strings.Cut(local, ".")
	if !ok || first == "" || last == "" || domain == "" {
		t.Fatalf("Email() = %q, want first.last@domain", email)
	}
}

func TestName_Order(t *testing.T) {
	for _, tt := range localeFormats {
		t.Run(tt.locale, func(t *testing.T) {
			// Name draws the first name before the last name, so a twin RNG
			// calling FirstName then LastName sees the same picks.
			f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), tt.locale)
			parts := NewWithLocale(rand.New(rand.NewPCG(1, 1)), tt.locale)
			for range 50 {
				name := f.Name()
				first, last := parts.FirstName(), parts.LastName()
				want := first + " " + last
				if tt.lastFirst {
					want = last + first
				}
				if name != want {
					t.Fatalf("Name() = %q, want %q (first %q, last %q)", name, want, first, last)
				}
			}
		})
	}
}

func TestNewWithLocale_UnknownPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown locale")
		}
	}()
	NewWithLocale(rand.New(rand.NewPCG(1, 1)), "xx")
}

func TestNewWithLocale_Determinism(t *testing.T) {
	for _, locale := range []string{"en", "ja", "zh", "ko", "de", "fr"} {
		t.Run(locale, func(t *testing.T) {
			f1 := NewWithLocale(rand.New(rand.NewPCG(99, 99)), locale)
			f2 := NewWithLocale(rand.New(rand.NewPCG(99, 99)), locale)
			for range 20 {
				if f1.Name() != f2.Name() {
					t.Fatal("Name() not deterministic")
				}
				if f1.Email() != f2.Email() {
					t.Fatal("Email() not deterministic")
				}
				if f1.Phone() != f2.Phone() {
					t.Fatal("Phone() not deterministic")
				}
				if f1.Address() != f2.Address() {
					t.Fatal("Address() not deterministic")
				}
			}
		})
	}
}

func TestNew_EqualsEnLocale(t *testing.T) {
	f1 := New(rand.New(rand.NewPCG(77, 77)))
	f2 := NewWithLocale(rand.New(rand.NewPCG(77, 77)), "en")
	for range 20 {
		if f1.Name() != f2.Name() {
			t.Fatal("New() and NewWithLocale(en) differ for Name()")
		}
		if f1.Email() != f2.Email() {
			t.Fatal("New() and NewWithLocale(en) differ for Email()")
		}
		if f1.Phone() != f2.Phone() {
			t.Fatal("New() and NewWithLocale(en) differ for Phone()")
		}
		if f1.Address() != f2.Address() {
			t.Fatal("New() and NewWithLocale(en) differ for Address()")
		}
		if f1.ZipCode() != f2.ZipCode() {
			t.Fatal("New() and NewWithLocale(en) differ for ZipCode()")
		}
	}
}
