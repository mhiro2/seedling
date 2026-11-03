package seedling

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
	"time"
)

// defaultScopeTime is the instant a [Scope] reports from [Scope.Now] unless
// [ScopeConfig.Now] sets another one.
var defaultScopeTime = time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)

// Scope is the generation environment shared by every record built in it.
// Blueprint Defaults receive it to draw values that stay unique and
// reproducible across a whole test, including every Insert call and every
// related record:
//
//	Defaults: func(s *seedling.Scope) User {
//	    return User{
//	        Email:     s.Unique("user") + "@example.com",
//	        Age:       18 + s.Rand().IntN(50),
//	        CreatedAt: s.Now(),
//	    }
//	}
//
// The testing APIs ([InsertOne], [InsertMany], [Build], [Scenario.Insert], and
// their [Session] and [Builder] forms) use [TestScope] for the test they are
// given. The error-returning APIs use a scope set with [InScope],
// [Session.WithScope], or [Scenario.WithScope], and otherwise one scope shared
// by the whole process. Values are reproducible when records are planned in the
// same order; records planned concurrently in one scope stay unique, but which
// of them draws which value depends on scheduling.
//
// Uniqueness holds within one scope only. A scope does not query the database,
// so rows written outside it, such as by another process or an earlier run that
// was not rolled back, can still collide; the namespace makes collisions
// between tests unlikely, not impossible.
//
// A Scope is safe for concurrent use.
type Scope struct {
	namespace string
	seed      uint64
	now       time.Time

	mu    sync.Mutex
	seqs  map[string]int
	draws uint64
}

// ScopeConfig configures [NewScope].
type ScopeConfig struct {
	// Namespace is appended to the values [Scope.Unique] returns, so that
	// scopes with different namespaces do not produce the same value.
	Namespace string

	// Seed seeds the random sources [Scope.Rand] returns.
	Seed uint64

	// Now is the instant [Scope.Now] reports. The zero value selects
	// 2024-01-01T00:00:00Z.
	Now time.Time
}

// NewScope returns a scope configured by cfg, with every sequence at zero.
func NewScope(cfg ScopeConfig) *Scope {
	now := cfg.Now
	if now.IsZero() {
		now = defaultScopeTime
	}
	return &Scope{
		namespace: cfg.Namespace,
		seed:      cfg.Seed,
		now:       now,
		seqs:      make(map[string]int),
	}
}

// processScope serves the error-returning APIs when no scope is set.
var processScope = NewScope(ScopeConfig{})

var testScopes sync.Map // testing.TB → *Scope

