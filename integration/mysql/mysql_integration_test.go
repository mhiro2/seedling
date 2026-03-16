//go:build integration

package mysql_test

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/integration/mysql/testutil"
	"github.com/mhiro2/seedling/seedlingtest"
)

type (
	company    = seedlingtest.Company
	user       = seedlingtest.User
	task       = seedlingtest.Task
	department = seedlingtest.Department
	deployment = seedlingtest.Deployment
	article    = seedlingtest.Article
)

func TestMySQLInsertOne_PersistsResolvedDependencies(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	// Act
	inserted := seedling.NewSession[task](reg).InsertOne(t, db,
		seedling.Set("Title", "integration-task"),
		seedling.Ref("project", seedling.Set("Name", "integration-project")),
		seedling.Ref("assignee", seedling.Set("Name", "integration-user")),
	).Root()

	// Assert
	if inserted.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if inserted.ProjectID == 0 {
		t.Fatal("expected non-zero ProjectID")
	}
	if inserted.AssigneeUserID == 0 {
		t.Fatal("expected non-zero AssigneeUserID")
	}

	var (
		title       string
		status      string
		projectName string
		userName    string
		companies   int
	)

	err := db.QueryRow(`
		SELECT t.title, t.status, p.name, u.name
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		JOIN users u ON u.id = t.assignee_user_id
		WHERE t.id = ?
	`, inserted.ID).Scan(&title, &status, &projectName, &userName)
	if err != nil {
		t.Fatalf("query inserted task: %v", err)
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM companies`).Scan(&companies)
	if err != nil {
		t.Fatalf("count companies: %v", err)
	}

	if got, want := title, "integration-task"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, want := status, "open"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, want := projectName, "integration-project"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, want := userName, "integration-user"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, want := companies, 2; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMySQLInsertManySeq_PersistsGeneratedNames(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	// Act
	companies := seedling.NewSession[company](reg).InsertMany(t, db, 3,
		seedling.Seq("Name", func(i int) string {
			return fmt.Sprintf("company-%d", i)
		}),
	)

	// Assert
	if len(companies) != 3 {
		t.Fatalf("got len %d, want %d", len(companies), 3)
	}

	rows, err := db.Query(`SELECT name FROM companies ORDER BY id`)
	if err != nil {
		t.Fatalf("list companies: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("close companies rows: %v", err)
		}
	}()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan company: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate companies: %v", err)
	}

	want := []string{"company-0", "company-1", "company-2"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", names, want)
	}
}

func TestMySQLInsertOneE_ReturnsForeignKeyViolation(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	// Act
	_, err := seedling.NewSession[user](reg).InsertOneE(t.Context(), db,
		seedling.Use("company", company{ID: 9999, Name: "missing-company"}),
	)

	// Assert
	if err == nil {
		t.Fatal("expected error")
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("got %v, want %v", count, 0)
	}
}

func TestMySQLInsertOneE_ReturnsConnectionError(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Act
	_, err := seedling.NewSession[company](reg).InsertOneE(t.Context(), db)

	// Assert
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMySQLInsertOne_RollsBackTransaction(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Act
	result, err := seedling.NewSession[task](reg).InsertOneE(t.Context(), tx,
		seedling.Set("Title", "rollback-task"),
	)
	if err != nil {
		t.Fatal(err)
	}
	inserted := result.Root()
	if inserted.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	for _, table := range []string{"companies", "users", "projects", "tasks"} {
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count)
		if err != nil {
			t.Fatalf("count rows in %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("expected rollback to clear %s: got %v, want %v", table, count, 0)
		}
	}
}

func TestMySQLInsertOne_HasManyPersistsResolvedDependencies(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	// Act
	inserted := seedling.NewSession[department](reg).InsertOne(t, db).Root()

	// Assert
	if inserted.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	rows, err := db.Query(`SELECT name, department_id FROM employees ORDER BY id`)
	if err != nil {
		t.Fatalf("list employees: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("close employees rows: %v", err)
		}
	}()

	var names []string
	for rows.Next() {
		var (
			name         string
			departmentID int
		)
		if err := rows.Scan(&name, &departmentID); err != nil {
			t.Fatalf("scan employee: %v", err)
		}
		if got, want := departmentID, inserted.ID; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate employees: %v", err)
	}
	sort.Strings(names)
	want := []string{"employee", "employee"}
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("mismatch:\ngot:  %+v\nwant: %+v", names, want)
	}
}

func TestMySQLInsertOne_CompositePKPersistsResolvedDependencies(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	// Act
	inserted := seedling.NewSession[deployment](reg).InsertOne(t, db).Root()

	// Assert
	if inserted.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if inserted.RegionCode == "" {
		t.Fatal("expected non-empty RegionCode")
	}
	if inserted.RegionNumber == 0 {
		t.Fatal("expected non-zero RegionNumber")
	}

	var regionName string
	err := db.QueryRow(`
		SELECT r.name
		FROM deployments d
		JOIN regions r ON r.code = d.region_code AND r.number = d.region_number
		WHERE d.id = ?
	`, inserted.ID).Scan(&regionName)
	if err != nil {
		t.Fatalf("query deployment region: %v", err)
	}
	if got, want := regionName, "tokyo"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMySQLInsertOne_ManyToManyPersistsResolvedDependencies(t *testing.T) {
	// Arrange
	h := testutil.NewHarness(t)
	db := h.DB
	reg := h.Registry

	// Act
	inserted := seedling.NewSession[article](reg).InsertOne(t, db).Root()

	// Assert
	if inserted.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	rows, err := db.Query(`SELECT tag_id FROM article_tags WHERE article_id = ? ORDER BY tag_id`, inserted.ID)
	if err != nil {
		t.Fatalf("list article tags: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("close article tag rows: %v", err)
		}
	}()

	var tagIDs []int
	for rows.Next() {
		var tagID int
		if err := rows.Scan(&tagID); err != nil {
			t.Fatalf("scan article tag: %v", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate article tags: %v", err)
	}
	if len(tagIDs) != 2 {
		t.Fatalf("got len %d, want %d", len(tagIDs), 2)
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&count)
	if err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if got, want := count, 2; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}
