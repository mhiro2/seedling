package seedling

import "github.com/mhiro2/seedling/internal/errx"

// Public error sentinels. Use errors.Is() to check.
var (
	ErrBlueprintNotFound = errx.ErrBlueprintNotFound
	ErrRelationNotFound  = errx.ErrRelationNotFound
	ErrFieldNotFound     = errx.ErrFieldNotFound
	ErrCycleDetected     = errx.ErrCycleDetected
	ErrTypeMismatch      = errx.ErrTypeMismatch

	// ErrInsertFailed reports that a blueprint Insert callback failed.
	// Returned errors also unwrap to the original callback error.
	ErrInsertFailed = errx.ErrInsertFailed

	// ErrDeleteFailed reports that a blueprint Delete callback failed.
	ErrDeleteFailed = errx.ErrDeleteFailed

	// ErrDeleteNotDefined reports that a blueprint has no Delete function
	// but Cleanup was called.
	ErrDeleteNotDefined = errx.ErrDeleteNotDefined

	ErrDuplicateBlueprint = errx.ErrDuplicateBlueprint
	ErrInvalidOption      = errx.ErrInvalidOption
)

// InsertFailedError wraps ErrInsertFailed with the blueprint name and the
// original insert callback error. Use errors.As to extract the blueprint name:
//
//	var ife *seedling.InsertFailedError
//	if errors.As(err, &ife) {
//	    log.Printf("blueprint %s failed", ife.Blueprint())
//	}
type InsertFailedError = errx.InsertFailedError

// DeleteFailedError wraps ErrDeleteFailed with the blueprint name and the
// original delete callback error. Use errors.As to extract the blueprint name:
//
//	var dfe *seedling.DeleteFailedError
//	if errors.As(err, &dfe) {
//	    log.Printf("blueprint %s failed", dfe.Blueprint())
//	}
type DeleteFailedError = errx.DeleteFailedError
