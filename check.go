package seedling

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

// CheckOption configures [CheckRegistry].
type CheckOption func(*checkConfig)

type checkConfig struct {
	repeat         int
	skipBlueprints map[string]bool
	skipTraits     map[string]map[string]bool
	combinations   map[string][][]string
	failures       map[string][][]string
}

// CheckRepeat inserts every case n times within its subtest, drawing fresh
// defaults each time, so a unique column whose Defaults repeat a value fails
// the check. The default is 1.
func CheckRepeat(n int) CheckOption {
	return func(cfg *checkConfig) { cfg.repeat = n }
}

// SkipBlueprint leaves the named blueprints out of the check, such as a
// join blueprint that only makes sense under its many-to-many relation.
func SkipBlueprint(names ...string) CheckOption {
	return func(cfg *checkConfig) {
		for _, name := range names {
			cfg.skipBlueprints[name] = true
		}
	}
}

// SkipTrait leaves the named traits of a blueprint out of the check.
func SkipTrait(blueprint string, traits ...string) CheckOption {
	return func(cfg *checkConfig) {
		if cfg.skipTraits[blueprint] == nil {
			cfg.skipTraits[blueprint] = make(map[string]bool)
		}
		for _, trait := range traits {
			cfg.skipTraits[blueprint][trait] = true
		}
	}
}

// CheckTraits adds a case that applies the given traits of a blueprint
// together, in order. Each trait is already checked on its own.
func CheckTraits(blueprint string, traits ...string) CheckOption {
	return func(cfg *checkConfig) {
		cfg.combinations[blueprint] = append(cfg.combinations[blueprint], slices.Clone(traits))
	}
}

// ExpectFailure marks the case of a blueprint with exactly the given traits,
// or its defaults when no trait is given, as one whose insert must fail, such
// as a trait that deliberately violates a constraint for a negative test. The
// check then fails if the insert succeeds.
func ExpectFailure(blueprint string, traits ...string) CheckOption {
	return func(cfg *checkConfig) {
		cfg.failures[blueprint] = append(cfg.failures[blueprint], slices.Clone(traits))
	}
}

// checkCase is one insert CheckRegistry runs.
type checkCase struct {
	name        string
	traits      []string
	expectError bool
}

