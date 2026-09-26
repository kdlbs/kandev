package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) CommitManagedAgentStreamEvent(ctx context.Context, event models.ManagedAgentStreamEvent) (bool, error) {
	if !managedAgentStreamEventIdentityComplete(event) {
		return false, fmt.Errorf("managed agent stream event identity is incomplete")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin managed agent stream event: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	operation, err := loadManagedAgentStreamOperation(ctx, tx, r.db, event)
	if err != nil {
		return false, err
	}
	recorded, err := recordManagedAgentStreamEvent(ctx, tx, r.db, event)
	if err != nil {
		return false, err
	}
	if !recorded {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit duplicate managed agent stream event: %w", err)
		}
		return false, nil
	}
	if event.Message != nil {
		if err := upsertManagedAgentMessage(ctx, tx, r.db, operation, event.Message, event.AppendMessage); err != nil {
			return false, err
		}
	}
	if err := upsertManagedAgentCheckpoint(ctx, tx, r.db, event); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit managed agent stream event: %w", err)
	}
	return true, nil
}

func managedAgentStreamEventIdentityComplete(event models.ManagedAgentStreamEvent) bool {
	for _, value := range []string{event.BindingID, event.OperationID, event.RemoteRunID, event.EventType} {
		if value == "" {
			return false
		}
	}
	return event.DispatchGeneration > 0
}

func loadManagedAgentStreamOperation(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	event models.ManagedAgentStreamEvent,
) (*models.ManagedAgentOperation, error) {
	query := `SELECT ` + managedAgentOperationColumns + ` FROM managed_agent_operations WHERE id = ?` + managedAgentLockSuffix(db.DriverName())
	operation, err := scanManagedAgentOperation(tx.QueryRowContext(ctx, db.Rebind(query), event.OperationID))
	if err != nil {
		return nil, fmt.Errorf("load managed agent stream operation: %w", err)
	}
	if operation.BindingID != event.BindingID || operation.RemoteRunID != event.RemoteRunID ||
		operation.DispatchGeneration != event.DispatchGeneration {
		return nil, ErrManagedAgentStreamIdentityConflict
	}
	return operation, nil
}

