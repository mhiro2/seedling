package seedling_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhiro2/seedling"
)

type scopeAccount struct {
	ID        int
	Email     string
	Score     int
	CreatedAt time.Time
}

type scopeTeam struct {
	ID   int
	Name string
}

type scopeMember struct {
	ID     int
	TeamID int
	Email  string
}

// newScopeRegistry registers accounts, and teams whose members are created
// through a has-many relation, all drawing unique values from their scope.
func newScopeRegistry(t *testing.T) *seedling.Registry {
	t.Helper()
	reg := seedling.NewRegistry()
	var mu sync.Mutex
	nextID := 0
	id := func() int {
		mu.Lock()
		defer mu.Unlock()
		nextID++
		return nextID
	}
	seedling.MustRegisterTo(reg, seedling.Blueprint[scopeAccount]{
		Name:    "scope_account",
		PKField: "ID",
		Defaults: func(s *seedling.Scope) scopeAccount {
			return scopeAccount{Email: s.Unique("account") + "@example.com", Score: s.Rand().IntN(1_000_000), CreatedAt: s.Now()}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v scopeAccount) (scopeAccount, error) {
			v.ID = id()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[scopeTeam]{
		Name:    "scope_team",
		PKField: "ID",
		Defaults: func(s *seedling.Scope) scopeTeam {
			return scopeTeam{Name: s.Unique("team")}
		},
		Relations: []seedling.Relation{
			{Name: "members", Kind: seedling.HasMany, LocalField: "TeamID", RefBlueprint: "scope_member", Count: 2},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v scopeTeam) (scopeTeam, error) {
			v.ID = id()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[scopeMember]{
		Name:    "scope_member",
		PKField: "ID",
		Defaults: func(s *seedling.Scope) scopeMember {
			return scopeMember{Email: s.Unique("member") + "@example.com"}
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v scopeMember) (scopeMember, error) {
			v.ID = id()
			return v, nil
		},
	})
	return reg
}

func TestScope_SeqAndUnique(t *testing.T) {
	// Arrange
	plain := seedling.NewScope(seedling.ScopeConfig{})
	named := seedling.NewScope(seedling.ScopeConfig{Namespace: "ns"})

	// Act
	seqs := []int{plain.Seq("a"), plain.Seq("a"), plain.Seq("b")}
	uniques := []string{plain.Unique("user"), plain.Unique("user"), named.Unique("user")}

	// Assert
	if want := []int{1, 2, 1}; !slices.Equal(seqs, want) {
		t.Fatalf("Seq = %v, want %v", seqs, want)
	}
	if want := []string{"user-1", "user-2", "user-1-ns"}; !slices.Equal(uniques, want) {
		t.Fatalf("Unique = %v, want %v", uniques, want)
	}
}

func TestScope_SeqIsUniqueUnderConcurrency(t *testing.T) {
	// Arrange
	scope := seedling.NewScope(seedling.ScopeConfig{})
	const n = 64
	values := make(chan int, n)
	var wg sync.WaitGroup

	// Act
	for range n {
		wg.Go(func() { values <- scope.Seq("id") })
	}
	wg.Wait()
	close(values)

	// Assert
	seen := make(map[int]bool, n)
	for v := range values {
		if seen[v] {
			t.Fatalf("Seq returned %d twice", v)
		}
		seen[v] = true
	}
}

func TestScope_RandIsReproducibleAndIndependent(t *testing.T) {
	// Arrange
	draw := func(scope *seedling.Scope) []uint64 {
		return []uint64{scope.Rand().Uint64(), scope.Rand().Uint64()}
	}

	// Act
	first := draw(seedling.NewScope(seedling.ScopeConfig{Seed: 7}))
	second := draw(seedling.NewScope(seedling.ScopeConfig{Seed: 7}))
	other := draw(seedling.NewScope(seedling.ScopeConfig{Seed: 8}))

	// Assert
	if !slices.Equal(first, second) {
		t.Fatalf("same seed drew %v and %v", first, second)
	}
	if first[0] == first[1] {
		t.Fatalf("consecutive sources drew the same value %d", first[0])
	}
	if slices.Equal(first, other) {
		t.Fatalf("different seeds drew the same values %v", first)
	}
}

func TestScope_Now(t *testing.T) {
	// Arrange
	fixed := time.Date(2030, time.May, 1, 9, 0, 0, 0, time.UTC)

	// Act
	defaultNow := seedling.NewScope(seedling.ScopeConfig{}).Now()
	configured := seedling.NewScope(seedling.ScopeConfig{Now: fixed}).Now()

	// Assert
	if want := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC); !defaultNow.Equal(want) {
		t.Fatalf("default Now = %v, want %v", defaultNow, want)
	}
	if !configured.Equal(fixed) {
		t.Fatalf("configured Now = %v, want %v", configured, fixed)
	}
}

func TestTestScope_SharedByTestingAPIsWithinATest(t *testing.T) {
	// Arrange
	reg := newScopeRegistry(t)
	accounts := seedling.NewSession[scopeAccount](reg)
	teams := seedling.NewSession[scopeTeam](reg)
	scope := seedling.TestScope(t)

	// Act
	first := accounts.InsertOne(t, nil).Root()
	batch := accounts.InsertMany(t, nil, 2).Roots()
	built := accounts.Build(t).Insert(t, nil).Root()
	team := teams.InsertOne(t, nil)
	viaBuilder := seedling.ForSession(accounts).Insert(t, nil).Root()

	// Assert
	if scope != seedling.TestScope(t) {
		t.Fatal("TestScope returned a different scope for the same test")
	}
	ns := scope.Namespace()
	if ns == "" {
		t.Fatal("TestScope has an empty namespace")
	}
	emails := []string{first.Email, batch[0].Email, batch[1].Email, built.Email, viaBuilder.Email}
	for i, email := range emails {
		if want := "account-" + strconv.Itoa(i+1) + "-" + ns + "@example.com"; email != want {
			t.Fatalf("emails = %v, want account-1..5 in namespace %q", emails, ns)
		}
	}
	members := team.Nodes("scope_member")
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	memberEmails := map[string]bool{}
	for _, node := range members {
		memberEmails[node.Value().(scopeMember).Email] = true
	}
	if !memberEmails["member-1-"+ns+"@example.com"] || !memberEmails["member-2-"+ns+"@example.com"] {
		t.Fatalf("member emails = %v, want member-1 and member-2 in namespace %q", memberEmails, ns)
	}
	if first.Score == batch[0].Score && batch[0].Score == batch[1].Score {
		t.Fatalf("every record drew the same random score %d", first.Score)
	}
	if !first.CreatedAt.Equal(scope.Now()) {
		t.Fatalf("CreatedAt = %v, want the scope clock %v", first.CreatedAt, scope.Now())
	}
}

func TestTestScope_DiffersBetweenTests(t *testing.T) {
	// Arrange
	emails := map[string]string{}

	// Act
	for _, name := range []string{"a", "b"} {
		t.Run(name, func(t *testing.T) {
			reg := newScopeRegistry(t)
			emails[name] = seedling.NewSession[scopeAccount](reg).InsertOne(t, nil).Root().Email
		})
	}

	// Assert
	if emails["a"] == emails["b"] {
		t.Fatalf("two tests produced the same unique email %q", emails["a"])
	}
}

func TestErrorAPIs_ShareTheProcessScopeUnlessGivenOne(t *testing.T) {
	// Arrange
	reg := newScopeRegistry(t)
	accounts := seedling.NewSession[scopeAccount](reg)
	scope := seedling.NewScope(seedling.ScopeConfig{Namespace: "explicit"})

	// Act
	first, err := accounts.InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := accounts.InsertOneE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := accounts.InsertOneE(t.Context(), nil, seedling.InScope(scope))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := accounts.WithScope(scope).InsertManyE(t.Context(), nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if first.Root().Email == second.Root().Email {
		t.Fatalf("two calls produced the same unique email %q", first.Root().Email)
	}
	if got, want := scoped.Root().Email, "account-1-explicit@example.com"; got != want {
		t.Fatalf("InScope email = %q, want %q", got, want)
	}
	if got, want := bound.Roots()[0].Email, "account-2-explicit@example.com"; got != want {
		t.Fatalf("WithScope email = %q, want %q", got, want)
	}
}

func TestInScope_OverridesTheTestScope(t *testing.T) {
	// Arrange
	reg := newScopeRegistry(t)
	scope := seedling.NewScope(seedling.ScopeConfig{Namespace: "own"})

	// Act
	account := seedling.NewSession[scopeAccount](reg).WithScope(seedling.NewScope(seedling.ScopeConfig{Namespace: "session"})).
		InsertOne(t, nil, seedling.InlineTrait(seedling.InScope(scope))).Root()

	// Assert
	if got, want := account.Email, "account-1-own@example.com"; got != want {
		t.Fatalf("email = %q, want %q", got, want)
	}
}

func TestInScope_RejectsInvalidPlacement(t *testing.T) {
	// Arrange
	reg := newScopeRegistry(t)
	scope := seedling.NewScope(seedling.ScopeConfig{})
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "under Ref",
			run: func() error {
				_, err := seedling.NewSession[scopeTeam](reg).BuildE(seedling.Ref("members", seedling.InScope(scope)))
				return fmt.Errorf("build: %w", err)
			},
			want: "applies only to the root blueprint",
		},
		{
			name: "nil scope",
			run: func() error {
				_, err := seedling.NewSession[scopeAccount](reg).BuildE(seedling.InScope(nil))
				return fmt.Errorf("build: %w", err)
			},
			want: "must not be nil",
		},
		{
			name: "scenario record",
			run: func() error {
				s := seedling.NewScenario(reg)
				seedling.Add[scopeAccount](s, seedling.InScope(scope))
				_, err := s.BuildE()
				return fmt.Errorf("build: %w", err)
			},
			want: "Scenario.WithScope",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.run()

			// Assert
			if !errors.Is(err, seedling.ErrInvalidOption) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want ErrInvalidOption containing %q", err, tt.want)
			}
		})
	}
}

