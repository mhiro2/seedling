package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mhiro2/seedling"
)

// Models for deep nesting tests
type L1 struct {
	ID   int
	Name string
}

type L2 struct {
	ID   int
	L1ID int
	Name string
}

type L3 struct {
	ID   int
	L2ID int
	Name string
}

type L4 struct {
	ID   int
	L3ID int
	Name string
}

type L5 struct {
	ID   int
	L4ID int
	Name string
}

// Model for nil defaults test
type Widget struct {
	ID   int
	Name string
}

// Model for zero PK test
type ZeroPK struct {
	ID   int
	Name string
}

type customError struct {
	Code    int
	Message string
}

func (e *customError) Error() string {
	return fmt.Sprintf("code=%d: %s", e.Code, e.Message)
}

func TestRefAndOmitCombination(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	// When both Ref and Omit are specified for a required relation,
	// it should now return an error because Omit on a required relation is invalid.
	_, err := buildE[Task](t,
		seedling.Ref("assignee", seedling.Set("Name", "x")),
		seedling.Omit("assignee"),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error for Omit on required relation")
	}
}

func TestUseAndRefConflict(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	company := insertOne[Company](t, nil)

	// Act
	_, err := buildE[User](t,
		seedling.Use("company", company),
		seedling.Ref("company", seedling.Set("Name", "other")),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error for Use+Ref on same relation")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestOmitRequiredRelation(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	_, err := buildE[User](t,
		seedling.Omit("company"),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error for Omit on required relation")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestSetOnFKField(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	_, err := buildE[User](t,
		seedling.Set("CompanyID", 999),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error for Set on FK field")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestDeepNesting(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	seedling.MustRegister(seedling.Blueprint[L1]{
		Name:    "l1",
		Table:   "l1s",
		PKField: "ID",
		Defaults: func() L1 {
			return L1{Name: "level1"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v L1) (L1, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[L2]{
		Name:    "l2",
		Table:   "l2s",
		PKField: "ID",
		Defaults: func() L2 {
			return L2{Name: "level2"}
		},
		Relations: []seedling.Relation{
			{Name: "l1", Kind: seedling.BelongsTo, LocalField: "L1ID", RefBlueprint: "l1"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v L2) (L2, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[L3]{
		Name:    "l3",
		Table:   "l3s",
		PKField: "ID",
		Defaults: func() L3 {
			return L3{Name: "level3"}
		},
		Relations: []seedling.Relation{
			{Name: "l2", Kind: seedling.BelongsTo, LocalField: "L2ID", RefBlueprint: "l2"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v L3) (L3, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[L4]{
		Name:    "l4",
		Table:   "l4s",
		PKField: "ID",
		Defaults: func() L4 {
			return L4{Name: "level4"}
		},
		Relations: []seedling.Relation{
			{Name: "l3", Kind: seedling.BelongsTo, LocalField: "L3ID", RefBlueprint: "l3"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v L4) (L4, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[L5]{
		Name:    "l5",
		Table:   "l5s",
		PKField: "ID",
		Defaults: func() L5 {
			return L5{Name: "level5"}
		},
		Relations: []seedling.Relation{
			{Name: "l4", Kind: seedling.BelongsTo, LocalField: "L4ID", RefBlueprint: "l4"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v L5) (L5, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	// Act
	l5 := seedling.InsertOne[L5](t, nil).Root()

	// Assert
	if l5.ID == 0 {
		t.Fatal("expected L5 ID to be set")
	}
	if l5.L4ID == 0 {
		t.Fatal("expected L5.L4ID to be set (deep nesting should resolve all 5 levels)")
	}
}

func TestNilDefaults(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	seedling.MustRegister(seedling.Blueprint[Widget]{
		Name:    "widget",
		Table:   "widgets",
		PKField: "ID",
		// Defaults is nil — should still work
		Insert: func(ctx context.Context, db seedling.DBTX, v Widget) (Widget, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	// Act
	widget := seedling.InsertOne[Widget](t, nil).Root()

	// Assert
	if widget.ID == 0 {
		t.Fatal("expected Widget ID to be set even with nil Defaults")
	}
	// Name should be the zero value since no defaults were provided
	if widget.Name != "" {
		t.Fatalf("expected empty Name with nil Defaults, got %v", widget.Name)
	}
}

func TestZeroPK(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	seedling.MustRegister(seedling.Blueprint[ZeroPK]{
		Name:    "zeropk",
		Table:   "zeropks",
		PKField: "ID",
		Defaults: func() ZeroPK {
			return ZeroPK{Name: "zero"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ZeroPK) (ZeroPK, error) {
			// Intentionally return zero PK — should not crash
			return v, nil
		},
	})

	// Act
	zpk := seedling.InsertOne[ZeroPK](t, nil).Root()

	// Assert
	if zpk.ID != 0 {
		t.Fatalf("got %v, want %v", zpk.ID, 0)
	}
	if zpk.Name != "zero" {
		t.Fatalf("got %v, want %v", zpk.Name, "zero")
	}
}

func TestMustNodePanicsOnNonexistent(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Task](t)
	result := plan.Insert(t, nil)

	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected MustNode to panic for nonexistent node")
		}
	}()
	result.MustNode("nonexistent")
}

func TestMustNodeAs_ReturnsTypedValue(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Task](t)
	result := plan.Insert(t, nil)

	// Act
	company := seedling.MustNodeAs[Company](result, "company")

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero company ID")
	}
	if company.Name == "" {
		t.Fatal("expected non-empty company Name")
	}
}

func TestMustNodeAs_PanicsOnMissing(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Task](t)
	result := plan.Insert(t, nil)

	// Act & Assert
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected MustNodeAs to panic for nonexistent node")
		}
	}()
	seedling.MustNodeAs[Company](result, "nonexistent")
}

func TestUse_WrongType(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	_, err := buildE[User](t,
		seedling.Use("company", "not-a-company"),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error when Use() receives wrong type")
	}
	if !errors.Is(err, seedling.ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, seedling.ErrTypeMismatch)
	}
}

func TestUse_WrongStructType(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	wrongUser := User{ID: 1, Name: "wrong"}

	// Act
	_, err := buildE[User](t,
		seedling.Use("company", wrongUser),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error when Use() receives wrong struct type")
	}
	if !errors.Is(err, seedling.ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, seedling.ErrTypeMismatch)
	}
}

func TestUse_Nil(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	_, err := buildE[User](t,
		seedling.Use("company", nil),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error when Use() receives nil")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestUse_PointerValueMatch(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	company := insertOne[Company](t, nil)

	// Act
	plan, err := buildE[User](t,
		seedling.Use("company", &company),
	)
	if err != nil {
		t.Fatal("expected pointer-to-value Use to succeed:", err)
	}

	result, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatal("expected insert to succeed:", err)
	}

	// Assert
	user := result.Root()
	if user.CompanyID != company.ID {
		t.Fatalf("got %v, want %v", user.CompanyID, company.ID)
	}
}

func TestErrInsertFailed_ErrorsIs(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	dbErr := fmt.Errorf("connection refused")
	seedling.MustRegister(seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "fail-test"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			return Company{}, dbErr
		},
	})

	// Act
	_, err := seedling.InsertOneE[Company](t.Context(), nil)

	// Assert
	if err == nil {
		t.Fatal("expected error from failing Insert")
	}
	if !errors.Is(err, seedling.ErrInsertFailed) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInsertFailed)
	}
}

func TestErrInsertFailed_WrapsOriginal(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	originalErr := fmt.Errorf("unique constraint violation")
	seedling.MustRegister(seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "fail-test"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			return Company{}, originalErr
		},
	})

	// Act
	_, err := seedling.InsertOneE[Company](t.Context(), nil)

	// Assert
	if err == nil {
		t.Fatal("expected error from failing Insert")
	}
	if !errors.Is(err, originalErr) {
		t.Fatalf("got %v, want %v", err, originalErr)
	}

	var ife *seedling.InsertFailedError
	if !errors.As(err, &ife) {
		t.Fatal("errors.As should match *InsertFailedError")
	}
	if ife.Blueprint() != "company" {
		t.Fatalf("got %q, want %q", ife.Blueprint(), "company")
	}
}

func TestErrInsertFailed_ErrorsAs(t *testing.T) {
	// Arrange
	seedling.ResetRegistry()

	seedling.MustRegister(seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "fail-test"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			return Company{}, &customError{Code: 42, Message: "test error"}
		},
	})

	// Act
	_, err := seedling.InsertOneE[Company](t.Context(), nil)

	// Assert
	if err == nil {
		t.Fatal("expected error from failing Insert")
	}
	var ce *customError
	if !errors.As(err, &ce) {
		t.Fatal("expected errors.As to extract customError")
	}
	if ce.Code != 42 {
		t.Errorf("got %v, want %v", ce.Code, 42)
	}
}

func TestNodeWithMultipleSameBlueprintNodes(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Building a Task creates:
	// - project (which depends on company)
	// - assignee/user (which depends on company)
	// So there are 2 company nodes in the graph.
	plan := build[Task](t)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("expected to find a 'company' node in result")
	}

	company := companyNode.Value().(Company)
	if company.ID == 0 {
		t.Fatal("expected company ID to be set")
	}
	if companyNode.Name() != "company" {
		t.Errorf("got %v, want %v", companyNode.Name(), "company")
	}
}
