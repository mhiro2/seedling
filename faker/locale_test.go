package faker

import (
	"math/rand/v2"
	"regexp"
	"testing"
	"unicode"
)

func TestNewWithLocale_AllLocales(t *testing.T) {
	for _, locale := range []string{"en", "ja", "zh", "ko", "de", "fr"} {
		t.Run(locale, func(t *testing.T) {
			f := NewWithLocale(rand.New(rand.NewPCG(42, 42)), locale)
			// Call every locale-sensitive method; must not panic.
			f.FirstName()
			f.LastName()
			f.Name()
			f.Email()
			f.Phone()
			f.Address()
			f.City()
			f.ZipCode()
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

func TestJA_PhoneFormat(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "ja")
	re := regexp.MustCompile(`^\+81-\d{2}-\d{4}-\d{4}$`)
	for range 50 {
		if !re.MatchString(f.Phone()) {
			t.Fatalf("ja Phone() format mismatch: %s", f.Phone())
		}
	}
}

func TestJA_ZipCodeFormat(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "ja")
	re := regexp.MustCompile(`^\d{3}-\d{4}$`)
	for range 50 {
		if !re.MatchString(f.ZipCode()) {
			t.Fatalf("ja ZipCode() format mismatch: %s", f.ZipCode())
		}
	}
}

func TestJA_NameOrder(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "ja")
	for range 50 {
		name := f.Name()
		// Japanese names should not contain spaces (LastFirst format).
		if len(name) == 0 {
			t.Fatal("ja Name() returned empty string")
		}
		for _, r := range name {
			if r == ' ' {
				t.Fatalf("ja Name() should not contain space: %s", name)
			}
		}
	}
}

func TestDE_PhoneFormat(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "de")
	re := regexp.MustCompile(`^\+49-\d{3}-\d{7}$`)
	for range 50 {
		if !re.MatchString(f.Phone()) {
			t.Fatalf("de Phone() format mismatch: %s", f.Phone())
		}
	}
}

func TestFR_PhoneFormat(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "fr")
	re := regexp.MustCompile(`^\+33-\d-\d{2}-\d{2}-\d{2}-\d{2}$`)
	for range 50 {
		if !re.MatchString(f.Phone()) {
			t.Fatalf("fr Phone() format mismatch: %s", f.Phone())
		}
	}
}

func TestKO_PhoneFormat(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "ko")
	re := regexp.MustCompile(`^\+82-10-\d{4}-\d{4}$`)
	for range 50 {
		if !re.MatchString(f.Phone()) {
			t.Fatalf("ko Phone() format mismatch: %s", f.Phone())
		}
	}
}

func TestZH_PhoneFormat(t *testing.T) {
	f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), "zh")
	re := regexp.MustCompile(`^\+86-1\d{2}-\d{4}-\d{4}$`)
	for range 50 {
		if !re.MatchString(f.Phone()) {
			t.Fatalf("zh Phone() format mismatch: %s", f.Phone())
		}
	}
}

func TestCJK_EmailIsASCII(t *testing.T) {
	for _, locale := range []string{"ja", "zh", "ko"} {
		t.Run(locale, func(t *testing.T) {
			f := NewWithLocale(rand.New(rand.NewPCG(1, 1)), locale)
			for range 50 {
				email := f.Email()
				for _, r := range email {
					if r > unicode.MaxASCII {
						t.Fatalf("%s Email() contains non-ASCII: %s", locale, email)
					}
				}
			}
		})
	}
}
