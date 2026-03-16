//go:build integration

package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

type (
	company    = seedlingtest.Company
	user       = seedlingtest.User
	project    = seedlingtest.Project
	task       = seedlingtest.Task
	department = seedlingtest.Department
	employee   = seedlingtest.Employee
	region     = seedlingtest.Region
	deployment = seedlingtest.Deployment
	article    = seedlingtest.Article
	tag        = seedlingtest.Tag
	articleTag = seedlingtest.ArticleTag
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func registerBlueprints(tb testing.TB) *seedling.Registry {
	tb.Helper()

	reg := seedlingtest.NewRegistry()
	ids := seedlingtest.NewIDSequence()

	seedlingtest.RegisterBasic(tb, reg, seedlingtest.BasicInserters{
		Company: func(ctx context.Context, db seedling.DBTX, v company) (company, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO companies (name) VALUES (?)`, v.Name)
			if err != nil {
				return company{}, err
			}
			v.ID = id
			return v, nil
		},
		User: func(ctx context.Context, db seedling.DBTX, v user) (user, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO users (company_id, name) VALUES (?, ?)`, v.CompanyID, v.Name)
			if err != nil {
				return user{}, err
			}
			v.ID = id
			return v, nil
		},
		Project: func(ctx context.Context, db seedling.DBTX, v project) (project, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO projects (company_id, name) VALUES (?, ?)`, v.CompanyID, v.Name)
			if err != nil {
				return project{}, err
			}
			v.ID = id
			return v, nil
		},
		Task: func(ctx context.Context, db seedling.DBTX, v task) (task, error) {
			id, err := insertID(
				ctx,
				db.(sqlExecutor),
				`INSERT INTO tasks (project_id, assignee_user_id, title, status) VALUES (?, ?, ?, ?)`,
				v.ProjectID,
				v.AssigneeUserID,
				v.Title,
				v.Status,
			)
			if err != nil {
				return task{}, err
			}
			v.ID = id
			return v, nil
		},
	})

	seedlingtest.RegisterHasMany(tb, reg, seedlingtest.HasManyInserters{
		Department: func(ctx context.Context, db seedling.DBTX, v department) (department, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO departments (name) VALUES (?)`, v.Name)
			if err != nil {
				return department{}, err
			}
			v.ID = id
			return v, nil
		},
		Employee: func(ctx context.Context, db seedling.DBTX, v employee) (employee, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO employees (department_id, name) VALUES (?, ?)`, v.DepartmentID, v.Name)
			if err != nil {
				return employee{}, err
			}
			v.ID = id
			return v, nil
		},
	})

	seedlingtest.RegisterCompositePK(tb, reg, seedlingtest.CompositePKInserters{
		Region: func(ctx context.Context, db seedling.DBTX, v region) (region, error) {
			if v.Code == "" {
				v.Code = fmt.Sprintf("region-%d", ids.Next())
			}
			if v.Number == 0 {
				v.Number = ids.Next()
			}
			if _, err := db.(sqlExecutor).ExecContext(ctx, `INSERT INTO regions (code, number, name) VALUES (?, ?, ?)`, v.Code, v.Number, v.Name); err != nil {
				return region{}, fmt.Errorf("insert region: %w", err)
			}
			return v, nil
		},
		Deployment: func(ctx context.Context, db seedling.DBTX, v deployment) (deployment, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO deployments (region_code, region_number, name) VALUES (?, ?, ?)`, v.RegionCode, v.RegionNumber, v.Name)
			if err != nil {
				return deployment{}, err
			}
			v.ID = id
			return v, nil
		},
	})

	seedlingtest.RegisterManyToMany(tb, reg, seedlingtest.ManyToManyInserters{
		Article: func(ctx context.Context, db seedling.DBTX, v article) (article, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO articles (title) VALUES (?)`, v.Title)
			if err != nil {
				return article{}, err
			}
			v.ID = id
			return v, nil
		},
		Tag: func(ctx context.Context, db seedling.DBTX, v tag) (tag, error) {
			id, err := insertID(ctx, db.(sqlExecutor), `INSERT INTO tags (name) VALUES (?)`, v.Name)
			if err != nil {
				return tag{}, err
			}
			v.ID = id
			return v, nil
		},
		ArticleTag: func(ctx context.Context, db seedling.DBTX, v articleTag) (articleTag, error) {
			if _, err := db.(sqlExecutor).ExecContext(ctx, `INSERT INTO article_tags (article_id, tag_id) VALUES (?, ?)`, v.ArticleID, v.TagID); err != nil {
				return articleTag{}, fmt.Errorf("insert article_tag: %w", err)
			}
			return v, nil
		},
	})

	return reg
}

func insertID(ctx context.Context, db sqlExecutor, query string, args ...any) (int, error) {
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		return 0, fmt.Errorf("execute insert %q: %w", query, err)
	}

	var id int
	if err := db.QueryRowContext(ctx, `SELECT last_insert_rowid()`).Scan(&id); err != nil {
		return 0, fmt.Errorf("read last insert id for %q: %w", query, err)
	}

	return id, nil
}
