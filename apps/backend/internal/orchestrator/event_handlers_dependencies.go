package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskDependencyReader is the orchestrator's read seam onto task dependency
// state. Implemented by the task service.
type TaskDependencyReader interface {
	// DependencyGate reports whether an automated path may start taskID.
	// Returns blocked=true on any read error so the gate fails closed.
	DependencyGate(ctx context.Context, taskID string) (bool, string, error)
	// ListDependentTaskIDs returns the tasks directly blocked by taskID.
	ListDependentTaskIDs(ctx context.Context, taskID string) ([]string, error)
	// ListPendingDependencyLaunches returns tasks that will start on their own
	// once their dependencies resolve. Used by startup reconciliation.
	ListPendingDependencyLaunches(ctx context.Context) ([]taskservice.PendingDependencyLaunch, error)
}

// SetTaskDependencyReader wires dependency reads for the auto-start gate and
// the dependency-resolution reaction.
func (s *Service) SetTaskDependencyReader(reader TaskDependencyReader) {
	s.dependencyReader = reader
}

// dependencyBlocksAutoStart reports whether taskID has an unresolved dependency
// and therefore must not be launched by an automated path.
//
// Fails CLOSED: a read error returns true. Failing open would launch work whose
// predecessor may never have run, which is the single outcome task dependencies
// exist to prevent.
func (s *Service) dependencyBlocksAutoStart(ctx context.Context, taskID, eventName string) bool {
	if s.dependencyReader == nil {
		return false
	}
	blocked, reason, err := s.dependencyReader.DependencyGate(ctx, taskID)
	if err != nil {
		s.logger.Warn(eventName+": dependency lookup failed; skipping auto-start",
			zap.String("task_id", taskID), zap.Error(err))
		return true
	}
	if blocked {
		s.logger.Debug(eventName+": task has unresolved dependencies; skipping auto-start",
			zap.String("task_id", taskID), zap.String("blocked_reason", reason))
	}
	return blocked
}

