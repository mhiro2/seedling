package seedling_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhiro2/seedling"
)

func TestOnly_SelectiveInsert(t *testing.T) {
	setupBlueprints(t)

	// Task has two BelongsTo relations: project (→ company), assignee (→ company).
	// Only("project") should insert: task, project, company (project's dep).
	// The assignee relation should be skipped.
	result := session[Task](t).InsertOne(t, nil, seedling.Only("project"))

	task := result.Root()
	if task.ID == 0 {
		t.Fatal("expected non-zero root ID")
	}
	if task.ProjectID == 0 {
		t.Fatal("expected non-zero ProjectID")
	}

	// project should exist in result
	projectNode, ok := result.Node("project")
	if !ok {
		t.Fatal("expected project node in result")
	}
	project := projectNode.Value().(Project)
	if project.ID == 0 {
		t.Fatal("expected non-zero project ID")
	}

	// company (project's parent) should exist
	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("expected company node (project's dependency) in result")
	}
	company := companyNode.Value().(Company)
	if company.ID == 0 {
		t.Fatal("expected non-zero company ID")
	}

	// assignee should NOT exist in result
	_, ok = result.Node("user")
	if ok {
		t.Fatal("expected user/assignee node to be absent from result")
	}
}

func TestOnly_RootOnly(t *testing.T) {
	setupBlueprints(t)

	// Only() with no arguments: insert root only, skip all relations.
	result := session[User](t).InsertOne(t, nil, seedling.Only())

	user := result.Root()
	if user.ID == 0 {
		t.Fatal("expected non-zero root ID")
	}
	// BelongsTo parent is skipped.
	if user.CompanyID != 0 {
		t.Fatalf("expected zero CompanyID, got %d", user.CompanyID)
	}

	all := result.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node, got %d", len(all))
	}
}

func TestOnly_WithRelationIncludesDeps(t *testing.T) {
	setupBlueprints(t)

	// Only("company") on User: root + company (the BelongsTo parent).
	result := session[User](t).InsertOne(t, nil, seedling.Only("company"))

	user := result.Root()
	if user.ID == 0 {
		t.Fatal("expected non-zero root ID")
	}
	if user.CompanyID == 0 {
		t.Fatal("expected non-zero CompanyID")
	}

	companyNode, ok := result.Node("company")
	if !ok {
		t.Fatal("expected company node in result")
	}
	company := companyNode.Value().(Company)
	if company.ID == 0 {
		t.Fatal("expected non-zero company ID")
	}
}

func TestOnly_TaskWithNoArgs(t *testing.T) {
	setupBlueprints(t)

	// Only() on Task with no args: only the root is inserted.
	// All BelongsTo relations are skipped, leaving FK fields at zero values.
	result := session[Task](t).InsertOne(t, nil, seedling.Only())

	task := result.Root()
	if task.ID == 0 {
		t.Fatal("expected non-zero root ID")
	}
	// BelongsTo parents are not included → FK fields are zero.
	if task.ProjectID != 0 {
		t.Fatalf("expected zero ProjectID, got %d", task.ProjectID)
	}
	if task.AssigneeUserID != 0 {
		t.Fatalf("expected zero AssigneeUserID, got %d", task.AssigneeUserID)
	}

	// No other nodes should exist.
	all := result.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 node (root only), got %d", len(all))
	}
}

func TestOnly_PlanDebugStringShowsLazyGraph(t *testing.T) {
	setupBlueprints(t)

	plan := build[Task](t, seedling.Only("project"))
	debug := plan.DebugString()

	// Lazy graph should include project subtree but not assignee.
	if !strings.Contains(debug, "project") {
		t.Fatal("expected 'project' in debug output")
	}
	// The assignee relation should NOT appear because it was not expanded.
	if strings.Contains(debug, "assignee") {
		t.Fatal("expected 'assignee' to be absent from lazy plan debug output")
	}
}

func TestOnly_InsertManyReturnsError(t *testing.T) {
	setupBlueprints(t)

	_, err := insertManyE[Task](t.Context(), t, nil, 3, seedling.Only("project"))
	if err == nil {
		t.Fatal("expected error when using Only with InsertMany")
	}
	if !errors.Is(err, seedling.ErrInvalidOption) {
		t.Fatalf("expected ErrInvalidOption, got: %v", err)
	}
}
