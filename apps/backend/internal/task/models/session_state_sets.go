package models

// IsActiveSessionState reports whether a session is in the "active" set used by
// GetActiveTaskSessionByTaskID's SQL filter
// (internal/task/repository/sqlite/session.go): Created, Starting, Running, and
// WaitingForInput. TestActiveSessionStateMatchesSQLFilter cross-checks this
// function against that SQL filter directly, so the two cannot drift silently —
// edit one without the other and the test fails.
//
// This is a different set from its neighbour IsResumableSessionState: active
// INCLUDES Created (a session not yet picked up still counts as the task's
// active one) and EXCLUDES Idle (an office session between turns is resumable
// but not the task's currently active session); resumable is the exact inverse
// on both points.
func IsActiveSessionState(state TaskSessionState) bool {
	switch state {
	case TaskSessionStateCreated, TaskSessionStateStarting,
		TaskSessionStateRunning, TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}
