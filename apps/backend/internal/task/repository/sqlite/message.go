package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// Message operations

// messageInsertArgs holds the derived values needed for a message INSERT.
type messageInsertArgs struct {
	requestsInput int
	messageType   string
	metadataJSON  string
}

// prepareMessageInsert applies defaults to a message (ID, CreatedAt, AuthorType)
// and derives the insert arguments (requestsInput, messageType, metadataJSON).
func prepareMessageInsert(message *models.Message, fallbackTime time.Time) (messageInsertArgs, error) {
	if message.ID == "" {
		message.ID = uuid.New().String()
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = fallbackTime
	}
	if message.AuthorType == "" {
		message.AuthorType = models.MessageAuthorUser
	}

	args := messageInsertArgs{}

	if message.RequestsInput {
		args.requestsInput = 1
	}

	args.messageType = string(message.Type)
	if args.messageType == "" {
		args.messageType = string(models.MessageTypeMessage)
	}

	args.metadataJSON = "{}"
	if message.Metadata != nil {
		metadataBytes, err := json.Marshal(message.Metadata)
		if err != nil {
			return args, fmt.Errorf("failed to serialize message metadata: %w", err)
		}
		args.metadataJSON = string(metadataBytes)
	}

	return args, nil
}

// CreateMessage creates a new message
func (r *Repository) CreateMessage(ctx context.Context, message *models.Message) error {
	args, err := prepareMessageInsert(message, time.Now().UTC())
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID, message.Content, args.requestsInput, args.messageType, args.metadataJSON, message.CreatedAt)

	return err
}

// CreateMessages batch-inserts multiple messages in a single transaction.
// This is more efficient than individual CreateMessage calls because it
// wraps all INSERTs in one transaction instead of N separate transactions.
func (r *Repository) CreateMessages(ctx context.Context, messages []*models.Message) error {
	if len(messages) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`))
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC()
	for _, message := range messages {
		args, prepErr := prepareMessageInsert(message, now)
		if prepErr != nil {
			return prepErr
		}

		if _, execErr := stmt.ExecContext(ctx, message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID, message.Content, args.requestsInput, args.messageType, args.metadataJSON, message.CreatedAt); execErr != nil {
			return fmt.Errorf("failed to insert message %s: %w", message.ID, execErr)
		}
	}

	return tx.Commit()
}

// AppendContent appends additional text to a message's content using SQL
// string concatenation. This avoids the read-modify-write cycle of
// GetMessage + string append + UpdateMessage.
func (r *Repository) AppendContent(ctx context.Context, messageID, additionalContent string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_messages SET content = content || ? WHERE id = ?
	`), additionalContent, messageID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}
	return nil
}

// GetMessage retrieves a message by ID
func (r *Repository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE id = ?
	`), id).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt)
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
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? ORDER BY created_at ASC
	`), sessionID)
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
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListMessagesPaginated")
	defer span.End()

	limit := opts.Limit
	if limit < 0 {
		limit = 0
	}
	sortDir := "ASC"
	if strings.EqualFold(opts.Sort, "desc") {
		sortDir = "DESC"
	}
	cursor, err := r.resolveMessageCursor(ctx, sessionID, opts)
	if err != nil {
		return nil, false, err
	}
	query, args := buildListMessagesQuery(sessionID, opts, cursor, sortDir, limit)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	return scanMessageRows(rows, limit)
}

func (r *Repository) resolveMessageCursor(ctx context.Context, sessionID string, opts models.ListMessagesOptions) (*models.Message, error) {
	if opts.Before != "" {
		cursor, err := r.GetMessage(ctx, opts.Before)
		if err != nil {
			return nil, err
		}
		if cursor.TaskSessionID != sessionID {
			return nil, fmt.Errorf("message cursor not found: %s", opts.Before)
		}
		return cursor, nil
	}
	if opts.After != "" {
		cursor, err := r.GetMessage(ctx, opts.After)
		if err != nil {
			return nil, err
		}
		if cursor.TaskSessionID != sessionID {
			return nil, fmt.Errorf("message cursor not found: %s", opts.After)
		}
		return cursor, nil
	}
	return nil, nil
}

func buildListMessagesQuery(sessionID string, opts models.ListMessagesOptions, cursor *models.Message, sortDir string, limit int) (string, []interface{}) {
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
	return query, args
}

func scanMessageRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}, limit int) ([]*models.Message, bool, error) {
	var result []*models.Message
	for rows.Next() {
		message := &models.Message{}
		var requestsInput int
		var messageType string
		var metadataJSON string
		if err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt); err != nil {
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
	if limit > 0 && len(result) > limit {
		return result[:limit], true, nil
	}
	return result, false, nil
}

// DeleteMessage deletes a message by ID
func (r *Repository) DeleteMessage(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_session_messages WHERE id = ?`), id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", id)
	}
	return nil
}

func (r *Repository) getMessageByMetadataField(ctx context.Context, sessionID, fieldName, fieldValue, orderSuffix string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE task_session_id = ? AND %s = ?
		%s
	`, dialect.JSONExtract(drv, "metadata", fieldName), orderSuffix)
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), sessionID, fieldValue).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
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

// GetMessageByToolCallID retrieves a tool message by session ID and tool_call_id in metadata.
// Searches all message types that have a tool_call_id in metadata (tool_call, tool_read, tool_edit,
// tool_execute, tool_search, todo, etc.).
func (r *Repository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	return r.getMessageByMetadataField(ctx, sessionID, "tool_call_id", toolCallID, "ORDER BY created_at ASC LIMIT 1")
}

// GetMessageByPendingID retrieves a message by session ID and pending_id in metadata
func (r *Repository) GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error) {
	return r.getMessageByMetadataField(ctx, sessionID, "pending_id", pendingID, "")
}

// FindMessageByPendingID finds a message by pending_id alone (without requiring session ID).
// This is useful when we only have the pending ID (e.g., from expired clarification responses).
func (r *Repository) FindMessageByPendingID(ctx context.Context, pendingID string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE %s = ?
		ORDER BY created_at DESC LIMIT 1
	`, dialect.JSONExtract(drv, "metadata", "pending_id"))
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), pendingID).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
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
	drv := r.db.DriverName()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s
		WHERE turn_id = ?
		  AND %s = 'running'
		  AND %s
	`, dialect.JSONSet(drv, "metadata", "status", "complete"),
		dialect.JSONExtract(drv, "metadata", "status"),
		dialect.JSONExtractIsNotNull(drv, "metadata", "tool_call_id"))
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), turnID)
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

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_messages SET content = ?, requests_input = ?, type = ?, metadata = ?
		WHERE id = ?
	`), message.Content, requestsInput, string(message.Type), string(metadataJSON), message.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", message.ID)
	}
	return nil
}
