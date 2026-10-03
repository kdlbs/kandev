package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) beginConversationMutation(ctx context.Context, sessionID string) (*sqlx.Tx, int64, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("begin conversation mutation: %w", err)
	}
	rollback := func(err error) (*sqlx.Tx, int64, error) {
		_ = tx.Rollback()
		return nil, 0, err
	}
	base, err := r.prepareConversationMutation(ctx, tx, sessionID)
	if err != nil {
		return rollback(err)
	}
	return tx, base, nil
}

func (r *Repository) prepareConversationMutation(ctx context.Context, tx *sqlx.Tx, sessionID string) (int64, error) {
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return 0, err
	}
	return r.ensureConversationRevisionTx(ctx, tx, sessionID)
}

func (r *Repository) completePendingToolCallsForTurnTx(ctx context.Context, tx *sqlx.Tx, turnID string) (int64, error) {
	drv := r.db.DriverName()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = CURRENT_TIMESTAMP
		WHERE turn_id = ?
		  AND type != 'permission_request'
		  AND %s NOT IN ('complete', 'error')
		  AND %s
	`, dialect.JSONSet(drv, "metadata", "status", "complete"),
		dialect.JSONExtract(drv, "metadata", "status"),
		dialect.JSONExtractIsNotNull(drv, "metadata", "tool_call_id"))
	result, err := tx.ExecContext(ctx, tx.Rebind(query), turnID)
	if err != nil {
		return 0, fmt.Errorf("failed to complete pending tool calls for turn %s: %w", turnID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *Repository) ensureConversationRevisionTx(ctx context.Context, tx *sqlx.Tx, sessionID string) (int64, error) {
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO conversation_session_revisions(session_id, revision)
		SELECT ?, 0
		WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = ?)
		ON CONFLICT(session_id) DO NOTHING
	`), sessionID, sessionID)
	if err != nil {
		return 0, fmt.Errorf("ensure conversation revision: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		var exists int
		if scanErr := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT 1 FROM task_sessions WHERE id = ?`), sessionID).Scan(&exists); scanErr != nil {
			if scanErr == sql.ErrNoRows {
				return 0, fmt.Errorf("task session %s not found", sessionID)
			}
			return 0, scanErr
		}
	}
	query := `SELECT revision FROM conversation_session_revisions WHERE session_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var revision int64
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), sessionID).Scan(&revision); err != nil {
		return 0, fmt.Errorf("read base conversation revision: %w", err)
	}
	return revision, nil
}

func (r *Repository) conversationMutationReceipt(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
	base int64,
	operation models.ConversationMutationOperation,
) (*models.ConversationMutationReceipt, error) {
	var revision int64
	if err := tx.QueryRowxContext(ctx, tx.Rebind(
		`SELECT revision FROM conversation_session_revisions WHERE session_id = ?`,
	), sessionID).Scan(&revision); err != nil {
		return nil, fmt.Errorf("read committed conversation revision: %w", err)
	}
	return &models.ConversationMutationReceipt{
		SessionID:    sessionID,
		BaseRevision: base,
		Revision:     revision,
		Operations:   []models.ConversationMutationOperation{operation},
		Complete:     revision == base+1,
	}, nil
}

func (r *Repository) finishConversationMutation(tx *sqlx.Tx) error {
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conversation mutation: %w", err)
	}
	return nil
}

func (r *Repository) populateConversationMessageReceipt(
	ctx context.Context,
	tx *sqlx.Tx,
	receipt *models.ConversationMutationReceipt,
	base int64,
	message *models.Message,
	kind models.ConversationMutationKind,
) error {
	result := message
	if kind == models.ConversationMutationUpsert {
		read, err := r.readConversationMessageTx(ctx, tx, message.ID)
		if err != nil {
			return err
		}
		result = read
	}
	operation := models.ConversationMutationOperation{
		Kind:       kind,
		Entity:     models.ConversationEntityMessage,
		ID:         message.ID,
		SessionID:  message.TaskSessionID,
		TaskID:     message.TaskID,
		AuthorType: string(message.AuthorType),
	}
	if kind == models.ConversationMutationUpsert {
		operation.Message = result
	}
	updated, err := r.conversationMutationReceipt(ctx, tx, message.TaskSessionID, base, operation)
	if err != nil {
		return err
	}
	*receipt = *updated
	return nil
}

