package orchestrator

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// ceilingReplayResult pairs a replay's disposition with the failure that
// produced it. err and detail are zero for every non-failure outcome, so
// settleCeilingReplay can log why a non-ceiling failure happened instead of
// only that one did.
type ceilingReplayResult struct {
	outcome ceilingReplayOutcome
	err     error
	detail  string
	// sessionID and turnID identify the launch an admitted replay created.
	// They are empty for outcomes that did not launch and for replay kinds
	// that create neither.
	sessionID string
	turnID    string
}

// failureDetail is the card-note and log text for a failed replay: the short
// detail followed by the error, when there is one.
func (r ceilingReplayResult) failureDetail() string {
	if r.err == nil {
		return r.detail
	}
	if r.detail == "" {
		return r.err.Error()
	}
	return r.detail + ": " + r.err.Error()
}

func ceilingReplaySucceededResult() ceilingReplayResult {
	return ceilingReplayResult{outcome: ceilingReplaySucceeded}
}

func ceilingReplayDeferredResult() ceilingReplayResult {
	return ceilingReplayResult{outcome: ceilingReplayStillDeferred}
}

func ceilingReplaySupersededResult() ceilingReplayResult {
	return ceilingReplayResult{outcome: ceilingReplaySuperseded}
}

func ceilingReplayRunClosedResult() ceilingReplayResult {
	return ceilingReplayResult{outcome: ceilingReplayRunClosed}
}

func ceilingReplayFailedResult(err error, detail string) ceilingReplayResult {
	return ceilingReplayResult{outcome: ceilingReplayFailed, err: err, detail: detail}
}

func ceilingReplayPermanentlyFailedResult(err error, detail string) ceilingReplayResult {
	return ceilingReplayResult{outcome: ceilingReplayPermanentlyFailed, err: err, detail: detail}
}

// ceilingReplayResultFromExecution is the shared classification for the entry
// points (start, start_created, resume — and, by the same contract, any future
// kind) that return (*executor.TaskExecution, error): a non-nil execution is
// success, a nil execution with a nil error is still-deferred (the seam's own
// gate re-persisted the record internally), a launch whose built command failed
// validation is permanently failed, and anything else is a non-ceiling failure
// whose error is retained for diagnostics.
func ceilingReplayResultFromExecution(execution *executor.TaskExecution, err error) ceilingReplayResult {
	switch {
	case errors.Is(err, ErrCeilingLaunchSuperseded):
		return ceilingReplaySupersededResult()
	case routingerr.IsBuiltCommandValidationError(err):
		return ceilingReplayPermanentlyFailedResult(err, "launch validation failed")
	case err != nil:
		return ceilingReplayFailedResult(err, "dispatch failed")
	case execution == nil:
		return ceilingReplayDeferredResult()
	default:
		result := ceilingReplaySucceededResult()
		result.sessionID = execution.SessionID
		result.turnID = execution.TurnID
		return result
	}
}

func ceilingReplayDispatchFailure(err error, detail string) ceilingReplayResult {
	if routingerr.IsBuiltCommandValidationError(err) {
		return ceilingReplayPermanentlyFailedResult(err, detail)
	}
	return ceilingReplayFailedResult(err, detail)
}
