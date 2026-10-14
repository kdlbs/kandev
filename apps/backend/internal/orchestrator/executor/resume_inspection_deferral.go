package executor

import (
	"errors"

	"github.com/kandev/kandev/internal/worktree"
)

type safeResumeInspectionDeferralError struct {
	cause error
}

func (e *safeResumeInspectionDeferralError) Error() string {
	return e.cause.Error()
}

func (e *safeResumeInspectionDeferralError) Unwrap() error {
	return e.cause
}

func safeResumeInspectionDeferral(err error) error {
	if !worktree.IsRecoveryInspectionContentionOnly(err) {
		return err
	}
	return &safeResumeInspectionDeferralError{cause: err}
}

// IsSafeResumeInspectionDeferral reports a pure inspection contention error
// that resume admission has verified is safe to retry without failure writes.
func IsSafeResumeInspectionDeferral(err error) bool {
	if !worktree.IsRecoveryInspectionContentionOnly(err) {
		return false
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if _, ok := current.(*safeResumeInspectionDeferralError); ok {
			return true
		}
	}
	return false
}
