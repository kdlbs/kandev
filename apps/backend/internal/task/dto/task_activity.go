package dto

import (
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// EnrichTaskForegroundActivity stamps the task-level MOST-ACTIVE-WINS activity
// aggregate onto a TaskDTO from the task's sessions. It is a no-op for a nil DTO
// or nil provider, so the field is emitted (via omitempty) only where a live
// activity tracker is wired and never fabricated otherwise. The provider is
// consulted for RUNNING sessions and for settled sessions that may still carry
// detached background liveness.
func EnrichTaskForegroundActivity(dto *TaskDTO, sessions []*models.TaskSession, provider ForegroundActivityProvider) {
	if dto == nil || provider == nil {
		return
	}
	dto.ForegroundActivity = v1.AggregateForegroundActivity(sessionForegroundActivities(sessions, provider))
}

// sessionForegroundActivities resolves generating activity for RUNNING sessions
// and detached background activity for any coarse state.
func sessionForegroundActivities(sessions []*models.TaskSession, provider ForegroundActivityProvider) []v1.ForegroundActivity {
	activities := make([]v1.ForegroundActivity, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if activity, ok := sessionForegroundActivity(session.ID, session.State, provider); ok {
			activities = append(activities, activity)
		}
	}
	return activities
}

func sessionForegroundActivity(
	sessionID string,
	state models.TaskSessionState,
	provider ForegroundActivityProvider,
) (v1.ForegroundActivity, bool) {
	activity := provider.ForegroundActivity(sessionID)
	if state == models.TaskSessionStateRunning || activity == v1.ForegroundActivityBackground {
		return activity, true
	}
	return "", false
}
