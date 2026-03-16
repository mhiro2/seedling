package seedling_test

import (
	"context"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func setupBuilderRegistry(tb testing.TB) *seedling.Registry {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(tb, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))
	return reg
}

func setupDefaultBuilderRegistry(tb testing.TB) {
	tb.Helper()

	seedling.ResetRegistry()
	tb.Cleanup(seedling.ResetRegistry)

	ids := seedlingtest.NewIDSequence()
	seedling.MustRegister(seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
}

func TestFor_CreatesBuilderWithDefaultRegistry(t *testing.T) {
	// Arrange
	setupDefaultBuilderRegistry(t)

	// Act
	company := seedling.For[Company]().
		Set("Name", "default-builder").
		Insert(t, nil).Root()

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if company.Name != "default-builder" {
		t.Fatalf("got %v, want %v", company.Name, "default-builder")
	}
}

func TestForSession_CreatesBuilderWithCustomSession(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		Set("Name", "builder-company").
		Insert(t, nil).Root()

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if company.Name != "builder-company" {
		t.Fatalf("got %v, want %v", company.Name, "builder-company")
	}
}

func TestBuilder_Set(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)

	// Act
	task := seedling.ForSession(sess).
		Set("Title", "builder-title").
		Set("Status", "closed").
		Insert(t, nil).Root()

	// Assert
	if task.Title != "builder-title" {
		t.Fatalf("got %v, want %v", task.Title, "builder-title")
	}
	if task.Status != "closed" {
		t.Fatalf("got %v, want %v", task.Status, "closed")
	}
}

func TestBuilder_Use(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[User](reg)
	existing := Company{ID: 42, Name: "existing"}

	// Act
	user := seedling.ForSession(sess).
		Use("company", existing).
		Insert(t, nil).Root()

	// Assert
	if user.CompanyID != 42 {
		t.Fatalf("got %v, want %v", user.CompanyID, 42)
	}
}

func TestBuilder_Ref(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)

	// Act
	plan := seedling.ForSession(sess).
		Ref("project", seedling.Set("Name", "ref-project")).
		Build(t)

	result := plan.Insert(t, nil)
	task := result.Root()

	// Assert
	if task.ProjectID == 0 {
		t.Fatal("expected non-zero ProjectID")
	}
	project, ok, err := seedling.NodeAs[Project](result, "project")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true")
	}
	if project.Name != "ref-project" {
		t.Fatalf("got %v, want %v", project.Name, "ref-project")
	}
}

func TestBuilder_Omit(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() User {
			return User{Name: "test-user"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Project]{
		Name:    "project",
		Table:   "projects",
		PKField: "ID",
		Defaults: func() Project {
			return Project{Name: "test-project"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Task]{
		Name:    "task",
		Table:   "tasks",
		PKField: "ID",
		Defaults: func() Task {
			return Task{Title: "test-task", Status: "open"}
		},
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user", Optional: true},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	// Act
	task := seedling.ForSession(seedling.NewSession[Task](reg)).
		Omit("assignee").
		Insert(t, nil).Root()

	// Assert
	if task.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if task.AssigneeUserID != 0 {
		t.Fatal("expected zero AssigneeUserID")
	}
}

func TestBuilder_With(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		With(func(c *Company) { c.Name = "mutated" }).
		Insert(t, nil).Root()

	// Assert
	if company.Name != "mutated" {
		t.Fatalf("got %v, want %v", company.Name, "mutated")
	}
}

func TestBuilder_Trait(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:     "company",
		Table:    "companies",
		PKField:  "ID",
		Defaults: func() Company { return Company{Name: "default"} },
		Traits: map[string][]seedling.Option{
			"big": {seedling.Set("Name", "big-corp")},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	// Act
	company := seedling.ForSession(seedling.NewSession[Company](reg)).
		BlueprintTrait("big").
		Insert(t, nil).Root()

	// Assert
	if company.Name != "big-corp" {
		t.Fatalf("got %v, want %v", company.Name, "big-corp")
	}
}

func TestBuilder_InlineTrait(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		InlineTrait(seedling.Set("Name", "inline-corp")).
		Insert(t, nil).Root()

	// Assert
	if company.Name != "inline-corp" {
		t.Fatalf("got %v, want %v", company.Name, "inline-corp")
	}
}

func TestBuilder_Generate(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		Generate(func(r *rand.Rand, c *Company) {
			c.Name = "generated"
		}).
		WithSeed(42).
		Insert(t, nil).Root()

	// Assert
	if company.Name != "generated" {
		t.Fatalf("got %v, want %v", company.Name, "generated")
	}
}

func TestBuilder_GenerateE(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		GenerateE(func(r *rand.Rand, c *Company) error {
			c.Name = "generated-e"
			return nil
		}).
		WithSeed(42).
		Insert(t, nil).Root()

	// Assert
	if company.Name != "generated-e" {
		t.Fatalf("got %v, want %v", company.Name, "generated-e")
	}
}

func TestBuilder_WithContext(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)
	ctx := context.WithValue(t.Context(), builderCtxKey{}, "test-val")

	// Act
	result, err := seedling.ForSession(sess).
		WithContext(ctx).
		InsertE(ctx, nil)
		// Assert
	if err != nil {
		t.Fatal(err)
	}
	company := result.Root()
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
}

type builderCtxKey struct{}

func TestBuilder_AfterInsert(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)
	var called bool

	// Act
	seedling.ForSession(sess).
		AfterInsert(func(c Company, db seedling.DBTX) {
			called = true
		}).
		Insert(t, nil)

	// Assert
	if !called {
		t.Fatal("expected true")
	}
}

func TestBuilder_AfterInsertE(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)
	var called bool

	// Act
	seedling.ForSession(sess).
		AfterInsertE(func(c Company, db seedling.DBTX) error {
			called = true
			return nil
		}).
		Insert(t, nil)

	// Assert
	if !called {
		t.Fatal("expected true")
	}
}

func TestBuilder_WithRand(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)
	rng := rand.New(rand.NewPCG(99, 99))

	// Act
	company := seedling.ForSession(sess).
		WithRand(rng).
		Generate(func(r *rand.Rand, c *Company) {
			c.Name = "with-rand"
		}).
		Insert(t, nil).Root()

	// Assert
	if company.Name != "with-rand" {
		t.Fatalf("got %v, want %v", company.Name, "with-rand")
	}
}

func TestBuilder_WithSeed(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		WithSeed(123).
		Generate(func(r *rand.Rand, c *Company) {
			c.Name = "with-seed"
		}).
		Insert(t, nil).Root()

	// Assert
	if company.Name != "with-seed" {
		t.Fatalf("got %v, want %v", company.Name, "with-seed")
	}
}

func TestBuilder_Apply(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)

	// Act
	task := seedling.ForSession(sess).
		Apply(
			seedling.Set("Title", "applied"),
			seedling.Set("Status", "done"),
		).
		Insert(t, nil).Root()

	// Assert
	if task.Title != "applied" {
		t.Fatalf("got %v, want %v", task.Title, "applied")
	}
	if task.Status != "done" {
		t.Fatalf("got %v, want %v", task.Status, "done")
	}
}

