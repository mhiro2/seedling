package seedling_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/faker"
)

type ExCompany struct {
	ID   int
	Name string
}

type ExUser struct {
	ID        int
	CompanyID int
	Name      string
}

type ExProject struct {
	ID        int
	CompanyID int
	Name      string
}

type ExTask struct {
	ID             int
	ProjectID      int
	AssigneeUserID int
	Title          string
	Status         string
}

func setupExampleBlueprints() {
	seedling.ResetRegistry()

	nextID := 0
	next := func() int {
		nextID++
		return nextID
	}

	seedling.MustRegister(seedling.Blueprint[ExCompany]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() ExCompany {
			return ExCompany{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ExCompany) (ExCompany, error) {
			v.ID = next()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[ExUser]{
		Name:    "user",
		Table:   "users",
		PKField: "ID",
		Defaults: func() ExUser {
			return ExUser{Name: "test-user"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Traits: map[string][]seedling.Option{
			"named": {seedling.Set("Name", "trait-user")},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ExUser) (ExUser, error) {
			v.ID = next()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[ExProject]{
		Name:    "project",
		Table:   "projects",
		PKField: "ID",
		Defaults: func() ExProject {
			return ExProject{Name: "test-project"}
		},
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ExProject) (ExProject, error) {
			v.ID = next()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[ExTask]{
		Name:    "task",
		Table:   "tasks",
		PKField: "ID",
		Defaults: func() ExTask {
			return ExTask{Title: "test-task", Status: "open"}
		},
		Relations: []seedling.Relation{
			{Name: "project", Kind: seedling.BelongsTo, LocalField: "ProjectID", RefBlueprint: "project"},
			{
				Name:         "assignee",
				Kind:         seedling.BelongsTo,
				LocalField:   "AssigneeUserID",
				RefBlueprint: "user",
				When: seedling.WhenFunc(func(t ExTask) bool {
					return t.Status == "assigned"
				}),
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v ExTask) (ExTask, error) {
			v.ID = next()
			return v, nil
		},
	})
}

func ExampleInsertOne() {
	setupExampleBlueprints()

	t := &testing.T{}
	task := seedling.InsertOne[ExTask](t, nil).Root()
	fmt.Printf("%s %d\n", task.Title, task.ProjectID)
	// Output: test-task 2
}

func ExampleInsertOneE() {
	setupExampleBlueprints()

	result, err := seedling.InsertOneE[ExTask](context.Background(), nil,
		seedling.Set("Status", "assigned"),
	)
	if err != nil {
		return
	}

	fmt.Println(result.Root().AssigneeUserID > 0)
	// Output: true
}

func ExampleInsertMany() {
	setupExampleBlueprints()

	t := &testing.T{}
	companies := seedling.InsertMany[ExCompany](t, nil, 3,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("company-%d", i)
		}),
	)
	fmt.Printf("%s, %s, %s\n", companies[0].Name, companies[1].Name, companies[2].Name)
	// Output: company-0, company-1, company-2
}

func ExampleInsertManyE() {
	setupExampleBlueprints()

	result, err := seedling.InsertManyE[ExTask](context.Background(), nil, 2,
		seedling.Ref("project", seedling.Set("Name", "shared-project")),
	)
	if err != nil {
		return
	}

	node0, ok0 := result.NodeAt(0, "project")
	if !ok0 {
		return
	}
	project0, ok := node0.Value().(ExProject)
	if !ok {
		return
	}

	node1, ok1 := result.NodeAt(1, "project")
	if !ok1 {
		return
	}
	project1, ok := node1.Value().(ExProject)
	if !ok {
		return
	}

	fmt.Printf("%s %t\n", project0.Name, project0.ID == project1.ID)
	// Output: shared-project true
}

func ExampleBuild() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExTask](t)
	fmt.Println(plan.DebugString())
	// Output:
	// task
	// └─ project
	//    └─ company
}

func ExampleBuildE() {
	setupExampleBlueprints()

	plan, err := seedling.BuildE[ExTask](seedling.Set("Status", "assigned"))
	if err != nil {
		return
	}

	fmt.Println(plan.DebugString())
	// Output:
	// task (Set: Status)
	// ├─ user
	// │  └─ company
	// └─ project
	//    └─ company
}

func ExamplePlan_DryRunString() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExTask](t,
		seedling.Use("project", ExProject{ID: 42, CompanyID: 7, Name: "existing-project"}),
	)
	fmt.Println(plan.DryRunString())
	// Output:
	// Step 1: SKIP projects (provided) (blueprint: project)
	// Step 2: INSERT INTO tasks (blueprint: task)
	//         SET ProjectID ← projects.ID
}

func ExampleResult_Node() {
	setupExampleBlueprints()

	t := &testing.T{}
	result := seedling.InsertOne[ExTask](t, nil)
	node, ok := result.Node("project")
	if !ok {
		return
	}

	project, ok := node.Value().(ExProject)
	if !ok {
		return
	}
	fmt.Printf("%s %s\n", node.Name(), project.Name)
	// Output: project test-project
}

func ExampleBatchResult_NodeAt() {
	setupExampleBlueprints()

	result, err := seedling.InsertManyE[ExTask](context.Background(), nil, 2,
		seedling.SeqRef("project", func(i int) []seedling.Option {
			return []seedling.Option{seedling.Set("Name", fmt.Sprintf("project-%d", i))}
		}),
	)
	if err != nil {
		return
	}

	node, ok := result.NodeAt(1, "project")
	if !ok {
		return
	}

	project, ok := node.Value().(ExProject)
	if !ok {
		return
	}
	fmt.Println(project.Name)
	// Output: project-1
}

func ExampleSet() {
	setupExampleBlueprints()

	t := &testing.T{}
	project := seedling.InsertOne[ExProject](t, nil,
		seedling.Set("Name", "custom-project"),
	).Root()
	fmt.Println(project.Name)
	// Output: custom-project
}

func ExampleRef() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExTask](t,
		seedling.Ref("project", seedling.Set("Name", "custom-project")),
	)
	result := plan.Insert(t, nil)
	project, ok, err := seedling.NodeAs[ExProject](result, "project")
	if err != nil || !ok {
		return
	}
	fmt.Println(project.Name)
	// Output: custom-project
}

