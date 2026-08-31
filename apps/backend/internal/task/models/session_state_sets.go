package models

// IsActiveSessionState reports whether a session is in the "active" set used by
// GetActiveTaskSessionByTaskID's SQL filter
// (internal/task/repository/sqlite/session.go): Created, Starting, Running, and
// WaitingForInput. TestActiveSessionStateMatchesSQLFilter cross-checks this
// function against that SQL filter for every state in AllTaskSessionStates, so
// editing the SQL filter or this switch without updating the other fails that
// test. That guard only covers the states already in AllTaskSessionStates —
// adding a new TaskSessionState constant without also adding it there (and,
// if it should be active, to this switch) is not caught by anything: Go does
// not enforce switch/slice exhaustiveness.
//
// This is a different set from its neighbour IsResumableSessionState: active
// INCLUDES Created (a session not yet picked up still counts as the task's
// active one) and EXCLUDES Idle (an office session between turns is resumable
// but not the task's currently active session); resumable is the exact inverse
// on both points.
//
// Currently has no production caller: it exists ahead of the pending
// delegation from office/engine_dispatcher.isActiveDeciderSessionState, which
// is blocked on PR #3218 merging (see follow-up task
// d6bb06fb-c97b-40d8-8f5f-b5cfde64873d).
func IsActiveSessionState(state TaskSessionState) bool {
	switch state {
	case TaskSessionStateCreated, TaskSessionStateStarting,
		TaskSessionStateRunning, TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}
