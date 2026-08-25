package seedling_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling"
)

type pointerCompany struct {
	ID   int
	Name string
}

type pointerUser struct {
	ID        int
	CompanyID int
	Name      string
}

type pointerNumber int

type namedPointerCompany *pointerCompany

type pointerState struct {
	nextID  int
	inserts []string
	deletes []string
}

func newPointerRegistry(tb testing.TB) (*seedling.Registry, *pointerState) {
	tb.Helper()

	reg := seedling.NewRegistry()
	state := &pointerState{}

	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerCompany]{
		Name:    "pointer_company",
		Table:   "pointer_companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) (*pointerCompany, error) {
			state.nextID++
			company.ID = state.nextID
			state.inserts = append(state.inserts, "company")
			return company, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, _ *pointerCompany) error {
			state.deletes = append(state.deletes, "company")
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerUser]{
		Name:    "pointer_user",
		Table:   "pointer_users",
		PKField: "ID",
		Defaults: func() *pointerUser {
			return &pointerUser{}
		},
		Relations: []seedling.Relation{
			seedling.BelongsToRelation("company", "pointer_company", false, "CompanyID"),
		},
		Insert: func(_ context.Context, _ seedling.DBTX, user *pointerUser) (*pointerUser, error) {
			state.nextID++
			user.ID = state.nextID
			state.inserts = append(state.inserts, "user")
			return user, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, _ *pointerUser) error {
			state.deletes = append(state.deletes, "user")
			return nil
		},
	})

	return reg, state
}

func TestPointerBlueprint_InsertAndCleanup(t *testing.T) {
	// Arrange
	reg, state := newPointerRegistry(t)
	var afterInsertRoots []*pointerUser
	plan, err := seedling.NewSession[*pointerUser](reg).BuildE(
		seedling.Generate(func(_ *rand.Rand, user *pointerUser) {
			user.Name = "generated"
		}),
		seedling.Set("Name", "set"),
		seedling.With(func(user *pointerUser) {
			user.Name += "-with"
		}),
		seedling.When("company", func(user *pointerUser) bool {
			return user.Name == "set-with"
		}),
		seedling.Ref("company", seedling.Set("Name", "parent")),
		seedling.AfterInsert(func(user *pointerUser, _ seedling.DBTX) {
			afterInsertRoots = append(afterInsertRoots, user)
		}),
	)
	if err != nil {
		t.Fatalf("build pointer plan: %v", err)
	}

	// Act
	first, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	second, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}

	// Assert
	if first.Root() == second.Root() {
		t.Fatal("reused plan returned the same root pointer")
	}
	if first.Root().Name != "set-with" || second.Root().Name != "set-with" {
		t.Fatalf("root names = %q, %q; want set-with", first.Root().Name, second.Root().Name)
	}
	if !reflect.DeepEqual(state.inserts, []string{"company", "user", "company", "user"}) {
		t.Fatalf("insert order = %v", state.inserts)
	}
	if len(afterInsertRoots) != 2 || afterInsertRoots[0] != first.Root() || afterInsertRoots[1] != second.Root() {
		t.Fatalf("after-insert roots = %v", afterInsertRoots)
	}

	firstCompany, ok, err := seedling.NodeAs[*pointerCompany](first, "pointer_company")
	if err != nil {
		t.Fatalf("read pointer company: %v", err)
	}
	if !ok {
		t.Fatal("pointer company node not found")
	}
	if firstCompany.Name != "parent" {
		t.Fatalf("company name = %q, want parent", firstCompany.Name)
	}
	if first.Root().CompanyID != firstCompany.ID {
		t.Fatalf("first CompanyID = %d, want %d", first.Root().CompanyID, firstCompany.ID)
	}

	if err := first.CleanupE(t.Context(), nil); err != nil {
		t.Fatalf("cleanup first result: %v", err)
	}
	if err := second.CleanupE(t.Context(), nil); err != nil {
		t.Fatalf("cleanup second result: %v", err)
	}
	if !reflect.DeepEqual(state.deletes, []string{"user", "company", "user", "company"}) {
		t.Fatalf("delete order = %v", state.deletes)
	}
}

func TestPointerBlueprint_InsertMany(t *testing.T) {
	// Arrange
	reg, state := newPointerRegistry(t)
	session := seedling.NewSession[*pointerCompany](reg)

	// Act
	result, err := session.InsertManyE(
		t.Context(),
		nil,
		2,
		seedling.Seq("Name", func(index int) string {
			if index == 0 {
				return "first"
			}
			return "second"
		}),
	)
	if err != nil {
		t.Fatalf("insert pointer batch: %v", err)
	}

	// Assert
	roots := result.Roots()
	if len(roots) != 2 {
		t.Fatalf("root count = %d, want 2", len(roots))
	}
	if roots[0] == roots[1] {
		t.Fatal("batch roots share the same pointer")
	}
	if roots[0].Name != "first" || roots[1].Name != "second" {
		t.Fatalf("root names = %q, %q", roots[0].Name, roots[1].Name)
	}
	if !reflect.DeepEqual(state.inserts, []string{"company", "company"}) {
		t.Fatalf("insert order = %v", state.inserts)
	}
	if err := result.CleanupE(t.Context(), nil); err != nil {
		t.Fatalf("cleanup pointer batch: %v", err)
	}
	if !reflect.DeepEqual(state.deletes, []string{"company", "company"}) {
		t.Fatalf("delete order = %v", state.deletes)
	}
}

