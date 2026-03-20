package errx

import (
	"errors"
	"fmt"
)

var (
	ErrBlueprintNotFound  = errors.New("seedling: blueprint not found")
	ErrRelationNotFound   = errors.New("seedling: relation not found")
	ErrFieldNotFound      = errors.New("seedling: field not found")
	ErrCycleDetected      = errors.New("seedling: cycle detected in dependency graph")
	ErrTypeMismatch       = errors.New("seedling: type mismatch")
	ErrInsertFailed       = errors.New("seedling: insert failed")
	ErrDeleteFailed       = errors.New("seedling: delete failed")
	ErrDeleteNotDefined   = errors.New("seedling: delete not defined")
	ErrDuplicateBlueprint = errors.New("seedling: duplicate blueprint")
	ErrInvalidOption      = errors.New("seedling: invalid option")
)

func BlueprintNotFound(name string) error {
	return fmt.Errorf("%w: %q", ErrBlueprintNotFound, name)
}

func RelationNotFound(blueprint, relation string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q", ErrRelationNotFound, relation, blueprint)
}

func FieldNotFound(typeName, field string) error {
	return fmt.Errorf("%w: field %q on type %q", ErrFieldNotFound, field, typeName)
}

func TypeMismatch(field, expected, got string) error {
	return fmt.Errorf("%w: field %q expects %s but got %s", ErrTypeMismatch, field, expected, got)
}

func InsertFailed(blueprint string, err error) error {
	return &InsertFailedError{
		blueprint: blueprint,
		err:       err,
	}
}

func DeleteFailed(blueprint string, err error) error {
	return &DeleteFailedError{
		blueprint: blueprint,
		err:       err,
	}
}

func DeleteNotDefined(blueprint string) error {
	return fmt.Errorf("%w: blueprint %q has no Delete function; define Blueprint.Delete to use Cleanup", ErrDeleteNotDefined, blueprint)
}

func DuplicateBlueprint(name string) error {
	return fmt.Errorf("%w: %q", ErrDuplicateBlueprint, name)
}

func CycleDetected(nodeIDs []string) error {
	return fmt.Errorf("%w: nodes %v", ErrCycleDetected, nodeIDs)
}

func FieldNotFoundWithHint(typeName, field string, available []string) error {
	return fmt.Errorf("%w: field %q on type %q; available fields: %v", ErrFieldNotFound, field, typeName, available)
}

func RelationNotFoundWithHint(blueprint, relation string, available []string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q; available relations: %v", ErrRelationNotFound, relation, blueprint, available)
}

func UseAndRefConflict(blueprint, relation string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q has both Use and Ref; remove one to resolve the conflict", ErrInvalidOption, relation, blueprint)
}

func OmitAndUseConflict(blueprint, relation string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q has both Omit and Use", ErrInvalidOption, relation, blueprint)
}

func OmitAndRefConflict(blueprint, relation string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q has both Omit and Ref", ErrInvalidOption, relation, blueprint)
}

func OmitAndWhenConflict(blueprint, relation string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q has both Omit and When", ErrInvalidOption, relation, blueprint)
}

func OmitRequiredRelation(blueprint, relation string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q is required but was Omit'd", ErrInvalidOption, relation, blueprint)
}

func OnlyOutsideRoot() error {
	return fmt.Errorf("%w: only must be declared on root options", ErrInvalidOption)
}

func SetOnFKField(blueprint, field, relation string) error {
	return fmt.Errorf("%w: field %q on blueprint %q is the FK for relation %q and will be overwritten by the executor; use Use(%q, ...) instead", ErrInvalidOption, field, blueprint, relation, relation)
}

func UseOnNonBelongsTo(blueprint, relation, kind string) error {
	return fmt.Errorf("%w: relation %q on blueprint %q is %s; Use is only supported for belongs_to relations", ErrInvalidOption, relation, blueprint, kind)
}

func UseTypeMismatch(relation, expected, got string) error {
	return fmt.Errorf("%w: Use(%q) expects type %s but got %s", ErrTypeMismatch, relation, expected, got)
}

// InsertFailedError wraps ErrInsertFailed with the blueprint name and the
// original insert callback error. Use errors.As to extract the blueprint name:
//
//	var ife *errx.InsertFailedError
//	if errors.As(err, &ife) {
//	    log.Printf("blueprint %s failed", ife.Blueprint())
//	}
type InsertFailedError struct {
	blueprint string
	err       error
}

// Blueprint returns the name of the blueprint whose Insert callback failed.
func (e *InsertFailedError) Blueprint() string { return e.blueprint }

func (e *InsertFailedError) Error() string {
	return fmt.Sprintf("%s: blueprint %q: %v", ErrInsertFailed, e.blueprint, e.err)
}

func (e *InsertFailedError) Unwrap() []error {
	return []error{e.err, ErrInsertFailed}
}

// DeleteFailedError wraps ErrDeleteFailed with the blueprint name and the
// original delete callback error. Use errors.As to extract the blueprint name:
//
//	var dfe *errx.DeleteFailedError
//	if errors.As(err, &dfe) {
//	    log.Printf("blueprint %s failed", dfe.Blueprint())
//	}
type DeleteFailedError struct {
	blueprint string
	err       error
}

// Blueprint returns the name of the blueprint whose Delete callback failed.
func (e *DeleteFailedError) Blueprint() string { return e.blueprint }

func (e *DeleteFailedError) Error() string {
	return fmt.Sprintf("%s: blueprint %q: %v", ErrDeleteFailed, e.blueprint, e.err)
}

func (e *DeleteFailedError) Unwrap() []error {
	return []error{e.err, ErrDeleteFailed}
}