// CheckRegistry inserts every blueprint of reg through its own Insert
// callback, once with its defaults and once with each of its traits, so a
// fixture that a schema change broke — a new NOT NULL column, a CHECK its
// defaults violate, an Insert that no longer matches the table — fails right
// after the migration rather than in whichever test uses it next:
//
//	func TestFixtures(t *testing.T) {
//	    seedling.CheckRegistry(t, reg, func(t *testing.T) seedling.DBTX {
//	        return seedling.WithTx(t, db)
//	    })
//	}
//
// Each case runs as a subtest named blueprint/default or blueprint/trait,
// with the database handle db returns for it, so returning a transaction
// that rolls back keeps the cases independent. A failing case reports the
// blueprint, the traits, and the relation path of the record whose insert
// failed. Records are planned in the subtest's [TestScope], and a db function
// that calls [AutoCleanup] makes the inserts clean up after themselves.
//
// Options add trait combinations ([CheckTraits]), repeat each insert
// ([CheckRepeat]), skip blueprints or traits ([SkipBlueprint], [SkipTrait]),
// and mark cases that must fail ([ExpectFailure]). A nil reg checks the
// package default registry.
func CheckRegistry(t *testing.T, reg *Registry, db func(t *testing.T) DBTX, opts ...CheckOption) {
	t.Helper()
	reg = resolveRegistry(reg)
	cfg := checkConfig{
		repeat:         1,
		skipBlueprints: make(map[string]bool),
		skipTraits:     make(map[string]map[string]bool),
		combinations:   make(map[string][][]string),
		failures:       make(map[string][][]string),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if db == nil {
		t.Fatal("seedling: CheckRegistry needs a db function")
	}
	if cfg.repeat < 1 {
		t.Fatalf("seedling: CheckRepeat must be >= 1, got %d", cfg.repeat)
	}

	defs := registeredBlueprints(reg)
	if err := cfg.validate(defs); err != nil {
		t.Fatal(err)
	}
	for _, name := range slices.Sorted(maps.Keys(defs)) {
		if cfg.skipBlueprints[name] {
			continue
		}
		def := defs[name]
		t.Run(name, func(t *testing.T) {
			for _, c := range cfg.cases(def) {
				t.Run(c.name, func(t *testing.T) {
					runCheckCase(t, reg, def, c, cfg.repeat, db(t))
				})
			}
		})
	}
}

func registeredBlueprints(reg *Registry) map[string]*blueprintDef {
	r := reg.reg
	r.mu.RLock()
	defer r.mu.RUnlock()
	return maps.Clone(r.byName)
}

// validate reports an option naming a blueprint or trait that is not
// registered, so a typo cannot silently drop a case.
func (cfg checkConfig) validate(defs map[string]*blueprintDef) error {
	check := func(option, blueprint string, traits []string) error {
		def, ok := defs[blueprint]
		if !ok {
			return fmt.Errorf("seedling: %s names blueprint %q, which is not registered", option, blueprint)
		}
		for _, trait := range traits {
			if _, ok := def.traits[trait]; !ok {
				return fmt.Errorf("seedling: %s names trait %q, which blueprint %q does not define", option, trait, blueprint)
			}
		}
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.skipBlueprints)) {
		if err := check("SkipBlueprint", name, nil); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.skipTraits)) {
		if err := check("SkipTrait", name, slices.Sorted(maps.Keys(cfg.skipTraits[name]))); err != nil {
			return err
		}
	}
	for option, sets := range map[string]map[string][][]string{"CheckTraits": cfg.combinations, "ExpectFailure": cfg.failures} {
		for _, name := range slices.Sorted(maps.Keys(sets)) {
			for _, traits := range sets[name] {
				if err := check(option, name, traits); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// cases lists the inserts to check for a blueprint: its defaults, each trait
// on its own, and each combination added with CheckTraits.
func (cfg checkConfig) cases(def *blueprintDef) []checkCase {
	cases := []checkCase{{name: "default"}}
	for _, trait := range slices.Sorted(maps.Keys(def.traits)) {
		if !cfg.skipTraits[def.name][trait] {
			cases = append(cases, checkCase{name: trait, traits: []string{trait}})
		}
	}
	for _, traits := range cfg.combinations[def.name] {
		cases = append(cases, checkCase{name: strings.Join(traits, "+"), traits: traits})
	}
	for i, c := range cases {
		cases[i].expectError = slices.ContainsFunc(cfg.failures[def.name], func(traits []string) bool {
			return slices.Equal(traits, c.traits)
		})
	}
	return cases
}

func runCheckCase(t *testing.T, reg *Registry, def *blueprintDef, c checkCase, repeat int, db DBTX) {
	t.Helper()
	label := fmt.Sprintf("blueprint %q with defaults", def.name)
	if len(c.traits) > 0 {
		label = fmt.Sprintf("blueprint %q with traits %s", def.name, strings.Join(c.traits, ", "))
	}
	opts := make([]Option, len(c.traits))
	for i, trait := range c.traits {
		opts[i] = BlueprintTrait(trait)
	}

	for i := range repeat {
		err := checkInsert(t, reg, def, opts, db)
		switch {
		case c.expectError && err == nil:
			t.Errorf("%s: insert %d succeeded, but ExpectFailure says it must fail", label, i+1)
			return
		case c.expectError:
			t.Logf("%s: insert failed as expected: %v", label, err)
			return
		case err != nil && repeat > 1:
			t.Errorf("%s: insert %d of %d: %v", label, i+1, repeat, err)
			return
		case err != nil:
			t.Errorf("%s: %v", label, err)
			return
		}
	}
}

// checkInsert plans and inserts one record of def as the testing APIs would,
// in the test's scope and with the tracker AutoCleanup enabled for it.
func checkInsert(t *testing.T, reg *Registry, def *blueprintDef, opts []Option, db DBTX) error {
	t.Helper()
	plan, err := buildPlan[any](reg, TestScope(t), db, nil, def.modelType, opts)
	if err != nil {
		return err
	}
	_, err = plan.insertE(t.Context(), db, resolveCleanup(plan.cleanup, cleanupTrackerFor(t)))
	return err
}
