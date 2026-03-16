package faker_test

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mhiro2/seedling/faker"
)

func newSeeded() *faker.Faker {
	return faker.New(rand.New(rand.NewPCG(42, 42)))
}

func TestDeterminism(t *testing.T) {
	f1 := newSeeded()
	f2 := newSeeded()

	if f1.Name() != f2.Name() {
		t.Error("Name not deterministic")
	}
	if f1.Email() != f2.Email() {
		t.Error("Email not deterministic")
	}
	if f1.Username() != f2.Username() {
		t.Error("Username not deterministic")
	}
	if f1.Phone() != f2.Phone() {
		t.Error("Phone not deterministic")
	}
	if f1.Address() != f2.Address() {
		t.Error("Address not deterministic")
	}
	if f1.City() != f2.City() {
		t.Error("City not deterministic")
	}
	if f1.Country() != f2.Country() {
		t.Error("Country not deterministic")
	}
	if f1.ZipCode() != f2.ZipCode() {
		t.Error("ZipCode not deterministic")
	}
	if f1.Sentence() != f2.Sentence() {
		t.Error("Sentence not deterministic")
	}
	if f1.UUID() != f2.UUID() {
		t.Error("UUID not deterministic")
	}
	if f1.HexColor() != f2.HexColor() {
		t.Error("HexColor not deterministic")
	}
	if f1.IPv4() != f2.IPv4() {
		t.Error("IPv4 not deterministic")
	}
	if f1.CreditCard() != f2.CreditCard() {
		t.Error("CreditCard not deterministic")
	}
	if f1.Int() != f2.Int() {
		t.Error("Int not deterministic")
	}
	if f1.Bool() != f2.Bool() {
		t.Error("Bool not deterministic")
	}
	if !f1.Date().Equal(f2.Date()) {
		t.Error("Date not deterministic")
	}
}

func TestEmailFormat(t *testing.T) {
	f := newSeeded()
	for range 100 {
		email := f.Email()
		if !strings.Contains(email, "@") {
			t.Errorf("Email missing @: %s", email)
		}
		parts := strings.SplitN(email, "@", 2)
		if !strings.Contains(parts[0], ".") {
			t.Errorf("Email local part missing dot: %s", email)
		}
	}
}

func TestPhoneFormat(t *testing.T) {
	f := newSeeded()
	re := regexp.MustCompile(`^\+1-\d{3}-\d{3}-\d{4}$`)
	for range 100 {
		phone := f.Phone()
		if !re.MatchString(phone) {
			t.Errorf("Phone format invalid: %s", phone)
		}
	}
}

func TestUUIDFormat(t *testing.T) {
	f := newSeeded()
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for range 100 {
		uuid := f.UUID()
		if !re.MatchString(uuid) {
			t.Errorf("UUID format invalid: %s", uuid)
		}
	}
}

func TestHexColorFormat(t *testing.T) {
	f := newSeeded()
	re := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for range 100 {
		color := f.HexColor()
		if !re.MatchString(color) {
			t.Errorf("HexColor format invalid: %s", color)
		}
	}
}

func TestIPv4Format(t *testing.T) {
	f := newSeeded()
	re := regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	for range 100 {
		ip := f.IPv4()
		if !re.MatchString(ip) {
			t.Errorf("IPv4 format invalid: %s", ip)
		}
	}
}

func TestCreditCardFormat(t *testing.T) {
	f := newSeeded()
	re := regexp.MustCompile(`^\d{4}-\d{4}-\d{4}-\d{4}$`)
	for range 100 {
		cc := f.CreditCard()
		if !re.MatchString(cc) {
			t.Errorf("CreditCard format invalid: %s", cc)
		}
	}
}

func TestZipCodeFormat(t *testing.T) {
	f := newSeeded()
	re := regexp.MustCompile(`^\d{5}$`)
	for range 100 {
		zc := f.ZipCode()
		if !re.MatchString(zc) {
			t.Errorf("ZipCode format invalid: %s", zc)
		}
	}
}

func TestIntBetween(t *testing.T) {
	f := newSeeded()
	for range 1000 {
		v := f.IntBetween(5, 10)
		if v < 5 || v > 10 {
			t.Errorf("IntBetween(5, 10) = %d, out of range", v)
		}
	}
}

