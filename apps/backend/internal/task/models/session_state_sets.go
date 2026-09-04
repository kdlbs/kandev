package models

// IsTaskLookupActiveSessionState reports whether a session belongs to the active
// set used by GetActiveTaskSessionByTaskID. The active states are Created,
// Starting, Running, and WaitingForInput. TestActiveSessionStateMatchesSQLFilter
// cross-checks this predicate against the SQL filter for every state in
// AllTaskSessionStates.
//
// This is a different set from its neighbour IsResumableSessionState: active
// INCLUDES Created (a session not yet picked up still counts as the task's
// active one) and EXCLUDES Idle (an office session between turns is resumable
// but not the task's currently active session); resumable is the exact inverse
// on both points.
func IsTaskLookupActiveSessionState(state TaskSessionState) bool {
	switch state {
	case TaskSessionStateCreated, TaskSessionStateStarting,
		TaskSessionStateRunning, TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}
