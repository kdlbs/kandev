package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) validateAgentDeliveryEventAdmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	event *models.AgentDeliveryEvent,
) error {
	if isLegacyAgentDeliveryEventOwner(event) {
		return r.ensureAgentDeliverySessionNotRetired(ctx, tx, event.SessionID, event.IncarnationID)
	}
	if !hasValidAgentDeliveryEventOwnerIdentity(event) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return r.validateLiveAgentDeliveryEventAdmissionTx(ctx, tx, event)
}

func isLegacyAgentDeliveryEventOwner(event *models.AgentDeliveryEvent) bool {
	return event.TaskID == "" && event.ExecutionID == "" && event.OwnerGeneration == 0 && event.ExpectedStreamID == ""
}

func hasValidAgentDeliveryEventOwnerIdentity(event *models.AgentDeliveryEvent) bool {
	return event.TaskID != "" && event.ExecutionID != "" && event.OwnerGeneration > 0 &&
		event.OwnerGeneration == event.HarnessGeneration && event.ExpectedStreamID != "" &&
		event.ExpectedStreamID == event.StreamID
}

const agentDeliveryLiveOwnerQuery = `
		SELECT EXISTS (
			SELECT 1
			FROM task_sessions ts
			JOIN executors_running er ON er.session_id = ts.id AND er.task_id = ts.task_id
			JOIN harness_session_generations hg
			  ON hg.session_id = ts.id AND hg.incarnation_id = ts.queue_incarnation_id
			WHERE ts.id = ? AND ts.task_id = ? AND ts.queue_incarnation_id = ?
			  AND er.agent_execution_id = ?
			  AND hg.generation = ?
			  AND NOT EXISTS (
				SELECT 1 FROM harness_session_generations newer
				WHERE newer.session_id = hg.session_id
				  AND newer.incarnation_id = hg.incarnation_id
				  AND newer.generation > hg.generation
		  )`

const agentDeliveryAssociatedOwnerQuery = `
			  AND EXISTS (
				SELECT 1 FROM agent_delivery_submissions submission
				WHERE submission.id = ? AND submission.session_id = ts.id
				  AND submission.incarnation_id = ts.queue_incarnation_id
				  AND submission.harness_generation = hg.generation
				  AND submission.owner_generation = ?
		  )`

const agentDeliveryOwnerRetirementQuery = `),
		EXISTS (SELECT 1 FROM agent_delivery_cleanup_jobs WHERE session_id = ?)`

func (r *Repository) validateLiveAgentDeliveryEventAdmissionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	event *models.AgentDeliveryEvent,
) error {
	args := []any{event.SessionID, event.TaskID, event.IncarnationID, event.ExecutionID, event.HarnessGeneration}
	if event.SubmissionID != "" {
		args = append(args, event.SubmissionID, event.OwnerGeneration)
	}
	args = append(args, event.SessionID)
	statement, err := r.agentDeliveryOwnerStatement(event.SubmissionID != "")
	if err != nil {
		return err
	}
	bound := tx.StmtxContext(ctx, statement)
	defer func() { _ = bound.Close() }()
	var ownerExists, retired bool
	if err := bound.QueryRowxContext(ctx, args...).Scan(&ownerExists, &retired); err != nil {
		return err
	}
	if !ownerExists {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if retired {
		return repoerrors.ErrAgentDeliveryIdentityRetiring
	}
	return nil
}
