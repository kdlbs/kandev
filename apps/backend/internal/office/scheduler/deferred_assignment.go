package scheduler

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/runs/dedupkeys"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// QueueDeferredAssignment re-enters the scheduler queue seam so a replayed
// assignment uses the same actor-aware payload and rolling allowance as a
// live task mutation. Rows created by the defensive event-subscriber path do
// not have an actor snapshot; those rows retain the task-boundary carrier
// behavior that predates actor-aware replay.
func (ss *SchedulerService) QueueDeferredAssignment(
	ctx context.Context, assignment models.DeferredAssignment,
) error {
	actorKind := models.ActorKind(assignment.ActorType)
	if !actorKind.Valid() {
		payloadBytes, _ := json.Marshal(map[string]string{"task_id": assignment.TaskID})
		return ss.svc.QueueRunFromTaskBoundary(
			ctx,
			assignment.AgentProfileID,
			RunReasonTaskAssigned,
			string(payloadBytes),
			dedupkeys.AssignmentKey(assignment.TaskID, assignment.AgentProfileID, assignment.AssignmentGeneration),
			assignment.TaskID,
		)
	}

	runCtx := RunContext{
		Reason:         RunReasonTaskAssigned,
		TaskID:         assignment.TaskID,
		WorkspaceID:    assignment.WorkspaceID,
		ActorType:      assignment.ActorType,
		ActorID:        assignment.ActorID,
		IdempotencyKey: dedupkeys.AssignmentKey(assignment.TaskID, assignment.AgentProfileID, assignment.AssignmentGeneration),
	}
	outcome, err := ss.QueueRunCtx(ctx, assignment.AgentProfileID, runCtx)
	if err != nil {
		return err
	}
	if outcome == runsservice.QueueOutcomeRateLimited {
		return service.ErrDeferredAssignmentRateLimited
	}
	return nil
}