// handleTaskDependenciesForTerminalState reacts to a task reaching a terminal
// state by re-evaluating every task that depends on it.
//
// Called from handleTaskStateChanged. Moves into a terminal workflow step
// converge here too: markTaskCompletedForTerminalStep persists
// state = COMPLETED and publishes task.state_changed, so this one hook covers
// both "agent finished" and "card dragged to Done".
func (s *Service) handleTaskDependenciesForTerminalState(ctx context.Context, taskID string, state v1.TaskState) {
	if s.dependencyReader == nil {
		return
	}
	dependents, err := s.dependencyReader.ListDependentTaskIDs(ctx, taskID)
	if err != nil {
		s.logger.Warn("dependency resolution: failed to list dependents",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	for _, dependentID := range dependents {
		s.evaluateDependentAfterPredecessorChange(ctx, dependentID, taskID, state)
	}
}

// evaluateDependentAfterPredecessorChange decides what one dependent should do
// now that a predecessor reached a terminal state.
func (s *Service) evaluateDependentAfterPredecessorChange(
	ctx context.Context, dependentID, predecessorID string, predecessorState v1.TaskState,
) {
	blocked, reason, err := s.dependencyReader.DependencyGate(ctx, dependentID)
	if err != nil {
		s.logger.Warn("dependency resolution: gate lookup failed",
			zap.String("task_id", dependentID), zap.Error(err))
		return
	}
	if blocked {
		// Still waiting on something. When nothing is pending and the only
		// obstacle is a failed predecessor, the chain has halted and needs a
		// human: surface it rather than retrying or dropping the edge.
		if reason == taskservice.BlockedReasonFailed {
			s.publishDependencyFailed(ctx, dependentID, predecessorID, predecessorState)
		}
		return
	}
	// Every predecessor resolved successfully. Publish, then enter the SAME
	// auto-start chokepoint every other automated launch uses, so the existing
	// deferred-launch claim keeps this to one session even if WIP promotion
	// reaches the task at the same moment.
	s.publishDependenciesResolved(ctx, dependentID, predecessorID)
	task, err := s.repo.GetTask(ctx, dependentID)
	if err != nil || task == nil {
		s.logger.Warn("dependency resolution: failed to load unblocked task",
			zap.String("task_id", dependentID), zap.Error(err))
		return
	}
	// Resolution starts a task only when it recorded a start-when-unblocked
	// intent. Without this, a blocked task parked in a step that has
	// on_enter:auto_start_agent would launch the moment its gate opened, even
	// though nobody asked for a start — the chokepoint's step-based auto-start
	// is meant for a task ENTERING that step, not for one already sitting in it.
	if !taskservice.HasStartWhenUnblockedIntent(task) {
		s.logger.Debug("dependency resolution: no start-when-unblocked intent; leaving task unstarted",
			zap.String("task_id", task.ID))
		return
	}
	s.autoStartTaskForStep(ctx, task.ID, task.WorkflowStepID, events.TaskDependenciesResolved, 0)
}

// publishDependenciesResolved announces that a task's last unresolved
// dependency resolved successfully.
func (s *Service) publishDependenciesResolved(ctx context.Context, taskID, resolvedByTaskID string) {
	if s.eventBus == nil {
		return
	}
	payload := map[string]any{metaKeyTaskID: taskID, "resolved_by_task_id": resolvedByTaskID}
	if err := s.eventBus.Publish(ctx, events.TaskDependenciesResolved,
		bus.NewEvent(events.TaskDependenciesResolved, "orchestrator", payload)); err != nil {
		s.logger.Warn("failed to publish task.dependencies_resolved",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

// publishDependencyFailed announces that a blocked task's chain has halted
// because a predecessor failed or was cancelled.
func (s *Service) publishDependencyFailed(
	ctx context.Context, taskID, failedTaskID string, failedState v1.TaskState,
) {
	if s.eventBus == nil {
		return
	}
	payload := map[string]any{
		metaKeyTaskID:    taskID,
		"failed_task_id": failedTaskID,
		"failed_state":   string(failedState),
	}
	if err := s.eventBus.Publish(ctx, events.TaskDependencyFailed,
		bus.NewEvent(events.TaskDependencyFailed, "orchestrator", payload)); err != nil {
		s.logger.Warn("failed to publish task.dependency_failed",
			zap.String("task_id", taskID), zap.Error(err))
	}
	// Refresh the dependent so its card and chip re-render with the failed
	// reason without waiting for the next board load.
	if task, err := s.repo.GetTask(ctx, taskID); err == nil && task != nil {
		s.publishTaskUpdated(ctx, task)
	}
}

// reconcileDependencyLaunchesOnStartup launches tasks whose dependencies were
// satisfied while the process was down.
//
// Recovery path for a restart between a predecessor's completion and the
// dependent's launch: task.dependencies_resolved is an in-memory event and is
// not replayed, so without this sweep a chain would stall silently across a
// restart. Idempotent — the deferred launch claim admits one launcher however
// many times this runs.
//
// Scoped to tasks that actually carry a start-when-unblocked intent, so the
// sweep is bounded by the number of pending chain steps rather than the board.
func (s *Service) reconcileDependencyLaunchesOnStartup(ctx context.Context) {
	if s.dependencyReader == nil {
		return
	}
	pending, err := s.dependencyReader.ListPendingDependencyLaunches(ctx)
	if err != nil {
		s.logger.Warn("startup: failed to list pending dependency launches", zap.Error(err))
		return
	}
	for _, candidate := range pending {
		blocked, _, err := s.dependencyReader.DependencyGate(ctx, candidate.TaskID)
		if err != nil || blocked {
			continue
		}
		s.logger.Info("startup: launching task whose dependencies resolved while down",
			zap.String("task_id", candidate.TaskID))
		s.autoStartTaskForStep(ctx, candidate.TaskID, candidate.WorkflowStepID, "startup.dependencies_resolved", 0)
	}
}
