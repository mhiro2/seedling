package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

// cleanupTB is a testing.TB whose cleanups run when the test calls finish,
// and whose failures are recorded instead of stopping the test, so the
// behavior of AutoCleanup at the end of a test can be observed.
type cleanupTB struct {
	testing.TB
	name     string
	mu       sync.Mutex
	cleanups []func()
	failures []string
}

func newCleanupTB(t *testing.T) *cleanupTB {
	t.Helper()
	return &cleanupTB{TB: t, name: t.Name() + "/fake"}
}

func (c *cleanupTB) Name() string { return c.name }

func (c *cleanupTB) Cleanup(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, fn)
}

func (c *cleanupTB) Fatal(args ...any)                 { c.fail(fmt.Sprint(args...)) }
func (c *cleanupTB) Fatalf(format string, args ...any) { c.fail(fmt.Sprintf(format, args...)) }
func (c *cleanupTB) Errorf(format string, args ...any) { c.fail(fmt.Sprintf(format, args...)) }

func (c *cleanupTB) fail(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = append(c.failures, msg)
}

func (c *cleanupTB) finish() {
	for _, fn := range slices.Backward(c.cleanups) {
		fn()
	}
	c.cleanups = nil
}

func TestAutoCleanup_DeletesEveryInsertWhenTestEnds(t *testing.T) {
	// Arrange
	var deleted []string
	setupCleanupBlueprints(t, &deleted)
	reg := getTestRegistry(t)
	tb := newCleanupTB(t)
	tracker := seedling.AutoCleanup(tb, nil)

	// Act
	company := seedling.NewSession[Company](reg).InsertOne(tb, nil).Root()
	user := seedling.NewSession[User](reg).InsertOne(tb, nil, seedling.Use("company", company)).Root()
	batch := seedling.NewSession[Project](reg).InsertMany(tb, nil, 2, seedling.Ref("company", seedling.Set("Name", "shared")))
	planned := seedling.NewSession[Company](reg).Build(tb).Insert(tb, nil).Root()
	built := seedling.ForSession(seedling.NewSession[Company](reg)).Insert(tb, nil).Root()
	s := seedling.NewScenario(reg)
	scenarioCompany := seedling.Add[Company](s)
	scenarioResult := s.Insert(tb, nil)
	pending := tracker.Len()
	before := slices.Clone(deleted)
	tb.finish()

	// Assert
	if len(before) != 0 {
		t.Fatalf("records were deleted before the test ended: %v", before)
	}
	if len(tb.failures) != 0 {
		t.Fatalf("test failed: %v", tb.failures)
	}
	if pending != 8 {
		t.Fatalf("Len() = %d, want 8", pending)
	}
	if len(deleted) != pending {
		t.Fatalf("deleted %d records, want %d: %v", len(deleted), pending, deleted)
	}
	// Newest insert first: the scenario company, then the builder and plan
	// companies, then the batch, the user, and finally the company it used.
	if got := deleted[0]; got != fmt.Sprintf("company:%d", scenarioCompany.Get(scenarioResult).ID) {
		t.Fatalf("first deletion = %s, want the scenario company", got)
	}
	if got := deleted[len(deleted)-2:]; !slices.Equal(got, []string{fmt.Sprintf("user:%d", user.ID), fmt.Sprintf("company:%d", company.ID)}) {
		t.Fatalf("last deletions = %v, want the user before the company it uses", got)
	}
	for _, id := range []string{fmt.Sprintf("company:%d", planned.ID), fmt.Sprintf("company:%d", built.ID), fmt.Sprintf("project:%d", batch.MustRootAt(1).ID)} {
		if !slices.Contains(deleted, id) {
			t.Fatalf("%s was not deleted: %v", id, deleted)
		}
	}
	if tracker.Len() != 0 {
		t.Fatalf("Len() after cleanup = %d, want 0", tracker.Len())
	}
}

