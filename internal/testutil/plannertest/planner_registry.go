package plannertest

import (
	"context"
	"fmt"
	"reflect"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/planner"
	"github.com/mhiro2/seedling/seedlingtest"
)

type PlannerRegistry struct {
	blueprints map[string]*planner.BlueprintDef
	types      map[reflect.Type]*planner.BlueprintDef
}

func NewPlannerRegistry() *PlannerRegistry {
	return &PlannerRegistry{
		blueprints: make(map[string]*planner.BlueprintDef),
		types:      make(map[reflect.Type]*planner.BlueprintDef),
	}
}

func (r *PlannerRegistry) LookupByName(name string) (*planner.BlueprintDef, error) {
	bp, ok := r.blueprints[name]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint %q: %w", name, errx.BlueprintNotFound(name))
	}
	return bp, nil
}

func (r *PlannerRegistry) LookupByType(t reflect.Type) (*planner.BlueprintDef, error) {
	bp, ok := r.types[t]
	if !ok {
		return nil, fmt.Errorf("lookup blueprint type %s: %w", t, errx.BlueprintNotFound(t.String()))
	}
	return bp, nil
}

func (r *PlannerRegistry) RegisterBasic() {
	r.register(&planner.BlueprintDef{
		Name:     "company",
		Table:    "companies",
		PKFields: []string{"ID"},
		Defaults: func() any {
			return seedlingtest.Company{Name: "test-company"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Company)
			value.ID = 1
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Company](),
	})

	r.register(&planner.BlueprintDef{
		Name:     "user",
		Table:    "users",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "company", Kind: planner.BelongsTo, LocalFields: []string{"CompanyID"}, RefBlueprint: "company", Required: true},
		},
		Defaults: func() any {
			return seedlingtest.User{Name: "test-user"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.User)
			value.ID = 2
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.User](),
	})

	r.register(&planner.BlueprintDef{
		Name:     "project",
		Table:    "projects",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "company", Kind: planner.BelongsTo, LocalFields: []string{"CompanyID"}, RefBlueprint: "company", Required: true},
		},
		Defaults: func() any {
			return seedlingtest.Project{Name: "test-project"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Project)
			value.ID = 3
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Project](),
	})

	r.register(&planner.BlueprintDef{
		Name:     "task",
		Table:    "tasks",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "project", Kind: planner.BelongsTo, LocalFields: []string{"ProjectID"}, RefBlueprint: "project", Required: true},
			{Name: "assignee", Kind: planner.BelongsTo, LocalFields: []string{"AssigneeUserID"}, RefBlueprint: "user", Required: true},
		},
		Defaults: func() any {
			return seedlingtest.Task{Title: "test-task", Status: "open"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Task)
			value.ID = 4
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Task](),
	})
}

func (r *PlannerRegistry) RegisterHasMany() {
	r.register(&planner.BlueprintDef{
		Name:     "department",
		Table:    "departments",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "employees", Kind: planner.HasMany, LocalFields: []string{"DepartmentID"}, RefBlueprint: "employee", Required: true, Count: 2},
		},
		Defaults: func() any {
			return seedlingtest.Department{Name: "engineering"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Department)
			value.ID = 5
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Department](),
	})

	r.register(&planner.BlueprintDef{
		Name:     "employee",
		Table:    "employees",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{Name: "department", Kind: planner.BelongsTo, LocalFields: []string{"DepartmentID"}, RefBlueprint: "department", Required: true},
		},
		Defaults: func() any {
			return seedlingtest.Employee{Name: "test-employee"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Employee)
			value.ID = 6
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Employee](),
	})
}

func (r *PlannerRegistry) RegisterManyToMany() {
	r.register(&planner.BlueprintDef{
		Name:     "article",
		Table:    "articles",
		PKFields: []string{"ID"},
		Relations: []planner.RelationDef{
			{
				Name:             "tags",
				Kind:             planner.ManyToMany,
				LocalFields:      []string{"ArticleID"},
				RemoteFields:     []string{"TagID"},
				RefBlueprint:     "tag",
				ThroughBlueprint: "article_tag",
				Required:         true,
				Count:            2,
			},
		},
		Defaults: func() any {
			return seedlingtest.Article{Title: "seedling"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Article)
			value.ID = 7
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Article](),
	})

	r.register(&planner.BlueprintDef{
		Name:     "tag",
		Table:    "tags",
		PKFields: []string{"ID"},
		Defaults: func() any {
			return seedlingtest.Tag{Name: "tag"}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			value := v.(seedlingtest.Tag)
			value.ID = 8
			return value, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.Tag](),
	})

	r.register(&planner.BlueprintDef{
		Name:     "article_tag",
		Table:    "article_tags",
		PKFields: []string{"ArticleID", "TagID"},
		Relations: []planner.RelationDef{
			{Name: "article", Kind: planner.BelongsTo, LocalFields: []string{"ArticleID"}, RefBlueprint: "article", Required: true},
			{Name: "tag", Kind: planner.BelongsTo, LocalFields: []string{"TagID"}, RefBlueprint: "tag", Required: true},
		},
		Defaults: func() any {
			return seedlingtest.ArticleTag{}
		},
		Insert: func(ctx context.Context, db, v any) (any, error) {
			return v, nil
		},
		ModelType: reflect.TypeFor[seedlingtest.ArticleTag](),
	})
}

func (r *PlannerRegistry) register(bp *planner.BlueprintDef) {
	r.blueprints[bp.Name] = bp
	r.types[bp.ModelType] = bp
}
