package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

// checkDB stands in for one transaction of a database with a NOT NULL
// company name and a unique user email.
type checkDB struct {
	mu      sync.Mutex
	emails  map[string]bool
	inserts []string
	deletes []string
}

func newCheckDB() *checkDB { return &checkDB{emails: make(map[string]bool)} }

func (d *checkDB) log(entry string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inserts = append(d.inserts, entry)
}

type checkUser struct {
	ID        int
	CompanyID int
	Email     string
	Active    bool
}

// newCheckRegistry registers company and user blueprints whose Insert
// callbacks enforce checkDB's constraints. email returns the user's default
// email.
func newCheckRegistry(t *testing.T, email func(*seedling.Scope) string, traits map[string][]seedling.Option) *seedling.Registry {
	t.Helper()
	ids := seedlingtest.NewIDSequence()
	reg := seedling.NewRegistry()
	seedling.MustRegisterTo(reg, seedling.Blueprint[Company]{
		Name: "company", Table: "companies", PKField: "ID",
		Defaults: func(*seedling.Scope) Company { return Company{Name: "acme"} },
		Traits: map[string][]seedling.Option{
			"unnamed": {seedling.Set("Name", "")},
		},
		Insert: func(_ context.Context, db seedling.DBTX, v Company) (Company, error) {
			if v.Name == "" {
				return v, errors.New(`null value in column "name" violates not-null constraint`)
			}
			v.ID = ids.Next()
			db.(*checkDB).log("company")
			return v, nil
		},
		Delete: func(_ context.Context, db seedling.DBTX, v Company) error {
			d := db.(*checkDB)
			d.deletes = append(d.deletes, "company")
			return nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[checkUser]{
		Name: "user", Table: "users", PKField: "ID",
		Defaults: func(s *seedling.Scope) checkUser { return checkUser{Email: email(s), Active: true} },
		Relations: []seedling.Relation{
			{Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
		},
		Traits: traits,
		Insert: func(_ context.Context, db seedling.DBTX, v checkUser) (checkUser, error) {
			d := db.(*checkDB)
			d.mu.Lock()
			defer d.mu.Unlock()
			if d.emails[v.Email] {
				return v, fmt.Errorf("duplicate key value violates unique constraint on email %q", v.Email)
			}
			d.emails[v.Email] = true
			v.ID = ids.Next()
			d.inserts = append(d.inserts, "user")
			return v, nil
		},
		Delete: func(_ context.Context, db seedling.DBTX, v checkUser) error {
			d := db.(*checkDB)
			d.deletes = append(d.deletes, "user")
			return nil
		},
	})
	return reg
}

func uniqueEmail(s *seedling.Scope) string { return s.Unique("user") + "@example.com" }

func TestCheckRegistry_InsertsEveryBlueprintAndTrait(t *testing.T) {
	// Arrange
	reg := newCheckRegistry(t, uniqueEmail, map[string][]seedling.Option{
		"inactive": {seedling.Set("Active", false)},
		"invalid":  {seedling.Ref("company", seedling.BlueprintTrait("unnamed"))},
		"skipped":  {seedling.Ref("company", seedling.BlueprintTrait("unnamed"))},
	})
	var mu sync.Mutex
	cases := map[string]*checkDB{}

	// Act
	seedling.CheckRegistry(t, reg, func(t *testing.T) seedling.DBTX {
		t.Helper()
		db := newCheckDB()
		mu.Lock()
		cases[t.Name()] = db
		mu.Unlock()
		return db
	},
		seedling.CheckRepeat(2),
		seedling.CheckTraits("user", "inactive", "inactive"),
		seedling.SkipTrait("user", "skipped"),
		seedling.ExpectFailure("user", "invalid"),
		seedling.ExpectFailure("company", "unnamed"),
	)

	// Assert
	prefix := t.Name() + "/"
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, strings.TrimPrefix(name, prefix))
	}
	slices.Sort(names)
	want := []string{"company/default", "company/unnamed", "user/default", "user/inactive", "user/inactive+inactive", "user/invalid"}
	if !slices.Equal(names, want) {
		t.Fatalf("cases = %v, want %v", names, want)
	}
	if got := cases[prefix+"user/inactive"].inserts; !slices.Equal(got, []string{"company", "user", "company", "user"}) {
		t.Fatalf("user/inactive inserts = %v, want two users with their companies", got)
	}
	if got := cases[prefix+"user/invalid"].inserts; len(got) != 0 {
		t.Fatalf("an expected failure inserted %v", got)
	}
}

func TestCheckRegistry_CleansUpWithAutoCleanup(t *testing.T) {
	// Arrange
	reg := newCheckRegistry(t, uniqueEmail, nil)
	var dbs []*checkDB

	// Act
	t.Run("check", func(t *testing.T) {
		seedling.CheckRegistry(t, reg, func(t *testing.T) seedling.DBTX {
			t.Helper()
			db := newCheckDB()
			dbs = append(dbs, db)
			seedling.AutoCleanup(t, db)
			return db
		}, seedling.ExpectFailure("company", "unnamed"))
	})

	// Assert
	if len(dbs) != 3 {
		t.Fatalf("cases = %d, want 3", len(dbs))
	}
	for _, db := range dbs {
		if len(db.deletes) != len(db.inserts) {
			t.Fatalf("inserted %v but deleted %v", db.inserts, db.deletes)
		}
	}
}

// TestCheckRegistry_ReportsFailures runs failing checks in a child test
// process, since a failing subtest cannot be contained in this one.
func TestCheckRegistry_ReportsFailures(t *testing.T) {
	if mode := os.Getenv("SEEDLING_CHECK_CHILD"); mode != "" {
		runFailingCheck(t, mode)
		return
	}

	tests := []struct {
		mode string
		want []string
	}{
		{mode: "broken-trait", want: []string{
			"--- FAIL: TestCheckRegistry_ReportsFailures/user/orphan",
			`blueprint "user" with traits orphan:`,
			`blueprint "company" at user.company: null value in column "name"`,
			"--- PASS: TestCheckRegistry_ReportsFailures/user/default",
		}},
		{mode: "repeated-default", want: []string{
			"--- FAIL: TestCheckRegistry_ReportsFailures/user/default",
			`blueprint "user" with defaults: insert 2 of 2:`,
			"duplicate key value",
		}},
		{mode: "unexpected-success", want: []string{
			`blueprint "company" with defaults: insert 1 succeeded, but ExpectFailure says it must fail`,
		}},
		{mode: "unknown-trait", want: []string{
			`SkipTrait names trait "missing", which blueprint "user" does not define`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			// Arrange
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCheckRegistry_ReportsFailures$", "-test.v")
			cmd.Env = append(os.Environ(), "SEEDLING_CHECK_CHILD="+tt.mode)

			// Act
			out, err := cmd.CombinedOutput()

			// Assert
			if _, ok := errors.AsType[*exec.ExitError](err); !ok {
				t.Fatalf("child test passed or did not run (err %v):\n%s", err, out)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(out), want) {
					t.Errorf("output does not contain %q:\n%s", want, out)
				}
			}
		})
	}
}

func runFailingCheck(t *testing.T, mode string) {
	t.Helper()
	db := func(*testing.T) seedling.DBTX { return newCheckDB() }
	switch mode {
	case "broken-trait":
		reg := newCheckRegistry(t, uniqueEmail, map[string][]seedling.Option{
			"orphan": {seedling.Ref("company", seedling.BlueprintTrait("unnamed"))},
		})
		seedling.CheckRegistry(t, reg, db, seedling.SkipBlueprint("company"))
	case "repeated-default":
		reg := newCheckRegistry(t, func(*seedling.Scope) string { return "same@example.com" }, nil)
		seedling.CheckRegistry(t, reg, db, seedling.CheckRepeat(2), seedling.SkipBlueprint("company"))
	case "unexpected-success":
		reg := newCheckRegistry(t, uniqueEmail, nil)
		seedling.CheckRegistry(t, reg, db, seedling.ExpectFailure("company"))
	case "unknown-trait":
		reg := newCheckRegistry(t, uniqueEmail, nil)
		seedling.CheckRegistry(t, reg, db, seedling.SkipTrait("user", "missing"))
	}
}
