package seedling_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

// countingInserters counts inserts per blueprint and can fail one blueprint.
func countingInserters(counts map[string]int, failOn string) seedlingtest.BasicInserters {
	ids := seedlingtest.NewIDSequence()
	insert := func(name string) error {
		if name == failOn {
			return errors.New("insert failed")
		}
		counts[name]++
		return nil
	}
	return seedlingtest.BasicInserters{
		Company: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, insert("company")
		},
		User: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, insert("user")
		},
		Project: func(_ context.Context, _ seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, insert("project")
		},
		Task: func(_ context.Context, _ seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, insert("task")
		},
	}
}

func newScenarioRegistry(t *testing.T, counts map[string]int, failOn string) *seedling.Registry {
	t.Helper()
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(t, reg, countingInserters(counts, failOn))
	return reg
}

func TestScenario_LinkSharesRecordAcrossPaths(t *testing.T) {
	t.Parallel()

	// Arrange
	counts := map[string]int{}
	s := seedling.NewScenario(newScenarioRegistry(t, counts, ""))
	company := seedling.Add[Company](s, seedling.Set("Name", "acme"))
	project := seedling.Add[Project](s, seedling.Link("company", company))
	assignee := seedling.Add[User](s, seedling.Link("company", company), seedling.Set("Name", "alice"))
	reviewer := seedling.Add[User](s, seedling.Link("company", company), seedling.Set("Name", "bob"))
	task := seedling.Add[Task](s,
		seedling.Link("project", project),
		seedling.Link("assignee", assignee),
	)

	// Act
	result := s.Insert(t, nil)

	// Assert
	if counts["company"] != 1 || counts["project"] != 1 || counts["user"] != 2 || counts["task"] != 1 {
		t.Fatalf("got insert counts %v, want one company, project, and task and two users", counts)
	}
	gotCompany := company.Get(result)
	if gotCompany.Name != "acme" {
		t.Fatalf("got company name %q, want %q", gotCompany.Name, "acme")
	}
	if got := project.Get(result).CompanyID; got != gotCompany.ID {
		t.Fatalf("got project CompanyID %d, want %d", got, gotCompany.ID)
	}
	gotAssignee, gotReviewer := assignee.Get(result), reviewer.Get(result)
	if gotAssignee.Name != "alice" || gotReviewer.Name != "bob" {
		t.Fatalf("got users %q and %q, want alice and bob by role", gotAssignee.Name, gotReviewer.Name)
	}
	if gotAssignee.CompanyID != gotCompany.ID || gotReviewer.CompanyID != gotCompany.ID {
		t.Fatalf("got user companies %d and %d, want %d", gotAssignee.CompanyID, gotReviewer.CompanyID, gotCompany.ID)
	}
	gotTask := task.Get(result)
	if gotTask.ProjectID != project.Get(result).ID || gotTask.AssigneeUserID != gotAssignee.ID {
		t.Fatalf("got task %+v, want it linked to project %d and assignee %d", gotTask, project.Get(result).ID, gotAssignee.ID)
	}
}

func TestScenario_LinkUnderRef(t *testing.T) {
	t.Parallel()

	// Arrange
	counts := map[string]int{}
	s := seedling.NewScenario(newScenarioRegistry(t, counts, ""))
	company := seedling.Add[Company](s)
	task := seedling.Add[Task](s,
		seedling.Ref("project", seedling.Link("company", company)),
		seedling.Ref("assignee", seedling.Link("company", company)),
	)

	// Act
	result := s.Insert(t, nil)

	// Assert
	if counts["company"] != 1 {
		t.Fatalf("got %d companies, want 1", counts["company"])
	}
	companyID := company.Get(result).ID
	project, err := seedling.NodesAs[Project](result, "project")
	if err != nil || len(project) != 1 || project[0].CompanyID != companyID {
		t.Fatalf("got projects %v (err %v), want one linked to company %d", project, err, companyID)
	}
	if task.Get(result).ProjectID != project[0].ID {
		t.Fatalf("got task ProjectID %d, want %d", task.Get(result).ProjectID, project[0].ID)
	}
}