// TestScope returns the scope the testing APIs use for tb, creating it on first
// use and discarding it when the test finishes. Its namespace and seed are
// derived from tb.Name(), so a test that draws in the same order draws the same
// values on every run, and two tests draw different unique values. Pass it with
// [InScope] to make an error-returning API share the test's scope:
//
//	seedling.InsertOneE[User](ctx, db, seedling.InScope(seedling.TestScope(t)))
//
// The scope is discarded by a cleanup registered on first use, so a cleanup
// function registered earlier runs after it. A record such a function creates
// is planned in a second scope with its own namespace and seed, which is kept
// until the process exits, so it never repeats the test's values.
func TestScope(tb testing.TB) *Scope {
	tb.Helper()
	if scope, ok := testScopes.Load(tb); ok {
		return scope.(*Scope)
	}
	name := tb.Name()
	// The test context is canceled just before cleanup functions run. A
	// testing.T that was not created by the test runner has none.
	ctx := tb.Context()
	cleaningUp := ctx != nil && ctx.Err() != nil
	if cleaningUp {
		name += "\x00cleanup"
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	sum := h.Sum64()
	scope, loaded := testScopes.LoadOrStore(tb, NewScope(ScopeConfig{
		Namespace: strconv.FormatUint(sum%(36*36*36*36*36*36), 36),
		Seed:      sum,
	}))
	if !loaded && !cleaningUp {
		tb.Cleanup(func() { testScopes.Delete(tb) })
	}
	return scope.(*Scope)
}

// Seq returns the next value of the named sequence, starting at 1. Each name
// counts independently, and no two calls in a scope return the same value
// for the same name.
func (s *Scope) Seq(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seqs[name]++
	return s.seqs[name]
}

// Unique returns prefix followed by the next value of the sequence named
// prefix and, when the scope has one, its namespace, such as "user-3" or
// "user-3-k2x9q1". The result is unique within the scope.
func (s *Scope) Unique(prefix string) string {
	value := prefix + "-" + strconv.Itoa(s.Seq(prefix))
	if s.namespace != "" {
		value += "-" + s.namespace
	}
	return value
}

// UniqueN is like [Scope.Unique] for a column that holds at most maxLen
// bytes. It shortens prefix to make room for the suffix, then drops the
// prefix, then the namespace, keeping the value unique within the scope. Once
// even the bare sequence number is longer than maxLen, the column cannot hold
// another distinct value, and the number is returned whole so the insert fails
// instead of repeating a value. A maxLen of zero or less means no limit.
func (s *Scope) UniqueN(prefix string, maxLen int) string {
	n := strconv.Itoa(s.Seq(prefix))
	suffix := "-" + n
	if s.namespace != "" {
		suffix += "-" + s.namespace
	}
	switch {
	case maxLen <= 0 || len(prefix)+len(suffix) <= maxLen:
		return prefix + suffix
	case len(suffix) < maxLen:
		return prefix[:maxLen-len(suffix)] + suffix
	case s.namespace != "" && len(suffix)-1 <= maxLen:
		return suffix[1:]
	default:
		return n
	}
}

// Rand returns a new random source. Every call returns an independent source
// seeded from the scope's seed and the number of sources drawn before it, so
// the values a test draws depend only on its seed and the order of its calls.
// The returned source is not safe for concurrent use.
func (s *Scope) Rand() *rand.Rand {
	s.mu.Lock()
	s.draws++
	stream := s.draws
	s.mu.Unlock()
	//nolint:gosec // fixtures need reproducible, not secure, randomness
	return rand.New(rand.NewPCG(s.seed, stream^goldenGamma))
}

// UUID returns a random version 4 UUID in canonical form, such as
// "0f8c4a52-6a0e-4c1b-9d4e-3f2a1b0c9d8e", drawn from a new source of the scope
// like [Scope.Rand].
func (s *Scope) UUID() string {
	r := s.Rand()
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], r.Uint64())
	binary.BigEndian.PutUint64(b[8:], r.Uint64())
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 9562 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Now returns the scope's fixed clock reading.
func (s *Scope) Now() time.Time {
	return s.now
}

// Namespace returns the scope's namespace.
func (s *Scope) Namespace() string {
	return s.namespace
}

// resolveScope picks the first non-nil scope, falling back to the process scope.
func resolveScope(scopes ...*Scope) *Scope {
	for _, scope := range scopes {
		if scope != nil {
			return scope
		}
	}
	return processScope
}

// InScope plans the record and everything created for it in scope, overriding
// the scope the API would otherwise use. Like [WithContext] it applies only to
// the root record, including from a trait, and a [Scenario] takes its scope
// from [Scenario.WithScope] instead.
//
//	scope := seedling.TestScope(t)
//	seedling.InsertOneE[User](ctx, db, seedling.InScope(scope))
func InScope(scope *Scope) Option {
	return scopeOption{scope: scope}
}

type scopeOption struct {
	scope *Scope
}

func (s scopeOption) applyOption(r *optionResolver, o *optionSet) error {
	if !o.root {
		return rootOnlyError("in-scope")
	}
	if r.scenario != nil {
		return fmt.Errorf("%w: InScope does not apply to a record added to a Scenario; use Scenario.WithScope instead", ErrInvalidOption)
	}
	if s.scope == nil {
		return fmt.Errorf("%w: InScope scope must not be nil", ErrInvalidOption)
	}
	o.scope = s.scope
	return nil
}
