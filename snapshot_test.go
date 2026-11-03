package seedling_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func snapshotPaths(snap seedling.PlanSnapshot) map[string]seedling.PlanNode {
	nodes := make(map[string]seedling.PlanNode)
	for _, node := range snap.Nodes() {
		nodes[node.Path] = node
	}
	return nodes
}

func TestPlanSnapshot_DescribesEveryRecord(t *testing.T) {
	// Arrange
	setupBlueprints(t)
	company := insertOne[Company](t, nil)

	// Act
	snap := build[Task](t, seedling.Ref("assignee", seedling.Use("company", company))).Snapshot()

	// Assert
	if got, want := snap.Len(), 5; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	if got, want := snap.Inserts(), 4; got != want {
		t.Fatalf("Inserts() = %d, want %d", got, want)
	}
	nodes := snapshotPaths(snap)
	if got, want := slices.Sorted(maps.Keys(nodes)), []string{"task", "task.assignee", "task.assignee.company", "task.project", "task.project.company"}; !slices.Equal(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if root := nodes["task"]; root.Reason != seedling.ReasonRoot || root.Relation != "" || len(root.DependsOn) != 2 {
		t.Fatalf("root = %+v", root)
	}
	if project := nodes["task.project"]; project.Reason != seedling.ReasonBelongsTo || project.Relation != "project" || project.Table != "projects" {
		t.Fatalf("project = %+v", project)
	}
	used := nodes["task.assignee.company"]
	if !used.Provided || used.Shared {
		t.Fatalf("used company = %+v, want a provided record", used)
	}
	if got := snap.Provided(); len(got) != 1 || got[0].ID != used.ID {
		t.Fatalf("Provided() = %+v", got)
	}
	if got, want := snap.InsertsByBlueprint(), map[string]int{"task": 1, "project": 1, "user": 1, "company": 1}; !maps.Equal(got, want) {
		t.Fatalf("InsertsByBlueprint() = %v, want %v", got, want)
	}
	if got, want := snap.InsertsByTable()["companies"], 1; got != want {
		t.Fatalf("InsertsByTable()[companies] = %d, want %d", got, want)
	}
	if got := snap.Nodes()[len(snap.Nodes())-1].Path; got != "task" {
		t.Fatalf("last node in insert order = %s, want task", got)
	}
}

func TestPlanSnapshot_CollectionReasons(t *testing.T) {
	// Arrange
	setupManyToManyBlueprints(t)
	setupHasMany := func(t *testing.T) *seedling.Plan[Department] {
		t.Helper()
		setupHasManyBlueprints(t)
		return build[Department](t, seedling.Ref("employees", seedling.Count(3)))
	}

	// Act
	articles := build[Article](t, seedling.Ref("tags", seedling.Count(2))).Snapshot()
	departments := setupHasMany(t).Snapshot()

	// Assert
	nodes := snapshotPaths(articles)
	if tag := nodes["article.tags[1]"]; tag.Reason != seedling.ReasonManyToMany || tag.Relation != "tags" {
		t.Fatalf("tag = %+v (paths %v)", tag, slices.Sorted(maps.Keys(nodes)))
	}
	if join := nodes["article.tags[1].through"]; join.Reason != seedling.ReasonThrough || join.Blueprint != "article_tag" || len(join.DependsOn) != 2 {
		t.Fatalf("join = %+v", join)
	}
	if got := departments.InsertsByBlueprint()["employee"]; got != 3 {
		t.Fatalf("employees = %d, want 3", got)
	}
	if employee := snapshotPaths(departments)["department.employees[2]"]; employee.Reason != seedling.ReasonHasMany || employee.Shared {
		t.Fatalf("employee = %+v", employee)
	}
	if got := departments.Shared(); len(got) != 0 {
		t.Fatalf("a parent with has-many children is not shared: %+v", got)
	}
}

func TestBuildMany_PlansBatchWithoutInserting(t *testing.T) {
	// Arrange
	counts := map[string]int{}
	reg := newScenarioRegistry(t, counts, "")

	// Act
	plan := seedling.NewSession[Task](reg).BuildMany(t, 3, seedling.Ref("project", seedling.Set("Name", "shared")))

	// Assert
	if len(counts) != 0 {
		t.Fatalf("BuildMany inserted records: %v", counts)
	}
	if plan.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", plan.Len())
	}
	snap := plan.Snapshot()
	shared := snap.Shared()
	if len(shared) != 2 {
		t.Fatalf("Shared() = %+v, want the shared project and assignee", shared)
	}
	if got, want := snap.InsertsByBlueprint(), map[string]int{"task": 3, "project": 1, "company": 2, "user": 1}; !maps.Equal(got, want) {
		t.Fatalf("InsertsByBlueprint() = %v, want %v", got, want)
	}
	// A shared parent keeps the path of the first record that reached it.
	paths := slices.Sorted(maps.Keys(snapshotPaths(snap)))
	if want := []string{"task[0]", "task[0].assignee", "task[0].assignee.company", "task[0].project", "task[0].project.company", "task[1]", "task[2]"}; !slices.Equal(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.DryRunString(), "INSERT") {
		t.Fatalf("DryRunString() = %q", plan.DryRunString())
	}
}

