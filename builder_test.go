package seedling_test

import (
	"context"
	"fmt"
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
	var userInserts int
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() User {
			return User{Name: "test-user"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			userInserts++
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
			// The optional assignee is expanded by default through the
			// blueprint-level predicate, so only Omit can keep it out.
			{
				Name:         "assignee",
				Kind:         seedling.BelongsTo,
				LocalField:   "AssigneeUserID",
				RefBlueprint: "user",
				Optional:     true,
				When:         func(any) bool { return true },
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	sess := seedling.NewSession[Task](reg)

	// Act
	expanded := seedling.ForSession(sess).Insert(t, nil).Root()
	expandedInserts := userInserts
	omitted := seedling.ForSession(sess).
		Omit("assignee").
		Insert(t, nil).Root()

	// Assert
	if expanded.AssigneeUserID == 0 || expandedInserts != 1 {
		t.Fatalf("baseline: got AssigneeUserID %d after %d user inserts, want an inserted assignee", expanded.AssigneeUserID, expandedInserts)
	}
	if omitted.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if omitted.AssigneeUserID != 0 {
		t.Fatalf("got AssigneeUserID %d, want 0", omitted.AssigneeUserID)
	}
	if got := userInserts - expandedInserts; got != 0 {
		t.Fatalf("got %d user inserts after Omit, want 0", got)
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
	ids := seedlingtest.NewIDSequence()
	inserters := seedlingtest.DefaultBasicInserters(ids)
	var got any
	inserters.Company = func(ctx context.Context, _ seedling.DBTX, v Company) (Company, error) {
		got = ctx.Value(builderCtxKey{})
		v.ID = ids.Next()
		return v, nil
	}
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(t, reg, inserters)
	sess := seedling.NewSession[Company](reg)
	builderCtx := context.WithValue(t.Context(), builderCtxKey{}, "builder")
	callerCtx := context.WithValue(t.Context(), builderCtxKey{}, "caller")

	// Act
	result, err := seedling.ForSession(sess).
		WithContext(builderCtx).
		InsertE(callerCtx, nil)
		// Assert
	if err != nil {
		t.Fatal(err)
	}
	if result.Root().ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if got != "builder" {
		t.Fatalf("insert saw context value %v, want %v", got, "builder")
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
	want := fmt.Sprint(rand.New(rand.NewPCG(99, 99)).Uint64())
	var gotRand *rand.Rand

	// Act
	company := seedling.ForSession(sess).
		WithRand(rng).
		Generate(func(r *rand.Rand, c *Company) {
			gotRand = r
			c.Name = fmt.Sprint(r.Uint64())
		}).
		Insert(t, nil).Root()

	// Assert
	if gotRand != rng {
		t.Fatal("expected Generate to receive the RNG passed to WithRand")
	}
	if company.Name != want {
		t.Fatalf("got %v, want %v", company.Name, want)
	}
}

func TestBuilder_WithSeed(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Company](reg)
	insertWithSeed := func(seed uint64) string {
		return seedling.ForSession(sess).
			WithSeed(seed).
			Generate(func(r *rand.Rand, c *Company) {
				c.Name = fmt.Sprintf("%x-%x", r.Uint64(), r.Uint64())
			}).
			Insert(t, nil).Root().Name
	}

	// Act
	first := insertWithSeed(123)
	again := insertWithSeed(123)
	other := insertWithSeed(456)

	// Assert
	if first != again {
		t.Fatalf("same seed produced %v and %v, want identical names", first, again)
	}
	// The default RNG is also fixed, so distinct seeds must diverge to prove
	// the seed reached Generate.
	if first == other {
		t.Fatalf("seeds 123 and 456 both produced %v, want different names", first)
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
	result, err := seedling.ForSession(sess).
		Set("Name", "batchE").
		InsertManyE(t.Context(), nil, 2)
		// Assert
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Len(), 2; got != want {
		t.Fatalf("got len %d, want %d", got, want)
	}
	for i, company := range result.Roots() {
		if company.ID == 0 {
			t.Fatalf("result[%d] has zero ID", i)
		}
		if company.Name != "batchE" {
			t.Fatalf("result[%d]: got Name %v, want %v", i, company.Name, "batchE")
		}
	}
}

func TestBuilder_Build(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T, *seedling.Builder[Task]) *seedling.Plan[Task]
	}{
		{
			name: "Build",
			build: func(t *testing.T, b *seedling.Builder[Task]) *seedling.Plan[Task] {
				t.Helper()
				return b.Build(t)
			},
		},
		{
			name: "BuildE",
			build: func(t *testing.T, b *seedling.Builder[Task]) *seedling.Plan[Task] {
				t.Helper()
				plan, err := b.BuildE()
				if err != nil {
					t.Fatal(err)
				}
				return plan
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ids := seedlingtest.NewIDSequence()
			inserters := seedlingtest.DefaultBasicInserters(ids)
			var taskInserts int
			inserters.Task = func(_ context.Context, _ seedling.DBTX, v Task) (Task, error) {
				taskInserts++
				v.ID = ids.Next()
				return v, nil
			}
			reg := seedlingtest.NewRegistry()
			seedlingtest.RegisterBasic(t, reg, inserters)
			b := seedling.ForSession(seedling.NewSession[Task](reg)).
				Set("Title", "planned")

			// Act
			plan := tt.build(t, b)
			insertsAfterBuild := taskInserts
			task := plan.Insert(t, nil).Root()

			// Assert
			if insertsAfterBuild != 0 {
				t.Fatalf("got %d task inserts while building, want 0", insertsAfterBuild)
			}
			if taskInserts != 1 {
				t.Fatalf("got %d task inserts after Plan.Insert, want 1", taskInserts)
			}
			if task.Title != "planned" {
				t.Fatalf("got %v, want %v", task.Title, "planned")
			}
		})
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
	if got, want := results.Len(), 3; got != want {
		t.Fatalf("got %d results, want 3", got)
	}
	for i, c := range results.Roots() {
		if c.ID == 0 {
			t.Fatalf("result[%d] has zero ID", i)
		}
	}
}

func TestBuilder_Chaining_AppliesEveryOption(t *testing.T) {
	// Arrange
	reg := setupBuilderRegistry(t)
	sess := seedling.NewSession[Task](reg)
	assignee := User{ID: 77, Name: "existing-user"}

	// Act
	result := seedling.ForSession(sess).
		Set("Title", "chained").
		Set("Status", "done").
		Ref("project", seedling.Set("Name", "chain-proj")).
		Use("assignee", assignee).
		Insert(t, nil)
	task := result.Root()

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
	project, ok, err := seedling.NodeAs[Project](result, "project")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected project node")
	}
	if project.Name != "chain-proj" {
		t.Fatalf("got %v, want %v", project.Name, "chain-proj")
	}
	if task.ProjectID != project.ID {
		t.Fatalf("got ProjectID %v, want %v", task.ProjectID, project.ID)
	}
	if task.AssigneeUserID != assignee.ID {
		t.Fatalf("got AssigneeUserID %v, want %v", task.AssigneeUserID, assignee.ID)
	}
}