func TestScenario_PlansInTheTestScopeOrItsOwn(t *testing.T) {
	// Arrange
	reg := newScopeRegistry(t)
	ns := seedling.TestScope(t).Namespace()
	scenario := seedling.NewScenario(reg)
	first := seedling.Add[scopeAccount](scenario)
	second := seedling.Add[scopeAccount](scenario)
	own := seedling.NewScenario(reg).WithScope(seedling.NewScope(seedling.ScopeConfig{Namespace: "scenario"}))
	ownAccount := seedling.Add[scopeAccount](own)

	// Act
	result := scenario.Insert(t, nil)
	ownResult, err := own.InsertE(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Assert
	if got, want := first.Get(result).Email, "account-1-"+ns+"@example.com"; got != want {
		t.Fatalf("first email = %q, want %q", got, want)
	}
	if got, want := second.Get(result).Email, "account-2-"+ns+"@example.com"; got != want {
		t.Fatalf("second email = %q, want %q", got, want)
	}
	if got, want := ownAccount.Get(ownResult).Email, "account-1-scenario@example.com"; got != want {
		t.Fatalf("scenario-scoped email = %q, want %q", got, want)
	}
}

func TestGenerate_DrawsFromTheScopeWithoutWithRand(t *testing.T) {
	// Arrange
	reg := newScopeRegistry(t)
	pick := seedling.Generate(func(r *rand.Rand, a *scopeAccount) { a.Score = r.IntN(1_000_000) })

	// Act
	batch := seedling.NewSession[scopeAccount](reg).InsertMany(t, nil, 3, pick).Roots()
	seeded := seedling.NewSession[scopeAccount](reg).InsertOne(t, nil, pick, seedling.WithSeed(1)).Root()
	reseeded := seedling.NewSession[scopeAccount](reg).InsertOne(t, nil, pick, seedling.WithSeed(1)).Root()

	// Assert
	if batch[0].Score == batch[1].Score && batch[1].Score == batch[2].Score {
		t.Fatalf("every record drew the same score %d", batch[0].Score)
	}
	if seeded.Score != reseeded.Score {
		t.Fatalf("WithSeed records drew %d and %d, want the same seeded value", seeded.Score, reseeded.Score)
	}
}

func TestTestScope_RecordsCreatedDuringCleanupDoNotRepeatValues(t *testing.T) {
	// Arrange
	var bodyEmail, cleanupEmail string
	t.Run("inner", func(t *testing.T) {
		reg := newScopeRegistry(t)
		t.Cleanup(func() {
			// The test context is canceled by now, so cleanup code inserts
			// through the error-returning API, joining the test's scope.
			result, err := seedling.NewSession[scopeAccount](reg).InsertOneE(context.Background(), nil, seedling.InScope(seedling.TestScope(t)))
			if err != nil {
				t.Error(err)
				return
			}
			cleanupEmail = result.Root().Email
		})

		// Act
		bodyEmail = seedling.NewSession[scopeAccount](reg).InsertOne(t, nil).Root().Email
	})

	// Assert
	if bodyEmail == "" || cleanupEmail == "" {
		t.Fatalf("emails = %q / %q, want both set", bodyEmail, cleanupEmail)
	}
	if bodyEmail == cleanupEmail {
		t.Fatalf("cleanup reused the test's unique email %q", bodyEmail)
	}
}

func TestScope_UUID(t *testing.T) {
	// Arrange
	scope := seedling.NewScope(seedling.ScopeConfig{Seed: 3})
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	// Act
	first, second := scope.UUID(), scope.UUID()
	replayed := seedling.NewScope(seedling.ScopeConfig{Seed: 3}).UUID()

	// Assert
	if !pattern.MatchString(first) || !pattern.MatchString(second) {
		t.Fatalf("UUIDs %q, %q are not canonical version 4 UUIDs", first, second)
	}
	if first == second {
		t.Fatalf("two draws returned the same UUID %q", first)
	}
	if replayed != first {
		t.Fatalf("same seed returned %q, want %q", replayed, first)
	}
}

func TestScope_UniqueNFitsMaxLength(t *testing.T) {
	// Arrange
	scope := seedling.NewScope(seedling.ScopeConfig{Namespace: "ns"})
	plain := seedling.NewScope(seedling.ScopeConfig{})

	// Act
	got := []string{
		scope.UniqueN("email", 0),
		scope.UniqueN("email", 8),
		scope.UniqueN("email", 6),
		scope.UniqueN("email", 4),
		scope.UniqueN("email", 2),
		plain.UniqueN("code", 1),
	}

	// Assert
	if want := []string{"email-1-ns", "ema-2-ns", "e-3-ns", "4-ns", "5", "1"}; !slices.Equal(got, want) {
		t.Fatalf("UniqueN = %q, want %q", got, want)
	}
}