func TestPointerBlueprint_UseNormalizesModelType(t *testing.T) {
	tests := []struct {
		name     string
		provided any
	}{
		{name: "value", provided: pointerCompany{ID: 42, Name: "existing"}},
		{name: "pointer", provided: &pointerCompany{ID: 42, Name: "existing"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			reg, state := newPointerRegistry(t)

			// Act
			result, err := seedling.NewSession[*pointerUser](reg).InsertOneE(
				t.Context(),
				nil,
				seedling.Use("company", tt.provided),
			)
			if err != nil {
				t.Fatalf("insert with provided company: %v", err)
			}

			// Assert
			company, ok, err := seedling.NodeAs[*pointerCompany](result, "pointer_company")
			if err != nil {
				t.Fatalf("read provided company: %v", err)
			}
			if !ok {
				t.Fatal("provided company node not found")
			}
			if company.ID != 42 || result.Root().CompanyID != 42 {
				t.Fatalf("company IDs = %d, %d; want 42", company.ID, result.Root().CompanyID)
			}
			if !reflect.DeepEqual(state.inserts, []string{"user"}) {
				t.Fatalf("insert order = %v", state.inserts)
			}
			if err := result.CleanupE(t.Context(), nil); err != nil {
				t.Fatalf("cleanup result: %v", err)
			}
			if !reflect.DeepEqual(state.deletes, []string{"user"}) {
				t.Fatalf("delete order = %v", state.deletes)
			}
		})
	}
}

func TestPointerBlueprint_BuilderMutations(t *testing.T) {
	// Arrange
	reg, _ := newPointerRegistry(t)
	builder := seedling.ForSession(seedling.NewSession[*pointerCompany](reg)).
		Generate(func(_ *rand.Rand, company **pointerCompany) {
			*company = &pointerCompany{Name: "generated"}
		}).
		With(func(company **pointerCompany) {
			(*company).Name += "-with"
		})

	// Act
	result, err := builder.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatalf("insert through pointer builder: %v", err)
	}

	// Assert
	if result.Root().Name != "generated-with" {
		t.Fatalf("company name = %q, want generated-with", result.Root().Name)
	}
}

