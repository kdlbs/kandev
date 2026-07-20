package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ForegroundActivityProvider surfaces the live fine-grained busy substate of a
// RUNNING session (ADR-0047), satisfied by the orchestrator. The task service
// depends only on this narrow seam so it takes no hard orchestrator dependency
// and can be faked in tests.
type ForegroundActivityProvider interface {
	ForegroundActivity(sessionID string) v1.ForegroundActivity
}

// SetForegroundActivityProvider wires the live per-session activity tracker used
// to compute the task-level MOST-ACTIVE-WINS aggregate. Optional; when unset the
// aggregate is left empty and task-level surfaces fall through to the coarse
// task state.
func (s *Service) SetForegroundActivityProvider(provider ForegroundActivityProvider) {
	s.foregroundActivity = provider
}

// computeTaskForegroundActivity resolves the task-level MOST-ACTIVE-WINS activity
// aggregate for a task from its currently-active sessions
// (§spec:task-level-indicator). Only RUNNING sessions carry a busy substate, so a
// finished primary session never masks a secondary session that is still working.
//
// The second return value reports whether the aggregate is KNOWN. It is false only
// when the session set could not be loaded: the substate is then unavailable, and
// callers must PRESERVE the last-known reading rather than clear it, so a transient
// DB error never resolves a still-working task to a coarse "done"
// (§spec:live-propagation-fallback safe fallback). A nil provider (feature not
// wired) is a known-empty result, not an error, and a running-but-empty aggregate
// returns ("", true) so it clears as usual.
func (s *Service) computeTaskForegroundActivity(ctx context.Context, taskID string) (v1.ForegroundActivity, bool) {
	if s.foregroundActivity == nil {
		return "", true
	}
	sessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list sessions for task activity aggregate",
			zap.String("task_id", taskID), zap.Error(err))
		return "", false
	}
	activities := make([]v1.ForegroundActivity, 0, len(sessions))
	for _, session := range sessions {
		if session == nil || session.State != models.TaskSessionStateRunning {
			continue
		}
		activities = append(activities, s.foregroundActivity.ForegroundActivity(session.ID))
	}
	return v1.AggregateForegroundActivity(activities), true
}

// PublishTaskActivityIfChanged recomputes the task-level activity aggregate and
// emits a task.updated ONLY when the aggregated three-state value differs from the
// value last carried to the client — including a generating↔background flip that
// leaves the coarse task/session state unchanged. This bounds the added
// live-propagation traffic to an actual change of the aggregated value
// (§spec:live-propagation-fallback tradeoff). Safe to call on every per-session
// activity flip; it no-ops when the task-level reading is unaffected. The emitted
// task.updated re-records the value (via addTaskSessionEventFields →
// recordTaskActivity), keeping the dedup map in step with every task event.
func (s *Service) PublishTaskActivityIfChanged(ctx context.Context, taskID string) {
	if taskID == "" || s.foregroundActivity == nil {
		return
	}
	current, known := s.computeTaskForegroundActivity(ctx, taskID)
	if !known {
		// The session set could not be loaded: leave the last-known aggregate in
		// place instead of emitting a spurious clear that could momentarily read
		// "done" while a turn is still open (§spec:live-propagation-fallback).
		return
	}

	s.taskActivityMu.Lock()
	previous, seen := s.lastTaskActivity[taskID]
	s.taskActivityMu.Unlock()
	if seen && previous == current {
		return
	}

	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			s.logger.Warn("failed to load task for activity update",
				zap.String("task_id", taskID), zap.Error(err))
		}
		return
	}
	s.PublishTaskUpdated(ctx, task)
}

// recordTaskActivity remembers the aggregate carried on a task event so the next
// per-session flip can tell whether the task-level reading actually changed. Any
// task.updated / task.state_changed / task.deleted carries the aggregate, so this
// keeps the dedup baseline fresh regardless of which path emitted the event.
func (s *Service) recordTaskActivity(taskID string, activity v1.ForegroundActivity) {
	if taskID == "" {
		return
	}
	s.taskActivityMu.Lock()
	s.lastTaskActivity[taskID] = activity
	s.taskActivityMu.Unlock()
}

// forgetTaskActivity drops the cached last-emitted aggregate for a task so the
// dedup map does not grow without bound as tasks are deleted.
func (s *Service) forgetTaskActivity(taskID string) {
	if taskID == "" {
		return
	}
	s.taskActivityMu.Lock()
	delete(s.lastTaskActivity, taskID)
	s.taskActivityMu.Unlock()
}