func TestBatchPlan_InsertRunsEachTime(t *testing.T) {
	// Arrange
	counts := map[string]int{}
	reg := newScenarioRegistry(t, counts, "")
	var after []int
	plan, err := seedling.NewSession[Company](reg).BuildManyE(2,
		seedling.Seq("Name", func(i int) string { return []string{"a", "b"}[i] }),
		seedling.AfterInsert(func(c Company, _ seedling.DBTX) { after = append(after, c.ID) }),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	first := plan.Insert(t, nil)
	second, err := plan.InsertE(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if counts["company"] != 4 {
		t.Fatalf("companies inserted = %d, want 4", counts["company"])
	}
	if got := []string{first.MustRootAt(0).Name, first.MustRootAt(1).Name}; !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("names = %v", got)
	}
	if first.MustRootAt(0).ID == second.MustRootAt(0).ID {
		t.Fatal("a reused plan inserted the same record twice")
	}
	if len(after) != 4 {
		t.Fatalf("AfterInsert ran %d times, want 4", len(after))
	}
}

func TestBuildMany_Empty(t *testing.T) {
	// Arrange
	reg := newScenarioRegistry(t, map[string]int{}, "")

	// Act
	empty := seedling.NewSession[Company](reg).BuildMany(t, 0)
	_, negErr := seedling.NewSession[Company](reg).BuildManyE(-1)

	// Assert
	if empty.Len() != 0 || empty.Snapshot().Len() != 0 {
		t.Fatalf("empty plan has %d records", empty.Snapshot().Len())
	}
	if result := empty.Insert(t, nil); result.Len() != 0 {
		t.Fatalf("empty plan inserted %d roots", result.Len())
	}
	if !errors.Is(negErr, seedling.ErrInvalidOption) {
		t.Fatalf("err = %v, want ErrInvalidOption", negErr)
	}
}

func TestMaxNodes_StopsExpansion(t *testing.T) {
	// Arrange
	counts := map[string]int{}
	reg := newScenarioRegistry(t, counts, "")
	sess := seedling.NewSession[Task](reg)

	// Act
	_, err := sess.InsertOneE(context.Background(), nil, seedling.MaxNodes(3))
	_, allowed := sess.BuildE(seedling.MaxNodes(5))
	_, batchErr := sess.BuildManyE(2, seedling.MaxNodes(5))

	// Assert
	if !errors.Is(err, seedling.ErrTooManyNodes) {
		t.Fatalf("err = %v, want ErrTooManyNodes", err)
	}
	if !strings.Contains(err.Error(), "MaxNodes(3)") || !strings.Contains(err.Error(), "task.") {
		t.Fatalf("err = %v, want the limit and the path of the record past it", err)
	}
	if len(counts) != 0 {
		t.Fatalf("records were inserted past the limit: %v", counts)
	}
	if allowed != nil {
		t.Fatalf("a plan at the limit failed: %v", allowed)
	}
	if !errors.Is(batchErr, seedling.ErrTooManyNodes) {
		t.Fatalf("batch err = %v, want the limit to cover the whole batch", batchErr)
	}
}

func TestMaxNodes_Placement(t *testing.T) {
	// Arrange
	reg := newScenarioRegistry(t, map[string]int{}, "")
	sess := seedling.NewSession[Task](reg)

	tests := []struct {
		name string
		opt  seedling.Option
	}{
		{name: "non-positive", opt: seedling.MaxNodes(0)},
		{name: "under ref", opt: seedling.Ref("project", seedling.MaxNodes(2))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := sess.BuildE(tt.opt)

			// Assert
			if !errors.Is(err, seedling.ErrInvalidOption) {
				t.Fatalf("err = %v, want ErrInvalidOption", err)
			}
		})
	}
}

