package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
)

// maxRecoveryTickDeferredReplay bounds how many pending deferred
// assignments ReplayPendingDeferredAssignments processes per recovery
// tick, mirroring scheduler_recovery.go's maxRecoveryPerTick — this is a
// backstop, not the primary replay path, and should never dominate a
// tick's budget.
const maxRecoveryTickDeferredReplay = 5

const (
	deferredAssignmentOutcomeReplayed = "replayed"
	deferredAssignmentOutcomeDropped  = "dropped"
)

// ErrDeferredAssignmentRateLimited keeps a deferred row pending when the
// scheduler's assignment-wake allowance temporarily refuses replay.
var ErrDeferredAssignmentRateLimited = errors.New("deferred assignment replay rate limited")

// RecordDeferredAssignment persists that taskID's task_assigned wake was
// blocked by pauseID (docs/specs/office/requirements/paused-assignment-replay.md),
// so pause.Service.Resume or the recovery tick can replay it once the
// workspace resumes. Reads the task's current runner and assignment
// generation itself rather than trusting the caller, so a reassignment
// that also lands during the same pause naturally overwrites the pending
// row with the latest assignee (models.DeferredAssignment) instead of
// recording a stale one. Best-effort: a failure here is logged, not
// propagated — the caller has already decided the paused write itself is
// not an error.
func (s *Service) RecordDeferredAssignment(ctx context.Context, taskID, pauseID string) {
	s.recordDeferredAssignment(ctx, taskID, pauseID, "", "", nil)
}

// RecordDeferredAssignmentWithActor is the scheduler-facing form of
// RecordDeferredAssignment. The actor snapshot lets replay keep the same
// priority and assignment-rate semantics as the original wake.
func (s *Service) RecordDeferredAssignmentWithActor(ctx context.Context, taskID, pauseID, actorType, actorID string) {
	s.recordDeferredAssignment(ctx, taskID, pauseID, actorType, actorID, nil)
}

// RecordDeferredAssignmentWithActorAtGeneration records the actor only when
// the task still has the generation that produced the paused wake. A late
// callback from an older assignment must not attach its actor to a newer row.
func (s *Service) RecordDeferredAssignmentWithActorAtGeneration(
	ctx context.Context, taskID, pauseID, actorType, actorID string, assignmentGeneration int64,
) {
	s.recordDeferredAssignment(ctx, taskID, pauseID, actorType, actorID, &assignmentGeneration)
}

func (s *Service) recordDeferredAssignment(ctx context.Context, taskID, pauseID, actorType, actorID string, expectedGeneration *int64) {
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		if !errors.Is(err, sqlite.ErrTaskNotFound) {
			s.logger.Warn("record deferred assignment: read task failed",
				zap.String("task_id", taskID), zap.Error(err))
		}
		return
	}
	if fields.AssigneeAgentProfileID == "" {
		return
	}
	if expectedGeneration != nil && fields.AssignmentGeneration != *expectedGeneration {
		return
	}
	recorded, err := s.repo.RecordDeferredAssignmentWithActor(
		ctx, taskID, fields.WorkspaceID, fields.AssigneeAgentProfileID, fields.AssignmentGeneration, pauseID, actorType, actorID,
	)
	if err != nil {
		s.logger.Warn("record deferred assignment failed",
			zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if !recorded {
		// The generation guard suppressed a stale write (a still-pending
		// row already carries a newer generation): nothing changed, so
		// logging a "deferred" activity entry here would be misleading.
		return
	}
	s.LogActivity(ctx, fields.WorkspaceID, "system", "",
		string(models.ActivityActionTaskAssignmentDeferred), "task", taskID,
		fmt.Sprintf(`{"pause_id":%q}`, pauseID))
}

// ReplayDeferredAssignments replays every pending deferred assignment for
// workspaceID. This is the primary replay path, called from
// pause.Service.Resume via the AssignmentReplayer hook right after the
// workspace's pause record is released — the caller already knows the
// workspace has no active pause, so this does not re-check.
func (s *Service) ReplayDeferredAssignments(ctx context.Context, workspaceID string) error {
	pending, err := s.repo.ListPendingDeferredAssignmentsForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("list pending deferred assignments: %w", err)
	}
	for _, da := range pending {
		s.replayDeferredAssignment(ctx, &da)
	}
	return nil
}

// ReplayPendingDeferredAssignments is the recovery tick's backstop: it
// re-derives which pending deferred assignments are replayable straight
// from current pause state, so a Resume hook that never ran (a process
// restart between release and replay) or that failed transiently is still
// eventually replayed. Bounded to maxRecoveryTickDeferredReplay rows per
// call, matching recoverUnstartedTasks' own per-tick budget.
func (s *Service) ReplayPendingDeferredAssignments(ctx context.Context) error {
	pending, err := s.repo.ListReplayablePendingDeferredAssignments(ctx, maxRecoveryTickDeferredReplay)
	if err != nil {
		return fmt.Errorf("list replayable deferred assignments: %w", err)
	}
	for _, da := range pending {
		s.replayDeferredAssignment(ctx, &da)
	}
	return nil
}

