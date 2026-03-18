package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

type (
	Company        = seedlingtest.Company
	User           = seedlingtest.User
	Project        = seedlingtest.Project
	Task           = seedlingtest.Task
	Department     = seedlingtest.Department
	Employee       = seedlingtest.Employee
	Region         = seedlingtest.Region
	Deployment     = seedlingtest.Deployment
	Article        = seedlingtest.Article
	Tag            = seedlingtest.Tag
	ArticleTag     = seedlingtest.ArticleTag
	PtrModel       = seedlingtest.PtrModel
	InterfaceModel = seedlingtest.InterfaceModel
)

var (
	testRegistries sync.Map // map[testing.TB]*seedling.Registry
	sharedIDs      = seedlingtest.NewIDSequence()
)

func nextID() int {
	return sharedIDs.Next()
}

func useTestRegistry(tb testing.TB, reg *seedling.Registry) {
	tb.Helper()
	testRegistries.Store(tb, reg)
	tb.Cleanup(func() {
		testRegistries.Delete(tb)
	})
}

func getTestRegistry(tb testing.TB) *seedling.Registry {
	tb.Helper()
	v, ok := testRegistries.Load(tb)
	if !ok {
		tb.Fatal("test registry not set; call useTestRegistry first")
	}
	return v.(*seedling.Registry)
}

func session[T any](tb testing.TB) seedling.Session[T] {
	tb.Helper()
	return seedling.NewSession[T](getTestRegistry(tb))
}

func insertOne[T any](tb testing.TB, db seedling.DBTX, opts ...seedling.Option) T {
	tb.Helper()
	return session[T](tb).InsertOne(tb, db, opts...).Root()
}

func insertMany[T any](tb testing.TB, db seedling.DBTX, n int, opts ...seedling.Option) []T {
	tb.Helper()
	return session[T](tb).InsertMany(tb, db, n, opts...)
}

func insertManyE[T any](ctx context.Context, tb testing.TB, db seedling.DBTX, n int, opts ...seedling.Option) (seedling.BatchResult[T], error) {
	tb.Helper()
	result, err := session[T](tb).InsertManyE(ctx, db, n, opts...)
	if err != nil {
		var zero seedling.BatchResult[T]
		return zero, fmt.Errorf("insert many test helper: %w", err)
	}
	return result, nil
}

func build[T any](tb testing.TB, opts ...seedling.Option) *seedling.Plan[T] {
	tb.Helper()
	return session[T](tb).Build(tb, opts...)
}

func buildE[T any](tb testing.TB, opts ...seedling.Option) (*seedling.Plan[T], error) {
	tb.Helper()
	plan, err := session[T](tb).BuildE(opts...)
	if err != nil {
		return nil, fmt.Errorf("build test helper: %w", err)
	}
	return plan, nil
}

func setupBlueprints(tb testing.TB) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(tb, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))
	useTestRegistry(tb, reg)
}

func setupHasManyBlueprints(tb testing.TB) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterHasMany(tb, reg, seedlingtest.DefaultHasManyInserters(seedlingtest.NewIDSequence()))
	useTestRegistry(tb, reg)
}

func setupCompositePKBlueprints(tb testing.TB) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterCompositePK(tb, reg, seedlingtest.DefaultCompositePKInserters(seedlingtest.NewIDSequence()))
	useTestRegistry(tb, reg)
}

func setupManyToManyBlueprints(tb testing.TB) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterManyToMany(tb, reg, seedlingtest.DefaultManyToManyInserters(seedlingtest.NewIDSequence()))
	useTestRegistry(tb, reg)
}

func setupOptionalBelongsToBlueprints(tb testing.TB) {
	tb.Helper()
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
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company", Optional: true},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	useTestRegistry(tb, reg)
}