func TestScenario_RecordsDoNotShareImplicitly(t *testing.T) {
	t.Parallel()

	// Arrange
	counts := map[string]int{}
	s := seedling.NewScenario(newScenarioRegistry(t, counts, ""))
	seedling.Add[User](s)
	seedling.Add[User](s)

	// Act
	s.Insert(t, nil)

	// Assert
	if counts["company"] != 2 {
		t.Fatalf("got %d companies, want 2", counts["company"])
	}
}

func TestScenario_AfterInsertRunsPerRecord(t *testing.T) {
	t.Parallel()

	// Arrange
	var got []string
	s := seedling.NewScenario(newScenarioRegistry(t, map[string]int{}, ""))
	seedling.Add[Company](s, seedling.AfterInsert(func(c Company, _ seedling.DBTX) {
		got = append(got, "company:"+c.Name)
	}))
	seedling.Add[User](s, seedling.AfterInsertE(func(u User, _ seedling.DBTX) error {
		got = append(got, "user:"+u.Name)
		return nil
	}))

	// Act
	s.Insert(t, nil)

	// Assert
	if strings.Join(got, ",") != "company:test-company,user:test-user" {
		t.Fatalf("got callbacks %v", got)
	}
}

func TestScenario_AfterInsertError(t *testing.T) {
	t.Parallel()

	// Arrange
	wantErr := errors.New("callback failed")
	s := seedling.NewScenario(newScenarioRegistry(t, map[string]int{}, ""))
	company := seedling.Add[Company](s, seedling.AfterInsertE(func(Company, seedling.DBTX) error { return wantErr }))

	// Act
	result, err := s.InsertE(t.Context(), nil)

	// Assert
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
	if _, ok := company.Lookup(result); !ok {
		t.Fatal("expected the inserted company in the result")
	}
}

func TestScenario_PartialResult(t *testing.T) {
	t.Parallel()

	// Arrange
	counts := map[string]int{}
	s := seedling.NewScenario(newScenarioRegistry(t, counts, "task"))
	company := seedling.Add[Company](s)
	task := seedling.Add[Task](s)

	// Act
	result, err := s.InsertE(t.Context(), nil)

	// Assert
	if !errors.Is(err, seedling.ErrInsertFailed) {
		t.Fatalf("got %v, want %v", err, seedling.ErrInsertFailed)
	}
	if _, ok := company.Lookup(result); !ok {
		t.Fatal("expected the company in the partial result")
	}
	if _, ok := task.Lookup(result); ok {
		t.Fatal("expected the failed task to be missing")
	}
}

func TestScenario_LookupRejectsOtherScenario(t *testing.T) {
	t.Parallel()

	// Arrange
	reg := newScenarioRegistry(t, map[string]int{}, "")
	first := seedling.NewScenario(reg)
	company := seedling.Add[Company](first)
	other := seedling.NewScenario(reg)
	seedling.Add[Company](other)

	// Act
	result := other.Insert(t, nil)

	// Assert
	if _, ok := company.Lookup(result); ok {
		t.Fatal("expected a node of another scenario not to resolve")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected Get to panic")
		}
	}()
	company.Get(result)
}

func TestScenario_PlanDiagnosticsAndCleanup(t *testing.T) {
	t.Parallel()

	// Arrange
	var deleted []string
	reg := seedling.NewRegistry()
	ids := seedlingtest.NewIDSequence()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", Table: "companies", PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, v Company) error {
			deleted = append(deleted, "company")
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name: "user", Table: "users", PKField: "ID",
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Delete: func(_ context.Context, _ seedling.DBTX, v User) error {
			deleted = append(deleted, "user")
			return nil
		},
	})
	s := seedling.NewScenario(reg)
	company := seedling.Add[Company](s)
	seedling.Add[User](s, seedling.Link("company", company))

	// Act
	plan := s.Build(t)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	dryRun := plan.DryRunString()
	result := plan.Insert(t, nil)
	result.Cleanup(t, nil)

	// Assert
	if strings.Count(dryRun, "INSERT INTO companies") != 1 || strings.Count(dryRun, "INSERT INTO users") != 1 {
		t.Fatalf("got dry run:\n%s", dryRun)
	}
	if !strings.Contains(plan.DebugString(), "user") || !strings.Contains(result.DebugString(), "company") {
		t.Fatalf("got debug output:\n%s\n%s", plan.DebugString(), result.DebugString())
	}
	if strings.Join(deleted, ",") != "user,company" {
		t.Fatalf("got delete order %v, want user then company", deleted)
	}
}

