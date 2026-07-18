package dto

import (
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// sessionActivity is the minimal per-session view the task-level aggregate needs:
// the coarse session state plus the fine-grained foreground activity resolved
// for a RUNNING session.
type sessionActivity struct {
	State    models.TaskSessionState
	Activity v1.ForegroundActivity
}

// aggregateForegroundActivity computes the task-level three-state activity from a
// task's sessions using MOST-ACTIVE-WINS (§spec:task-level-indicator):
//
//   - "generating" — any session is generating (the foreground agent is producing
//     output);
//   - "background" — none is generating but at least one RUNNING session is
//     holding a turn open for spawned background work;
//   - ""           — no session is running, so task-level surfaces fall through to
//     the coarse task state (done / waiting / failed).
//
// Only RUNNING sessions carry a meaningful ForegroundActivity; a non-RUNNING
// session never contributes a busy substate. A finished primary session therefore
// does not mask a secondary session that is still working — the intended
// consequence: a task is not "done" while any of its sessions is still working.
func aggregateForegroundActivity(sessions []sessionActivity) v1.ForegroundActivity {
	sawBackground := false
	for _, s := range sessions {
		if s.State != models.TaskSessionStateRunning {
			continue
		}
		if s.Activity == v1.ForegroundActivityGenerating {
			return v1.ForegroundActivityGenerating
		}
		if s.Activity == v1.ForegroundActivityBackground {
			sawBackground = true
		}
	}
	if sawBackground {
		return v1.ForegroundActivityBackground
	}
	return ""
}

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
	acts := make([]sessionActivity, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		var activity v1.ForegroundActivity
		if session.State == models.TaskSessionStateRunning {
			activity = provider.ForegroundActivity(session.ID)
		}
		acts = append(acts, sessionActivity{State: session.State, Activity: activity})
	}
	dto.ForegroundActivity = aggregateForegroundActivity(acts)
}
