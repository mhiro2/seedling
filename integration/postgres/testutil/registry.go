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
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO companies (name) VALUES ($1) RETURNING id, name`,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.Name); err != nil {
				return company{}, fmt.Errorf("insert company: %w", err)
			}
			return v, nil
		},
		User: func(ctx context.Context, db seedling.DBTX, v user) (user, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO users (company_id, name) VALUES ($1, $2) RETURNING id, company_id, name`,
				v.CompanyID,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.CompanyID, &v.Name); err != nil {
				return user{}, fmt.Errorf("insert user: %w", err)
			}
			return v, nil
		},
		Project: func(ctx context.Context, db seedling.DBTX, v project) (project, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO projects (company_id, name) VALUES ($1, $2) RETURNING id, company_id, name`,
				v.CompanyID,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.CompanyID, &v.Name); err != nil {
				return project{}, fmt.Errorf("insert project: %w", err)
			}
			return v, nil
		},
		Task: func(ctx context.Context, db seedling.DBTX, v task) (task, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO tasks (project_id, assignee_user_id, title, status) VALUES ($1, $2, $3, $4) RETURNING id, project_id, assignee_user_id, title, status`,
				v.ProjectID,
				v.AssigneeUserID,
				v.Title,
				v.Status,
			)
			if err := row.Scan(&v.ID, &v.ProjectID, &v.AssigneeUserID, &v.Title, &v.Status); err != nil {
				return task{}, fmt.Errorf("insert task: %w", err)
			}
			return v, nil
		},
	})

	seedlingtest.RegisterHasMany(tb, reg, seedlingtest.HasManyInserters{
		Department: func(ctx context.Context, db seedling.DBTX, v department) (department, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO departments (name) VALUES ($1) RETURNING id, name`,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.Name); err != nil {
				return department{}, fmt.Errorf("insert department: %w", err)
			}
			return v, nil
		},
		Employee: func(ctx context.Context, db seedling.DBTX, v employee) (employee, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO employees (department_id, name) VALUES ($1, $2) RETURNING id, department_id, name`,
				v.DepartmentID,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.DepartmentID, &v.Name); err != nil {
				return employee{}, fmt.Errorf("insert employee: %w", err)
			}
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
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO regions (code, number, name) VALUES ($1, $2, $3) RETURNING code, number, name`,
				v.Code,
				v.Number,
				v.Name,
			)
			if err := row.Scan(&v.Code, &v.Number, &v.Name); err != nil {
				return region{}, fmt.Errorf("insert region: %w", err)
			}
			return v, nil
		},
		Deployment: func(ctx context.Context, db seedling.DBTX, v deployment) (deployment, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO deployments (region_code, region_number, name) VALUES ($1, $2, $3) RETURNING id, region_code, region_number, name`,
				v.RegionCode,
				v.RegionNumber,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.RegionCode, &v.RegionNumber, &v.Name); err != nil {
				return deployment{}, fmt.Errorf("insert deployment: %w", err)
			}
			return v, nil
		},
	})

	seedlingtest.RegisterManyToMany(tb, reg, seedlingtest.ManyToManyInserters{
		Article: func(ctx context.Context, db seedling.DBTX, v article) (article, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO articles (title) VALUES ($1) RETURNING id, title`,
				v.Title,
			)
			if err := row.Scan(&v.ID, &v.Title); err != nil {
				return article{}, fmt.Errorf("insert article: %w", err)
			}
			return v, nil
		},
		Tag: func(ctx context.Context, db seedling.DBTX, v tag) (tag, error) {
			row := db.(sqlExecutor).QueryRowContext(ctx,
				`INSERT INTO tags (name) VALUES ($1) RETURNING id, name`,
				v.Name,
			)
			if err := row.Scan(&v.ID, &v.Name); err != nil {
				return tag{}, fmt.Errorf("insert tag: %w", err)
			}
			return v, nil
		},
		ArticleTag: func(ctx context.Context, db seedling.DBTX, v articleTag) (articleTag, error) {
			if _, err := db.(sqlExecutor).ExecContext(ctx, `INSERT INTO article_tags (article_id, tag_id) VALUES ($1, $2)`, v.ArticleID, v.TagID); err != nil {
				return articleTag{}, fmt.Errorf("insert article_tag: %w", err)
			}
			return v, nil
		},
	})

	return reg
}