func (r *Repository) readConversationMessageTx(ctx context.Context, tx *sqlx.Tx, messageID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var turnID sql.NullString
	var messageType string
	var metadataJSON string
	err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content,
		       requests_input, type, metadata, created_at, updated_at,
		       CASE WHEN author_type = 'user' THEN prompt_seq ELSE 0 END
		FROM task_session_messages WHERE id = ?
	`), messageID).Scan(
		&message.ID, &message.TaskSessionID, &message.TaskID, &turnID,
		&message.AuthorType, &message.AuthorID, &message.Content, &requestsInput,
		&messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt,
		&message.PromptIndex,
	)
	if err != nil {
		return nil, err
	}
	message.TurnID = turnID.String
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
			return nil, fmt.Errorf("deserialize conversation message: %w", err)
		}
	}
	return message, nil
}

func (r *Repository) populateConversationTurnReceipt(
	ctx context.Context,
	tx *sqlx.Tx,
	receipt *models.ConversationMutationReceipt,
	base int64,
	turn *models.Turn,
	kind models.ConversationMutationKind,
) error {
	result := turn
	if kind == models.ConversationMutationUpsert {
		read, err := r.readConversationTurnTx(ctx, tx, turn.ID)
		if err != nil {
			return err
		}
		result = read
	}
	operation := models.ConversationMutationOperation{
		Kind:      kind,
		Entity:    models.ConversationEntityTurn,
		ID:        turn.ID,
		SessionID: turn.TaskSessionID,
		TaskID:    turn.TaskID,
	}
	if kind == models.ConversationMutationUpsert {
		operation.Turn = result
	}
	updated, err := r.conversationMutationReceipt(ctx, tx, turn.TaskSessionID, base, operation)
	if err != nil {
		return err
	}
	*receipt = *updated
	return nil
}

func (r *Repository) readConversationTurnTx(ctx context.Context, tx *sqlx.Tx, turnID string) (*models.Turn, error) {
	return scanTurn(tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT id, task_session_id, task_id, execution_profile_id, route_generation,
		       started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns WHERE id = ?
	`), turnID))
}

