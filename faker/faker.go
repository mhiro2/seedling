// Package faker provides deterministic fake data generators for use with seedling's
// Generate option. All methods derive output solely from the wrapped *rand.Rand,
// ensuring reproducible test data when the same seed is used.
//
// Basic usage with seedling.Generate:
//
//	seedling.Generate(func(r *rand.Rand, u *User) {
//	    f := faker.New(r)
//	    u.Name = f.Name()
//	    u.Email = f.Email()
//	})
//
// Faker is NOT safe for concurrent use.
package faker

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
	"unicode"
)

// Faker generates deterministic fake data from a *rand.Rand source.
type Faker struct {
	rng *rand.Rand
}

// New creates a Faker backed by the given RNG.
// Passing the *rand.Rand from seedling.Generate ensures deterministic output.
// It panics if r is nil.
func New(r *rand.Rand) *Faker {
	if r == nil {
		panic("faker: New requires a non-nil *rand.Rand")
	}
	return &Faker{rng: r}
}

// Default creates a Faker with a non-deterministic seed.
// Use New with a fixed seed for reproducible tests.
func Default() *Faker {
	//nolint:gosec // Faker intentionally uses a pseudo-random generator for non-security test data.
	return New(rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
}

// --- Person ---

// FirstName returns a random first name.
func (f *Faker) FirstName() string { return f.pick(firstNames) }

// LastName returns a random last name.
func (f *Faker) LastName() string { return f.pick(lastNames) }

// Name returns a random full name (first + last).
func (f *Faker) Name() string { return f.FirstName() + " " + f.LastName() }

// --- Internet ---

// Email returns a random email address.
func (f *Faker) Email() string {
	first := strings.ToLower(f.FirstName())
	last := strings.ToLower(f.LastName())
	domain := f.pick(domains)
	return fmt.Sprintf("%s.%s@%s", first, last, domain)
}

// Username returns a random username (lowercase first name + digits).
func (f *Faker) Username() string {
	name := strings.ToLower(f.FirstName())
	digits := f.rng.IntN(9000) + 1000 // 1000-9999
	return fmt.Sprintf("%s%d", name, digits)
}

// URL returns a random URL.
func (f *Faker) URL() string {
	return fmt.Sprintf("https://%s/%s", f.pick(domains), f.Username())
}

// HexColor returns a random hex color string like "#a3c1f0".
func (f *Faker) HexColor() string {
	return fmt.Sprintf("#%06x", f.rng.IntN(0x1000000))
}

// IPv4 returns a random IPv4 address string.
func (f *Faker) IPv4() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		f.rng.IntN(255)+1,
		f.rng.IntN(256),
		f.rng.IntN(256),
		f.rng.IntN(254)+1,
	)
}

// --- Phone ---

// Phone returns a random phone number in +1-XXX-XXX-XXXX format.
func (f *Faker) Phone() string {
	return fmt.Sprintf("+1-%03d-%03d-%04d",
		f.rng.IntN(900)+100,
		f.rng.IntN(900)+100,
		f.rng.IntN(10000),
	)
}

// --- Address ---

// Address returns a random street address.
func (f *Faker) Address() string {
	return fmt.Sprintf("%d %s %s",
		f.rng.IntN(9999)+1,
		f.pick(streets),
		f.pick(streetSuffixes),
	)
}

// City returns a random city name.
func (f *Faker) City() string { return f.pick(cities) }

// Country returns a random country name.
func (f *Faker) Country() string { return f.pick(countries) }

// ZipCode returns a random 5-digit US zip code.
func (f *Faker) ZipCode() string { return fmt.Sprintf("%05d", f.rng.IntN(100000)) }

// --- Text ---

// Sentence returns a random sentence of 5-12 words.
func (f *Faker) Sentence() string {
	n := f.rng.IntN(8) + 5 // 5-12 words
	parts := make([]string, n)
	for i := range parts {
		parts[i] = f.pick(words)
	}
	s := strings.Join(parts, " ")
	// Capitalize first letter.
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes) + "."
}

// Paragraph returns a random paragraph of 3-6 sentences.
func (f *Faker) Paragraph() string {
	n := f.rng.IntN(4) + 3 // 3-6 sentences
	parts := make([]string, n)
	for i := range parts {
		parts[i] = f.Sentence()
	}
	return strings.Join(parts, " ")
}

// Word returns a random common English word.
func (f *Faker) Word() string { return f.pick(words) }

// --- Identifiers ---

// UUID returns a random v4 UUID string.
func (f *Faker) UUID() string {
	var buf [16]byte
	for i := range buf {
		//nolint:gosec // IntN(256) guarantees the value fits into a byte.
		buf[i] = byte(f.rng.IntN(256))
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// CreditCard returns a random credit card number in XXXX-XXXX-XXXX-XXXX format.
func (f *Faker) CreditCard() string {
	return fmt.Sprintf("%04d-%04d-%04d-%04d",
		f.rng.IntN(10000),
		f.rng.IntN(10000),
		f.rng.IntN(10000),
		f.rng.IntN(10000),
	)
}

// --- Numeric ---

// Int returns a random non-negative int.
func (f *Faker) Int() int { return f.rng.Int() }

// IntBetween returns a random int in [min, max].
// It panics if min > max.
func (f *Faker) IntBetween(min, max int) int {
	if min > max {
		panic(fmt.Sprintf("faker: IntBetween requires min <= max, got min=%d max=%d", min, max))
	}
	return min + f.rng.IntN(max-min+1)
}

// Float returns a random float64 in [0.0, 1.0).
func (f *Faker) Float() float64 { return f.rng.Float64() }

// FloatBetween returns a random float64 in [min, max).
func (f *Faker) FloatBetween(min, max float64) float64 {
	return min + f.rng.Float64()*(max-min)
}

// Bool returns a random boolean.
func (f *Faker) Bool() bool { return f.rng.IntN(2) == 1 }

// --- Time ---

// Date returns a random date between 2000-01-01 and 2030-12-31.
func (f *Faker) Date() time.Time {
	return f.DateBetween(
		time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC),
	)
}

// DateBetween returns a random time between from and to (inclusive).
// It panics if from is after to.
func (f *Faker) DateBetween(from, to time.Time) time.Time {
	if from.After(to) {
		panic(fmt.Sprintf("faker: DateBetween requires from <= to, got from=%s to=%s", from, to))
	}
	delta := to.Sub(from)
	offset := f.rng.Int64N(int64(delta) + 1)
	return from.Add(time.Duration(offset))
}

// --- Generic ---

// Pick returns a random element from items.
// It panics if items is empty.
func Pick[T any](f *Faker, items []T) T {
	return items[f.rng.IntN(len(items))]
}

// pick is an internal helper for string slices.
func (f *Faker) pick(items []string) string {
	return items[f.rng.IntN(len(items))]
}