func TestScenario_WithMaxNodes(t *testing.T) {
	// Arrange
	reg := newScenarioRegistry(t, map[string]int{}, "")
	s := seedling.NewScenario(reg)
	company := seedling.Add[Company](s)
	seedling.Add[Project](s, seedling.Link("company", company))
	seedling.Add[User](s, seedling.Link("company", company))

	// Act
	_, addErr := seedling.NewScenario(reg).BuildE()
	limited, err := s.WithMaxNodes(3).BuildE()
	_, tooSmall := s.WithMaxNodes(2).BuildE()
	other := seedling.NewScenario(reg)
	seedling.Add[Company](other, seedling.MaxNodes(1))
	_, rejectErr := other.BuildE()

	// Assert
	if addErr != nil {
		t.Fatal(addErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	snap := limited.Snapshot()
	if snap.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", snap.Len())
	}
	if shared := snap.Shared(); len(shared) != 1 || shared[0].Blueprint != "company" || shared[0].Path != "company[0]" {
		t.Fatalf("Shared() = %+v, want the linked company", shared)
	}
	if !errors.Is(tooSmall, seedling.ErrTooManyNodes) {
		t.Fatalf("err = %v, want ErrTooManyNodes", tooSmall)
	}
	if !errors.Is(rejectErr, seedling.ErrInvalidOption) {
		t.Fatalf("err = %v, want MaxNodes rejected in Add", rejectErr)
	}
}

func TestInsertFailedError_Path(t *testing.T) {
	// Arrange
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(t, reg, countingInserters(map[string]int{}, "company"))

	// Act
	_, err := seedling.NewSession[Task](reg).InsertOneE(context.Background(), nil)

	// Assert
	var ife *seedling.InsertFailedError
	if !errors.As(err, &ife) {
		t.Fatalf("err = %v, want InsertFailedError", err)
	}
	if ife.Blueprint() != "company" || !strings.HasPrefix(ife.Path(), "task.") || !strings.HasSuffix(ife.Path(), ".company") {
		t.Fatalf("blueprint = %s, path = %s", ife.Blueprint(), ife.Path())
	}
	if !strings.Contains(err.Error(), "at "+ife.Path()) {
		t.Fatalf("err = %v, want the path in the message", err)
	}
}

func TestPlanSnapshot_SharedCountsDistinctRecords(t *testing.T) {
	// Arrange
	setupManyToManyBlueprints(t)
	type person struct{ ID int }
	type review struct{ ID, AuthorID, ReviewerID int }
	reg := seedling.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	seedling.MustRegisterTo(reg, seedling.Blueprint[person]{
		Name: "person", PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v person) (person, error) { v.ID = ids.Next(); return v, nil },
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[review]{
		Name: "review", PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "author", Kind: seedling.BelongsTo, LocalField: "AuthorID", RefBlueprint: "person"},
			{Name: "reviewer", Kind: seedling.BelongsTo, LocalField: "ReviewerID", RefBlueprint: "person"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v review) (review, error) { v.ID = ids.Next(); return v, nil },
	})
	s := seedling.NewScenario(reg)
	someone := seedling.Add[person](s)
	seedling.Add[review](s, seedling.Link("author", someone), seedling.Link("reviewer", someone))

	// Act
	articles := build[Article](t, seedling.Ref("tags", seedling.Count(2))).Snapshot()
	scenario, err := s.BuildE()
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if shared := articles.Shared(); len(shared) != 0 {
		t.Fatalf("join records of the article's own relation made it shared: %+v", shared)
	}
	if shared := scenario.Snapshot().Shared(); len(shared) != 0 {
		t.Fatalf("a person linked twice by one review is not shared: %+v", shared)
	}
}

func TestMaxNodes_RejectsLargeCollectionsBeforeExpanding(t *testing.T) {
	// Arrange
	setupHasManyBlueprints(t)

	// Act
	_, refErr := buildE[Department](t, seedling.MaxNodes(5), seedling.Ref("employees", seedling.Count(1_000_000_000)))
	_, countErr := buildE[Department](t, seedling.MaxNodes(2))

	// Assert
	if !errors.Is(refErr, seedling.ErrTooManyNodes) {
		t.Fatalf("err = %v, want ErrTooManyNodes", refErr)
	}
	if !errors.Is(countErr, seedling.ErrTooManyNodes) || !strings.Contains(countErr.Error(), "department.employees") {
		t.Fatalf("err = %v, want ErrTooManyNodes at department.employees", countErr)
	}
}
