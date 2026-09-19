package dto

import "github.com/kandev/kandev/internal/task/models"

// IsInputCapableSession reports whether session is in a state that can carry
// a pending user-input action (permission or clarification): RUNNING or
// WAITING_FOR_INPUT. A session in any other state cannot be blocking on user
// input, regardless of stale message metadata.
func IsInputCapableSession(session *models.TaskSession) bool {
	return session != nil &&
		(session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateWaitingForInput)
}

// PendingActionPtr resolves the pending action for a single session ID
// against a pending-actions projection (as returned by
// Service.GetPendingActionsForSessions), matching the *string, no-omitempty
// JSON shape used by TaskDTO.PrimarySessionPendingAction and
// TaskSessionDTO.PendingAction: nil (JSON null) when the session ID is nil or
// carries no pending action.
func PendingActionPtr(sessionID *string, actionsBySession map[string]models.TaskPendingAction) *string {
	if sessionID == nil {
		return nil
	}
	action, ok := actionsBySession[*sessionID]
	if !ok {
		return nil
	}
	value := string(action)
	return &value
}

// TaskPendingActionPtr aggregates a task-level pending action across its
// input-capable sessions: a permission request on any session wins
// immediately, otherwise a clarification request on any session wins. A task
// with no blocked input-capable session gets nil (JSON null), matching
// TaskDTO.TaskPendingAction's *string field with no omitempty.
func TaskPendingActionPtr(sessions []*models.TaskSession, actionsBySession map[string]models.TaskPendingAction) *string {
	var clarification bool
	for _, session := range sessions {
		if !IsInputCapableSession(session) {
			continue
		}
		switch actionsBySession[session.ID] {
		case models.TaskPendingActionPermission:
			value := string(models.TaskPendingActionPermission)
			return &value
		case models.TaskPendingActionClarification:
			clarification = true
		}
	}
	if clarification {
		value := string(models.TaskPendingActionClarification)
		return &value
	}
	return nil
}
