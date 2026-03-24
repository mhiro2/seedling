package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/mhiro2/seedling"
)

type testCompany struct {
	ID   int
	Name string
}

func TestRegister(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	// Act
	err := seedling.Register(seedling.Blueprint[testCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() testCompany {
			return testCompany{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
			v.ID = 1
			return v, nil
		},
	})
	// Assert
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegisterTo_IsolatedRegistry(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	mkBlueprint := func(id int) seedling.Blueprint[testCompany] {
		return seedling.Blueprint[testCompany]{
			Name:    "company",
			Table:   "companies",
			PKField: "ID",
			Defaults: func() testCompany {
				return testCompany{Name: fmt.Sprintf("test-company-%d", id)}
			},
			Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
				v.ID = id
				return v, nil
			},
		}
	}

	reg1 := seedling.NewRegistry()
	reg2 := seedling.NewRegistry()

	err := seedling.RegisterTo(reg1, mkBlueprint(1))
	if err != nil {
		t.Fatalf("register to reg1: %v", err)
	}
	err = seedling.RegisterTo(reg2, mkBlueprint(2))
	if err != nil {
		t.Fatalf("register to reg2: %v", err)
	}

	// Act
	got1, err := seedling.NewSession[testCompany](reg1).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatalf("insert with reg1 session: %v", err)
	}
	got2, err := seedling.NewSession[testCompany](reg2).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatalf("insert with reg2 session: %v", err)
	}

	// Assert
	if got1.Root().ID != 1 {
		t.Fatalf("got %v, want %v", got1.Root().ID, 1)
	}
	if got2.Root().ID != 2 {
		t.Fatalf("got %v, want %v", got2.Root().ID, 2)
	}

	_, err = seedling.InsertOneE[testCompany](t.Context(), nil)
	if !errors.Is(err, seedling.ErrBlueprintNotFound) {
		t.Fatalf("got %v, want %v", err, seedling.ErrBlueprintNotFound)
	}
}

func TestRegistry_Reset(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	reg := seedling.NewRegistry()
	err := seedling.RegisterTo(reg, seedling.Blueprint[testCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
			v.ID = 1
			return v, nil
		},
	})
	if err != nil {
		t.Fatalf("register to custom registry: %v", err)
	}

	// Act
	reg.Reset()

	// Assert
	_, err = seedling.NewSession[testCompany](reg).InsertOneE(t.Context(), nil)
	if !errors.Is(err, seedling.ErrBlueprintNotFound) {
		t.Fatalf("got %v, want %v", err, seedling.ErrBlueprintNotFound)
	}
}

func TestRegister_Duplicate(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	bp := seedling.Blueprint[testCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() testCompany {
			return testCompany{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
			return v, nil
		},
	}
	err := seedling.Register(bp)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	err = seedling.Register(bp)

	// Assert
	if !errors.Is(err, seedling.ErrDuplicateBlueprint) {
		t.Fatalf("got %v, want %v", err, seedling.ErrDuplicateBlueprint)
	}
}

func TestMustRegister_Panics(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	bp := seedling.Blueprint[testCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() testCompany {
			return testCompany{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
			return v, nil
		},
	}
	seedling.MustRegister(bp)

	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate MustRegister")
		}
	}()
	seedling.MustRegister(bp)
}

func TestRegister_InsertNil(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	// Act
	err := seedling.Register(seedling.Blueprint[testCompany]{
		Name:  "company",
		Table: "companies",
		// Insert is nil
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for nil Insert")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestRegister_EmptyName(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	// Act
	err := seedling.Register(seedling.Blueprint[testCompany]{
		Name: "", // empty
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
			return v, nil
		},
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for empty Name")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

// testCompanyAlt is a separate type alias to avoid collision with testCompany
// in the duplicate Go type test.
type testCompanyAlt struct {
	ID   int
	Name string
}

func TestRegister_DuplicateGoType(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	err := seedling.Register(seedling.Blueprint[testCompanyAlt]{
		Name:    "company_alt",
		Table:   "companies",
		PKField: "ID",
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompanyAlt) (testCompanyAlt, error) {
			return v, nil
		},
	})
	if err != nil {
		t.Fatalf("first register should succeed: %v", err)
	}

	// Act
	err = seedling.Register(seedling.Blueprint[testCompanyAlt]{
		Name:    "company_alt2",
		Table:   "companies",
		PKField: "ID",
		Insert: func(ctx context.Context, db seedling.DBTX, v testCompanyAlt) (testCompanyAlt, error) {
			return v, nil
		},
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for duplicate Go type")
	}
	if !errors.Is(err, seedling.ErrDuplicateBlueprint) {
		t.Fatalf("got %v, want %v", err, seedling.ErrDuplicateBlueprint)
	}
}

func TestRegister_PointerTypeRejected(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()

	// Act
	err := seedling.RegisterTo(reg, seedling.Blueprint[*testCompany]{
		Name:    "ptr-company",
		Table:   "companies",
		PKField: "ID",
		Insert: func(ctx context.Context, db seedling.DBTX, v *testCompany) (*testCompany, error) {
			return v, nil
		},
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for pointer type blueprint")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestRegister_InterfaceTypeRejected(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()

	// Act
	err := seedling.RegisterTo(reg, seedling.Blueprint[any]{
		Name:    "iface-row",
		Table:   "rows",
		PKField: "ID",
		Insert: func(ctx context.Context, db seedling.DBTX, v any) (any, error) {
			_ = ctx
			_ = db
			return v, nil
		},
	})

	// Assert
	if err == nil {
		t.Fatal("expected error for interface type blueprint")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

// concurrencyItem is a dedicated type for the concurrency test to avoid
// collisions with other test types.
type concurrencyItem struct {
	ID   int
	Name string
}

func TestRegistry_ConcurrentRegisterAndLookup(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	const numRegisters = 20
	const numLookups = 20

	// Pre-register a blueprint so that lookup goroutines have something to find.
	seedling.MustRegister(seedling.Blueprint[concurrencyItem]{
		Name:    "preregistered",
		Table:   "items",
		PKField: "ID",
		Defaults: func() concurrencyItem {
			return concurrencyItem{Name: "pre"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v concurrencyItem) (concurrencyItem, error) {
			v.ID = 1
			return v, nil
		},
	})

	var wg sync.WaitGroup

	// Act
	// Launch goroutines that concurrently Register blueprints with unique names.
	for i := range numRegisters {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("company-%d", idx)
			_ = seedling.Register(seedling.Blueprint[testCompany]{
				Name:    name,
				Table:   "companies",
				PKField: "ID",
				Defaults: func() testCompany {
					return testCompany{Name: name}
				},
				Insert: func(ctx context.Context, db seedling.DBTX, v testCompany) (testCompany, error) {
					v.ID = idx
					return v, nil
				},
			})
		}(i)
	}

	// Launch goroutines that concurrently call InsertOneE for the
	// pre-registered blueprint, which internally does a registry lookup.
	for range numLookups {
		wg.Go(func() {
			_, err := seedling.InsertOneE[concurrencyItem](t.Context(), nil)
			// Assert
			if err != nil {
				t.Errorf("InsertOneE failed: %v", err)
			}
		})
	}

	wg.Wait()
}
