package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ForegroundActivityProvider surfaces the live fine-grained busy substate of a
// session (ADR-0049), satisfied by the orchestrator. The task service
// depends only on this narrow seam so it takes no hard orchestrator dependency
// and can be faked in tests.
type ForegroundActivityProvider interface {
	ForegroundActivity(sessionID string) v1.ForegroundActivity
}

type activeSubagentCountProvider interface {
	ActiveSubagentCount(sessionID string) int
}

// SetForegroundActivityProvider wires the live per-session activity tracker used
// to compute the task-level MOST-ACTIVE-WINS aggregate. Optional; when unset the
// aggregate is left empty and task-level surfaces fall through to the coarse
// task state.
func (s *Service) SetForegroundActivityProvider(provider ForegroundActivityProvider) {
	s.foregroundActivity = provider
}

// computeTaskActivitySnapshot resolves the task-wide activity and subagent
// count from one active-session read. A load failure is unknown so callers
// preserve their last published snapshot; an unwired provider is known-empty.
func (s *Service) computeTaskActivitySnapshot(
	ctx context.Context,
	taskID string,
) (taskActivitySnapshot, bool) {
	if s.foregroundActivity == nil {
		return taskActivitySnapshot{known: true}, true
	}
	sessions, err := s.sessions.ListActiveTaskSessionsByTaskID(ctx, taskID)
	if err != nil {
		s.logger.Warn("failed to list sessions for task activity aggregate",
			zap.String("task_id", taskID), zap.Error(err))
		return taskActivitySnapshot{}, false
	}
	return taskActivitySnapshot{
		activity:            s.computeTaskForegroundActivityForSessions(sessions),
		activeSubagentCount: s.computeTaskActiveSubagentCountForSessions(sessions),
		known:               true,
	}, true
}

func (s *Service) computeTaskActiveSubagentCountForSessions(
	sessions []*models.TaskSession,
) int {
	countProvider, ok := s.foregroundActivity.(activeSubagentCountProvider)
	if !ok {
		return 0
	}
	total := 0
	for _, session := range sessions {
		if session != nil {
			total += countProvider.ActiveSubagentCount(session.ID)
		}
	}
	return total
}

// computeTaskForegroundActivityForSessions is computeTaskForegroundActivity's
// core aggregation, split out so callers that already hold the task's active
// session list (e.g. addTaskSessionEventFieldsWithActivity) can reuse it
// without a second ListActiveTaskSessionsByTaskID query for the same event.
func (s *Service) computeTaskForegroundActivityForSessions(sessions []*models.TaskSession) v1.ForegroundActivity {
	if s.foregroundActivity == nil {
		return ""
	}
	activities := make([]v1.ForegroundActivity, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		activity := s.foregroundActivity.ForegroundActivity(session.ID)
		if session.State == models.TaskSessionStateRunning || activity == v1.ForegroundActivityBackground {
			activities = append(activities, activity)
		}
	}
	return v1.AggregateForegroundActivity(activities)
}

// PublishTaskActivityIfChanged emits task.updated when either the task-level
// activity aggregate or live subagent count changes. It is safe to call on
// every session activity flip; unchanged snapshots are deduplicated.
func (s *Service) PublishTaskActivityIfChanged(ctx context.Context, taskID string) {
	if taskID == "" || s.foregroundActivity == nil {
		return
	}
	s.enqueueTaskPublication(ctx, taskID, events.TaskUpdated, func(publicationCtx context.Context) {
		current, known := s.computeTaskActivitySnapshot(publicationCtx, taskID)
		if !known {
			// The session set could not be loaded: leave the last-known aggregate in
			// place instead of emitting a spurious clear that could momentarily read
			// "done" while a turn is still open.
			return
		}

		s.taskActivityMu.Lock()
		previousActivity, activitySeen := s.lastTaskActivity[taskID]
		previousCount, countSeen := s.lastTaskSubagentCount[taskID]
		s.taskActivityMu.Unlock()
		if activitySeen && countSeen &&
			previousActivity == current.activity &&
			previousCount == current.activeSubagentCount {
			return
		}

		task, err := s.tasks.GetTask(publicationCtx, taskID)
		if err != nil || task == nil {
			if err != nil {
				s.logger.Warn("failed to load task for activity update",
					zap.String("task_id", taskID), zap.Error(err))
			}
			return
		}
		s.publishTaskEventNow(publicationCtx, "task.updated", task, nil, nil, nil, &current)
	})
}

// recordTaskActivity remembers the aggregate carried on a task event so the next
// per-session flip can tell whether the task-level reading actually changed. Any
// task.updated / task.state_changed / task.deleted carries the aggregate, so this
// keeps the dedup baseline fresh regardless of which path emitted the event.
func (s *Service) recordTaskActivity(taskID string, activity v1.ForegroundActivity) {
	s.recordTaskActivitySnapshot(taskID, &taskActivitySnapshot{activity: activity, known: true})
}

func (s *Service) recordTaskActivitySnapshot(taskID string, snapshot *taskActivitySnapshot) {
	if taskID == "" {
		return
	}
	s.taskActivityMu.Lock()
	if s.lastTaskActivity == nil {
		s.lastTaskActivity = make(map[string]v1.ForegroundActivity)
	}
	if s.lastTaskSubagentCount == nil {
		s.lastTaskSubagentCount = make(map[string]int)
	}
	s.lastTaskActivity[taskID] = snapshot.activity
	s.lastTaskSubagentCount[taskID] = snapshot.activeSubagentCount
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
	delete(s.lastTaskSubagentCount, taskID)
	s.taskActivityMu.Unlock()
}
