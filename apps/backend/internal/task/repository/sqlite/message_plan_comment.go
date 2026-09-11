package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

// ValidateMessagePlanComments performs the exact task/session/comment CAS
// checks without creating a turn, message, or queue row. Handlers use it
// before workflow hooks so an already-stale request has no task side effects.
func (r *Repository) ValidateMessagePlanComments(
	ctx context.Context,
	taskID, sessionID, content string,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
) error {
	tx, release, err := r.beginPlanCommentTx(ctx, taskID)
	if err != nil {
		return fmt.Errorf("begin plan-comment message validation: %w", err)
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActivePlanCommentTaskTx(ctx, tx, taskID); err != nil {
		return err
	}
	_, err = plancommenttx.ResolveDirect(
		ctx, tx, r.db, taskID, sessionID, content, refs, requirePrimary, expectedState,
	)
	return err
}

// CreateMessageWithPlanComments persists one user message and consumes its
// exact task-plan comment references in the same transaction.
func (r *Repository) CreateMessageWithPlanComments(
	ctx context.Context,
	message *models.Message,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
) (*models.TaskPlanCommentSnapshot, error) {
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}
	if message.AuthorType != models.MessageAuthorUser {
		return nil, fmt.Errorf("plan comments require a user message")
	}
	messageType := string(message.Type)
	if messageType == "" {
		messageType = string(models.MessageTypeMessage)
	}
	metadataJSON := []byte("{}")
	if message.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(message.Metadata)
		if err != nil {
			return nil, fmt.Errorf("serialize plan-comment message metadata: %w", err)
		}
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	return r.createMessagePlanCommentBoundary(
		ctx, message, refs, requirePrimary, expectedState, claim, requestsInput, messageType, string(metadataJSON),
	)
}

// CreateMessageWithPlanCommentsAndQueue persists the visible user message,
// its deferred queue delivery, and comment consumption in one transaction.
func (r *Repository) CreateMessageWithPlanCommentsAndQueue(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, error) {
	fields, err := prepareQueuedPlanCommentMessage(message, queued)
	if err != nil {
		return nil, err
	}
	return r.createQueuedMessagePlanCommentBoundary(
		ctx, message, queued, refs, requirePrimary, expectedState, claim, maxPerSession, fields,
	)
}

type queuedPlanCommentMessageFields struct {
	requestsInput int
	messageType   string
	metadataJSON  string
}

func prepareQueuedPlanCommentMessage(
	message *models.Message,
	queued *messagequeue.QueuedMessage,
) (queuedPlanCommentMessageFields, error) {
	if queued == nil || queued.ID == "" {
		return queuedPlanCommentMessageFields{}, fmt.Errorf("queued message is required")
	}
	if message == nil || message.TaskID != queued.TaskID || message.TaskSessionID != queued.SessionID {
		return queuedPlanCommentMessageFields{}, fmt.Errorf("queued message target does not match user message")
	}
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}
	if message.AuthorType != models.MessageAuthorUser || queued.QueuedBy != messagequeue.QueuedByUser {
		return queuedPlanCommentMessageFields{}, fmt.Errorf("queued plan comments require a user message")
	}
	messageType := string(message.Type)
	if messageType == "" {
		messageType = string(models.MessageTypeMessage)
	}
	metadataJSON := []byte("{}")
	if message.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(message.Metadata)
		if err != nil {
			return queuedPlanCommentMessageFields{}, fmt.Errorf("serialize queued plan-comment message metadata: %w", err)
		}
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	return queuedPlanCommentMessageFields{
		requestsInput: requestsInput,
		messageType:   messageType,
		metadataJSON:  string(metadataJSON),
	}, nil
}