func failingUserRegistry(t *testing.T, deleted *[]string) *seedling.Registry {
	t.Helper()
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) { v.ID = ids.Next(); return v, nil },
		Delete: func(_ context.Context, _ seedling.DBTX, v Company) error {
			*deleted = append(*deleted, fmt.Sprintf("company:%d", v.ID))
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[User]{
		Name: "user", PKField: "ID",
		Relations: []seedling.Relation{{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"}},
		Insert: func(context.Context, seedling.DBTX, User) (User, error) {
			return User{}, errors.New("users table is gone")
		},
		Delete: func(context.Context, seedling.DBTX, User) error { return nil },
	})
	return reg
}

func TestAutoCleanup_RecordsPartialInsertBeforeFailing(t *testing.T) {
	// Arrange
	var deleted []string
	reg := failingUserRegistry(t, &deleted)
	tb := newCleanupTB(t)
	tracker := seedling.AutoCleanup(tb, nil)

	// Act
	seedling.NewSession[User](reg).InsertOne(tb, nil)
	pending := tracker.Len()
	tb.finish()

	// Assert
	if len(tb.failures) != 1 || !strings.Contains(tb.failures[0], "users table is gone") {
		t.Fatalf("failures = %v, want the insert error", tb.failures)
	}
	if pending != 1 || len(deleted) != 1 || !strings.HasPrefix(deleted[0], "company:") {
		t.Fatalf("pending = %d, deleted = %v, want the company inserted before the failure", pending, deleted)
	}
}

func TestAutoCleanup_RequiresDeleteBeforeInserting(t *testing.T) {
	// Arrange
	counts := map[string]int{}
	reg := newScenarioRegistry(t, counts, "")
	tracker := seedling.AutoCleanup(t, nil)

	// Act
	_, err := seedling.NewSession[Task](reg).InsertOneE(context.Background(), nil, seedling.WithCleanup(tracker))
	_, scenarioErr := seedling.NewScenario(reg).WithCleanup(tracker).InsertE(context.Background(), nil)
	_, batchErr := seedling.NewSession[Company](reg).WithCleanup(tracker).InsertManyE(context.Background(), nil, 2)

	// Assert
	if !errors.Is(err, seedling.ErrDeleteNotDefined) || !errors.Is(batchErr, seedling.ErrDeleteNotDefined) {
		t.Fatalf("errs = %v / %v, want ErrDeleteNotDefined", err, batchErr)
	}
	if scenarioErr != nil {
		t.Fatalf("an empty scenario inserts nothing and needs no Delete: %v", scenarioErr)
	}
	if len(counts) != 0 {
		t.Fatalf("records were inserted without a way to delete them: %v", counts)
	}
}

func TestAutoCleanup_CoversSubtests(t *testing.T) {
	// Arrange
	var deleted []string
	setupCleanupBlueprints(t, &deleted)
	reg := getTestRegistry(t)
	var afterChild []string

	// Act
	t.Run("parent", func(t *testing.T) {
		seedling.AutoCleanup(t, nil)
		t.Run("child", func(t *testing.T) {
			seedling.NewSession[User](reg).InsertOne(t, nil)
		})
		afterChild = slices.Clone(deleted)
	})

	// Assert
	if len(afterChild) != 0 {
		t.Fatalf("subtest records were deleted when the subtest ended: %v", afterChild)
	}
	if len(deleted) != 2 {
		t.Fatalf("deleted = %v, want the subtest's user and company", deleted)
	}
}

func TestAutoCleanup_TwiceFails(t *testing.T) {
	// Arrange
	tb := newCleanupTB(t)
	first := seedling.AutoCleanup(tb, nil)

	// Act
	second := seedling.AutoCleanup(tb, nil)

	// Assert
	if len(tb.failures) != 1 || !strings.Contains(tb.failures[0], "already enabled") {
		t.Fatalf("failures = %v", tb.failures)
	}
	if second != first {
		t.Fatal("a second AutoCleanup replaced the test's tracker")
	}
	tb.finish()
}

func TestCleanupTracker_CleanupEKeepsFailedInserts(t *testing.T) {
	// Arrange
	fail := true
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	var deleted []int
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", PKField: "ID",
		Insert: func(_ context.Context, _ seedling.DBTX, v Company) (Company, error) { v.ID = ids.Next(); return v, nil },
		Delete: func(_ context.Context, _ seedling.DBTX, v Company) error {
			if fail && v.ID == 1 {
				return errors.New("locked")
			}
			deleted = append(deleted, v.ID)
			return nil
		},
	})
	tracker := seedling.AutoCleanup(t, nil)
	sess := seedling.NewSession[Company](reg).WithCleanup(tracker)
	for range 2 {
		if _, err := sess.InsertOneE(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}

	// Act
	err := tracker.CleanupE(context.Background())
	pending := tracker.Len()
	fail = false
	retryErr := tracker.CleanupE(context.Background())

	// Assert
	if !errors.Is(err, seedling.ErrDeleteFailed) {
		t.Fatalf("err = %v, want ErrDeleteFailed", err)
	}
	if pending != 1 {
		t.Fatalf("Len() after a failed delete = %d, want 1", pending)
	}
	if retryErr != nil {
		t.Fatal(retryErr)
	}
	if !slices.Equal(deleted, []int{2, 1}) {
		t.Fatalf("deleted = %v, want [2 1]", deleted)
	}
}

func TestWithCleanup_Placement(t *testing.T) {
	// Arrange
	reg := newScenarioRegistry(t, map[string]int{}, "")
	tracker := seedling.AutoCleanup(t, nil)
	s := seedling.NewScenario(reg)
	seedling.Add[Company](s, seedling.WithCleanup(tracker))

	// Act
	_, refErr := seedling.NewSession[Task](reg).BuildE(seedling.Ref("project", seedling.WithCleanup(tracker)))
	_, nilErr := seedling.NewSession[Task](reg).BuildE(seedling.WithCleanup(nil))
	_, scenarioErr := s.BuildE()

	// Assert
	for _, err := range []error{refErr, nilErr, scenarioErr} {
		if !errors.Is(err, seedling.ErrInvalidOption) {
			t.Fatalf("err = %v, want ErrInvalidOption", err)
		}
	}
}