func setupOptionalHasManyBlueprints(tb testing.TB) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()

	seedling.MustRegisterTo(reg, seedling.Blueprint[Department]{
		Name:    "department",
		Table:   "departments",
		PKField: "ID",
		Defaults: func() Department {
			return Department{Name: "engineering"}
		},
		Relations: []seedling.Relation{
			{Name: "employees", Kind: seedling.HasMany, LocalField: "DepartmentID", RefBlueprint: "employee", Count: 2, Optional: true},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Department) (Department, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[Employee]{
		Name:    "employee",
		Table:   "employees",
		PKField: "ID",
		Defaults: func() Employee {
			return Employee{Name: "employee"}
		},
		Relations: []seedling.Relation{
			{Name: "department", Kind: seedling.BelongsTo, LocalField: "DepartmentID", RefBlueprint: "department"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Employee) (Employee, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	useTestRegistry(tb, reg)
}

func setupOptionalManyToManyBlueprints(tb testing.TB) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()

	seedling.MustRegisterTo(reg, seedling.Blueprint[Article]{
		Name:    "article",
		Table:   "articles",
		PKField: "ID",
		Defaults: func() Article {
			return Article{Title: "seedling"}
		},
		Relations: []seedling.Relation{
			{
				Name:             "tags",
				Kind:             seedling.ManyToMany,
				LocalField:       "ArticleID",
				RemoteField:      "TagID",
				RefBlueprint:     "tag",
				ThroughBlueprint: "article_tag",
				Count:            2,
				Optional:         true,
			},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Article) (Article, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[Tag]{
		Name:    "tag",
		Table:   "tags",
		PKField: "ID",
		Defaults: func() Tag {
			return Tag{Name: "tag"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Tag) (Tag, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[ArticleTag]{
		Name:     "article_tag",
		Table:    "article_tags",
		PKFields: []string{"ArticleID", "TagID"},
		Defaults: func() ArticleTag {
			return ArticleTag{}
		},
		Relations: []seedling.Relation{
			{Name: "article", Kind: seedling.BelongsTo, LocalField: "ArticleID", RefBlueprint: "article"},
			{Name: "tag", Kind: seedling.BelongsTo, LocalField: "TagID", RefBlueprint: "tag"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v ArticleTag) (ArticleTag, error) {
			return v, nil
		},
	})

	useTestRegistry(tb, reg)
}

func TestInsertOne_Simple(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	company := insertOne[Company](t, nil)

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if company.Name != "test-company" {
		t.Fatalf("got %v, want %v", company.Name, "test-company")
	}
}

func TestInsertOne_WithDependencies(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	task := insertOne[Task](t, nil)

	// Assert
	if task.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if task.ProjectID == 0 {
		t.Fatal("expected non-zero ProjectID")
	}
	if task.AssigneeUserID == 0 {
		t.Fatal("expected non-zero AssigneeUserID")
	}
	if task.Title != "test-task" {
		t.Fatalf("got %v, want %v", task.Title, "test-task")
	}
}

func TestInsertOne_BelongsToDefaultsToRequired(t *testing.T) {
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
	useTestRegistry(t, reg)

	// Act
	user := insertOne[User](t, nil)

	// Assert
	if user.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if user.CompanyID == 0 {
		t.Fatal("expected non-zero CompanyID")
	}
}

func TestInsertOne_Set(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	task := insertOne[Task](t, nil,
		seedling.Set("Title", "custom title"),
		seedling.Set("Status", "closed"),
	)

	// Assert
	if task.Title != "custom title" {
		t.Fatalf("got %v, want %v", task.Title, "custom title")
	}
	if task.Status != "closed" {
		t.Fatalf("got %v, want %v", task.Status, "closed")
	}
}

func TestInsertOne_Ref(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	plan := build[Task](t,
		seedling.Ref("project",
			seedling.Set("Name", "custom-project"),
		),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	task := result.Root()
	if task.ProjectID == 0 {
		t.Fatal("expected non-zero ProjectID")
	}

	project, ok, err := seedling.NodeAs[Project](result, "project")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected project node in result")
	}
	if project.Name != "custom-project" {
		t.Fatalf("got %v, want %v", project.Name, "custom-project")
	}
}

func TestRef_OptionalBelongsToExpandsRelation(t *testing.T) {
	// Arrange
	setupOptionalBelongsToBlueprints(t)
	plan := build[User](t,
		seedling.Ref("company", seedling.Set("Name", "custom-company")),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	user := result.Root()
	if user.CompanyID == 0 {
		t.Fatal("expected non-zero CompanyID")
	}
	company, ok, err := seedling.NodeAs[Company](result, "company")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected company node in result")
	}
	if company.Name != "custom-company" {
		t.Fatalf("got %v, want %v", company.Name, "custom-company")
	}
}

func TestInsertMany(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	tasks := insertMany[Task](t, nil, 3,
		seedling.Set("Status", "open"),
	)

	// Assert
	if len(tasks) != 3 {
		t.Fatalf("got len %d, want %d", len(tasks), 3)
	}
	for i, task := range tasks {
		if task.ID == 0 {
			t.Fatalf("task[%d] ID should be set", i)
		}
		if task.Status != "open" {
			t.Fatalf("task[%d] Status should be open, got %v", i, task.Status)
		}
	}
}

func TestInsertMany_BatchSharesBelongsToDependencies(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	inserters := seedlingtest.DefaultBasicInserters(ids)
	var companyCount int
	var userCount int
	var projectCount int
	var taskCount int

	inserters.Company = func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
		companyCount++
		v.ID = ids.Next()
		return v, nil
	}
	inserters.User = func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
		userCount++
		v.ID = ids.Next()
		return v, nil
	}
	inserters.Project = func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
		projectCount++
		v.ID = ids.Next()
		return v, nil
	}
	inserters.Task = func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
		taskCount++
		v.ID = ids.Next()
		return v, nil
	}

	seedlingtest.RegisterBasic(t, reg, inserters)
	useTestRegistry(t, reg)

	// Act
	tasks := insertMany[Task](t, nil, 3)

	// Assert
	if len(tasks) != 3 {
		t.Fatalf("got len %d, want %d", len(tasks), 3)
	}
	if companyCount != 2 {
		t.Fatalf("got %v, want %v", companyCount, 2)
	}
	if userCount != 1 {
		t.Fatalf("got %v, want %v", userCount, 1)
	}
	if projectCount != 1 {
		t.Fatalf("got %v, want %v", projectCount, 1)
	}
	if taskCount != 3 {
		t.Fatalf("got %v, want %v", taskCount, 3)
	}
	if tasks[0].ProjectID != tasks[1].ProjectID {
		t.Fatalf("got %v, want %v", tasks[0].ProjectID, tasks[1].ProjectID)
	}
	if tasks[1].ProjectID != tasks[2].ProjectID {
		t.Fatalf("got %v, want %v", tasks[1].ProjectID, tasks[2].ProjectID)
	}
	if tasks[0].AssigneeUserID != tasks[1].AssigneeUserID {
		t.Fatalf("got %v, want %v", tasks[0].AssigneeUserID, tasks[1].AssigneeUserID)
	}
	if tasks[1].AssigneeUserID != tasks[2].AssigneeUserID {
		t.Fatalf("got %v, want %v", tasks[1].AssigneeUserID, tasks[2].AssigneeUserID)
	}
}

func TestInsertMany_BatchSharesStaticRefDependencies(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	inserters := seedlingtest.DefaultBasicInserters(ids)
	var projectCount int
	var projectNames []string

	inserters.Project = func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
		projectCount++
		projectNames = append(projectNames, v.Name)
		v.ID = ids.Next()
		return v, nil
	}

	seedlingtest.RegisterBasic(t, reg, inserters)
	useTestRegistry(t, reg)

	// Act
	tasks := insertMany[Task](t, nil, 2,
		seedling.Ref("project", seedling.Set("Name", "shared-project")),
	)

	// Assert
	if len(tasks) != 2 {
		t.Fatalf("got len %d, want %d", len(tasks), 2)
	}
	if projectCount != 1 {
		t.Fatalf("got %v, want %v", projectCount, 1)
	}
	if !reflect.DeepEqual(projectNames, []string{"shared-project"}) {
		t.Fatalf("got %v, want %v", projectNames, []string{"shared-project"})
	}
	if tasks[0].ProjectID != tasks[1].ProjectID {
		t.Fatalf("got %v, want %v", tasks[0].ProjectID, tasks[1].ProjectID)
	}
}

func TestInsertMany_BatchDoesNotShareSequenceSpecificDependencies(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	inserters := seedlingtest.DefaultBasicInserters(ids)
	var projectCount int

	inserters.Project = func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
		projectCount++
		v.ID = ids.Next()
		return v, nil
	}

	seedlingtest.RegisterBasic(t, reg, inserters)
	useTestRegistry(t, reg)

	// Act
	tasks := insertMany[Task](t, nil, 2,
		seedling.SeqRef("project", func(i int) []seedling.Option {
			return []seedling.Option{seedling.Set("Name", fmt.Sprintf("project-%d", i))}
		}),
	)

	// Assert
	if len(tasks) != 2 {
		t.Fatalf("got len %d, want %d", len(tasks), 2)
	}
	if projectCount != 2 {
		t.Fatalf("got %v, want %v", projectCount, 2)
	}
	if tasks[0].ProjectID == tasks[1].ProjectID {
		t.Fatalf("expected different ProjectIDs, got %v and %v", tasks[0].ProjectID, tasks[1].ProjectID)
	}
}

func TestBuild_DebugString(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	plan := build[Task](t)
	out := plan.DebugString()
	t.Log(out)

	// Assert
	if out == "" {
		t.Fatal("expected non-empty DebugString")
	}
}

func TestPlan_InsertE_ReusesOriginalNodeValues(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	var inserted []Company

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			inserted = append(inserted, v)
			v.ID = ids.Next()
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	plan := build[Company](t)

	// Act
	first, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	second, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if len(inserted) != 2 {
		t.Fatalf("got len %d, want %d", len(inserted), 2)
	}
	want := Company{Name: "test-company"}
	if !reflect.DeepEqual(inserted[0], want) {
		t.Fatalf("got %v, want %v", inserted[0], want)
	}
	if !reflect.DeepEqual(inserted[1], want) {
		t.Fatalf("got %v, want %v", inserted[1], want)
	}
	if first.Root().ID == second.Root().ID {
		t.Fatalf("expected different IDs, got %v and %v", first.Root().ID, second.Root().ID)
	}
}

func TestInsertOne_With(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	task := insertOne[Task](t, nil,
		seedling.With(func(tk *Task) {
			tk.Title = "with-title"
			tk.Status = "in_progress"
		}),
	)

	// Assert
	if task.Title != "with-title" {
		t.Fatalf("got %v, want %v", task.Title, "with-title")
	}
	if task.Status != "in_progress" {
		t.Fatalf("got %v, want %v", task.Status, "in_progress")
	}
}

func TestInsertOne_WithContext(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	ctx := t.Context()

	// Act
	company := insertOne[Company](t, nil,
		seedling.WithContext(ctx),
	)

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
}

func TestInsertMany_WithContext(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	companies := insertMany[Company](t, nil, 2,
		seedling.WithContext(t.Context()),
	)

	// Assert
	if len(companies) != 2 {
		t.Fatalf("got len %d, want %d", len(companies), 2)
	}
}

func TestInsertOne_UsesTestingContextByDefault(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	wantCtx := t.Context()
	var sameCtx bool
	inserters := seedlingtest.DefaultBasicInserters(ids)
	inserters.Company = func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
		sameCtx = ctx == wantCtx
		v.ID = ids.Next()
		return v, nil
	}
	seedlingtest.RegisterBasic(t, reg, inserters)
	useTestRegistry(t, reg)

	// Act
	company := insertOne[Company](t, nil)

	// Assert
	if company.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if !sameCtx {
		t.Fatal("expected true")
	}
}

func TestPlan_Insert_UsesTestingContextByDefault(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	wantCtx := t.Context()
	var sameCtx bool
	inserters := seedlingtest.DefaultBasicInserters(ids)
	inserters.Company = func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
		sameCtx = ctx == wantCtx
		v.ID = ids.Next()
		return v, nil
	}
	seedlingtest.RegisterBasic(t, reg, inserters)
	useTestRegistry(t, reg)

	plan := build[Company](t)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	if result.Root().ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if !sameCtx {
		t.Fatal("expected true")
	}
}

func TestBuildE_RejectsSequenceOptionsOutsideInsertMany(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	tests := []struct {
		name string
		act  func() error
	}{
		{
			name: "seq",
			act: func() error {
				_, err := buildE[Company](t, seedling.Seq("Name", func(i int) string {
					return fmt.Sprintf("company-%d", i)
				}))
				return err
			},
		},
		{
			name: "seq_ref",
			act: func() error {
				_, err := buildE[Task](t, seedling.SeqRef("project", func(i int) []seedling.Option {
					return []seedling.Option{seedling.Set("Name", fmt.Sprintf("project-%d", i))}
				}))
				return err
			},
		},
		{
			name: "seq_use",
			act: func() error {
				_, err := buildE[Task](t, seedling.SeqUse("project", func(i int) Project {
					return Project{ID: i + 1}
				}))
				return err
			},
		},
		{
			name: "nested_ref_seq",
			act: func() error {
				_, err := buildE[Task](t, seedling.Ref("project", seedling.Seq("Name", func(i int) string {
					return fmt.Sprintf("project-%d", i)
				})))
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act & Assert
			err := tt.act()

			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, seedling.ErrInvalidOption) {
				t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
			}
			if !strings.Contains(err.Error(), "InsertMany") {
				t.Fatalf("expected error containing %q, got %v", "InsertMany", err)
			}
		})
	}
}

func TestInsertMany_ResolvesNestedSequenceOptions(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	var projectNames []string
	inserters := seedlingtest.DefaultBasicInserters(ids)
	inserters.Project = func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
		projectNames = append(projectNames, v.Name)
		v.ID = ids.Next()
		return v, nil
	}
	seedlingtest.RegisterBasic(t, reg, inserters)
	useTestRegistry(t, reg)

	// Act
	tasks := insertMany[Task](t, nil, 2,
		seedling.Ref("project", seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("project-%d", i)
		})),
	)

	// Assert
	if len(tasks) != 2 {
		t.Fatalf("got len %d, want %d", len(tasks), 2)
	}
	if !reflect.DeepEqual(projectNames, []string{"project-0", "project-1"}) {
		t.Fatalf("got %v, want %v", projectNames, []string{"project-0", "project-1"})
	}
}