// replayDeferredAssignment re-validates one pending deferred assignment
// against the task's current state and either replays or drops it
// (AC-OFFICE-PAUSE-REPLAY-001.3/.4): archived, unassigned, reassigned to a
// different runner, or a changed assignment generation all resolve it as
// dropped without queuing a run — the run such a change is itself owed
// goes through the ordinary live assignment path, not this one. Any other
// outcome (queue failure, an in-progress re-pause) leaves the row pending
// for the next resume or recovery tick to retry.
func (s *Service) replayDeferredAssignment(ctx context.Context, da *models.DeferredAssignment) {
	fields, err := s.repo.GetTaskExecutionFields(ctx, da.TaskID)
	if err != nil {
		if errors.Is(err, sqlite.ErrTaskNotFound) {
			s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeDropped, "task_not_found")
			return
		}
		s.logger.Warn("replay deferred assignment: read task failed",
			zap.String("task_id", da.TaskID), zap.Error(err))
		return
	}

	switch {
	case fields.Archived:
		s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeDropped, "task_archived")
		return
	case fields.AssigneeAgentProfileID == "":
		s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeDropped, "unassigned")
		return
	case fields.AssigneeAgentProfileID != da.AgentProfileID:
		s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeDropped, "reassigned")
		return
	case fields.AssignmentGeneration != da.AssignmentGeneration:
		s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeDropped, "generation_mismatch")
		return
	}

	payload := mustJSON(map[string]string{"task_id": da.TaskID}) //nolint:goconst // "task_id" is the run-payload wire key used throughout this package
	key := dedupkeys.AssignmentKey(da.TaskID, da.AgentProfileID, da.AssignmentGeneration)
	var queueErr error
	if s.deferredAssignmentQueue != nil {
		queueErr = s.deferredAssignmentQueue.QueueDeferredAssignment(ctx, *da)
	} else if actorKind := models.ActorKind(da.ActorType); actorKind.Valid() {
		queueErr = s.QueueRunFromTaskBoundaryWithActor(ctx, da.AgentProfileID, RunReasonTaskAssigned, payload, key, da.TaskID, actorKind, da.ActorID)
	} else {
		queueErr = s.QueueRunFromTaskBoundary(ctx, da.AgentProfileID, RunReasonTaskAssigned, payload, key, da.TaskID)
	}
	if queueErr != nil {
		if errors.Is(queueErr, ErrDeferredAssignmentRateLimited) {
			// The scheduler's rolling allowance is a temporary refusal. Keep
			// the row pending so a later recovery tick can retry it.
			return
		}
		if errors.Is(queueErr, shared.ErrWorkspacePaused) {
			// A fresh pause raced the replay. Keep the row pending for the
			// next resume or recovery tick.
			return
		}
		var pe *pausedQueueError
		if errors.As(queueErr, &pe) {
			// Paused again by the time replay ran (a fresh pause raced the
			// resume/tick that read this row as replayable) — leave it
			// pending so the next resume or tick retries it.
			return
		}
		if errors.Is(queueErr, ErrAgentNotRunnable) || errors.Is(queueErr, sql.ErrNoRows) {
			// A deterministic, non-retryable refusal: the assignee is
			// paused/stopped/pending-approval, or no longer exists. This is
			// not a transient failure that a later retry could resolve, so
			// leaving it pending would head-of-line-block the bounded
			// recovery-tick backstop forever (R1-F1) — drop it instead,
			// same as the live wake path already refuses a paused agent.
			s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeDropped, "agent_not_runnable")
			return
		}
		s.logger.Warn("replay deferred assignment: queue run failed",
			zap.String("task_id", da.TaskID), zap.Error(queueErr))
		return
	}
	s.resolveDeferredAssignment(ctx, da, deferredAssignmentOutcomeReplayed, "")
}

// resolveDeferredAssignment CAS-resolves a pending row and logs the paired
// activity entry, only when this call actually won the CAS (a concurrent
// replay from the other producer already resolved it, or a fresh
// RecordDeferredAssignment overwrote it with a newer generation since da
// was read, otherwise).
func (s *Service) resolveDeferredAssignment(ctx context.Context, da *models.DeferredAssignment, outcome, dropReason string) {
	won, err := s.repo.ResolveDeferredAssignment(ctx, da.TaskID, da.AssignmentGeneration, da.AgentProfileID, da.PauseID, outcome)
	if err != nil {
		s.logger.Warn("resolve deferred assignment failed",
			zap.String("task_id", da.TaskID), zap.String("outcome", outcome), zap.Error(err))
		return
	}
	if !won {
		return
	}
	action := models.ActivityActionTaskAssignmentReplayed
	details := fmt.Sprintf(`{"pause_id":%q,"agent_profile_id":%q}`, da.PauseID, da.AgentProfileID)
	if outcome == deferredAssignmentOutcomeDropped {
		action = models.ActivityActionTaskAssignmentDropped
		details = fmt.Sprintf(`{"pause_id":%q,"reason":%q}`, da.PauseID, dropReason)
	}
	s.LogActivity(ctx, da.WorkspaceID, "system", "", string(action), "task", da.TaskID, details)
}
