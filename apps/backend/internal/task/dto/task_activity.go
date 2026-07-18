package dto

import (
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// EnrichTaskForegroundActivity stamps the task-level MOST-ACTIVE-WINS activity
// aggregate onto a TaskDTO from the task's sessions. It is a no-op for a nil DTO
// or nil provider, so the field is emitted (via omitempty) only where a live
// activity tracker is wired and never fabricated otherwise. The provider is
// consulted only for RUNNING sessions, matching the per-session enrich contract:
// a non-RUNNING session never fabricates a substate.
func EnrichTaskForegroundActivity(dto *TaskDTO, sessions []*models.TaskSession, provider ForegroundActivityProvider) {
	if dto == nil || provider == nil {
		return
	}
	dto.ForegroundActivity = v1.AggregateForegroundActivity(sessionForegroundActivities(sessions, provider))
}

// sessionForegroundActivities resolves the foreground activity of each RUNNING
// session via the provider, leaving non-RUNNING sessions empty (they carry no
// busy substate). The result feeds v1.AggregateForegroundActivity.
func sessionForegroundActivities(sessions []*models.TaskSession, provider ForegroundActivityProvider) []v1.ForegroundActivity {
	activities := make([]v1.ForegroundActivity, 0, len(sessions))
	for _, session := range sessions {
		if session == nil || session.State != models.TaskSessionStateRunning {
			continue
		}
		activities = append(activities, provider.ForegroundActivity(session.ID))
	}
	return activities
}
