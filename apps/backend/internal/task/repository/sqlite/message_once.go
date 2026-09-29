package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// CreateMessageAndQueueOnce persists one user message and its queue entry in a
// single transaction, keyed by message.ID. The message insert is
// INSERT ... ON CONFLICT (id) DO NOTHING: one affected row inserts the queue
// entry as well and returns created = true; zero affected rows inserts nothing,
// rolls back and returns created = false.
func (r *Repository) CreateMessageAndQueueOnce(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	maxPerSession int,
) (created bool, err error) {
	fields, err := prepareQueuedPlanCommentMessage(message, queued)
	if err != nil {
		return false, err
	}
	originalMessage := *message
	originalQueued := *queued
	tx, release, err := r.beginPlanCommentTx(ctx, message.TaskID)
	if err != nil {
		return false, fmt.Errorf("begin once-only queued message creation: %w", err)
	}
	defer release()
	defer func() {
		_ = tx.Rollback()
		if err != nil || !created {
			*message = originalMessage
			*queued = originalQueued
		}
	}()
	if err = r.guardActivePlanCommentTaskTx(ctx, tx, message.TaskID); err != nil {
		return false, err
	}
	if err = lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return false, err
	}
	normalizedTime := dialect.NormalizedMicrosecond(r.db.DriverName(), "created_at")
	if err = r.assignUserMessageBoundary(ctx, tx, message, r.db.DriverName(), normalizedTime); err != nil {
		return false, err
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at, prompt_seq, payload_digest, payload_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`), message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID,
		message.Content, fields.requestsInput, fields.messageType, fields.metadataJSON, message.CreatedAt,
		message.UpdatedAt, message.PromptIndex, message.PayloadDigest, message.PayloadSize)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return false, nil
	}
	if err = messagequeue.InsertTaskOwnedInTransaction(ctx, tx, r.db, queued, maxPerSession); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("commit once-only queued message creation: %w", err)
	}
	return true, nil
}
