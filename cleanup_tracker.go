package seedling

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mhiro2/seedling/internal/errx"
	"github.com/mhiro2/seedling/internal/executor"
	"github.com/mhiro2/seedling/internal/graph"
)

// CleanupTracker records the records seedling inserts and deletes them
// together, for tests that cannot roll back a transaction, such as tests of an
// HTTP server or worker that reads the database over its own connection.
//
// Inserts made with a tracker are recorded as soon as their Insert callbacks
// return, before a testing API fails the test, so a partial insert is cleaned
// up as well. Records passed with [Use] are owned by the caller and never
// deleted. Before inserting anything, every blueprint the plan inserts is
// checked for a [Blueprint.Delete] callback; a missing one fails with
// [ErrDeleteNotDefined] instead of leaving rows behind.
//
// A CleanupTracker is safe for concurrent use.
type CleanupTracker struct {
	db      DBTX
	mu      sync.Mutex
	entries []*cleanupEntry // in the order the inserts started
}

// cleanupEntry is what one insert left for cleanup: the same graph, delete
// callbacks, and as-inserted values a Result cleans up with, and the records
// already deleted by an earlier cleanup that stopped at an error. It is
// reserved when the insert starts, so inserts running concurrently are
// deleted in the reverse of the order they started, and filled when it
// returns.
type cleanupEntry struct {
	tracker   *CleanupTracker
	graph     *graph.Graph
	deleteFns map[string]deleteFn
	values    map[string]any
	deleted   map[string]bool
}

var autoCleanups sync.Map // test name → *CleanupTracker

// AutoCleanup makes every testing API ([InsertOne], [InsertMany], [Build] and
// [Plan.Insert], [Scenario.Insert], and their [Session], [Builder], and
// generated typed-builder forms) record what it inserts for tb and its
// subtests, and deletes those records through db when tb finishes, newest
// insert first and children before parents within each insert:
//
//	func TestAPI(t *testing.T) {
//	    seedling.AutoCleanup(t, db)
//	    task := seedling.InsertOne[Task](t, db).Root()
//	    // ... call the server, which reads task over its own connection
//	}
//
// Records of a subtest are deleted when tb finishes, not when the subtest
// does. Subtests are found by name, so a sibling subtest whose own name
// starts with tb's subtest name and a slash, such as "group/other" next to
// "group", counts as a subtest of it; avoid such names under t.Parallel. The error-returning APIs record only when given the tracker through
// [WithCleanup], [Session.WithCleanup], or [Scenario.WithCleanup]. Calling
// AutoCleanup twice for one test fails it.
func AutoCleanup(tb testing.TB, db DBTX, opts ...CleanupOption) *CleanupTracker {
	tb.Helper()
	tracker := &CleanupTracker{db: db}
	name := tb.Name()
	if existing, loaded := autoCleanups.LoadOrStore(name, tracker); loaded {
		tb.Fatalf("seedling: AutoCleanup is already enabled for %s", name)
		return existing.(*CleanupTracker)
	}
	tb.Cleanup(func() {
		autoCleanups.Delete(name)
		ctx, cancel := cleanupContext(tb, opts)
		defer cancel()
		if err := tracker.CleanupE(ctx); err != nil {
			tb.Errorf("seedling: auto cleanup: %v", err)
		}
	})
	return tracker
}

// cleanupTrackerFor returns the tracker AutoCleanup enabled for tb or for the
// test it runs under, or nil.
func cleanupTrackerFor(tb testing.TB) *CleanupTracker {
	tb.Helper()
	name := tb.Name()
	for {
		if tracker, ok := autoCleanups.Load(name); ok {
			return tracker.(*CleanupTracker)
		}
		i := strings.LastIndexByte(name, '/')
		if i < 0 {
			return nil
		}
		name = name[:i]
	}
}

// WithCleanup records the insert in tracker, for the error-returning APIs or
// to choose a tracker other than the test's. It applies to the root record
// only; a [Scenario] takes its tracker from [Scenario.WithCleanup].
//
//	tracker := seedling.AutoCleanup(t, db)
//	result, err := seedling.InsertOneE[Task](ctx, db, seedling.WithCleanup(tracker))
func WithCleanup(tracker *CleanupTracker) Option {
	return cleanupOption{tracker: tracker}
}