func TestInsertMany_Seq(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	companies := insertMany[Company](t, nil, 3,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("company-%d", i)
		}),
	)

	// Assert
	if len(companies) != 3 {
		t.Fatalf("got len %d, want %d", len(companies), 3)
	}
	for i, c := range companies {
		expected := fmt.Sprintf("company-%d", i)
		if c.Name != expected {
			t.Fatalf("companies[%d].Name: got %v, want %v", i, c.Name, expected)
		}
	}
}

func TestInsertMany_SeqWithDependencies(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	users := insertMany[User](t, nil, 2,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("user-%d", i)
		}),
	)

	// Assert
	if users[0].Name != "user-0" {
		t.Fatalf("got %v, want %v", users[0].Name, "user-0")
	}
	if users[1].Name != "user-1" {
		t.Fatalf("got %v, want %v", users[1].Name, "user-1")
	}
}

func TestBuild_DebugStringWithSetFields(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	plan := build[Task](t,
		seedling.Set("Title", "custom"),
		seedling.Ref("project", seedling.Set("Name", "proj")),
	)
	out := plan.DebugString()
	t.Log(out)

	// Assert
	if !strings.Contains(out, "Set:") {
		t.Fatal("expected output to contain \"Set:\"")
	}
}

func TestResult_DebugString(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Task](t)

	// Act
	result := plan.Insert(t, nil)
	out := result.DebugString()
	t.Log(out)

	// Assert
	if !strings.Contains(out, "[inserted]") {
		t.Fatal("expected output to contain \"[inserted]\"")
	}
	if !strings.Contains(out, "ID=") {
		t.Fatal("expected output to contain \"ID=\"")
	}
}

func TestResult_DebugStringWithProvided(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	company := insertOne[Company](t, nil)
	plan := build[User](t,
		seedling.Use("company", company),
	)

	// Act
	result := plan.Insert(t, nil)
	out := result.DebugString()
	t.Log(out)

	// Assert
	if !strings.Contains(out, "[provided]") {
		t.Fatal("expected output to contain \"[provided]\"")
	}
}

func TestAfterInsert(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	var called bool
	var capturedCompany Company

	// Act
	company := insertOne[Company](t, nil,
		seedling.Set("Name", "after-insert-co"),
		seedling.AfterInsert(func(c Company, db seedling.DBTX) {
			called = true
			capturedCompany = c
		}),
	)

	// Assert
	if !called {
		t.Fatal("expected AfterInsert callback to be called")
	}
	if capturedCompany.ID != company.ID {
		t.Fatalf("got %v, want %v", capturedCompany.ID, company.ID)
	}
	if capturedCompany.Name != "after-insert-co" {
		t.Fatalf("got %v, want %v", capturedCompany.Name, "after-insert-co")
	}
}

func TestAfterInsert_WithDB(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	var capturedDB seedling.DBTX

	// Act
	// db is nil in tests, but AfterInsert should still pass it through.
	insertOne[Company](t, nil,
		seedling.AfterInsert(func(c Company, db seedling.DBTX) {
			capturedDB = db
		}),
	)

	// Assert
	if capturedDB != nil {
		t.Fatalf("expected nil, got %v", capturedDB)
	}
}

func TestAfterInsert_MultipleCallbacks(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	var order []int

	// Act
	insertOne[Company](t, nil,
		seedling.AfterInsert(func(c Company, db seedling.DBTX) {
			order = append(order, 1)
		}),
		seedling.AfterInsert(func(c Company, db seedling.DBTX) {
			order = append(order, 2)
		}),
	)

	// Assert
	if !reflect.DeepEqual(order, []int{1, 2}) {
		t.Fatalf("got %v, want %v", order, []int{1, 2})
	}
}

func TestTrait(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	adminTrait := seedling.InlineTrait(
		seedling.Set("Name", "admin-user"),
	)

	// Act
	user := insertOne[User](t, nil, adminTrait)

	// Assert
	if user.Name != "admin-user" {
		t.Fatalf("got %v, want %v", user.Name, "admin-user")
	}
}

