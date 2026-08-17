package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// deferredLaunchClaim is a direct start's hold on a task's pending launch
// intent, taken before the launch and released only if the launch fails.
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
// # Why the claim is taken BEFORE the launch
//
// Consuming after a successful launch leaves the whole duration of that launch
// unguarded, and a launch is slow — worktree creation, a container health check.
// A gate opening inside that window loads the task, still sees the intent,
// claims it, and launches its own session while the direct start is mid-flight.
// Both succeed and the task ends with two sessions: the same bug, reached by
// interleaving rather than by ordering.
//
// So a direct start reserves the intent first and holds it across the launch,
// which is exactly the protocol the gate already follows (claimDeferredLaunch
// before LaunchSession, restoreDeferredLaunch on failure). One atomic
// RemoveTaskMetadataKey elects a single starter, whichever path gets there
// first, and the loser finds nothing to launch.
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
type deferredLaunchClaim struct {
	svc    *Service
	taskID string
	// intent is the record as it was read at claim time, kept so a failed
	// launch can put it back. It is read before the atomic remove rather than
	// returned by it, matching restoreDeferredLaunch: a prompt edit landing in
	// that gap would be restored at its previous value, which costs one stale
	// brief on a launch that already failed.
	intent interface{}
	held   bool
}

// claimDeferredLaunchForStart reserves a task's pending launch intent for a
// direct start. The returned claim is inert when the task had no intent, or
// when another path claimed it first — in the latter case that path owns the
// launch and this one is simply a second session the user asked for.
func (s *Service) claimDeferredLaunchForStart(ctx context.Context, taskID string) *deferredLaunchClaim {
	claim := &deferredLaunchClaim{svc: s, taskID: taskID}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.Metadata == nil {
		return claim
	}
	intent, ok := task.Metadata[models.MetaKeyDeferredLaunch]
	if !ok {
		return claim
	}
	claimed, err := s.repo.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyDeferredLaunch)
	if err != nil {
		s.logger.Warn("failed to claim deferred launch intent on start",
			zap.String("task_id", taskID), zap.Error(err))
		return claim
	}
	if !claimed {
		return claim
	}
	claim.intent = intent
	claim.held = true
	return claim
}

// consume finalizes the claim after the launch succeeded: the intent is spent
// and the board should stop advertising it.
func (c *deferredLaunchClaim) consume(ctx context.Context) {
	if c == nil || !c.held {
		return
	}
	c.held = false
	c.svc.logger.Info("consumed deferred launch intent: task was started directly",
		zap.String("task_id", c.taskID))
	// Republish so the board drops the "starts when unblocked" chip; the intent
	// it advertises no longer exists.
	task, err := c.svc.repo.GetTask(ctx, c.taskID)
	if err != nil || task == nil {
		return
	}
	c.svc.publishTaskUpdated(ctx, task)
}

// releaseIfHeld puts the intent back when the start did not reach a running
// agent. A no-op after consume, so callers can defer it unconditionally and no
// early return can leak the reservation — losing the intent would silently mean
// a chain step that never runs, which is worse than the extra session this
// whole mechanism exists to prevent.
func (c *deferredLaunchClaim) releaseIfHeld(ctx context.Context) {
	if c == nil || !c.held {
		return
	}
	c.held = false
	if err := c.svc.repo.SetTaskMetadataKey(ctx, c.taskID, models.MetaKeyDeferredLaunch, c.intent); err != nil {
		c.svc.logger.Warn("failed to restore deferred launch intent after a failed start",
			zap.String("task_id", c.taskID), zap.Error(err))
	}
}
