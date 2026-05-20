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

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID, message.Content, requestsInput, messageType, metadataJSON, message.CreatedAt)

	return err
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
		query += sqlLimitClause
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

// SearchMessages returns messages in a session whose content matches the query
// (case-insensitive substring). Newest-first ordering. Limit is capped.
func (r *Repository) SearchMessages(ctx context.Context, sessionID string, opts models.SearchMessagesOptions) ([]*models.Message, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	like := dialect.Like(r.ro.DriverName())
	// Escape LIKE metacharacters so a user query of "%" or "_" matches
	// literally rather than as SQL wildcards.
	escaper := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	pattern := "%" + escaper.Replace(query) + "%"
	sql := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages
		WHERE task_session_id = ? AND content %s ? ESCAPE '\'
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, like)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(sql), sessionID, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result, _, err := scanMessageRows(rows, 0)
	return result, err
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
//
// Permission_request messages also store tool_call_id but represent the user-approval card,
// not the tool call itself. Excluded so a tool_update never lands on a permission_request
// row — that would overwrite metadata.status (the user's approve/reject) and retype it to
// tool_execute, making the prompt buttons reappear after the turn ends.
func (r *Repository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	return r.getMessageByMetadataField(ctx, sessionID, "tool_call_id", toolCallID,
		"AND type != 'permission_request' ORDER BY created_at ASC LIMIT 1")
}

// GetMessageByPendingID retrieves a message by session ID and pending_id in metadata
func (r *Repository) GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error) {
	return r.getMessageByMetadataField(ctx, sessionID, "pending_id", pendingID, "")
}

// FindMessageByPendingID finds the most-recent message by pending_id alone.
// This is useful when we only have the pending ID (e.g., from expired clarification responses).
// For multi-question clarification requests, prefer FindMessagesByPendingID to retrieve
// every related message in one shot.
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

// FindMessagesByPendingID returns every message that carries the given pending_id
// in its metadata, ordered by creation time (oldest first). Multi-question
// clarification requests emit one message per question, all sharing the same
// pending_id; this lookup lets the canceller / status-update path touch all of
// them without N round-trips.
func (r *Repository) FindMessagesByPendingID(ctx context.Context, pendingID string) ([]*models.Message, error) {
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages WHERE %s = ?
		ORDER BY created_at ASC
	`, dialect.JSONExtract(drv, "metadata", "pending_id"))
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), pendingID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result, _, err := scanMessageRows(rows, 0)
	return result, err
}

// FindMessageByPendingIDAndQuestion finds the message for a specific (pending_id,
// question_id) pair within a session. Used to flip per-question status (answered /
// rejected) on multi-question clarification bundles. The persistence layer stores
// the question id at metadata.question_id (flat, alongside metadata.question) so
// the JSON path lookup works on both SQLite and Postgres.
func (r *Repository) FindMessageByPendingIDAndQuestion(ctx context.Context, sessionID, pendingID, questionID string) (*models.Message, error) {
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at
		FROM task_session_messages
		WHERE task_session_id = ? AND %s = ? AND %s = ?
		ORDER BY created_at ASC LIMIT 1
	`,
		dialect.JSONExtract(drv, "metadata", "pending_id"),
		dialect.JSONExtract(drv, "metadata", "question_id"),
	)
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), sessionID, pendingID, questionID).Scan(
		&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt,
	)
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

// CompletePendingToolCallsForTurn marks all non-terminal tool call messages for a turn as "complete".
// This is a safety net to ensure no tool calls remain stuck in a non-terminal state (pending,
// running, in_progress, etc.) after a turn completes.
//
// Excludes permission_request messages: they share `tool_call_id` in metadata but their
// `status` is the user's approve/reject decision, not the tool call state. Forcing them to
// "complete" wipes "approved"/"rejected" and re-shows the prompt buttons in the UI.
func (r *Repository) CompletePendingToolCallsForTurn(ctx context.Context, turnID string) (int64, error) {
	drv := r.db.DriverName()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s
		WHERE turn_id = ?
		  AND type != 'permission_request'
		  AND %s NOT IN ('complete', 'error')
		  AND %s
	`, dialect.JSONSet(drv, "metadata", "status", "complete"),
		dialect.JSONExtract(drv, "metadata", "status"),
		dialect.JSONExtractIsNotNull(drv, "metadata", "tool_call_id"))
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), turnID)
	if err != nil {
		return 0, fmt.Errorf("failed to complete pending tool calls for turn %s: %w", turnID, err)
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

// CountToolCallMessagesBySession returns the number of tool_call messages
// per session for the given session IDs. Sessions with zero tool calls are
// omitted from the result map. Powers the "ran N commands" segment of the
// inline session timeline entry without requiring the frontend to fetch the
// full message list for each row.
func (r *Repository) CountToolCallMessagesBySession(ctx context.Context, sessionIDs []string) (map[string]int, error) {
	if len(sessionIDs) == 0 {
		return map[string]int{}, nil
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, 0, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := `SELECT task_session_id, COUNT(*) AS cnt
	          FROM task_session_messages
	          WHERE type = 'tool_call'
	            AND task_session_id IN (` + strings.Join(placeholders, ",") + `)
	          GROUP BY task_session_id`
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int)
	for rows.Next() {
		var sessionID string
		var count int
		if err := rows.Scan(&sessionID, &count); err != nil {
			return nil, err
		}
		out[sessionID] = count
	}
	return out, rows.Err()
}