func TestPointerBlueprint_RegistrationValidation(t *testing.T) {
	tests := []struct {
		name     string
		register func(*seedling.Registry) error
	}{
		{
			name: "pointer to pointer",
			register: func(reg *seedling.Registry) error {
				return seedling.RegisterTo(reg, seedling.Blueprint[**pointerCompany]{
					Name: "double_pointer",
					Insert: func(_ context.Context, _ seedling.DBTX, value **pointerCompany) (**pointerCompany, error) {
						return value, nil
					},
				})
			},
		},
		{
			name: "pointer to scalar",
			register: func(reg *seedling.Registry) error {
				return seedling.RegisterTo(reg, seedling.Blueprint[*pointerNumber]{
					Name: "pointer_number",
					Insert: func(_ context.Context, _ seedling.DBTX, value *pointerNumber) (*pointerNumber, error) {
						return value, nil
					},
				})
			},
		},
		{
			name: "scalar value",
			register: func(reg *seedling.Registry) error {
				return seedling.RegisterTo(reg, seedling.Blueprint[pointerNumber]{
					Name: "number",
					Insert: func(_ context.Context, _ seedling.DBTX, value pointerNumber) (pointerNumber, error) {
						return value, nil
					},
				})
			},
		},
		{
			name: "named pointer",
			register: func(reg *seedling.Registry) error {
				return seedling.RegisterTo(reg, seedling.Blueprint[namedPointerCompany]{
					Name: "named_pointer",
					Insert: func(_ context.Context, _ seedling.DBTX, value namedPointerCompany) (namedPointerCompany, error) {
						return value, nil
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.register(seedling.NewRegistry())

			// Assert
			if !errors.Is(err, seedling.ErrInvalidOption) {
				t.Fatalf("registration error = %v, want %v", err, seedling.ErrInvalidOption)
			}
		})
	}
}

func TestPointerBlueprint_NilDefaultsReturnsErrorAtBuild(t *testing.T) {
	// Arrange: Defaults is a per-record hook, so registration must not invoke
	// it; a nil result surfaces when the first record is planned.
	reg := seedling.NewRegistry()
	err := seedling.RegisterTo(reg, seedling.Blueprint[*pointerCompany]{
		Name: "nil_defaults",
		Defaults: func() *pointerCompany {
			return nil
		},
		Insert: func(_ context.Context, _ seedling.DBTX, value *pointerCompany) (*pointerCompany, error) {
			return value, nil
		},
	})
	if err != nil {
		t.Fatalf("register pointer blueprint: %v", err)
	}

	// Act
	_, err = seedling.NewSession[*pointerCompany](reg).BuildE()

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("build error = %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestPointerBlueprint_NilInsertResultReturnsError(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerCompany]{
		Name: "nil_insert",
		Defaults: func() *pointerCompany {
			return &pointerCompany{}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, _ *pointerCompany) (*pointerCompany, error) {
			return nil, nil //nolint:nilnil // An invalid successful result exercises executor validation.
		},
	})

	// Act
	_, err := seedling.NewSession[*pointerCompany](reg).InsertOneE(t.Context(), nil)

	// Assert
	if !errors.Is(err, seedling.ErrInsertFailed) {
		t.Fatalf("insert error = %v, want %v", err, seedling.ErrInsertFailed)
	}
}

func TestPointerBlueprint_UseRejectsExtraIndirection(t *testing.T) {
	// Arrange
	reg, _ := newPointerRegistry(t)
	company := &pointerCompany{ID: 42}

	// Act
	_, err := seedling.NewSession[*pointerUser](reg).InsertOneE(
		t.Context(),
		nil,
		seedling.Use("company", &company),
	)

	// Assert
	if !errors.Is(err, seedling.ErrTypeMismatch) {
		t.Fatalf("insert error = %v, want %v", err, seedling.ErrTypeMismatch)
	}
}

func TestPointerBlueprint_CleanupUsesInsertedSnapshot(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	var deleted []int
	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerCompany]{
		Name:    "snapshot_company",
		Table:   "snapshot_companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) (*pointerCompany, error) {
			company.ID = 7
			return company, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) error {
			deleted = append(deleted, company.ID)
			return nil
		},
	})

	result, err := seedling.NewSession[*pointerCompany](reg).InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatalf("insert snapshot company: %v", err)
	}

	// Act: the caller holds the live pointer and repoints its PK.
	result.Root().ID = 999
	if err := result.CleanupE(t.Context(), nil); err != nil {
		t.Fatalf("cleanup snapshot company: %v", err)
	}

	// Assert
	if !reflect.DeepEqual(deleted, []int{7}) {
		t.Fatalf("deleted IDs = %v, want [7]", deleted)
	}
}

func TestPointerBlueprint_PartialResultCleanupUsesInsertedValues(t *testing.T) {
	// Arrange
	reg := seedling.NewRegistry()
	nextID := 0
	var deletedCompanies []int
	insertFailure := errors.New("user insert failed")

	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerCompany]{
		Name:    "pointer_company",
		Table:   "pointer_companies",
		PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) (*pointerCompany, error) {
			nextID++
			company.ID = nextID
			return company, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, company *pointerCompany) error {
			deletedCompanies = append(deletedCompanies, company.ID)
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[*pointerUser]{
		Name:    "pointer_user",
		Table:   "pointer_users",
		PKField: "ID",
		Defaults: func() *pointerUser {
			return &pointerUser{}
		},
		Relations: []seedling.Relation{
			seedling.BelongsToRelation("company", "pointer_company", false, "CompanyID"),
		},
		Insert: func(_ context.Context, _ seedling.DBTX, _ *pointerUser) (*pointerUser, error) {
			return nil, insertFailure
		},
		Delete: func(_ context.Context, _ seedling.DBTX, _ *pointerUser) error {
			t.Fatal("delete called for a node that was never inserted")
			return nil
		},
	})

	// Act
	result, err := seedling.NewSession[*pointerUser](reg).InsertOneE(t.Context(), nil)

	// Assert
	if !errors.Is(err, insertFailure) {
		t.Fatalf("insert error = %v, want wrapped %v", err, insertFailure)
	}

	company, ok, err := seedling.NodeAs[*pointerCompany](result, "pointer_company")
	if err != nil {
		t.Fatalf("read partial company: %v", err)
	}
	if !ok {
		t.Fatal("expected inserted company in partial result")
	}
	if company.ID != 1 {
		t.Fatalf("company ID = %d, want 1", company.ID)
	}

	// The caller holds the same pointer the graph does; cleanup must still
	// delete the row as Insert returned it.
	company.ID = 99

	if err := result.CleanupE(t.Context(), nil); err != nil {
		t.Fatalf("cleanup partial result: %v", err)
	}
	if !reflect.DeepEqual(deletedCompanies, []int{1}) {
		t.Fatalf("deleted company IDs = %v, want [1]", deletedCompanies)
	}
}
