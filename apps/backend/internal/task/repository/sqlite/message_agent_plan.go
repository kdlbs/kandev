package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const agentPlanMessageLockNamespace = "agent-plan-message:"

// UpsertAgentPlanMessageWithConversationReceipt serializes the complete
// read/create/update decision for one deterministic agent-plan message.
func (r *Repository) UpsertAgentPlanMessageWithConversationReceipt(
	ctx context.Context,
	message *models.Message,
) (*models.Message, *models.ConversationMutationReceipt, bool, error) {
	if message == nil || message.ID == "" {
		return nil, nil, false, errors.New("agent plan message identity is required")
	}
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return nil, nil, false, fmt.Errorf("serialize agent plan metadata: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("begin agent plan upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockAgentPlanMessageIdentity(ctx, tx, message); err != nil {
		return nil, nil, false, err
	}
	base, err := r.prepareConversationMutation(ctx, tx, message.TaskSessionID)
	if err != nil {
		return nil, nil, false, err
	}

	existing, err := r.readConversationMessageTx(ctx, tx, message.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return r.insertAgentPlanMessage(ctx, tx, base, message, string(metadataJSON))
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("read agent plan message: %w", err)
	}
	if r.agentPlanUpsertAfterRead != nil {
		r.agentPlanUpsertAfterRead()
	}
	return r.updateAgentPlanMessage(ctx, tx, base, existing, message)
}

func (r *Repository) updateAgentPlanMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	base int64,
	existing, incoming *models.Message,
) (*models.Message, *models.ConversationMutationReceipt, bool, error) {
	if !sameAgentPlanMessageIdentity(existing, incoming) {
		return nil, nil, false, repoerrors.ErrMessageIdentityConflict
	}
	if existing.Content == incoming.Content {
		return existing, nil, false, nil
	}

	existing.Content = incoming.Content
	existing.UpdatedAt = r.nowUTC()
	existingMetadata, err := json.Marshal(existing.Metadata)
	if err != nil {
		return nil, nil, false, fmt.Errorf("serialize stored agent plan metadata: %w", err)
	}
	requestsInput := 0
	if existing.RequestsInput {
		requestsInput = 1
	}
	if err := r.updateMessageWithPayloadGuardTx(ctx, tx, existing, existingMetadata, requestsInput); err != nil {
		return nil, nil, false, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationMessageReceipt(
		ctx, tx, receipt, base, existing, models.ConversationMutationUpsert,
	); err != nil {
		return nil, nil, false, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, nil, false, err
	}
	return existing, receipt, false, nil
}

func (r *Repository) lockAgentPlanMessageIdentity(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			agentPlanMessageLockNamespace+message.ID,
		); err != nil {
			return fmt.Errorf("lock agent plan message %q: %w", message.ID, err)
		}
		return nil
	}
	result, err := tx.ExecContext(
		ctx,
		tx.Rebind(`UPDATE task_sessions SET id = id WHERE id = ?`),
		message.TaskSessionID,
	)
	if err != nil {
		return fmt.Errorf("lock agent plan session %q: %w", message.TaskSessionID, err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return fmt.Errorf("task session %s not found", message.TaskSessionID)
	}
	return nil
}

func (r *Repository) insertAgentPlanMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	base int64,
	message *models.Message,
	metadataJSON string,
) (*models.Message, *models.ConversationMutationReceipt, bool, error) {
	now := r.nowUTC()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	if err := r.insertMessageWithPayloadGuardTx(
		ctx, tx, message, requestsInput, string(message.Type), metadataJSON,
	); err != nil {
		return nil, nil, false, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationMessageReceipt(
		ctx, tx, receipt, base, message, models.ConversationMutationUpsert,
	); err != nil {
		return nil, nil, false, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, nil, false, err
	}
	return message, receipt, true, nil
}

func sameAgentPlanMessageIdentity(existing, incoming *models.Message) bool {
	if existing.ID != incoming.ID ||
		existing.TaskSessionID != incoming.TaskSessionID ||
		existing.TaskID != incoming.TaskID ||
		existing.TurnID != incoming.TurnID ||
		existing.AuthorType != incoming.AuthorType ||
		existing.AuthorID != incoming.AuthorID ||
		existing.Type != incoming.Type ||
		existing.RequestsInput != incoming.RequestsInput {
		return false
	}
	for _, key := range []string{"tool_call_id", "agent_plan_tool_call_id"} {
		existingValue, existingOK := existing.Metadata[key].(string)
		incomingValue, incomingOK := incoming.Metadata[key].(string)
		if !existingOK || !incomingOK || existingValue != incomingValue {
			return false
		}
	}
	return true
}
