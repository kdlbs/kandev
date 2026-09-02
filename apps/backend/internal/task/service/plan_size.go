package service

import (
	"errors"
	"fmt"
)

// MaxPlanContentBytes bounds the plan content a single write may store,
// measured in UTF-8 bytes via len(content). Stored plan content is read back
// and injected into every session handover for its task, so an unbounded
// write lets one caller impose a repeated, unbounded cost on every later
// launch. The unit is bytes rather than runes on purpose: unlike
// planTruncationDetected, which counts runes because it reasons about how
// much of a document survived, this bounds memory and scan cost, which is
// proportional to bytes. Matches maxUserStateBodyBytes in
// internal/plugins/user_state_handlers.go.
const MaxPlanContentBytes = 256 << 10 // 256 KiB

// ErrPlanContentTooLarge is the sentinel matched via errors.Is. The message
// carries the submitted size and is only available through
// PlanContentTooLargeError, so a fixed sentinel alone is not enough for
// callers that need to report the actual numbers.
var ErrPlanContentTooLarge = errors.New("plan content exceeds the size limit")

// PlanContentTooLargeError reports a plan write refused for exceeding
// MaxPlanContentBytes, carrying both numbers so the caller (agent or browser)
// can act on the rejection instead of retrying it unchanged.
type PlanContentTooLargeError struct {
	Submitted int
	Limit     int
}

func (e *PlanContentTooLargeError) Error() string {
	return fmt.Sprintf(
		"plan content is %d bytes, exceeding the %d-byte limit; nothing was stored and the task's existing plan is unchanged. Shorten the document you are holding before writing again — do not resubmit this content unchanged, and do not reconstruct the plan from memory.",
		e.Submitted, e.Limit,
	)
}

// Is lets errors.Is(err, ErrPlanContentTooLarge) match this type without
// exposing the exact submitted/limit values to a caller that only cares
// about the class of failure.
func (e *PlanContentTooLargeError) Is(target error) bool {
	return target == ErrPlanContentTooLarge
}

// checkPlanContentSize is the shared write-seam admission check: a pure
// function of the submitted content, called from both CreatePlan and
// UpdatePlan after task access/existence checks and before plan storage or the
// per-task write lock.
func checkPlanContentSize(content string) error {
	if n := len(content); n > MaxPlanContentBytes {
		return &PlanContentTooLargeError{Submitted: n, Limit: MaxPlanContentBytes}
	}
	return nil
}
