package reuseparent

import (
	"context"
	"sync/atomic"

	"github.com/mhiro2/seedling"
)

var idSeq atomic.Int64

func nextID() int {
	return int(idSeq.Add(1))
}

// SetupBlueprints registers Company, Project, and Task blueprints.
func SetupBlueprints() {
	seedling.MustRegister(seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[Project]{
		Name:    "project",
		Table:   "projects",
		PKField: "ID",
		Defaults: func() Project {
			return Project{Name: "test-project"}
		},
		Relations: []seedling.Relation{
			{
				Name:         "company",
				Kind:         seedling.BelongsTo,
				LocalField:   "CompanyID",
				RefBlueprint: "company",
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = nextID()
			return v, nil
		},
	})

	seedling.MustRegister(seedling.Blueprint[Task]{
		Name:    "task",
		Table:   "tasks",
		PKField: "ID",
		Defaults: func() Task {
			return Task{Title: "test-task"}
		},
		Relations: []seedling.Relation{
			{
				Name:         "project",
				Kind:         seedling.BelongsTo,
				LocalField:   "ProjectID",
				RefBlueprint: "project",
			},
		},
		Insert: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = nextID()
			return v, nil
		},
	})
}