func TestTrait_Combined(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	urgentTrait := seedling.InlineTrait(
		seedling.Set("Status", "urgent"),
	)

	// Act
	task := insertOne[Task](t, nil,
		urgentTrait,
		seedling.Set("Title", "important task"),
	)

	// Assert
	if task.Status != "urgent" {
		t.Fatalf("got %v, want %v", task.Status, "urgent")
	}
	if task.Title != "important task" {
		t.Fatalf("got %v, want %v", task.Title, "important task")
	}
}

func TestTrait_Nested(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	baseTrait := seedling.InlineTrait(
		seedling.Set("Status", "open"),
	)
	fullTrait := seedling.InlineTrait(
		baseTrait,
		seedling.Set("Title", "nested-trait-title"),
	)

	// Act
	task := insertOne[Task](t, nil, fullTrait)

	// Assert
	if task.Status != "open" {
		t.Fatalf("got %v, want %v", task.Status, "open")
	}
	if task.Title != "nested-trait-title" {
		t.Fatalf("got %v, want %v", task.Title, "nested-trait-title")
	}
}

func TestResult_All(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Task](t)

	// Act
	result := plan.Insert(t, nil)
	all := result.All()

	// Assert
	if len(all) == 0 {
		t.Fatal("expected non-empty All()")
	}

	// Task blueprint creates: task (root), project, company (for project),
	// assignee/user, company (for user) — at least 4-5 nodes.
	if len(all) < 4 {
		t.Fatalf("got len %d, want >= 4", len(all))
	}

	// Verify that all nodes have non-empty names and values.
	for id, nr := range all {
		if nr.Name() == "" {
			t.Errorf("node %q has empty name", id)
		}
		if nr.Value() == nil {
			t.Errorf("node %q has nil value", id)
		}
	}
}

func TestPlan_Validate(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	plan := build[Task](t)

	// Assert
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPlan_Validate_Simple(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	plan := build[Company](t)

	// Assert
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResult_All_ContainsRoot(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Company](t)

	// Act
	result := plan.Insert(t, nil)
	all := result.All()

	// Assert
	if len(all) != 1 {
		t.Fatalf("got len %d, want %d", len(all), 1)
	}

	// The single node should be the company.
	for _, nr := range all {
		if nr.Name() != "company" {
			t.Fatalf("got %v, want %v", nr.Name(), "company")
		}
		company := nr.Value().(Company)
		if company.ID == 0 {
			t.Fatal("expected non-zero ID")
		}
	}
}

func TestInsertOne_HasManyAutoExpand(t *testing.T) {
	// Arrange
	setupHasManyBlueprints(t)
	plan := build[Department](t)

	// Act
	result := plan.Insert(t, nil)
	department := result.Root()

	// Assert
	if department.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	all := result.All()
	if len(all) != 3 {
		t.Fatalf("got len %d, want %d", len(all), 3)
	}

	employeeCount := 0
	for _, node := range all {
		if node.Name() != "employee" {
			continue
		}
		employeeCount++
		employee := node.Value().(Employee)
		if employee.ID == 0 {
			t.Fatal("expected non-zero employee ID")
		}
		if employee.DepartmentID != department.ID {
			t.Fatalf("got %v, want %v", employee.DepartmentID, department.ID)
		}
	}
	if employeeCount != 2 {
		t.Fatalf("got %v, want %v", employeeCount, 2)
	}
}

func TestRef_HasManyAppliesToAllChildren(t *testing.T) {
	// Arrange
	setupHasManyBlueprints(t)
	plan := build[Department](t,
		seedling.Ref("employees", seedling.Set("Name", "custom-employee")),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	for id, node := range result.All() {
		if node.Name() != "employee" {
			continue
		}
		employee := node.Value().(Employee)
		if employee.Name != "custom-employee" {
			t.Fatalf("%s Name mismatch: got %v, want %v", id, employee.Name, "custom-employee")
		}
	}
}

func TestRef_OptionalHasManyExpandsRelation(t *testing.T) {
	// Arrange
	setupOptionalHasManyBlueprints(t)
	plan := build[Department](t,
		seedling.Ref("employees", seedling.Set("Name", "custom-employee")),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	employeeCount := 0
	for id, node := range result.All() {
		if node.Name() != "employee" {
			continue
		}
		employeeCount++
		employee := node.Value().(Employee)
		if employee.Name != "custom-employee" {
			t.Fatalf("%s Name mismatch: got %v, want %v", id, employee.Name, "custom-employee")
		}
	}
	if employeeCount != 2 {
		t.Fatalf("got %v, want %v", employeeCount, 2)
	}
}

func TestBuild_DebugString_HasMany(t *testing.T) {
	// Arrange
	setupHasManyBlueprints(t)

	// Act
	plan := build[Department](t)
	out := plan.DebugString()

	// Assert
	if !strings.Contains(out, "employee") {
		t.Fatal("expected output to contain \"employee\"")
	}
}

func TestResult_DebugString_HasMany(t *testing.T) {
	// Arrange
	setupHasManyBlueprints(t)
	plan := build[Department](t)

	// Act
	result := plan.Insert(t, nil)
	out := result.DebugString()

	// Assert
	if !strings.Contains(out, "employee") {
		t.Fatal("expected output to contain \"employee\"")
	}
}

func TestUseOnHasManyRelation(t *testing.T) {
	// Arrange
	setupHasManyBlueprints(t)

	// Act
	_, err := buildE[Department](t,
		seedling.Use("employees", []Employee{{ID: 1, Name: "existing"}}),
	)

	// Assert
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
	}
}

func TestUse_PointerValueNormalizedForNodeAs(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	company := insertOne[Company](t, nil, seedling.Set("Name", "ptr-company"))
	plan := build[User](t,
		seedling.Use("company", &company),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert: NodeAs[Company] must succeed even though a *Company was passed to Use.
	got, ok, err := seedling.NodeAs[Company](result, "company")
	if err != nil {
		t.Fatalf("NodeAs[Company] returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected company node in result")
	}
	if got.Name != "ptr-company" {
		t.Fatalf("got %v, want %v", got.Name, "ptr-company")
	}
}

func TestInsertOne_CompositePKBelongsTo(t *testing.T) {
	// Arrange
	setupCompositePKBlueprints(t)
	plan := build[Deployment](t)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}

	// Act
	result := plan.Insert(t, nil)
	deployment := result.Root()

	// Assert
	if deployment.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if deployment.RegionCode == "" {
		t.Fatal("expected RegionCode to be set from composite parent PK")
	}
	if deployment.RegionNumber == 0 {
		t.Fatal("expected RegionNumber to be set from composite parent PK")
	}

	regionNode, ok := result.Node("region")
	if !ok {
		t.Fatal("expected region node in result")
	}
	region := regionNode.Value().(Region)
	if region.Code != deployment.RegionCode {
		t.Fatalf("got %v, want %v", deployment.RegionCode, region.Code)
	}
	if region.Number != deployment.RegionNumber {
		t.Fatalf("got %v, want %v", deployment.RegionNumber, region.Number)
	}
}

func TestInsertOne_ManyToManyAutoExpand(t *testing.T) {
	// Arrange
	setupManyToManyBlueprints(t)
	plan := build[Article](t)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}

	// Act
	result := plan.Insert(t, nil)
	article := result.Root()

	// Assert
	if article.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	all := result.All()
	var tags []Tag
	var joins []ArticleTag
	for _, node := range all {
		switch node.Name() {
		case "tag":
			tags = append(tags, node.Value().(Tag))
		case "article_tag":
			joins = append(joins, node.Value().(ArticleTag))
		}
	}
	if len(tags) != 2 {
		t.Fatalf("got len %d, want %d", len(tags), 2)
	}
	if len(joins) != 2 {
		t.Fatalf("got len %d, want %d", len(joins), 2)
	}

	tagIDs := make(map[int]bool, len(tags))
	for _, tag := range tags {
		if tag.ID == 0 {
			t.Fatal("expected non-zero tag ID")
		}
		tagIDs[tag.ID] = true
	}
	for _, join := range joins {
		if join.ArticleID != article.ID {
			t.Fatalf("got %v, want %v", join.ArticleID, article.ID)
		}
		if !tagIDs[join.TagID] {
			t.Fatalf("join.TagID = %d does not match any generated tag", join.TagID)
		}
	}
}

func TestRef_ManyToManyAppliesToChildren(t *testing.T) {
	// Arrange
	setupManyToManyBlueprints(t)
	plan := build[Article](t,
		seedling.Ref("tags", seedling.Set("Name", "custom-tag")),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	tagCount := 0
	for _, node := range result.All() {
		if node.Name() != "tag" {
			continue
		}
		tagCount++
		tag := node.Value().(Tag)
		if tag.Name != "custom-tag" {
			t.Fatalf("got %v, want %v", tag.Name, "custom-tag")
		}
	}
	if tagCount != 2 {
		t.Fatalf("got %v, want %v", tagCount, 2)
	}
}

func TestRef_OptionalManyToManyExpandsRelation(t *testing.T) {
	// Arrange
	setupOptionalManyToManyBlueprints(t)
	plan := build[Article](t,
		seedling.Ref("tags", seedling.Set("Name", "custom-tag")),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	tagCount := 0
	for _, node := range result.All() {
		if node.Name() != "tag" {
			continue
		}
		tagCount++
		tag := node.Value().(Tag)
		if tag.Name != "custom-tag" {
			t.Fatalf("got %v, want %v", tag.Name, "custom-tag")
		}
	}
	if tagCount != 2 {
		t.Fatalf("got %v, want %v", tagCount, 2)
	}
}

// ctxKey is a custom context key for testing.
type ctxKey struct{}

func registerContextCompany(tb testing.TB, captured *context.Context) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	useTestRegistry(tb, reg)

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "ctx-test"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			*captured = ctx
			v.ID = ids.Next()
			return v, nil
		},
	})
}

