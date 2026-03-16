package planner

import (
	"fmt"
	"sort"

	"github.com/mhiro2/seedling/internal/errx"
)

// validate checks the option set against the blueprint for invalid references.
func validate(bp *BlueprintDef, opts *OptionSet) error {
	if opts == nil {
		return nil
	}

	relNames := make(map[string]bool, len(bp.Relations))
	relKinds := make(map[string]RelationKind, len(bp.Relations))
	for _, r := range bp.Relations {
		relNames[r.Name] = true
		relKinds[r.Name] = r.Kind
	}

	availableRels := func() []string {
		names := make([]string, 0, len(relNames))
		for n := range relNames {
			names = append(names, n)
		}
		sort.Strings(names)
		return names
	}

	// Validate Use targets.
	for name := range opts.Uses {
		if !relNames[name] {
			return fmt.Errorf("validate use %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, availableRels()))
		}
		if relKinds[name] != BelongsTo {
			return fmt.Errorf("validate use %q: %w", name, errx.UseOnNonBelongsTo(bp.Name, name, string(relKinds[name])))
		}
	}

	// Validate Ref targets.
	for name := range opts.Refs {
		if !relNames[name] {
			return fmt.Errorf("validate ref %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, availableRels()))
		}
	}

	// Validate Omit targets.
	for name := range opts.Omits {
		if !relNames[name] {
			return fmt.Errorf("validate omit %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, availableRels()))
		}
	}

	// Check contradiction: Use and Ref on the same relation.
	for name := range opts.Uses {
		if _, ok := opts.Refs[name]; ok {
			return fmt.Errorf("validate relation %q options: %w", name, errx.UseAndRefConflict(bp.Name, name))
		}
	}

	// Check contradiction: Omit on a Required relation.
	relRequired := make(map[string]bool, len(bp.Relations))
	for _, r := range bp.Relations {
		relRequired[r.Name] = r.Required
	}
	for name := range opts.Omits {
		if relRequired[name] {
			return fmt.Errorf("validate omit %q: %w", name, errx.OmitRequiredRelation(bp.Name, name))
		}
	}

	// Validate When targets.
	for name := range opts.Whens {
		if !relNames[name] {
			return fmt.Errorf("validate when %q: %w", name, errx.RelationNotFoundWithHint(bp.Name, name, availableRels()))
		}
	}

	// Check contradiction: Set on a FK field.
	localFieldToRel := make(map[string]string, len(bp.Relations))
	for _, r := range bp.Relations {
		if r.Kind != BelongsTo {
			continue
		}
		for _, fieldName := range localFieldsForRelation(r) {
			localFieldToRel[fieldName] = r.Name
		}
	}
	for field := range opts.Sets {
		if relName, ok := localFieldToRel[field]; ok {
			return fmt.Errorf("validate set %q: %w", field, errx.SetOnFKField(bp.Name, field, relName))
		}
	}

	return nil
}
