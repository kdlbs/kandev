package move

import (
	"errors"
)

// Stable classification sentinels for move validation. Error messages never
// include option values because instructions may be sensitive user input.
var (
	ErrConflictingInstructions       = errors.New("entry options instructions conflict with legacy prompt")
	ErrEntryOptionsRequireStepChange = errors.New("entry options require a workflow step change")
	ErrEntryTargetUnavailable        = errors.New("workflow move entry target has no session or auto-start")
	ErrEntryOptionsUnsupported       = errors.New("workflow move entry options unsupported")
	ErrMoveConflict                  = errors.New("another workflow move is already pending")
	ErrPermanentPendingMoveMismatch  = errors.New("pending workflow move is permanently invalid")
	// ErrPendingMoveActiveSession is transient: a deferred move must remain
	// queued until every session other than its source has stopped running.
	ErrPendingMoveActiveSession = errors.New("pending workflow move has another active session")
)

// ConflictingInstructionsError reports that the nested instructions and the
// legacy prompt alias both supplied different values. It intentionally carries
// no user-provided strings.
type ConflictingInstructionsError struct{}

func (ConflictingInstructionsError) Error() string {
	return ErrConflictingInstructions.Error()
}

func (ConflictingInstructionsError) Unwrap() error {
	return ErrConflictingInstructions
}

// EntryOptionsNotAllowedError reports that options were supplied for a move
// which does not change workflow step. Change is a closed, non-sensitive enum.
type EntryOptionsNotAllowedError struct {
	Change MoveChange
}

func (e EntryOptionsNotAllowedError) Error() string {
	if e.Change == MoveChangePositionOnly {
		return "entry options are not allowed for a position-only move"
	}
	return "entry options are not allowed for a move without a workflow step change"
}

func (EntryOptionsNotAllowedError) Unwrap() error {
	return ErrEntryOptionsRequireStepChange
}