func (r *Repository) createQueuedMessagePlanCommentBoundary(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
	fields queuedPlanCommentMessageFields,
) (*models.TaskPlanCommentSnapshot, error) {
	originalMessage := *message
	originalQueued := *queued
	restore := func() {
		*message = originalMessage
		*queued = originalQueued
	}
	tx, release, err := r.beginPlanCommentTx(ctx, message.TaskID)
	if err != nil {
		return nil, fmt.Errorf("begin queued plan-comment message creation: %w", err)
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	// Task lifecycle is the outer authority. Lock it before session/turn rows so
	// archive and admission share one task -> session lock order.
	if err := r.guardActivePlanCommentTaskTx(ctx, tx, message.TaskID); err != nil {
		return nil, err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return nil, err
	}
	resolved, err := plancommenttx.ResolveDirect(
		ctx, tx, r.db, message.TaskID, message.TaskSessionID, message.Content, refs,
		requirePrimary, expectedState,
	)
	if err != nil {
		return nil, err
	}
	message.Content = resolved.Content
	queued.Content = resolved.Content
	if err := r.claimDirectMessageAttachmentsTx(
		ctx, tx, claim, message.TaskID, message.TaskSessionID, message.ID,
	); err != nil {
		return nil, err
	}
	normalizedTime := dialect.NormalizedMicrosecond(r.db.DriverName(), "created_at")
	if err := r.assignUserMessageBoundary(ctx, tx, message, r.db.DriverName(), normalizedTime); err != nil {
		restore()
		return nil, err
	}
	if err := r.insertMessageRow(
		ctx, tx, message, fields.requestsInput, fields.messageType, fields.metadataJSON,
	); err != nil {
		restore()
		return nil, err
	}
	if err := messagequeue.InsertTaskOwnedInTransaction(ctx, tx, r.db, queued, maxPerSession); err != nil {
		restore()
		return nil, err
	}
	snapshot, err := plancommenttx.Consume(ctx, tx, r.db, resolved)
	if err != nil {
		restore()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		restore()
		return nil, fmt.Errorf("commit queued plan-comment message creation: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) createMessagePlanCommentBoundary(
	ctx context.Context,
	message *models.Message,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	requestsInput int,
	messageType, metadataJSON string,
) (*models.TaskPlanCommentSnapshot, error) {
	original := *message
	tx, release, err := r.beginPlanCommentTx(ctx, message.TaskID)
	if err != nil {
		return nil, fmt.Errorf("begin plan-comment message creation: %w", err)
	}
	defer release()
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActivePlanCommentTaskTx(ctx, tx, message.TaskID); err != nil {
		return nil, err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return nil, err
	}
	resolved, err := plancommenttx.ResolveDirect(
		ctx, tx, r.db, message.TaskID, message.TaskSessionID, message.Content, refs,
		requirePrimary, expectedState,
	)
	if err != nil {
		return nil, err
	}
	message.Content = resolved.Content
	if err := r.claimDirectMessageAttachmentsTx(
		ctx, tx, claim, message.TaskID, message.TaskSessionID, message.ID,
	); err != nil {
		return nil, err
	}
	normalizedTime := dialect.NormalizedMicrosecond(r.db.DriverName(), "created_at")
	if err := r.assignUserMessageBoundary(ctx, tx, message, r.db.DriverName(), normalizedTime); err != nil {
		*message = original
		return nil, err
	}
	if err := r.insertMessageRow(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		*message = original
		return nil, err
	}
	snapshot, err := plancommenttx.Consume(ctx, tx, r.db, resolved)
	if err != nil {
		*message = original
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		*message = original
		return nil, fmt.Errorf("commit plan-comment message creation: %w", err)
	}
	return snapshot, nil
}

func (r *Repository) guardActivePlanCommentTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET updated_at = updated_at
		WHERE id = ? AND archived_at IS NULL
	`), taskID)
	if err != nil {
		return fmt.Errorf("guard active task for plan-comment admission: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("guard active task rows affected: %w", err)
	}
	if affected == 0 {
		return messagequeue.ErrTaskInactive
	}
	return nil
}
