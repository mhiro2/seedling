package planner

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/mhiro2/seedling/internal/errx"
)

// validatePlan checks the option set against the blueprint tree before graph expansion.
func validatePlan(reg Registry, bp *BlueprintDef, opts *OptionSet) error {
	return validateOptions(reg, bp, opts)
}

// validateOptions checks one node's options and recurses into its Refs. Only is
// resolved for the root record alone, so a nested option set never carries it.
func validateOptions(reg Registry, bp *BlueprintDef, opts *OptionSet) error {
	if opts == nil {
		return nil
	}

	rels := indexRelations(bp)

	for name := range opts.Only {
		if _, ok := rels.byName[name]; !ok {
			return fmt.Errorf("validate only %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
		}
	}

	// Validate Use targets and values.
	for name, value := range opts.Uses {
		rel, ok := rels.byName[name]
		if !ok {
			return fmt.Errorf("validate use %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
		}
		if rel.Kind != BelongsTo {
			return fmt.Errorf("validate use %q: %w", name, errx.UseOnNonBelongsTo(bp.Name, name, string(rel.Kind)))
		}
		if value == nil {
			return fmt.Errorf("%w: use %q value must not be nil", errx.ErrInvalidOption, name)
		}
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			return fmt.Errorf("%w: use %q value must not be nil", errx.ErrInvalidOption, name)
		}

		refBP, err := reg.LookupByName(rel.RefBlueprint)
		if err != nil {
			return fmt.Errorf("lookup use blueprint %q: %w", rel.RefBlueprint, err)
		}
		if err := validateUseValueType(rel.Name, refBP.ModelType, value); err != nil {
			return err
		}
	}

	// Validate Ref targets. Collection relations carry their options per
	// element in Collections instead.
	for name := range opts.Refs {
		rel, ok := rels.byName[name]
		if !ok {
			return fmt.Errorf("validate ref %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
		}
		if rel.Kind != BelongsTo {
			return fmt.Errorf("%w: ref %q on blueprint %q targets a %s relation; configure it per element", errx.ErrInvalidOption, name, bp.Name, rel.Kind)
		}
	}

	// Validate collection targets.
	for name := range opts.Collections {
		rel, ok := rels.byName[name]
		if !ok {
			return fmt.Errorf("validate ref %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
		}
		if rel.Kind != HasMany && rel.Kind != ManyToMany {
			return fmt.Errorf("%w: relation %q on blueprint %q is %s; per-element options apply only to has_many and many_to_many relations", errx.ErrInvalidOption, name, bp.Name, rel.Kind)
		}
	}

	// Validate Omit targets.
	for name := range opts.Omits {
		rel, ok := rels.byName[name]
		if !ok {
			return fmt.Errorf("validate omit %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
		}
		if rel.Required {
			return fmt.Errorf("validate omit %q: %w", name, errx.OmitRequiredRelation(bp.Name, name))
		}
	}

	// Check contradictions between relation options.
	for name := range opts.Uses {
		if _, ok := opts.Refs[name]; ok {
			return fmt.Errorf("validate relation %q options: %w", name, errx.UseAndRefConflict(bp.Name, name))
		}
		if opts.Omits[name] {
			return fmt.Errorf("validate relation %q options: %w", name, errx.OmitAndUseConflict(bp.Name, name))
		}
		// A Use'd relation is always bound, so a When predicate can never take
		// effect, whether it is passed as an option or declared on the relation.
		if _, ok := opts.Whens[name]; ok {
			return fmt.Errorf("validate relation %q options: %w", name, errx.UseAndWhenConflict(bp.Name, name))
		}
		if rel, ok := rels.byName[name]; ok && rel.When != nil {
			return fmt.Errorf("validate relation %q options: %w", name, errx.UseAndWhenConflict(bp.Name, name))
		}
	}
	for name := range opts.Refs {
		if opts.Omits[name] {
			return fmt.Errorf("validate relation %q options: %w", name, errx.OmitAndRefConflict(bp.Name, name))
		}
	}
	for name := range opts.Collections {
		if opts.Omits[name] {
			return fmt.Errorf("validate relation %q options: %w", name, errx.OmitAndRefConflict(bp.Name, name))
		}
	}

	// Validate When targets.
	for name := range opts.Whens {
		if _, ok := rels.byName[name]; !ok {
			return fmt.Errorf("validate when %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
		}
		if opts.Omits[name] {
			return fmt.Errorf("validate relation %q options: %w", name, errx.OmitAndWhenConflict(bp.Name, name))
		}
	}

	// Check contradiction: Set on a FK field.
	for _, set := range opts.Sets {
		if relName, ok := rels.localFieldToRel[set.Field]; ok {
			return fmt.Errorf("validate set %q: %w", set.Field, errx.SetOnFKField(bp.Name, set.Field, relName))
		}
	}

	// When Only is active, a relation that carries an explicit Use/Ref/When but is
	// not in the Only set is contradictory: Only would exclude it from expansion,
	// silently discarding the user's configuration. Surface it instead of dropping
	// it. (A required relation simply absent from Only is intentional minimal-graph
	// behaviour; see the Only documentation.) Only mode is keyed on a non-nil set,
	// matching the expander, so Only() with no relations still triggers the check.
	if opts.Only != nil {
		for name := range opts.Uses {
			if !opts.Only[name] {
				return fmt.Errorf("validate relation %q options: %w", name, errx.OnlyExcludesConfigured(bp.Name, name, "Use"))
			}
		}
		for name := range opts.Refs {
			if !opts.Only[name] {
				return fmt.Errorf("validate relation %q options: %w", name, errx.OnlyExcludesConfigured(bp.Name, name, "Ref"))
			}
		}
		for name := range opts.Collections {
			if !opts.Only[name] {
				return fmt.Errorf("validate relation %q options: %w", name, errx.OnlyExcludesConfigured(bp.Name, name, "Ref"))
			}
		}
		for name := range opts.Whens {
			if !opts.Only[name] {
				return fmt.Errorf("validate relation %q options: %w", name, errx.OnlyExcludesConfigured(bp.Name, name, "When"))
			}
		}
	}

	for name, refOpts := range opts.Refs {
		rel := rels.byName[name]
		refBP, err := reg.LookupByName(rel.RefBlueprint)
		if err != nil {
			return fmt.Errorf("lookup ref blueprint %q: %w", rel.RefBlueprint, err)
		}
		if err := validateOptions(reg, refBP, refOpts); err != nil {
			return fmt.Errorf("validate ref %q: %w", name, err)
		}
	}

	for name, elements := range opts.Collections {
		if err := validateCollection(reg, bp.Name, rels.byName[name], elements); err != nil {
			return fmt.Errorf("validate ref %q: %w", name, err)
		}
	}

	return nil
}

// validateCollection checks the per-element options of a has-many or
// many-to-many relation.
func validateCollection(reg Registry, blueprint string, rel RelationDef, elements []ElementOptions) error {
	refBP, err := reg.LookupByName(rel.RefBlueprint)
	if err != nil {
		return fmt.Errorf("lookup ref blueprint %q: %w", rel.RefBlueprint, err)
	}
	var joinBP *BlueprintDef
	if rel.Kind == ManyToMany && rel.ThroughBlueprint != "" {
		if joinBP, err = reg.LookupByName(rel.ThroughBlueprint); err != nil {
			return fmt.Errorf("lookup through blueprint %q: %w", rel.ThroughBlueprint, err)
		}
	}

	for i, elem := range elements {
		if elem.Through != nil && joinBP == nil {
			return fmt.Errorf("%w: element %d of relation %q on blueprint %q has Through options, but only many_to_many relations have a join record", errx.ErrInvalidOption, i, rel.Name, blueprint)
		}
		if elem.Use != nil {
			if rel.Kind != ManyToMany {
				return fmt.Errorf("validate use %q: %w", rel.Name, errx.UseOnNonBelongsTo(blueprint, rel.Name, string(rel.Kind)))
			}
			if !optionSetEmpty(elem.Opts) {
				return fmt.Errorf("%w: element %d of relation %q on blueprint %q is an existing record, so only Through options can apply to it", errx.ErrInvalidOption, i, rel.Name, blueprint)
			}
			rv := reflect.ValueOf(elem.Use)
			if rv.Kind() == reflect.Pointer && rv.IsNil() {
				return fmt.Errorf("%w: use %q element %d must not be nil", errx.ErrInvalidOption, rel.Name, i)
			}
			if err := validateUseValueType(rel.Name, refBP.ModelType, elem.Use); err != nil {
				return err
			}
		}
		if err := validateOptions(reg, refBP, elem.Opts); err != nil {
			return fmt.Errorf("element %d: %w", i, err)
		}
		if elem.Through != nil {
			if err := validateOptions(reg, joinBP, elem.Through); err != nil {
				return fmt.Errorf("element %d through %q: %w", i, rel.ThroughBlueprint, err)
			}
		}
	}
	return nil
}

// optionSetEmpty reports whether opts configures nothing.
func optionSetEmpty(opts *OptionSet) bool {
	return opts == nil ||
		len(opts.Sets) == 0 && len(opts.Uses) == 0 && len(opts.Refs) == 0 && len(opts.Collections) == 0 &&
			len(opts.Omits) == 0 && len(opts.Whens) == 0 && len(opts.WithFns) == 0 && len(opts.GenFns) == 0 &&
			opts.Rand == nil && opts.Only == nil
}

type relationIndex struct {
	byName          map[string]RelationDef
	localFieldToRel map[string]string
	available       []string
}

func indexRelations(bp *BlueprintDef) relationIndex {
	index := relationIndex{
		byName:          make(map[string]RelationDef, len(bp.Relations)),
		localFieldToRel: make(map[string]string, len(bp.Relations)),
		available:       make([]string, 0, len(bp.Relations)),
	}

	for _, rel := range bp.Relations {
		index.byName[rel.Name] = rel
		index.available = append(index.available, rel.Name)

		if rel.Kind != BelongsTo {
			continue
		}
		for _, fieldName := range localFieldsForRelation(rel) {
			index.localFieldToRel[fieldName] = rel.Name
		}
	}

	sort.Strings(index.available)
	return index
}

func validateUseValueType(relation string, expectedType reflect.Type, useVal any) error {
	useType := reflect.TypeOf(useVal)
	if useType == expectedType {
		return nil
	}
	// A value blueprint accepts a pointer to its model, and a pointer blueprint
	// accepts the bare struct. Both are normalized in normalizeUseValue. Any
	// deeper indirection (e.g. **T for a *T blueprint) stays rejected here so
	// the mismatch surfaces as a use error instead of a reflection failure
	// during FK binding.
	if expectedType != nil && expectedType.Kind() != reflect.Pointer &&
		useType != nil && useType.Kind() == reflect.Pointer && useType.Elem() == expectedType {
		return nil
	}
	if expectedType != nil && expectedType.Kind() == reflect.Pointer && expectedType.Elem() == useType {
		return nil
	}

	gotName := "<nil>"
	if useType != nil {
		gotName = useType.String()
	}
	expectedName := "<nil>"
	if expectedType != nil {
		expectedName = expectedType.String()
	}

	return fmt.Errorf("validate use %q: %w", relation, errx.UseTypeMismatch(relation, expectedName, gotName))
}
