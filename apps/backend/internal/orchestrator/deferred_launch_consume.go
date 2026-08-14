package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// consumeDeferredLaunchOnStart discards a task's pending deferred launch record
// because the task has just been started by some other path.
//
// A deferred launch is a *start intent*: "start this task later, with this
// prompt". Two things can consume it — the gate that was waiting (WIP promotion
// or dependency resolution) and an actual start by any other means. Only the
// first was ever implemented, so a task created with blocked_by and then started
// manually (UI start button, or message_task_kandev on a session that was never
// launched) kept its intent, and the gate fired again when the last predecessor
// completed: a SECOND session on a task that was already running, replaying a
// prompt written before any of the work happened.
//
// # Why "started by any means" and not "has any non-cancelled session"
//
// Session presence is the wrong predicate in both directions:
//
//   - A task can hold a session that never started an agent. Opening a blocked
//     task in the UI prepares a workspace-only CREATED session (session.ensure
//     with auto_start=false). Treating that as a start would silently drop the
//     chain launch the user is waiting for, and the only symptom would be a step
//     that never runs.
//   - A cancelled session still means the task WAS started. Reading
//     "non-cancelled" as "not started yet" re-arms the gate after a stop, which
//     is the same double-session outcome by a different route.
//
// The agent start is a single, observable transition, so it is consumed exactly
// once at the two chokepoints that perform it — startTask (new session) and
// StartCreatedSession (agent on a prepared session) — after the launch has
// actually succeeded. A failed launch keeps the intent so the gate can still
// run the task later.
//
// Removal is the same atomic RemoveTaskMetadataKey claim the gate itself uses,
// so a start racing a gate resolves to one winner rather than to two sessions.
func (s *Service) consumeDeferredLaunchOnStart(ctx context.Context, taskID string) {
	remover, ok := s.repo.(taskMetadataKeyRemover)
	if !ok {
		return
	}
	claimed, err := remover.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyDeferredLaunch)
	if err != nil {
		s.logger.Warn("failed to consume deferred launch intent on start",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if !claimed {
		return
	}
	s.logger.Info("consumed deferred launch intent: task was started directly",
		zap.String("task_id", taskID))
	// Republish so the board drops the "starts when unblocked" chip; the intent
	// it advertises no longer exists.
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	s.publishTaskUpdated(ctx, task)
}
