package planner

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/mhiro2/seedling/internal/errx"
)

// validatePlan checks the option set against the blueprint tree before graph expansion.
func validatePlan(reg Registry, bp *BlueprintDef, opts *OptionSet) error {
	return validateOptions(reg, bp, opts, true)
}

func validateOptions(reg Registry, bp *BlueprintDef, opts *OptionSet, allowOnly bool) error {
	if opts == nil {
		return nil
	}

	rels := indexRelations(bp)

	if !allowOnly && len(opts.Only) > 0 {
		return fmt.Errorf("validate only: %w", errx.OnlyOutsideRoot())
	}
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

		refBP, err := reg.LookupByName(rel.RefBlueprint)
		if err != nil {
			return fmt.Errorf("lookup use blueprint %q: %w", rel.RefBlueprint, err)
		}
		if err := validateUseValueType(rel.Name, refBP.ModelType, value); err != nil {
			return err
		}
	}

	// Validate Ref targets.
	for name := range opts.Refs {
		if _, ok := rels.byName[name]; !ok {
			return fmt.Errorf("validate ref %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, rels.available))
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
	}
	for name := range opts.Refs {
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
	for field := range opts.Sets {
		if relName, ok := rels.localFieldToRel[field]; ok {
			return fmt.Errorf("validate set %q: %w", field, errx.SetOnFKField(bp.Name, field, relName))
		}
	}

	for name, refOpts := range opts.Refs {
		rel := rels.byName[name]
		refBP, err := reg.LookupByName(rel.RefBlueprint)
		if err != nil {
			return fmt.Errorf("lookup ref blueprint %q: %w", rel.RefBlueprint, err)
		}
		if err := validateOptions(reg, refBP, refOpts, false); err != nil {
			return fmt.Errorf("validate ref %q: %w", name, err)
		}
	}

	return nil
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
	if useType != nil && useType.Kind() == reflect.Pointer && useType.Elem() == expectedType {
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
