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

// admittedSessionStates classifies every TaskSessionState by whether a session in
// that state holds a unit of the instance-wide session ceiling. Every state is
// listed explicitly: a state added to AllTaskSessionStates without a decision here
// is absent from this map rather than defaulted into the counted set, and
// TestAdmittedSessionStatesCoverEveryState fails on the omission.
var admittedSessionStates = map[TaskSessionState]bool{
	TaskSessionStateCreated:         false,
	TaskSessionStateStarting:        true,
	TaskSessionStateRunning:         true,
	TaskSessionStateIdle:            false,
	TaskSessionStateWaitingForInput: false,
	TaskSessionStateCompleted:       false,
	TaskSessionStateFailed:          false,
	TaskSessionStateCancelled:       false,
}

// IsAdmittedSessionState reports whether a session in this state is running or
// starting an agent process, and so occupies a unit of the instance-wide session
// ceiling.
//
// This is a different set from its neighbour IsTaskLookupActiveSessionState, which
// additionally counts Created and WaitingForInput: those hold a task's active-session
// slot and a worktree, but no process and no core.
func IsAdmittedSessionState(state TaskSessionState) bool {
	return admittedSessionStates[state]
}
