package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

const taskTransferAuditTimeout = 2 * time.Second

func (r *Repository) persistTaskTransfer(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	requestDigest string,
	receipt *models.TaskTransferReceipt,
) error {
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_transfer_operations
			(id, source_workspace_id, idempotency_key, request_digest, actor_kind, actor_id, actor_session_id,
			 task_id, receipt_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		receipt.OperationID, receipt.SourceWorkspaceID, receipt.IdempotencyKey, requestDigest,
		command.Actor.Kind, command.Actor.ID, command.Actor.SessionID,
		receipt.TaskID, string(receiptJSON), receipt.TransferredAt)
	if err != nil {
		return err
	}
	return r.insertTaskTransferAudit(ctx, tx, command, receipt, "transferred")
}

func (r *Repository) insertTaskTransferAudit(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	receipt *models.TaskTransferReceipt,
	result string,
) error {
	sessionsJSON, err := json.Marshal(receipt.Sessions)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_transfer_audit
			(id, operation_id, actor_kind, actor_id, actor_session_id, task_id,
			 source_workspace_id, source_workflow_id, source_step_id,
			 destination_workspace_id, destination_workflow_id, destination_step_id,
			 task_generation, session_census_json, preservation_digest, idempotency_key,
			 preservation_policy, result, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		uuid.NewString(), receipt.OperationID, command.Actor.Kind, command.Actor.ID, command.Actor.SessionID,
		receipt.TaskID, receipt.SourceWorkspaceID, receipt.SourceWorkflowID, receipt.SourceStepID,
		receipt.DestinationWorkspaceID, receipt.DestinationWorkflowID, receipt.DestinationStepID,
		receipt.TaskGeneration, string(sessionsJSON), receipt.PreservationDigest, receipt.IdempotencyKey,
		receipt.PreservationPolicy, result, r.nowUTC())
	return err
}

// RecordTaskTransferAttempt stores a redacted audit row for an attempt that
// did not reach the committing transfer transaction.
func (r *Repository) RecordTaskTransferAttempt(
	ctx context.Context,
	command models.TaskTransferCommand,
	result string,
) error {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), taskTransferAuditTimeout)
	defer cancel()
	if result != taskTransferResultDenied && result != taskTransferResultConflict && result != taskTransferResultFailed {
		result = taskTransferResultFailed
	}
	stepID := command.DestinationStepID
	if stepID == "" {
		stepID = command.DestinationStepName
	}
	_, err := r.db.ExecContext(auditCtx, r.db.Rebind(`
		INSERT INTO task_transfer_audit
			(id, operation_id, actor_kind, actor_id, actor_session_id, task_id,
			 source_workspace_id, source_workflow_id, source_step_id,
			 destination_workspace_id, destination_workflow_id, destination_step_id,
			 task_generation, session_census_json, preservation_digest, idempotency_key,
			 preservation_policy, result, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', '', ?, ?, ?, ?)`),
		uuid.NewString(), uuid.NewString(), command.Actor.Kind, command.Actor.ID, command.Actor.SessionID,
		command.TaskID, command.ExpectedSourceWorkspaceID, command.ExpectedSourceWorkflowID,
		command.ExpectedSourceStepID, command.DestinationWorkspaceID, command.DestinationWorkflowID,
		stepID, command.ExpectedTaskUpdatedAt, command.IdempotencyKey, command.PreservationPolicy,
		result, r.nowUTC())
	return err
}
