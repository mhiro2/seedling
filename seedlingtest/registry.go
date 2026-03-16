package seedlingtest

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/mhiro2/seedling"
)

type IDSequence struct {
	next atomic.Int64
}

func NewIDSequence() *IDSequence {
	return &IDSequence{}
}

func (s *IDSequence) Next() int {
	return int(s.next.Add(1))
}

func NewRegistry() *seedling.Registry {
	return seedling.NewRegistry()
}

type BasicInserters struct {
	Company func(context.Context, seedling.DBTX, Company) (Company, error)
	User    func(context.Context, seedling.DBTX, User) (User, error)
	Project func(context.Context, seedling.DBTX, Project) (Project, error)
	Task    func(context.Context, seedling.DBTX, Task) (Task, error)
}

func DefaultBasicInserters(ids *IDSequence) BasicInserters {
	return BasicInserters{
		Company: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
			v.ID = ids.Next()
			return v, nil
		},
		User: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Project: func(ctx context.Context, db seedling.DBTX, v Project) (Project, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Task: func(ctx context.Context, db seedling.DBTX, v Task) (Task, error) {
			v.ID = ids.Next()
			return v, nil
		},
	}
}

func RegisterBasic(tb testing.TB, reg *seedling.Registry, inserters BasicInserters) {
	tb.Helper()
	requireBasicInserters(tb, inserters)

	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name:    "company",
		Table:   "companies",
		PKField: "ID",
		Defaults: func() Company {
			return Company{Name: "test-company"}
		},
		Insert: inserters.Company,
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
		Insert: inserters.User,
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
		Insert: inserters.Project,
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
			{Name: "assignee", Kind: seedling.BelongsTo, LocalField: "AssigneeUserID", RefBlueprint: "user"},
		},
		Insert: inserters.Task,
	})
}

type HasManyInserters struct {
	Department func(context.Context, seedling.DBTX, Department) (Department, error)
	Employee   func(context.Context, seedling.DBTX, Employee) (Employee, error)
}

func DefaultHasManyInserters(ids *IDSequence) HasManyInserters {
	return HasManyInserters{
		Department: func(ctx context.Context, db seedling.DBTX, v Department) (Department, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Employee: func(ctx context.Context, db seedling.DBTX, v Employee) (Employee, error) {
			v.ID = ids.Next()
			return v, nil
		},
	}
}

func RegisterHasMany(tb testing.TB, reg *seedling.Registry, inserters HasManyInserters) {
	tb.Helper()
	if inserters.Department == nil || inserters.Employee == nil {
		tb.Fatal("seedlingtest: all has-many inserters must be provided")
	}

	seedling.MustRegisterTo(reg, seedling.Blueprint[Department]{
		Name:    "department",
		Table:   "departments",
		PKField: "ID",
		Defaults: func() Department {
			return Department{Name: "engineering"}
		},
		Relations: []seedling.Relation{
			{Name: "employees", Kind: seedling.HasMany, LocalField: "DepartmentID", RefBlueprint: "employee", Count: 2},
		},
		Insert: inserters.Department,
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
		Insert: inserters.Employee,
	})
}

type CompositePKInserters struct {
	Region     func(context.Context, seedling.DBTX, Region) (Region, error)
	Deployment func(context.Context, seedling.DBTX, Deployment) (Deployment, error)
}

func DefaultCompositePKInserters(ids *IDSequence) CompositePKInserters {
	return CompositePKInserters{
		Region: func(ctx context.Context, db seedling.DBTX, v Region) (Region, error) {
			if v.Code == "" {
				v.Code = fmt.Sprintf("region-%d", ids.Next())
			}
			if v.Number == 0 {
				v.Number = ids.Next()
			}
			return v, nil
		},
		Deployment: func(ctx context.Context, db seedling.DBTX, v Deployment) (Deployment, error) {
			v.ID = ids.Next()
			return v, nil
		},
	}
}

func RegisterCompositePK(tb testing.TB, reg *seedling.Registry, inserters CompositePKInserters) {
	tb.Helper()
	if inserters.Region == nil || inserters.Deployment == nil {
		tb.Fatal("seedlingtest: all composite PK inserters must be provided")
	}

	seedling.MustRegisterTo(reg, seedling.Blueprint[Region]{
		Name:     "region",
		Table:    "regions",
		PKFields: []string{"Code", "Number"},
		Defaults: func() Region {
			return Region{Name: "tokyo"}
		},
		Insert: inserters.Region,
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[Deployment]{
		Name:    "deployment",
		Table:   "deployments",
		PKField: "ID",
		Defaults: func() Deployment {
			return Deployment{Name: "deployment"}
		},
		Relations: []seedling.Relation{
			{
				Name:         "region",
				Kind:         seedling.BelongsTo,
				LocalFields:  []string{"RegionCode", "RegionNumber"},
				RefBlueprint: "region",
			},
		},
		Insert: inserters.Deployment,
	})
}

type ManyToManyInserters struct {
	Article    func(context.Context, seedling.DBTX, Article) (Article, error)
	Tag        func(context.Context, seedling.DBTX, Tag) (Tag, error)
	ArticleTag func(context.Context, seedling.DBTX, ArticleTag) (ArticleTag, error)
}

func DefaultManyToManyInserters(ids *IDSequence) ManyToManyInserters {
	return ManyToManyInserters{
		Article: func(ctx context.Context, db seedling.DBTX, v Article) (Article, error) {
			v.ID = ids.Next()
			return v, nil
		},
		Tag: func(ctx context.Context, db seedling.DBTX, v Tag) (Tag, error) {
			v.ID = ids.Next()
			return v, nil
		},
		ArticleTag: func(ctx context.Context, db seedling.DBTX, v ArticleTag) (ArticleTag, error) {
			return v, nil
		},
	}
}

func RegisterManyToMany(tb testing.TB, reg *seedling.Registry, inserters ManyToManyInserters) {
	tb.Helper()
	if inserters.Article == nil || inserters.Tag == nil || inserters.ArticleTag == nil {
		tb.Fatal("seedlingtest: all many-to-many inserters must be provided")
	}

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
			},
		},
		Insert: inserters.Article,
	})

	seedling.MustRegisterTo(reg, seedling.Blueprint[Tag]{
		Name:    "tag",
		Table:   "tags",
		PKField: "ID",
		Defaults: func() Tag {
			return Tag{Name: "tag"}
		},
		Insert: inserters.Tag,
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
		Insert: inserters.ArticleTag,
	})
}

func requireBasicInserters(tb testing.TB, inserters BasicInserters) {
	tb.Helper()
	if inserters.Company == nil || inserters.User == nil || inserters.Project == nil || inserters.Task == nil {
		tb.Fatal("seedlingtest: all basic inserters must be provided")
	}
}
