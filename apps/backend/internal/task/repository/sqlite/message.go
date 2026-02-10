package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// Message operations

// CreateMessage creates a new message
func (r *Repository) CreateMessage(ctx context.Context, message *models.Message) error {
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
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
		metadataBytes, err := json.Marshal(message.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize message metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID, message.Content, requestsInput, messageType, metadataJSON, message.CreatedAt)

	return err
}

// GetMessage retrieves a message by ID
func (r *Repository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE id = ?
	`, id).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)

	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize message metadata: %w", err)
		}
	}

	return message, nil
}

// ListMessages returns all messages for a session ordered by creation time.
func (r *Repository) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
		if err != nil {
			return nil, err
		}
		message.RequestsInput = requestsInput == 1
		message.Type = models.MessageType(messageType)

		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
				return nil, fmt.Errorf("failed to deserialize message metadata: %w", err)
			}
		}

		result = append(result, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListMessagesPaginated returns messages for a session ordered by creation time with pagination.
func (r *Repository) ListMessagesPaginated(ctx context.Context, sessionID string, opts models.ListMessagesOptions) ([]*models.Message, bool, error) {
	limit := opts.Limit
	if limit < 0 {
		limit = 0
	}

	sortDir := "ASC"
	if strings.EqualFold(opts.Sort, "desc") {
		sortDir = "DESC"
	}

	var cursor *models.Message
	if opts.Before != "" {
		var err error
		cursor, err = r.GetMessage(ctx, opts.Before)
		if err != nil {
			return nil, false, err
		}
		if cursor.TaskSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.Before)
		}
	}
	if opts.After != "" {
		var err error
		cursor, err = r.GetMessage(ctx, opts.After)
		if err != nil {
			return nil, false, err
		}
		if cursor.TaskSessionID != sessionID {
			return nil, false, fmt.Errorf("message cursor not found: %s", opts.After)
		}
	}

	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ?`
	args := []interface{}{sessionID}
	if cursor != nil {
		if opts.Before != "" {
			query += " AND (created_at < ? OR (created_at = ? AND id < ?))"
		} else if opts.After != "" {
			query += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		}
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	query += fmt.Sprintf(" ORDER BY created_at %s, id %s", sortDir, sortDir)
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit+1)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()

	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
		if err != nil {
			return nil, false, err
		}
		message.RequestsInput = requestsInput == 1
		message.Type = models.MessageType(messageType)

		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
				return nil, false, fmt.Errorf("failed to deserialize message metadata: %w", err)
			}
		}

		result = append(result, message)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := false
	if limit > 0 && len(result) > limit {
		hasMore = true
		result = result[:limit]
	}
	return result, hasMore, nil
}

// DeleteMessage deletes a message by ID
func (r *Repository) DeleteMessage(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM task_session_messages WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", id)
	}
	return nil
}

// GetMessageByToolCallID retrieves a tool message by session ID and tool_call_id in metadata.
// Searches all message types that have a tool_call_id in metadata (tool_call, tool_read, tool_edit,
// tool_execute, tool_search, todo, etc.).
func (r *Repository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? AND json_extract(metadata, '$.tool_call_id') = ?
		ORDER BY created_at ASC LIMIT 1
	`, sessionID, toolCallID).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &message.Metadata)
	}
	return message, nil
}

// GetMessageByPendingID retrieves a message by session ID and pending_id in metadata
func (r *Repository) GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? AND json_extract(metadata, '$.pending_id') = ?
	`, sessionID, pendingID).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &message.Metadata)
	}
	return message, nil
}

// CompleteRunningToolCallsForTurn marks all "running" tool call messages for a turn as "complete".
// This is a safety net to ensure no tool calls remain stuck in "running" state after a turn completes.
func (r *Repository) CompleteRunningToolCallsForTurn(ctx context.Context, turnID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE task_session_messages
		SET metadata = json_set(metadata, '$.status', 'complete')
		WHERE turn_id = ?
		  AND json_extract(metadata, '$.status') = 'running'
		  AND json_extract(metadata, '$.tool_call_id') IS NOT NULL
	`, turnID)
	if err != nil {
		return 0, fmt.Errorf("failed to complete running tool calls for turn %s: %w", turnID, err)
	}

	rows, _ := result.RowsAffected()
	return rows, nil
}

// UpdateMessage updates an existing message
func (r *Repository) UpdateMessage(ctx context.Context, message *models.Message) error {
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	requestsInput := 0
	if message.RequestsInput {
		requestsInput = 1
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE task_session_messages SET content = ?, requests_input = ?, type = ?, metadata = ?
		WHERE id = ?
	`, message.Content, requestsInput, string(message.Type), string(metadataJSON), message.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", message.ID)
	}
	return nil
}

