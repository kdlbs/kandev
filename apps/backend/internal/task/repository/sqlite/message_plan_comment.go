package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

// CreateMessageWithPlanComments persists one user message and consumes its
// exact task-plan comment references in the same transaction.
func (r *Repository) CreateMessageWithPlanComments(
	ctx context.Context,
	message *models.Message,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
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
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return nil, fmt.Errorf("serialize plan-comment message metadata: %w", err)
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	return r.createMessagePlanCommentBoundary(
		ctx, message, refs, requirePrimary, requestsInput, messageType, string(metadataJSON),
	)
}

func (r *Repository) createMessagePlanCommentBoundary(
	ctx context.Context,
	message *models.Message,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	requestsInput int,
	messageType, metadataJSON string,
) (*models.TaskPlanCommentSnapshot, error) {
	original := *message
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin plan-comment message creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := plancommenttx.LockTask(ctx, tx, r.db, message.TaskID); err != nil {
		return nil, err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return nil, err
	}
	resolved, err := plancommenttx.Resolve(
		ctx, tx, r.db, message.TaskID, message.TaskSessionID, message.Content, refs, requirePrimary,
	)
	if err != nil {
		return nil, err
	}
	message.Content = resolved.Content
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
