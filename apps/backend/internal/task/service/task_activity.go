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

// TaskParkedProvider surfaces the orchestrator's runtime task-level
// parked-on-background-work projection (the OR over a task's sessions),
// satisfied by the orchestrator. A narrow local seam, mirroring
// ForegroundActivityProvider, rather than importing internal/task/dto's
// equivalent interface directly — internal/task/dto imports this package, so
// importing it back here would be a cycle.
type TaskParkedProvider interface {
	TaskParkedProjectionSnapshot(taskID string) (parked bool, epoch int64, revision uint64)
}

// SetForegroundActivityProvider wires the live per-session activity tracker used
// to compute the task-level MOST-ACTIVE-WINS aggregate. Optional; when unset the
// aggregate is left empty and task-level surfaces fall through to the coarse
// task state.
func (s *Service) SetForegroundActivityProvider(provider ForegroundActivityProvider) {
	s.foregroundActivity = provider
}

// SetTaskParkedProvider wires the live task-level parked-on-background-work
// projection into task.updated publications. Optional; when unset every task
// carries the D9 zero-value triple (false, 0, 0).
func (s *Service) SetTaskParkedProvider(provider TaskParkedProvider) {
	s.taskParked = provider
}

// currentTaskParked reads the task-level parked projection from the wired
// provider. A nil provider (not yet wired) resolves to the zero-value triple,
// matching D9's default serialization — never an error, since this is an
// in-memory read with no failure mode once wired.
func (s *Service) currentTaskParked(taskID string) (parked bool, epoch int64, revision uint64) {
	if s.taskParked == nil {
		return false, 0, 0
	}
	return s.taskParked.TaskParkedProjectionSnapshot(taskID)
}

// computeTaskActivitySnapshot resolves the task-wide activity, subagent count,
// and parked-on-background-work triple from one active-session read plus one
// TaskParkedProvider read. A load failure leaves activity/count unknown so
// callers preserve their last published snapshot; parked has no unknown state
// (see currentTaskParked) and is always populated on both return paths, so a
// parked-only transition is never lost behind an unrelated session-list error.
func (s *Service) computeTaskActivitySnapshot(
	ctx context.Context,
	taskID string,
) (taskActivitySnapshot, bool) {
	parked, parkedEpoch, parkedRevision := s.currentTaskParked(taskID)
	if s.foregroundActivity == nil {
		return taskActivitySnapshot{
			parked: parked, parkedEpoch: parkedEpoch, parkedRevision: parkedRevision,
			known: true,
		}, true
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
		parked:              parked,
		parkedEpoch:         parkedEpoch,
		parkedRevision:      parkedRevision,
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

// PublishTaskActivityIfChanged emits task.updated when the task-level activity
// aggregate, live subagent count, OR parked-on-background-work projection
// changes (MUST-FIX #1, Review round 3 — a parked-only flip with unchanged
// activity/count must still reach task.updated, per AC-62/AC-78/AC-85/AC-49a).
// It is safe to call on every session activity flip; unchanged snapshots are
// deduplicated.
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
		previousParked, parkedSeen := s.lastTaskParked[taskID]
		previousParkedRevision := s.lastTaskParkedRevision[taskID]
		s.taskActivityMu.Unlock()
		if activitySeen && countSeen && parkedSeen &&
			previousActivity == current.activity &&
			previousCount == current.activeSubagentCount &&
			previousParked == current.parked &&
			previousParkedRevision == current.parkedRevision {
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
	if s.lastTaskParked == nil {
		s.lastTaskParked = make(map[string]bool)
	}
	if s.lastTaskParkedRevision == nil {
		s.lastTaskParkedRevision = make(map[string]uint64)
	}
	s.lastTaskActivity[taskID] = snapshot.activity
	s.lastTaskSubagentCount[taskID] = snapshot.activeSubagentCount
	s.lastTaskParked[taskID] = snapshot.parked
	s.lastTaskParkedRevision[taskID] = snapshot.parkedRevision
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
	delete(s.lastTaskParked, taskID)
	delete(s.lastTaskParkedRevision, taskID)
	s.taskActivityMu.Unlock()
}