func (r *Repository) CreateMessageWithConversationReceipt(ctx context.Context, message *models.Message) (*models.ConversationMutationReceipt, error) {
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	messageType := string(message.Type)
	if messageType == "" {
		messageType = string(models.MessageTypeMessage)
	}
	metadataJSON := "{}"
	if message.Metadata != nil {
		encoded, err := json.Marshal(message.Metadata)
		if err != nil {
			return nil, fmt.Errorf("serialize message metadata: %w", err)
		}
		metadataJSON = string(encoded)
	}
	if message.AuthorType == models.MessageAuthorUser {
		return r.createUserMessageWithBoundaryReceipt(ctx, message, requestsInput, messageType, metadataJSON)
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt
	}
	tx, base, err := r.beginConversationMutation(ctx, message.TaskSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.insertMessageWithPayloadGuardTx(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		return nil, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationMessageReceipt(ctx, tx, receipt, base, message, models.ConversationMutationUpsert); err != nil {
		return nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) UpdateMessageWithConversationReceipt(ctx context.Context, message *models.Message) (*models.ConversationMutationReceipt, error) {
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return nil, fmt.Errorf("serialize message metadata: %w", err)
	}
	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}
	message.UpdatedAt = time.Now().UTC()
	tx, base, err := r.beginConversationMutation(ctx, message.TaskSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.updateMessageWithPayloadGuardTx(ctx, tx, message, metadataJSON, requestsInput); err != nil {
		return nil, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationMessageReceipt(ctx, tx, receipt, base, message, models.ConversationMutationUpsert); err != nil {
		return nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) DeleteMessageWithConversationReceipt(ctx context.Context, messageID string) (*models.ConversationMutationReceipt, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin message deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	message, err := r.readConversationMessageTx(ctx, tx, messageID)
	if err != nil {
		return nil, err
	}
	base, err := r.prepareConversationMutation(ctx, tx, message.TaskSessionID)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_session_messages WHERE id = ?`), messageID)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("message not found: %s", messageID)
	}
	receipt, err := r.conversationMutationReceipt(ctx, tx, message.TaskSessionID, base, models.ConversationMutationOperation{
		Kind: models.ConversationMutationRemove, Entity: models.ConversationEntityMessage,
		ID: messageID, SessionID: message.TaskSessionID, TaskID: message.TaskID,
		AuthorType: string(message.AuthorType),
	})
	if err != nil {
		return nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) CreateTurnWithConversationReceipt(ctx context.Context, turn *models.Turn) (*models.ConversationMutationReceipt, error) {
	stampTurnDefaults(turn)
	tx, base, err := r.beginConversationMutation(ctx, turn.TaskSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.insertTurnRow(ctx, tx, turn); err != nil {
		return nil, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationTurnReceipt(ctx, tx, receipt, base, turn, models.ConversationMutationUpsert); err != nil {
		return nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) CreateTurnWithStepStampConversationReceipt(ctx context.Context, turn *models.Turn) (bool, *models.ConversationMutationReceipt, error) {
	stampTurnDefaults(turn)
	tx, base, err := r.beginConversationMutation(ctx, turn.TaskSessionID)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, stepID, found, stepErr := r.readTaskStepInTx(ctx, tx, turn.TaskID)
	stamped := false
	if stepErr == nil && found && stepID != "" {
		if turn.Metadata == nil {
			turn.Metadata = map[string]interface{}{}
		}
		turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart] = stepID
		stamped = true
	}
	if err := r.insertTurnRow(ctx, tx, turn); err != nil {
		return false, nil, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationTurnReceipt(ctx, tx, receipt, base, turn, models.ConversationMutationUpsert); err != nil {
		return false, nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return false, nil, err
	}
	return stamped, receipt, nil
}

func (r *Repository) UpdateTurnWithConversationReceipt(ctx context.Context, turn *models.Turn) (*models.ConversationMutationReceipt, error) {
	metadataJSON, err := serializeTurnMetadata(turn.Metadata)
	if err != nil {
		return nil, err
	}
	tx, base, err := r.beginConversationMutation(ctx, turn.TaskSessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	updatedAt := r.nowUTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_session_turns
		SET completed_at = ?, metadata = ?, execution_profile_id = ?, route_generation = ?, updated_at = ?
		WHERE id = ? AND task_session_id = ? AND updated_at = ?
	`), turn.CompletedAt, metadataJSON, turn.ExecutionProfileID, turn.RouteGeneration, updatedAt, turn.ID, turn.TaskSessionID, turn.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("update turn %s: stale metadata snapshot", turn.ID)
	}
	turn.UpdatedAt = updatedAt
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationTurnReceipt(ctx, tx, receipt, base, turn, models.ConversationMutationUpsert); err != nil {
		return nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) CompleteTurnWithConversationReceipt(ctx context.Context, turnID string) (*models.ConversationMutationReceipt, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin turn completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	turn, err := r.readConversationTurnTx(ctx, tx, turnID)
	if err != nil {
		return nil, err
	}
	base, err := r.prepareConversationMutation(ctx, tx, turn.TaskSessionID)
	if err != nil {
		return nil, err
	}
	if _, err := r.completePendingToolCallsForTurnTx(ctx, tx, turnID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_session_turns SET completed_at = ?, updated_at = ? WHERE id = ?
	`), now, now, turnID); err != nil {
		return nil, err
	}
	turn, err = r.readConversationTurnTx(ctx, tx, turnID)
	if err != nil {
		return nil, err
	}
	receipt := &models.ConversationMutationReceipt{}
	if err := r.populateConversationTurnReceipt(ctx, tx, receipt, base, turn, models.ConversationMutationUpsert); err != nil {
		return nil, err
	}
	if err := r.finishConversationMutation(tx); err != nil {
		return nil, err
	}
	return receipt, nil
}