func TestScenario_Errors(t *testing.T) {
	t.Parallel()

	reg := newScenarioRegistry(t, map[string]int{}, "")

	t.Run("link outside scenario", func(t *testing.T) {
		t.Parallel()
		s := seedling.NewScenario(reg)
		company := seedling.Add[Company](s)
		_, err := seedling.NewSession[User](reg).BuildE(seedling.Link("company", company))
		if !errors.Is(err, seedling.ErrInvalidOption) {
			t.Fatalf("got %v, want %v", err, seedling.ErrInvalidOption)
		}
	})

	tests := []struct {
		name    string
		add     func(s, other *seedling.Scenario)
		wantErr error
	}{
		{name: "zero node", wantErr: seedling.ErrInvalidOption, add: func(s, _ *seedling.Scenario) {
			seedling.Add[User](s, seedling.Link("company", seedling.Node[Company]{}))
		}},
		{name: "other scenario", wantErr: seedling.ErrInvalidOption, add: func(s, other *seedling.Scenario) {
			seedling.Add[User](s, seedling.Link("company", seedling.Add[Company](other)))
		}},
		{name: "wrong blueprint", wantErr: seedling.ErrTypeMismatch, add: func(s, _ *seedling.Scenario) {
			project := seedling.Add[Project](s)
			seedling.Add[User](s, seedling.Link("company", project))
		}},
		{name: "unknown relation", wantErr: seedling.ErrRelationNotFound, add: func(s, _ *seedling.Scenario) {
			seedling.Add[User](s, seedling.Link("missing", seedling.Add[Company](s)))
		}},
		{name: "link and use", wantErr: seedling.ErrInvalidOption, add: func(s, _ *seedling.Scenario) {
			company := seedling.Add[Company](s)
			seedling.Add[User](s, seedling.Link("company", company), seedling.Use("company", Company{ID: 1}))
		}},
		{name: "link and ref", wantErr: seedling.ErrInvalidOption, add: func(s, _ *seedling.Scenario) {
			company := seedling.Add[Company](s)
			seedling.Add[User](s, seedling.Link("company", company), seedling.Ref("company"))
		}},
		{name: "only excludes link", wantErr: seedling.ErrInvalidOption, add: func(s, _ *seedling.Scenario) {
			company := seedling.Add[Company](s)
			seedling.Add[User](s, seedling.Only(), seedling.Link("company", company))
		}},
		{name: "with context", wantErr: seedling.ErrInvalidOption, add: func(s, _ *seedling.Scenario) {
			seedling.Add[Company](s, seedling.WithContext(context.Background()))
		}},
		{name: "with insert log", wantErr: seedling.ErrInvalidOption, add: func(s, _ *seedling.Scenario) {
			seedling.Add[Company](s, seedling.WithInsertLog(func(seedling.InsertLog) {}))
		}},
		{name: "wrong blueprint under skipped relation", wantErr: seedling.ErrTypeMismatch, add: func(s, _ *seedling.Scenario) {
			project := seedling.Add[Project](s)
			seedling.Add[Task](s,
				seedling.When("assignee", func(Task) bool { return false }),
				seedling.Ref("assignee", seedling.Link("company", project)),
			)
		}},
		{name: "unregistered type", wantErr: seedling.ErrBlueprintNotFound, add: func(s, _ *seedling.Scenario) {
			seedling.Add[Region](s)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, other := seedling.NewScenario(reg), seedling.NewScenario(reg)
			tt.add(s, other)
			if _, err := s.BuildE(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestScenario_AddCopiesOptions(t *testing.T) {
	t.Parallel()

	// Arrange
	s := seedling.NewScenario(newScenarioRegistry(t, map[string]int{}, ""))
	opts := []seedling.Option{seedling.Set("Name", "first")}
	first := seedling.Add[Company](s, opts...)
	opts[0] = seedling.Set("Name", "second")
	second := seedling.Add[Company](s, opts...)

	// Act
	result := s.Insert(t, nil)

	// Assert
	if got := first.Get(result).Name; got != "first" {
		t.Fatalf("got first name %q, want %q", got, "first")
	}
	if got := second.Get(result).Name; got != "second" {
		t.Fatalf("got second name %q, want %q", got, "second")
	}
}