type cleanupOption struct {
	tracker *CleanupTracker
}

func (c cleanupOption) applyOption(r *optionResolver, o *optionSet) error {
	if !o.root {
		return rootOnlyError("with-cleanup")
	}
	if r.scenario != nil {
		return fmt.Errorf("%w: WithCleanup does not apply to a record added to a Scenario; use Scenario.WithCleanup instead", ErrInvalidOption)
	}
	if c.tracker == nil {
		return fmt.Errorf("%w: WithCleanup tracker must not be nil", ErrInvalidOption)
	}
	o.cleanup = c.tracker
	return nil
}

// Len returns the number of records the tracker will delete.
func (c *CleanupTracker) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, entry := range c.entries {
		if entry.graph == nil {
			continue
		}
		for _, node := range entry.graph.Nodes() {
			if !node.IsProvided && !entry.deleted[node.ID] {
				n++
			}
		}
	}
	return n
}

// CleanupE deletes every record recorded so far, newest insert first, and
// forgets them, so a later cleanup does not delete them again. Each insert is
// deleted children first and stops at its first delete error, as
// [Result.CleanupE] does; the inserts recorded before a failed one are still
// attempted, and every error is returned joined. The records of an insert
// that could not be deleted stay recorded, and a later CleanupE resumes with
// them without deleting the others again.
func (c *CleanupTracker) CleanupE(ctx context.Context) error {
	// Take the filled entries; an unfilled one belongs to an insert still
	// running, or to one that inserted nothing, and stays.
	c.mu.Lock()
	var pending, running []*cleanupEntry
	for _, e := range c.entries {
		if e.graph != nil {
			pending = append(pending, e)
		} else {
			running = append(running, e)
		}
	}
	c.entries = running
	c.mu.Unlock()

	var errs []error
	var failed []*cleanupEntry
	for _, entry := range slices.Backward(pending) {
		if err := cleanupResultGraph(ctx, entry.graph, entry.deleteFns, entry.values, entry.deleted, c.db); err != nil {
			errs = append(errs, err)
			failed = append(failed, entry)
		}
	}
	if len(failed) > 0 {
		slices.Reverse(failed)
		c.mu.Lock()
		c.entries = append(failed, c.entries...)
		c.mu.Unlock()
	}
	return errors.Join(errs...)
}

// begin reports a blueprint the plan inserts that has no Delete callback,
// before anything is inserted, and reserves the entry the insert is recorded
// in. A nil tracker checks and records nothing.
func (c *CleanupTracker) begin(reg *Registry, g *graph.Graph) (*cleanupEntry, error) {
	if c == nil || g == nil {
		return nil, nil //nolint:nilnil // a nil entry finishes as a no-op, so callers need no tracker check
	}
	if err := checkDeleteFns(reg, g); err != nil {
		return nil, err
	}
	entry := &cleanupEntry{tracker: c, deleted: make(map[string]bool)}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
	return entry, nil
}

func checkDeleteFns(reg *Registry, g *graph.Graph) error {
	r := resolveRegistry(reg).reg
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, node := range g.Nodes() {
		if node.IsProvided {
			continue
		}
		if def, ok := r.byName[node.BlueprintName]; !ok || def.delete == nil {
			return fmt.Errorf("auto cleanup: %w", errx.DeleteNotDefined(node.BlueprintName))
		}
	}
	return nil
}

// finish records what an execution inserted, including a partial one, and
// drops the entry when nothing was inserted.
func (e *cleanupEntry) finish(reg *Registry, result *executor.Result) {
	if e == nil {
		return
	}
	c := e.tracker
	if result == nil || len(result.Inserted) == 0 {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.entries = slices.DeleteFunc(c.entries, func(other *cleanupEntry) bool { return other == e })
		return
	}
	deleteFns := snapshotDeleteFns(reg, result.Nodes)
	c.mu.Lock()
	defer c.mu.Unlock()
	e.graph, e.deleteFns, e.values = result.Graph, deleteFns, result.Inserted
}

// resolveCleanup picks the tracker an insert records in.
func resolveCleanup(trackers ...*CleanupTracker) *CleanupTracker {
	for _, tracker := range trackers {
		if tracker != nil {
			return tracker
		}
	}
	return nil
}