func TestBuilder_Insert(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	company := seedling.ForSession(sess).
		Set("Name", "insert").
		Insert(t, nil).Root()

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if company.Name != "insert" {
		t.Fatalf("got %v, want %v", company.Name, "insert")
	}
}

func TestBuilder_InsertE(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	result, err := seedling.ForSession(sess).
		Set("Name", "insertE").
		InsertE(t.Context(), nil)
		// Assert
	if err != nil {
		t.Fatal(err)
	}
	company := result.Root()
	if company.Name != "insertE" {
		t.Fatalf("got %v, want %v", company.Name, "insertE")
	}
}

func TestBuilder_InsertMany(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	companies := seedling.ForSession(sess).
		Set("Name", "batch").
		InsertMany(t, nil, 3)

	// Assert
	if len(companies) != 3 {
		t.Fatalf("got len %d, want %d", len(companies), 3)
	}
	names := make([]string, 0, len(companies))
	for _, c := range companies {
		names = append(names, c.Name)
	}

	want := []string{"batch", "batch", "batch"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", names, want)
	}
}

func TestBuilder_InsertManyE(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)

	// Act
	companies, err := seedling.ForSession(sess).
		Set("Name", "batchE").
		InsertManyE(t.Context(), nil, 2)
		// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(companies) != 2 {
		t.Fatalf("got len %d, want %d", len(companies), 2)
	}
}

func TestBuilder_Build(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)

	// Act
	plan := seedling.ForSession(sess).
		Set("Title", "built").
		Build(t)

	// Assert
	if plan == nil {
		t.Fatal("expected non-nil")
	}
	if plan.DebugString() == "" {
		t.Fatal("expected non-empty DebugString")
	}
}

func TestBuilder_BuildE(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)

	// Act
	plan, err := seedling.ForSession(sess).
		Set("Title", "planned").
		BuildE()
		// Assert
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("expected non-nil")
	}
}

func TestPackageLevelBuildE(t *testing.T) {
	// Arrange
	setupDefaultBuilderRegistry(t)

	// Act
	plan, err := seedling.BuildE[Company]()
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
}

func TestPackageLevelInsertManyE(t *testing.T) {
	// Arrange
	setupDefaultBuilderRegistry(t)

	// Act
	results, err := seedling.InsertManyE[Company](t.Context(), nil, 3)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for i, c := range results {
		if c.ID == 0 {
			t.Fatalf("result[%d] has zero ID", i)
		}
	}
}

func TestBuilder_Chaining_MultipleMethodCalls(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)

	// Act
	task := seedling.ForSession(sess).
		Set("Title", "chained").
		Set("Status", "done").
		Ref("project", seedling.Set("Name", "chain-proj")).
		Insert(t, nil).Root()

	// Assert
	if task.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if task.Title != "chained" {
		t.Fatalf("got %v, want %v", task.Title, "chained")
	}
	if task.Status != "done" {
		t.Fatalf("got %v, want %v", task.Status, "done")
	}
	if task.ProjectID == 0 {
		t.Fatal("expected non-zero ProjectID")
	}
}