func TestContextPropagation_InsertPaths(t *testing.T) {
	// Arrange
	tests := []struct {
		name string
		want string
		run  func(t *testing.T, ctx context.Context)
	}{
		{
			name: "insert one",
			want: "hello",
			run: func(t *testing.T, ctx context.Context) {
				t.Helper()
				insertOne[Company](t, nil, seedling.WithContext(ctx))
			},
		},
		{
			name: "build insert",
			want: "world",
			run: func(t *testing.T, ctx context.Context) {
				t.Helper()
				build[Company](t, seedling.WithContext(ctx)).Insert(t, nil)
			},
		},
		{
			name: "insert e",
			want: "explicit",
			run: func(t *testing.T, ctx context.Context) {
				t.Helper()
				plan := build[Company](t)
				_, err := plan.InsertE(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var capturedCtx context.Context
			registerContextCompany(t, &capturedCtx)
			ctx := context.WithValue(t.Context(), ctxKey{}, tt.want)

			// Act
			tt.run(t, ctx)

			// Assert
			if capturedCtx == nil {
				t.Fatal("expected context to be captured")
			}
			val, ok := capturedCtx.Value(ctxKey{}).(string)
			if !ok {
				t.Fatal("expected context value to be a string")
			}
			if val != tt.want {
				t.Fatalf("got %v, want %v", val, tt.want)
			}
		})
	}
}

func TestInsertMany_CountValidation(t *testing.T) {
	// Arrange
	tests := []struct {
		name    string
		run     func(t *testing.T) (int, error)
		wantLen int
		wantErr error
	}{
		{
			name: "insert many e rejects negative one",
			run: func(t *testing.T) (int, error) {
				t.Helper()
				result, err := insertManyE[Company](t.Context(), t, nil, -1)
				return result.Len(), err
			},
			wantErr: seedling.ErrInvalidOption,
		},
		{
			name: "insert many e rejects large negative",
			run: func(t *testing.T) (int, error) {
				t.Helper()
				result, err := insertManyE[Company](t.Context(), t, nil, -100)
				return result.Len(), err
			},
			wantErr: seedling.ErrInvalidOption,
		},
		{
			name: "insert many e allows zero",
			run: func(t *testing.T) (int, error) {
				t.Helper()
				result, err := insertManyE[Company](t.Context(), t, nil, 0)
				return result.Len(), err
			},
			wantLen: 0,
		},
		{
			name: "insert many allows zero",
			run: func(t *testing.T) (int, error) {
				t.Helper()
				result := insertMany[Company](t, nil, 0)
				return len(result), nil
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			setupBlueprints(t)

			// Act
			gotLen, err := tt.run(t)

			// Assert
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
			if gotLen != tt.wantLen {
				t.Fatalf("got %v, want %v", gotLen, tt.wantLen)
			}
		})
	}
}

func TestPlanInsertE_ReusedAfterInsertSharesClosureState(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	var seen []int
	plan := build[Company](t,
		seedling.AfterInsert(func(c Company, db seedling.DBTX) {
			seen = append(seen, c.ID)
		}),
	)

	// Act
	first := plan.Insert(t, nil).Root()
	second := plan.Insert(t, nil).Root()

	// Assert
	if len(seen) != 2 {
		t.Fatalf("got %d callback calls, want 2", len(seen))
	}
	if seen[0] != first.ID {
		t.Fatalf("got first callback ID %d, want %d", seen[0], first.ID)
	}
	if seen[1] != second.ID {
		t.Fatalf("got second callback ID %d, want %d", seen[1], second.ID)
	}
}

// Diamond: Task depends on both Project and User.
// Both Project and User depend on Company.
// Without Use(), each gets its own Company (no reuse).
// With Use(), both share the same Company.

func TestDiamondDependency_NoReuse(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	plan := build[Task](t)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	var companyIDs []int
	for _, node := range result.All() {
		if node.Name() != "company" {
			continue
		}
		c := node.Value().(Company)
		companyIDs = append(companyIDs, c.ID)
	}

	if len(companyIDs) != 2 {
		t.Fatalf("expected 2 separate companies in diamond (no reuse), got %d", len(companyIDs))
	}
	if companyIDs[0] == companyIDs[1] {
		t.Fatalf("expected different Company IDs without Use(), got %v and %v", companyIDs[0], companyIDs[1])
	}
}

func TestDiamondDependency_WithReuse(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	sharedCompany := insertOne[Company](t, nil,
		seedling.Set("Name", "shared-diamond-co"),
	)

	plan := build[Task](t,
		seedling.Ref("project",
			seedling.Use("company", sharedCompany),
		),
		seedling.Ref("assignee",
			seedling.Use("company", sharedCompany),
		),
	)

	// Act
	result := plan.Insert(t, nil)

	// Assert
	task := result.Root()
	if task.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	projectNode, ok := result.Node("project")
	if !ok {
		t.Fatal("expected project node")
	}
	project := projectNode.Value().(Project)
	if project.CompanyID != sharedCompany.ID {
		t.Fatalf("got %v, want %v", project.CompanyID, sharedCompany.ID)
	}

	var user User
	for _, node := range result.All() {
		if node.Name() == "user" {
			user = node.Value().(User)
			break
		}
	}
	if user.CompanyID != sharedCompany.ID {
		t.Fatalf("got %v, want %v", user.CompanyID, sharedCompany.ID)
	}
}

func registerPtrModel(tb testing.TB, defaults func() PtrModel) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	useTestRegistry(tb, reg)

	seedling.MustRegisterTo(reg, seedling.Blueprint[PtrModel]{
		Name:     "ptr_model",
		Table:    "ptr_models",
		PKField:  "ID",
		Defaults: defaults,
		Insert: func(ctx context.Context, db seedling.DBTX, v PtrModel) (PtrModel, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
}

func registerInterfaceModel(tb testing.TB, defaults func() InterfaceModel) {
	tb.Helper()
	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	useTestRegistry(tb, reg)

	seedling.MustRegisterTo(reg, seedling.Blueprint[InterfaceModel]{
		Name:     "interface_model",
		Table:    "interface_models",
		PKField:  "ID",
		Defaults: defaults,
		Insert: func(ctx context.Context, db seedling.DBTX, v InterfaceModel) (InterfaceModel, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
}

func TestPtrModel_FieldOptions(t *testing.T) {
	// Arrange
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "uses defaults",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				defaultName := "ptr-default"
				registerPtrModel(t, func() PtrModel {
					return PtrModel{Name: &defaultName}
				})

				// Act
				m := insertOne[PtrModel](t, nil)

				// Assert
				if m.ID == 0 {
					t.Fatal("expected non-zero ID")
				}
				if m.Name == nil {
					t.Fatal("expected non-nil")
				}
				if *m.Name != "ptr-default" {
					t.Fatalf("got %v, want %v", *m.Name, "ptr-default")
				}
				if m.Optional != nil {
					t.Fatalf("expected nil, got %v", m.Optional)
				}
			},
		},
		{
			name: "set pointer",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				registerPtrModel(t, func() PtrModel { return PtrModel{} })
				customName := "custom-ptr"

				// Act
				m := insertOne[PtrModel](t, nil, seedling.Set("Name", &customName))

				// Assert
				if m.Name == nil {
					t.Fatal("expected non-nil")
				}
				if *m.Name != "custom-ptr" {
					t.Fatalf("got %v, want %v", *m.Name, "custom-ptr")
				}
			},
		},
		{
			name: "with function",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				registerPtrModel(t, func() PtrModel { return PtrModel{} })
				opt := 42

				// Act
				m := insertOne[PtrModel](t, nil,
					seedling.With(func(p *PtrModel) {
						name := "with-ptr"
						p.Name = &name
						p.Optional = &opt
					}),
				)

				// Assert
				if m.Name == nil {
					t.Fatal("expected non-nil")
				}
				if *m.Name != "with-ptr" {
					t.Fatalf("got %v, want %v", *m.Name, "with-ptr")
				}
				if m.Optional == nil {
					t.Fatal("expected non-nil")
				}
				if *m.Optional != 42 {
					t.Fatalf("got %v, want %v", *m.Optional, 42)
				}
			},
		},
		{
			name: "insert many with seq",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				registerPtrModel(t, func() PtrModel { return PtrModel{} })

				// Act
				models := insertMany[PtrModel](t, nil, 3,
					seedling.Seq("Name", func(i int) *string {
						s := fmt.Sprintf("ptr-%d", i)
						return &s
					}),
				)

				// Assert
				if len(models) != 3 {
					t.Fatalf("got len %d, want %d", len(models), 3)
				}
				for i, m := range models {
					expected := fmt.Sprintf("ptr-%d", i)
					if m.Name == nil {
						t.Fatalf("models[%d].Name: expected non-nil", i)
					}
					if *m.Name != expected {
						t.Fatalf("models[%d].Name: got %v, want %v", i, *m.Name, expected)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestInterfaceModel_FieldOptions(t *testing.T) {
	// Arrange
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "uses defaults",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				registerInterfaceModel(t, func() InterfaceModel {
					return InterfaceModel{Name: "iface-default", Metadata: map[string]any{"key": "value"}}
				})

				// Act
				m := insertOne[InterfaceModel](t, nil)

				// Assert
				if m.ID == 0 {
					t.Fatal("expected non-zero ID")
				}
				md, ok := m.Metadata.(map[string]any)
				if !ok {
					t.Fatalf("expected map[string]any, got %T", m.Metadata)
				}
				if md["key"] != "value" {
					t.Fatalf("got %v, want %v", md["key"], "value")
				}
			},
		},
		{
			name: "set nil",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				registerInterfaceModel(t, func() InterfaceModel {
					return InterfaceModel{Name: "iface-nil", Metadata: "initial"}
				})

				// Act
				m := insertOne[InterfaceModel](t, nil, seedling.Set("Metadata", nil))

				// Assert
				if m.Metadata != nil {
					t.Fatalf("expected nil, got %v", m.Metadata)
				}
			},
		},
		{
			name: "set override",
			run: func(t *testing.T) {
				t.Helper()
				// Arrange
				registerInterfaceModel(t, func() InterfaceModel {
					return InterfaceModel{Name: "iface-override"}
				})

				// Act
				m := insertOne[InterfaceModel](t, nil, seedling.Set("Metadata", []int{1, 2, 3}))

				// Assert
				slice, ok := m.Metadata.([]int)
				if !ok {
					t.Fatalf("expected []int, got %T", m.Metadata)
				}
				if len(slice) != 3 {
					t.Fatalf("got len %d, want %d", len(slice), 3)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestGenerate_WithSeed(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	gen := seedling.Generate(func(r *rand.Rand, c *Company) {
		c.Name = fmt.Sprintf("company-%d", r.IntN(1000))
	})

	// Act
	first := insertOne[Company](t, nil, seedling.WithSeed(42), gen)
	second := insertOne[Company](t, nil, seedling.WithSeed(42), gen)

	// Assert
	if first.Name != second.Name {
		t.Fatalf("expected deterministic generated names: got %v and %v", first.Name, second.Name)
	}
	if first.Name == "test-company" {
		t.Fatalf("expected generated name to differ from default, got %v", first.Name)
	}
}

func TestPlan_DryRunString(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	plan := build[Task](t)
	out := plan.DryRunString()
	t.Log(out)

	// Assert
	if !strings.Contains(out, "Step 1:") {
		t.Fatalf("expected Step 1, got:\n%s", out)
	}
	if !strings.Contains(out, "INSERT INTO") {
		t.Fatalf("expected INSERT INTO, got:\n%s", out)
	}
	if !strings.Contains(out, "SET") {
		t.Fatalf("expected SET for FK bindings, got:\n%s", out)
	}
	// Task depends on project and user, so there should be at least 3 steps
	if !strings.Contains(out, "Step 3:") {
		t.Fatalf("expected at least 3 steps, got:\n%s", out)
	}
}

func TestWithInsertLog(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	var logs []seedling.InsertLog
	logFn := seedling.WithInsertLog(func(log seedling.InsertLog) {
		logs = append(logs, log)
	})

	// Act
	plan := build[Task](t, logFn)
	plan.Insert(t, nil)

	// Assert
	if len(logs) == 0 {
		t.Fatal("expected at least one log entry")
	}

	// Verify steps are sequential.
	for i, log := range logs {
		if log.Step != i+1 {
			t.Errorf("log[%d].Step = %d, want %d", i, log.Step, i+1)
		}
	}

	// The last step should be the root (task).
	last := logs[len(logs)-1]
	if last.Blueprint != "task" {
		t.Errorf("expected last step to be task, got %q", last.Blueprint)
	}
	if len(last.FKBindings) == 0 {
		t.Error("expected task to have FK bindings")
	}

	// Verify FK bindings have values.
	for _, fk := range last.FKBindings {
		if fk.Value == nil {
			t.Errorf("expected FK %s to have a value", fk.ChildField)
		}
	}
}

func TestWithInsertLog_ProvidedNode(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	company := insertOne[Company](t, nil)

	var logs []seedling.InsertLog
	logFn := seedling.WithInsertLog(func(log seedling.InsertLog) {
		logs = append(logs, log)
	})

	// Act
	insertOne[User](t, nil, seedling.Use("company", company), logFn)

	// Assert
	var foundProvided bool
	for _, log := range logs {
		if log.Blueprint == "company" && log.Provided {
			foundProvided = true
		}
	}
	if !foundProvided {
		t.Error("expected a provided (skipped) log entry for company")
	}
}

// --- When (conditional relation expansion) ---

func TestWhen_OptionExpandsRelationWhenTrue(t *testing.T) {
	// Arrange: register Task with assignee as Required
	setupBlueprints(t)

	// Act: When predicate returns true → assignee should be expanded
	task := insertOne[Task](t, nil,
		seedling.Set("Status", "assigned"),
		seedling.When("assignee", func(t Task) bool {
			return t.Status == "assigned"
		}),
	)

	// Assert: assignee was created
	if task.AssigneeUserID == 0 {
		t.Fatal("expected AssigneeUserID to be populated when When returns true")
	}
}

func TestWhen_OptionExpandsOptionalRelationWhenTrue(t *testing.T) {
	// Arrange
	setupOptionalBelongsToBlueprints(t)

	// Act
	user := insertOne[User](t, nil,
		seedling.Set("Name", "expand-company"),
		seedling.When("company", func(u User) bool {
			return u.Name == "expand-company"
		}),
	)

	// Assert
	if user.CompanyID == 0 {
		t.Fatal("expected CompanyID to be populated when When returns true")
	}
}

func TestWhen_OptionSkipsRelationWhenFalse(t *testing.T) {
	// Arrange: register Task with assignee as Optional so we can skip it
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", Table: "companies", PKField: "ID",
		Defaults: func() Company { return Company{Name: "test-company"} },
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name: "user", Table: "users", PKField: "ID",
		Defaults: func() User { return User{Name: "test-user"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Project]{
		Name: "project", Table: "projects", PKField: "ID",
		Defaults: func() Project { return Project{Name: "test-project"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Task]{
		Name: "task", Table: "tasks", PKField: "ID",
		Defaults: func() Task { return Task{Title: "test-task", Status: "open"} },
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	// Act: When predicate returns false → assignee should be skipped
	task := insertOne[Task](t, nil,
		seedling.Set("Status", "open"),
		seedling.When("assignee", func(t Task) bool {
			return t.Status == "assigned"
		}),
	)

	// Assert: assignee was NOT created
	if task.AssigneeUserID != 0 {
		t.Fatalf("expected AssigneeUserID to be zero when When returns false, got %d", task.AssigneeUserID)
	}
	// project should still be created
	if task.ProjectID == 0 {
		t.Fatal("expected ProjectID to be populated")
	}
}

func TestWhen_BlueprintLevelPredicate(t *testing.T) {
	// Arrange: register Task with a blueprint-level When on assignee
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", Table: "companies", PKField: "ID",
		Defaults: func() Company { return Company{Name: "test-company"} },
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name: "user", Table: "users", PKField: "ID",
		Defaults: func() User { return User{Name: "test-user"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Project]{
		Name: "project", Table: "projects", PKField: "ID",
		Defaults: func() Project { return Project{Name: "test-project"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Task]{
		Name: "task", Table: "tasks", PKField: "ID",
		Defaults: func() Task { return Task{Title: "test-task", Status: "open"} },
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{
				Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user",
				When: func(v any) bool {
					return v.(Task).Status == "assigned"
				},
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	// Act & Assert: default Status is "open" → assignee skipped
	task1 := insertOne[Task](t, nil)
	if task1.AssigneeUserID != 0 {
		t.Fatalf("expected AssigneeUserID=0 for status=open, got %d", task1.AssigneeUserID)
	}

	// Act & Assert: Status "assigned" → assignee expanded
	task2 := insertOne[Task](t, nil, seedling.Set("Status", "assigned"))
	if task2.AssigneeUserID == 0 {
		t.Fatal("expected AssigneeUserID to be populated for status=assigned")
	}
}

func TestWhen_OptionOverridesBlueprintWhen(t *testing.T) {
	// Arrange: blueprint-level When always returns false
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", Table: "companies", PKField: "ID",
		Defaults: func() Company { return Company{Name: "test-company"} },
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name: "user", Table: "users", PKField: "ID",
		Defaults: func() User { return User{Name: "test-user"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Project]{
		Name: "project", Table: "projects", PKField: "ID",
		Defaults: func() Project { return Project{Name: "test-project"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Task]{
		Name: "task", Table: "tasks", PKField: "ID",
		Defaults: func() Task { return Task{Title: "test-task", Status: "open"} },
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{
				Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user",
				When: func(v any) bool {
					return false // blueprint says never expand
				},
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	// Act: option-level When returns true → should override blueprint's When
	task := insertOne[Task](t, nil,
		seedling.When("assignee", func(t Task) bool {
			return true // always expand at insert time
		}),
	)

	// Assert: option-level When overrode blueprint-level When
	if task.AssigneeUserID == 0 {
		t.Fatal("expected option-level When to override blueprint-level When")
	}
}

func TestWhen_HasMany(t *testing.T) {
	// Arrange: department with conditional HasMany employees
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Department]{
		Name: "department", Table: "departments", PKField: "ID",
		Defaults: func() Department { return Department{Name: "engineering"} },
		Relations: []seedling.Relation{
			{
				Name: "employees", Kind: seedling.HasMany, LocalField: "DepartmentID",
				RefBlueprint: "employee", Count: 2,
				When: func(v any) bool {
					return v.(Department).Name != "empty"
				},
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Department) (Department, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Employee]{
		Name: "employee", Table: "employees", PKField: "ID",
		Defaults: func() Employee { return Employee{Name: "employee"} },
		Relations: []seedling.Relation{
			{Name: "department", Kind: seedling.BelongsTo, LocalField: "DepartmentID", RefBlueprint: "department"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Employee) (Employee, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	// Act & Assert: "engineering" → employees created
	plan1 := build[Department](t)
	result1 := plan1.Insert(t, nil)
	employees1, _ := seedling.NodesAs[Employee](result1, "employee")
	if len(employees1) != 2 {
		t.Fatalf("expected 2 employees for engineering, got %d", len(employees1))
	}

	// Act & Assert: "empty" → employees skipped
	plan2 := build[Department](t, seedling.Set("Name", "empty"))
	result2 := plan2.Insert(t, nil)
	employees2, _ := seedling.NodesAs[Employee](result2, "employee")
	if len(employees2) != 0 {
		t.Fatalf("expected 0 employees for empty department, got %d", len(employees2))
	}
}

func TestWhenFunc_BlueprintLevel(t *testing.T) {
	// Arrange: use WhenFunc for type-safe blueprint-level When predicate
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", Table: "companies", PKField: "ID",
		Defaults: func() Company { return Company{Name: "test-company"} },
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name: "user", Table: "users", PKField: "ID",
		Defaults: func() User { return User{Name: "test-user"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Project]{
		Name: "project", Table: "projects", PKField: "ID",
		Defaults: func() Project { return Project{Name: "test-project"} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[Task]{
		Name: "task", Table: "tasks", PKField: "ID",
		Defaults: func() Task { return Task{Title: "test-task", Status: "open"} },
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{
				Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user",
				When: seedling.WhenFunc(func(t Task) bool {
					return t.Status == "assigned"
				}),
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	useTestRegistry(t, reg)

	// Act & Assert: default Status is "open" → assignee skipped
	task1 := insertOne[Task](t, nil)
	if task1.AssigneeUserID != 0 {
		t.Fatalf("expected AssigneeUserID=0 for status=open, got %d", task1.AssigneeUserID)
	}

	// Act & Assert: Status "assigned" → assignee expanded
	task2 := insertOne[Task](t, nil, seedling.Set("Status", "assigned"))
	if task2.AssigneeUserID == 0 {
		t.Fatal("expected AssigneeUserID to be populated for status=assigned")
	}
}

// ---------------------------------------------------------------------------
// P1 regression: AfterInsertE failure returns Result for cleanup
// ---------------------------------------------------------------------------

func TestInsertOneE_AfterInsertEFailure_ReturnsResult(t *testing.T) {
	// Arrange
	ids := seedlingtest.NewIDSequence()
	reg := seedlingtest.NewRegistry()

	var deleted []int
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "cleanup-test"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v Company) error {
			deleted = append(deleted, v.ID)
			return nil
		},
	})

	sess := seedling.NewSession[Company](reg)
	afterErr := fmt.Errorf("after-insert boom")

	// Act
	result, err := sess.InsertOneE(t.Context(), nil,
		seedling.AfterInsertE(func(c Company, db seedling.DBTX) error {
			return afterErr
		}),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error from AfterInsertE callback")
	}
	if !errors.Is(err, afterErr) {
		t.Fatalf("got %v, want wrapped %v", err, afterErr)
	}
	if result.Root().ID == 0 {
		t.Fatal("expected result to have a valid root with non-zero ID")
	}

	// CleanupE should not panic and should invoke Delete
	cleanupErr := result.CleanupE(t.Context(), nil)
	if cleanupErr != nil {
		t.Fatalf("CleanupE returned unexpected error: %v", cleanupErr)
	}
	if len(deleted) == 0 {
		t.Fatal("expected CleanupE to call Delete at least once")
	}
}

func TestInsertManyE_AfterInsertEFailure_ReturnsBatchResult(t *testing.T) {
	// Arrange
	ids := seedlingtest.NewIDSequence()
	reg := seedlingtest.NewRegistry()
	var deleted []int

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "many-test"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(ctx context.Context, db seedling.DBTX, v Company) error {
			deleted = append(deleted, v.ID)
			return nil
		},
	})

	sess := seedling.NewSession[Company](reg)
	callCount := 0
	afterErr := fmt.Errorf("after-insert fail on 2nd")

	// Act
	result, err := sess.InsertManyE(t.Context(), nil, 3,
		seedling.AfterInsertE(func(c Company, db seedling.DBTX) error {
			callCount++
			if callCount == 2 {
				return afterErr
			}
			return nil
		}),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error from AfterInsertE callback on 2nd record")
	}
	if !errors.Is(err, afterErr) {
		t.Fatalf("got %v, want wrapped %v", err, afterErr)
	}
	if got, want := result.Len(), 3; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	roots := result.Roots()
	if len(roots) != 3 {
		t.Fatalf("got %d roots, want 3", len(roots))
	}
	if roots[0].ID == 0 || roots[1].ID == 0 || roots[2].ID == 0 {
		t.Fatal("expected all inserted roots to have non-zero IDs")
	}
	if result.DebugString() == "" {
		t.Fatal("expected non-empty DebugString")
	}
	if err := result.CleanupE(t.Context(), nil); err != nil {
		t.Fatalf("CleanupE returned unexpected error: %v", err)
	}
	if len(deleted) != 3 {
		t.Fatalf("got %d deleted records, want 3", len(deleted))
	}
}

// ---------------------------------------------------------------------------
// P1 regression: When type mismatch returns error
// ---------------------------------------------------------------------------

func TestBuildE_WhenTypeMismatch_ReturnsError(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	// Task has an "assignee" relation to User, not Company.
	// When[Company] will fail because the root value is a Task, not a Company.
	_, err := buildE[Task](t,
		seedling.When[Company]("assignee", func(c Company) bool {
			return c.Name == "test"
		}),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error for When type mismatch")
	}
	if !errors.Is(err, seedling.ErrTypeMismatch) {
		t.Fatalf("got %v, want %v", err, seedling.ErrTypeMismatch)
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("expected error to contain %q, got %v", "type mismatch", err)
	}
}

func TestBuildE_WhenUnknownRelation_ReturnsError(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	_, err := buildE[Task](t,
		seedling.When[Task]("nonexistent", func(task Task) bool {
			return true
		}),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error for When on unknown relation")
	}
	if !errors.Is(err, seedling.ErrRelationNotFound) {
		t.Fatalf("got %v, want %v", err, seedling.ErrRelationNotFound)
	}
}

// ---------------------------------------------------------------------------
// P1 regression: Nested BlueprintTrait resolution
// ---------------------------------------------------------------------------

func TestInsertOneE_NestedBlueprintTrait(t *testing.T) {
	// Arrange
	ids := seedlingtest.NewIDSequence()
	reg := seedlingtest.NewRegistry()

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "default-company"}
		},
		Traits: map[string][]seedling.Option{
			"big": {seedling.Set("Name", "big-corp")},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
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
		Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	sess := seedling.NewSession[User](reg)

	// Act
	result, err := sess.InsertOneE(t.Context(), nil,
		seedling.Ref("company", seedling.BlueprintTrait("big")),
	)
	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("expected to find a 'company' node in result")
	}
	company := companyNode.Value().(Company)
	if company.Name != "big-corp" {
		t.Fatalf("got company.Name = %q, want %q", company.Name, "big-corp")
	}
}

func TestBuildE_TraitOfTrait(t *testing.T) {
	// Arrange
	ids := seedlingtest.NewIDSequence()
	reg := seedlingtest.NewRegistry()

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "default-company"}
		},
		Traits: map[string][]seedling.Option{
			"base":     {seedling.Set("Name", "base-company")},
			"extended": {seedling.BlueprintTrait("base")},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})

	sess := seedling.NewSession[Company](reg)

	// Act
	plan, err := sess.BuildE(seedling.BlueprintTrait("extended"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := plan.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatalf("unexpected insert error: %v", err)
	}

	// Assert
	company := result.Root()
	if company.Name != "base-company" {
		t.Fatalf("got company.Name = %q, want %q (inherited from base trait)", company.Name, "base-company")
	}
}

// ---------------------------------------------------------------------------
// P1 regression: InsertOne returns Result
// ---------------------------------------------------------------------------

func TestInsertOne_ReturnsResult(t *testing.T) {
	// Arrange
	setupBlueprints(t)

	// Act
	result := session[Task](t).InsertOne(t, nil)

	// Assert
	task := result.Root()
	if task.ID == 0 {
		t.Fatal("expected task to have non-zero ID")
	}
	if task.Title != "test-task" {
		t.Fatalf("got task.Title = %q, want %q", task.Title, "test-task")
	}

	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("expected to find a 'company' node in result")
	}
	company := companyNode.Value().(Company)
	if company.ID == 0 {
		t.Fatal("expected company node to have non-zero ID")
	}
}