func ExampleUse() {
	setupExampleBlueprints()

	t := &testing.T{}
	project := seedling.InsertOne[ExProject](t, nil,
		seedling.Set("Name", "existing-project"),
	).Root()

	task := seedling.InsertOne[ExTask](t, nil,
		seedling.Use("project", project),
	).Root()
	fmt.Println(task.ProjectID)
	// Output: 2
}

func ExampleSeqRef() {
	setupExampleBlueprints()

	result, err := seedling.InsertManyE[ExTask](context.Background(), nil, 2,
		seedling.SeqRef("project", func(i int) []seedling.Option {
			return []seedling.Option{seedling.Set("Name", fmt.Sprintf("project-%d", i))}
		}),
	)
	if err != nil {
		return
	}

	node0, ok0 := result.NodeAt(0, "project")
	if !ok0 {
		return
	}
	project0, ok := node0.Value().(ExProject)
	if !ok {
		return
	}

	node1, ok1 := result.NodeAt(1, "project")
	if !ok1 {
		return
	}
	project1, ok := node1.Value().(ExProject)
	if !ok {
		return
	}
	fmt.Printf("%s, %s\n", project0.Name, project1.Name)
	// Output: project-0, project-1
}

func ExampleSeqUse() {
	setupExampleBlueprints()

	t := &testing.T{}
	companies := seedling.InsertMany[ExCompany](t, nil, 2,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("company-%d", i)
		}),
	)

	users := seedling.InsertMany[ExUser](t, nil, 2,
		seedling.SeqUse("company", func(i int) ExCompany {
			return companies[i]
		}),
	)
	fmt.Printf("%d, %d\n", users[0].CompanyID, users[1].CompanyID)
	// Output: 1, 2
}

func ExampleWith() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.With(func(u *ExUser) {
			u.Name = "modified-user"
		}),
	).Root()
	fmt.Println(user.Name)
	// Output: modified-user
}

func ExampleBlueprintTrait() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil, seedling.BlueprintTrait("named")).Root()
	fmt.Println(user.Name)
	// Output: trait-user
}

func ExampleInlineTrait() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.InlineTrait(seedling.Set("Name", "inline-user")),
	).Root()
	fmt.Println(user.Name)
	// Output: inline-user
}

func ExampleFor() {
	setupExampleBlueprints()

	t := &testing.T{}
	result := seedling.For[ExTask]().
		Set("Title", "builder-task").
		Ref("project", seedling.Set("Name", "builder-project")).
		Insert(t, nil)

	project, ok, err := seedling.NodeAs[ExProject](result, "project")
	if err != nil || !ok {
		return
	}
	fmt.Printf("%s %s\n", result.Root().Title, project.Name)
	// Output: builder-task builder-project
}

func ExampleOnly() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExTask](t,
		seedling.Set("Status", "assigned"),
		seedling.Only("project"),
	)
	fmt.Println(plan.DebugString())
	// Output:
	// task (Set: Status)
	// └─ project
	//    └─ company
}

func ExampleOnly_rootOnly() {
	setupExampleBlueprints()

	t := &testing.T{}
	plan := seedling.Build[ExTask](t, seedling.Only())
	fmt.Println(plan.DebugString())
	// Output:
	// task
}

func ExampleWhen() {
	setupExampleBlueprints()

	t := &testing.T{}
	task := seedling.InsertOne[ExTask](t, nil,
		seedling.Set("Status", "assigned"),
	).Root()
	fmt.Println(task.AssigneeUserID > 0)
	// Output: true
}

func ExampleWithInsertLog() {
	setupExampleBlueprints()

	t := &testing.T{}
	var logs []seedling.InsertLog
	seedling.InsertOne[ExTask](t, nil,
		seedling.WithInsertLog(func(log seedling.InsertLog) {
			logs = append(logs, log)
		}),
	)

	last := logs[len(logs)-1]
	fmt.Printf("%d %s %s\n", len(logs), last.Blueprint, last.FKBindings[0].ChildField)
	// Output: 3 task ProjectID
}

func ExampleAfterInsert() {
	setupExampleBlueprints()

	t := &testing.T{}
	var names []string
	seedling.InsertOne[ExCompany](t, nil,
		seedling.AfterInsert(func(c ExCompany, db seedling.DBTX) {
			names = append(names, c.Name)
		}),
	)
	fmt.Println(names[0])
	// Output: test-company
}

func ExampleGenerate() {
	setupExampleBlueprints()

	t := &testing.T{}
	user := seedling.InsertOne[ExUser](t, nil,
		seedling.WithSeed(42),
		seedling.Generate(func(r *rand.Rand, u *ExUser) {
			f := faker.New(r)
			u.Name = f.Name()
		}),
	).Root()
	fmt.Println(user.Name)
	// Output: Amanda Sanders
}