func recordManagedAgentStreamEvent(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	event models.ManagedAgentStreamEvent,
) (bool, error) {
	if event.EventID == "" {
		return true, nil
	}
	result, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO managed_agent_stream_events (binding_id, remote_run_id, event_id, event_type, seen_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT (binding_id, remote_run_id, event_id, event_type) DO NOTHING
	`), event.BindingID, event.RemoteRunID, event.EventID, event.EventType, time.Now().UTC())
	if err != nil {
		return false, fmt.Errorf("record managed agent stream event: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirm managed agent stream event: %w", err)
	}
	return rows == 1, nil
}

func upsertManagedAgentCheckpoint(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	event models.ManagedAgentStreamEvent,
) error {
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO managed_agent_streams (
			binding_id, remote_run_id, last_event_id, cursor, terminal_event_type,
			history_gap, dispatch_generation, updated_at, assistant_message_started
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (binding_id, remote_run_id) DO UPDATE SET
			last_event_id = CASE WHEN excluded.last_event_id = '' THEN managed_agent_streams.last_event_id ELSE excluded.last_event_id END,
			cursor = CASE WHEN excluded.last_event_id = '' THEN managed_agent_streams.cursor ELSE excluded.cursor END,
			terminal_event_type = CASE WHEN managed_agent_streams.terminal_event_type <> '' THEN managed_agent_streams.terminal_event_type ELSE excluded.terminal_event_type END,
			history_gap = CASE WHEN managed_agent_streams.history_gap = 1 OR excluded.history_gap = 1 THEN 1 ELSE 0 END,
			assistant_message_started = CASE WHEN managed_agent_streams.assistant_message_started = 1 OR excluded.assistant_message_started = 1 THEN 1 ELSE 0 END,
			updated_at = excluded.updated_at
		WHERE managed_agent_streams.dispatch_generation = excluded.dispatch_generation
	`), event.BindingID, event.RemoteRunID, event.EventID, event.Cursor, event.TerminalEventType,
		managedAgentBool(event.HistoryGap), event.DispatchGeneration, now,
		managedAgentBool(event.AssistantMessageStarted || event.AppendMessage)); err != nil {
		return fmt.Errorf("upsert managed agent stream checkpoint: %w", err)
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT dispatch_generation FROM managed_agent_streams WHERE binding_id = ? AND remote_run_id = ?
	`), event.BindingID, event.RemoteRunID).Scan(&generation); err != nil {
		return fmt.Errorf("read managed agent stream checkpoint generation: %w", err)
	}
	if generation != event.DispatchGeneration {
		return ErrManagedAgentStreamIdentityConflict
	}
	return nil
}

func upsertManagedAgentMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
	message *models.Message,
	appendMessage bool,
) error {
	metadata, messageType, err := prepareManagedAgentStreamMessage(ctx, tx, db, operation, message)
	if err != nil {
		return err
	}
	requestsInput := managedAgentBool(message.RequestsInput)
	conflictContent := "content = excluded.content"
	if appendMessage {
		conflictContent = "content = task_session_messages.content || excluded.content"
	}
	query := `
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata,
			 created_at, updated_at, prompt_seq, payload_digest, payload_size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET content = excluded.content, requests_input = excluded.requests_input,
			type = excluded.type, metadata = excluded.metadata, updated_at = excluded.updated_at,
			payload_digest = excluded.payload_digest, payload_size = excluded.payload_size
		WHERE task_session_messages.task_session_id = excluded.task_session_id
			AND task_session_messages.task_id = excluded.task_id
	`
	query = strings.Replace(query, "content = excluded.content, requests_input", conflictContent+", requests_input", 1)
	result, err := tx.ExecContext(ctx, db.Rebind(query), message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID,
		message.Content, requestsInput, messageType, metadata, message.CreatedAt, message.UpdatedAt,
		message.PromptIndex, message.PayloadDigest, message.PayloadSize)
	if err != nil {
		return fmt.Errorf("upsert managed agent stream message: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm managed agent stream message: %w", err)
	}
	if rows != 1 {
		return ErrManagedAgentStreamIdentityConflict
	}
	return nil
}

func prepareManagedAgentStreamMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	operation *models.ManagedAgentOperation,
	message *models.Message,
) (string, models.MessageType, error) {
	if message.TaskSessionID == "" || message.TaskID == "" {
		return "", "", fmt.Errorf("managed agent stream message identity is incomplete")
	}
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorAgent
	}
	if message.AuthorType != models.MessageAuthorAgent {
		return "", "", ErrManagedAgentStreamIdentityConflict
	}
	if err := validateManagedAgentMessageOwner(ctx, tx, db, operation.BindingID, message); err != nil {
		return "", "", err
	}
	metadata, err := marshalManagedAgentMessageMetadata(message)
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	message.UpdatedAt = now
	messageType := message.Type
	if messageType == "" {
		messageType = models.MessageTypeMessage
	}
	return metadata, messageType, nil
}

func validateManagedAgentMessageOwner(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	bindingID string,
	message *models.Message,
) error {
	var sessionID, taskID string
	if err := tx.QueryRowContext(ctx, db.Rebind(`SELECT session_id, task_id FROM managed_agent_bindings WHERE id = ?`), bindingID).Scan(&sessionID, &taskID); err != nil {
		return fmt.Errorf("load managed agent message owner: %w", err)
	}
	if message.TaskSessionID != sessionID || message.TaskID != taskID {
		return ErrManagedAgentStreamIdentityConflict
	}
	return nil
}

func marshalManagedAgentMessageMetadata(message *models.Message) (string, error) {
	if message.Metadata == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(message.Metadata)
	if err != nil {
		return "", fmt.Errorf("marshal managed agent message metadata: %w", err)
	}
	return string(encoded), nil
}

func (r *Repository) GetManagedAgentStreamCheckpoint(
	ctx context.Context,
	bindingID, remoteRunID string,
) (*models.ManagedAgentStreamCheckpoint, error) {
	checkpoint := &models.ManagedAgentStreamCheckpoint{}
	var historyGap, assistantMessageStarted int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT binding_id, remote_run_id, last_event_id, cursor, terminal_event_type,
			history_gap, dispatch_generation, updated_at, assistant_message_started
		FROM managed_agent_streams WHERE binding_id = ? AND remote_run_id = ?
	`), bindingID, remoteRunID).Scan(
		&checkpoint.BindingID, &checkpoint.RemoteRunID, &checkpoint.LastEventID, &checkpoint.Cursor,
		&checkpoint.TerminalEventType, &historyGap, &checkpoint.DispatchGeneration, &checkpoint.UpdatedAt, &assistantMessageStarted,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManagedAgentStreamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed agent stream checkpoint: %w", err)
	}
	checkpoint.HistoryGap = historyGap != 0
	checkpoint.AssistantMessageStarted = assistantMessageStarted != 0
	return checkpoint, nil
}