func TestFloatBetween(t *testing.T) {
	f := newSeeded()
	for range 1000 {
		v := f.FloatBetween(1.0, 5.0)
		if v < 1.0 || v >= 5.0 {
			t.Errorf("FloatBetween(1.0, 5.0) = %f, out of range", v)
		}
	}
}

func TestDateBetween(t *testing.T) {
	f := newSeeded()
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2020, 12, 31, 23, 59, 59, 0, time.UTC)
	for range 100 {
		d := f.DateBetween(from, to)
		if d.Before(from) || d.After(to) {
			t.Errorf("DateBetween out of range: %v", d)
		}
	}
}

func TestSentence(t *testing.T) {
	f := newSeeded()
	for range 100 {
		s := f.Sentence()
		if !strings.HasSuffix(s, ".") {
			t.Errorf("Sentence missing period: %s", s)
		}
		if s[0] < 'A' || s[0] > 'Z' {
			t.Errorf("Sentence not capitalized: %s", s)
		}
	}
}

func TestParagraph(t *testing.T) {
	f := newSeeded()
	p := f.Paragraph()
	sentences := strings.Count(p, ".")
	if sentences < 3 || sentences > 6 {
		t.Errorf("Paragraph has %d sentences, expected 3-6", sentences)
	}
}

func TestPick(t *testing.T) {
	f := newSeeded()
	items := []int{10, 20, 30}
	for range 100 {
		v := faker.Pick(f, items)
		if v != 10 && v != 20 && v != 30 {
			t.Errorf("Pick returned unexpected value: %d", v)
		}
	}
}

func TestPickPanicsOnEmpty(t *testing.T) {
	f := newSeeded()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Pick did not panic on empty slice")
		}
	}()
	faker.Pick(f, []int{})
}

func TestNew_NilPanics(t *testing.T) {
	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil *rand.Rand")
		}
	}()
	faker.New(nil)
}

func TestIntBetween_ReversedArgsPanics(t *testing.T) {
	// Arrange
	f := newSeeded()

	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when min > max")
		}
	}()
	f.IntBetween(10, 5)
}

func TestDateBetween_ReversedArgsPanics(t *testing.T) {
	// Arrange
	f := newSeeded()
	later := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	earlier := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when from > to")
		}
	}()
	f.DateBetween(later, earlier)
}

func TestURLFormat(t *testing.T) {
	f := newSeeded()
	for range 100 {
		u := f.URL()
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("URL missing https prefix: %s", u)
		}
		if strings.Count(u, "/") < 3 {
			t.Errorf("URL missing path: %s", u)
		}
	}
}

func TestWord(t *testing.T) {
	f := newSeeded()
	for range 100 {
		w := f.Word()
		if w == "" {
			t.Error("Word returned empty string")
		}
	}
}

func TestFloat(t *testing.T) {
	f := newSeeded()
	for range 1000 {
		v := f.Float()
		if v < 0.0 || v >= 1.0 {
			t.Errorf("Float() = %f, out of range [0.0, 1.0)", v)
		}
	}
}

func TestDefault(t *testing.T) {
	f := faker.Default()
	if f == nil {
		t.Fatal("Default() returned nil")
	}
	// Smoke test: should not panic.
	_ = f.Name()
	_ = f.Email()
}

func ExampleNew() {
	f := faker.New(rand.New(rand.NewPCG(42, 42)))

	fmt.Println(f.Name())
	fmt.Println(f.Email())
	fmt.Println(f.Phone())
	fmt.Println(f.Address())
	fmt.Println(f.UUID())
	// Output:
	// Cynthia Torres
	// kathleen.roberts@local.test
	// +1-339-323-7574
	// 8197 North St
	// 5171f6ef-7782-4870-83f6-83d1b4fa890c
}

func ExampleFaker_City() {
	f := faker.New(rand.New(rand.NewPCG(1, 1)))
	fmt.Println(f.City())
	// Output: Detroit
}

func ExamplePick() {
	f := faker.New(rand.New(rand.NewPCG(1, 1)))
	colors := []string{"red", "green", "blue"}
	fmt.Println(faker.Pick(f, colors))
	// Output: blue
}
