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

	if message.AuthorType == models.MessageAuthorUser {
		// User messages get their prompt-ordering timestamp and ordinal from
		// the atomic per-session create boundary (see
		// createUserMessageWithBoundary). Agent/tool messages stay on the hot
		// path below.
		return r.createUserMessageWithBoundary(ctx, message, requestsInput, messageType, metadataJSON)
	}

	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt
	}
	return r.insertMessageWithSessionLock(ctx, message, requestsInput, messageType, metadataJSON)
}

func (r *Repository) insertMessageRow(
	ctx context.Context,
	execer taskSessionExecutor,
	message *models.Message,
	requestsInput int,
	messageType, metadataJSON string,
) error {
	_, err := execer.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_messages (id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at, prompt_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), message.ID, message.TaskSessionID, message.TaskID, message.TurnID, message.AuthorType, message.AuthorID, message.Content, requestsInput, messageType, metadataJSON, message.CreatedAt, message.UpdatedAt, message.PromptIndex)
	return err
}

// insertMessageWithSessionLock serializes message insertion with rollback of
// the message's turn on PostgreSQL. SQLite's writer pool is configured with a
// single connection in db.OpenSQLite, which provides equivalent serialization.
// If SQLite ever allows concurrent writers, add the equivalent session lock.
func (r *Repository) insertMessageWithSessionLock(
	ctx context.Context,
	message *models.Message,
	requestsInput int,
	messageType, metadataJSON string,
) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return r.insertMessageRow(ctx, r.db, message, requestsInput, messageType, metadataJSON)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin message creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), message.TaskSessionID); err != nil {
		return err
	}
	if err := r.insertMessageRow(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message creation: %w", err)
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
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages WHERE id = ?
	`), id).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt)
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
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages WHERE task_session_id = ? ORDER BY created_at ASC
	`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	msgs, _, err := scanMessageRows(rows, 0)
	return msgs, err
}

// ListMessagesByTurnID returns all messages for a single turn ordered by
// creation time. Backed by idx_messages_turn_id, so it reads only the turn's
// own rows rather than the whole session's history.
func (r *Repository) ListMessagesByTurnID(ctx context.Context, turnID string) ([]*models.Message, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages WHERE turn_id = ? ORDER BY created_at ASC
	`), turnID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	msgs, _, err := scanMessageRows(rows, 0)
	return msgs, err
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
	// The outer cursor bound is the cursor row's normalized-microsecond key,
	// derived through the same SQL expression the per-row key uses — never a
	// driver-parsed Go time.Time (mattn/go-sqlite3 serializes time.Time as
	// RFC3339Nano, which mis-compares lexicographically against the
	// space-separated per-row expression and silently shifts page boundaries).
	var cursorKey string
	if cursor != nil {
		cursorKey, err = r.messageCursorKey(ctx, cursor.ID)
		if err != nil {
			return nil, false, err
		}
	}
	query, args := buildListMessagesQuery(r.ro.DriverName(), sessionID, opts, cursor, cursorKey, sortDir, limit)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	return scanPromptIndexedMessageRows(rows, limit)
}

// messageCursorKey returns the normalized-microsecond ordering key of the
// cursor row, derived through the same SQL expression the per-row key uses.
// On SQLite the expression yields the key text directly; on PostgreSQL the
// stored timestamp is scanned and formatted to the exact key bytes (the
// stored value is UTC by convention, and a naive timestamp carries no offset
// whose parsing could diverge from the per-row expression).
func (r *Repository) messageCursorKey(ctx context.Context, id string) (string, error) {
	drv := r.ro.DriverName()
	query := fmt.Sprintf(
		"SELECT %s FROM task_session_messages WHERE id = ?",
		dialect.NormalizedMicrosecond(drv, "created_at"),
	)
	if dialect.IsPostgres(drv) {
		var ts time.Time
		if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), id).Scan(&ts); err != nil {
			return "", err
		}
		return formatPromptKey(ts), nil
	}
	var key string
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), id).Scan(&key); err != nil {
		return "", err
	}
	return key, nil
}

// ListMessagesForPlugin returns messages matching the plugin Host data API
// filter (ADR 0047): session/task ids, a created_at time range, and message
// types, ordered oldest-first by (created_at, id) with SQL Limit/Offset. It
// backs internal/plugins' capability-gated Messages().List reader — always via
// the service layer, never called directly by a plugin.
func (r *Repository) ListMessagesForPlugin(ctx context.Context, filter models.PluginMessageFilter) ([]*models.Message, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListMessagesForPlugin")
	defer span.End()

	query, args := buildPluginMessageQuery(filter)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	msgs, _, err := scanMessageRows(rows, 0)
	return msgs, err
}

// buildPluginMessageQuery assembles the WHERE/ORDER/LIMIT/OFFSET clauses for
// ListMessagesForPlugin. session_ids and task_ids each become an IN (...)
// clause (ANDed together when both are set); Since is an inclusive and Until
// an exclusive created_at bound; types filters by message type. A zero/negative
// Limit yields no LIMIT clause (returns all matching rows from the offset).
func buildPluginMessageQuery(filter models.PluginMessageFilter) (string, []interface{}) {
	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages`
	var conditions []string
	var args []interface{}

	if ph, inArgs := buildInPlaceholders(filter.SessionIDs); ph != "" {
		conditions = append(conditions, "task_session_id IN ("+ph+")")
		args = append(args, inArgs...)
	}
	if ph, inArgs := buildInPlaceholders(filter.TaskIDs); ph != "" {
		conditions = append(conditions, "task_id IN ("+ph+")")
		args = append(args, inArgs...)
	}
	if ph, inArgs := buildInPlaceholders(filter.Types); ph != "" {
		conditions = append(conditions, "type IN ("+ph+")")
		args = append(args, inArgs...)
	}
	if filter.Since != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		conditions = append(conditions, "created_at < ?")
		args = append(args, *filter.Until)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at ASC, id ASC"
	// OFFSET is only emitted alongside a LIMIT: standalone OFFSET is a syntax
	// error in SQLite, and the only caller (the plugin message reader) always
	// requests a positive Limit (page-limit+1), so Offset never travels alone.
	if filter.Limit > 0 {
		query += sqlLimitClause
		args = append(args, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filter.Offset)
		}
	}
	return query, args
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

// buildListMessagesQuery assembles the paginated message list query and bound arguments, ordering rows by the normalized-microsecond key with cursor and limit bounds.
func buildListMessagesQuery(driverName, sessionID string, opts models.ListMessagesOptions, cursor *models.Message, cursorKey string, sortDir string, limit int) (string, []interface{}) {
	nm := dialect.NormalizedMicrosecond(driverName, "created_at")
	// The cursor bound is the normalized key literal `YYYY-MM-DD HH:MM:SS.ffffff`
	// (space separator, zero-padded 6-digit fraction, no `Z`, UTC). PostgreSQL
	// casts it to `timestamp` so it compares against its native per-row key;
	// SQLite compares the text key directly.
	bound := "?"
	if dialect.IsPostgres(driverName) {
		bound = "CAST(? AS timestamp)"
	}
	// prompt_index is the durable per-session ordinal persisted at creation
	// (prompt_seq), so it is a plain column: absolute rather than
	// page-relative by construction, no window pass needed. The cursor
	// predicate, ORDER BY, and LIMIT (limit+1) use the same normalized key as
	// the rest of the package, so every page is contiguous in ordinal order
	// and the sentinel `promptNumber === 1` stop can never skip a
	// higher-ordinal prompt sharing a microsecond.
	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at,
		       CASE WHEN author_type = 'user' THEN prompt_seq ELSE 0 END AS prompt_index
		FROM task_session_messages
		WHERE task_session_id = ?`
	args := []interface{}{sessionID}
	if cursor != nil {
		if opts.Before != "" {
			query += fmt.Sprintf(" AND (%s < %s OR (%s = %s AND id < ?))", nm, bound, nm, bound)
		} else if opts.After != "" {
			query += fmt.Sprintf(" AND (%s > %s OR (%s = %s AND id > ?))", nm, bound, nm, bound)
		}
		args = append(args, cursorKey, cursorKey, cursor.ID)
	}
	query += fmt.Sprintf(" ORDER BY %s %s, id %s", nm, sortDir, sortDir)
	if limit > 0 {
		query += sqlLimitClause
		args = append(args, limit+1)
	}
	return query, args
}

// scanPromptIndexedMessageRows scans the 13-column indexed message projection
// (the 12 legacy columns plus the computed prompt_index), returning the
// page-truncated slice and whether more rows remain, like scanMessageRows.
func scanPromptIndexedMessageRows(rows interface {
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
		if err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt, &message.PromptIndex); err != nil {
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
		if err := rows.Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID, &message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt); err != nil {
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
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
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
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages WHERE task_session_id = ? AND %s = ?
		%s
	`, dialect.JSONExtract(drv, "metadata", fieldName), orderSuffix)
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), sessionID, fieldValue).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt)
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

// GetPermissionMessageByIdentity retrieves a permission_request message by its
// full (task, session, request, pending) identity. A provider may reuse
// pending_id for a later, unrelated request once the original is resolved, so
// matching request_id too prevents a delayed event about the old request from
// being applied to the new one.
func (r *Repository) GetPermissionMessageByIdentity(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.Message, error) {
	return r.getPermissionMessageByIdentity(ctx, taskID, sessionID, requestID, pendingID)
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
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages WHERE %s = ?
		ORDER BY created_at DESC LIMIT 1
	`, dialect.JSONExtract(drv, "metadata", "pending_id"))
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), pendingID).Scan(&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt)
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
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
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

// FindActiveClarificationMessagesBySessionID returns pending clarification rows
// owned by the session's newest durable turn. Older pending rows remain history.
// The canceller still drains live legacy waiters before this lookup. Persisted
// rows without their schema-required turn are malformed and need explicit data
// cleanup; they never become current input authority.
func (r *Repository) FindActiveClarificationMessagesBySessionID(ctx context.Context, sessionID string) ([]*models.Message, error) {
	drv := r.ro.DriverName()
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	query := fmt.Sprintf(`
		WITH current_turn AS (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = ?
			  AND %s
			ORDER BY %s
			LIMIT 1
		)
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
		       m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at
		FROM task_session_messages m
		JOIN current_turn current ON current.id = m.turn_id
		WHERE m.task_session_id = ?
		  AND m.type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		ORDER BY m.created_at ASC, m.id ASC
	`, predicate, orderBy, dialect.JSONExtract(drv, "m.metadata", "status"))
	// First sessionID selects current-turn authority; second scopes messages so
	// a malformed cross-session turn reference cannot leak another session's row.
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), sessionID, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result, _, err := scanMessageRows(rows, 0)
	return result, err
}

// GetPendingActionsBySessionIDs returns the compact pending-action projection
// for sessions whose task-list state needs to render before messages are loaded.
func (r *Repository) GetPendingActionsBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error) {
	result := make(map[string]models.TaskPendingAction)
	if len(sessionIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, 0, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := pendingActionsBySessionQuery(r.ro.DriverName(), placeholders)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sessionID, action string
		if err := rows.Scan(&sessionID, &action); err != nil {
			return nil, err
		}
		// Clarification is the explicit per-session priority when malformed or
		// transitional history contains both pending action kinds. Enforce it
		// here so correctness never depends on UNION ALL row order.
		if action == string(models.TaskPendingActionClarification) {
			result[sessionID] = models.TaskPendingActionClarification
			continue
		}
		if _, hasClarification := result[sessionID]; hasClarification {
			continue
		}
		if action == string(models.TaskPendingActionPermission) {
			result[sessionID] = models.TaskPendingActionPermission
		}
	}
	return result, rows.Err()
}

func pendingActionsBySessionQuery(driverName string, placeholders []string) string {
	placeholderList := strings.Join(placeholders, ",")
	statusExpr := dialect.JSONExtract(driverName, "m.metadata", "status")
	permissionOrderExpr := pendingActionMessageOrder(driverName, "m")
	// Durable turns are authoritative. A message without a matching turn is
	// malformed under the schema foreign key and must not reactivate old input.
	// Terminal sessions quarantine pending history even when best-effort expiry
	// persistence failed. Both message CTEs inherit the requested-session
	// boundary through their task_session_id and turn_id joins to current_turn.
	predicate, orderBy := currentTurnAuthority(driverName, "turn_row")
	return fmt.Sprintf(`
		WITH current_turn AS (
			SELECT task_session_id, id AS turn_id
			FROM (
				SELECT task_session_id,
				       id,
				       ROW_NUMBER() OVER (
				         PARTITION BY task_session_id
				         ORDER BY %s
				       ) AS rn
				FROM task_session_turns turn_row
				WHERE turn_row.task_session_id IN (%s)
				  AND %s
				  AND %s
			) ranked
			WHERE rn = 1
		),
		pending_clarifications AS (
			SELECT DISTINCT m.task_session_id, 'clarification' AS action
			FROM task_session_messages m
			JOIN current_turn current
			  ON current.task_session_id = m.task_session_id
			 AND current.turn_id = m.turn_id
			WHERE m.type = 'clarification_request'
			  AND COALESCE(%s, '') IN ('', 'pending')
		),
		latest_permissions AS (
			SELECT m.task_session_id,
			       COALESCE(%s, '') AS status,
			       ROW_NUMBER() OVER (
			         PARTITION BY m.task_session_id
			         ORDER BY m.created_at DESC, %s DESC
			       ) AS rn
			FROM task_session_messages m
			JOIN current_turn current
			  ON current.task_session_id = m.task_session_id
			 AND current.turn_id = m.turn_id
			WHERE m.type = 'permission_request'
		)
		SELECT task_session_id, action
		FROM pending_clarifications
		UNION ALL
		SELECT task_session_id, 'permission' AS action
		FROM latest_permissions
		WHERE rn = 1 AND status IN ('', 'pending')
	`, orderBy, placeholderList, predicate,
		nonTerminalSessionPredicate("turn_row"), statusExpr, statusExpr, permissionOrderExpr)
}

func pendingActionMessageOrder(driverName string, qualifier string) string {
	column := "id"
	if driverName == dialect.SQLite3 {
		column = "rowid"
	}
	if qualifier == "" {
		return column
	}
	return qualifier + "." + column
}

// FindMessageByPendingIDAndQuestion finds the message for a specific (pending_id,
// question_id) pair within a session. Used to flip per-question status (answered /
// rejected) on multi-question clarification bundles. The persistence layer stores
// the question id at metadata.question_id (flat, alongside metadata.question) so
// the JSON path lookup works on both SQLite and Postgres.
func (r *Repository) FindMessageByPendingIDAndQuestion(ctx context.Context, sessionID, pendingID, questionID string) (*models.Message, error) {
	drv := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at
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
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt,
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
		SET metadata = %s, updated_at = CURRENT_TIMESTAMP
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

	message.UpdatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_messages SET content = ?, requests_input = ?, type = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`), message.Content, requestsInput, string(message.Type), string(metadataJSON), message.UpdatedAt, message.ID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("message not found: %s", message.ID)
	}
	return nil
}

// ClaimPermissionResolution installs the first durable dispatch claim for one
// exact pending permission. The conditional UPDATE is the serialization point
// on both SQLite and PostgreSQL.
func (r *Repository) ClaimPermissionResolution(ctx context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
	request.Audit.Result = models.PermissionResolutionDispatching
	auditJSON, err := json.Marshal(request.Audit)
	if err != nil {
		return nil, fmt.Errorf("marshal permission resolution claim: %w", err)
	}
	driver := r.db.DriverName()
	now := time.Now().UTC()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_id = ? AND task_session_id = ? AND type = 'permission_request'
		  AND %s = ? AND %s = ?
		  AND COALESCE(%s, '') = ''
		  AND %s IS NULL
	`, permissionClaimJSONExpression(driver),
		permissionJSONExtract(driver, "metadata", "request_id"),
		permissionJSONExtract(driver, "metadata", "pending_id"),
		permissionJSONExtract(driver, "metadata", "status"),
		permissionJSONExtract(driver, "metadata", "permission_resolution"))
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), string(auditJSON), now,
		request.TaskID, request.SessionID, request.Audit.RequestID, request.Audit.PendingID)
	if err != nil {
		return nil, fmt.Errorf("claim permission resolution: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("claim permission resolution rows: %w", err)
	}
	message, getErr := r.getPermissionMessageByIdentity(ctx, request.TaskID, request.SessionID, request.Audit.RequestID, request.Audit.PendingID)
	if rows == 1 {
		if getErr != nil {
			return nil, fmt.Errorf("read claimed permission resolution: %w", getErr)
		}
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimed, Message: message}, nil
	}
	if errors.Is(getErr, sql.ErrNoRows) {
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimNotFound}, nil
	}
	if getErr != nil {
		return nil, fmt.Errorf("read unclaimed permission resolution: %w", getErr)
	}
	audit, ok := permissionAuditFromMetadata(message.Metadata)
	if ok && audit.Result == models.PermissionResolutionDispatching {
		return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimInProgress, Message: message}, nil
	}
	return &models.PermissionResolutionClaimResult{Outcome: models.PermissionClaimAlreadyFinal, Message: message}, nil
}

// FinalizePermissionResolution closes only the caller's dispatch claim. A
// failed finalization never reopens the request for another provider response.
func (r *Repository) FinalizePermissionResolution(ctx context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
	driver := r.db.DriverName()
	finalizedAt := request.FinalizedAt.UTC()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_id = ? AND task_session_id = ? AND type = 'permission_request'
		  AND %s = ? AND %s = ?
		  AND %s = ? AND %s = ?
	`, permissionFinalizeJSONExpression(driver),
		permissionJSONExtract(driver, "metadata", "request_id"),
		permissionJSONExtract(driver, "metadata", "pending_id"),
		permissionJSONExtract(driver, "metadata", "permission_resolution", "claim_id"),
		permissionJSONExtract(driver, "metadata", "permission_resolution", "result"))
	result, err := r.db.ExecContext(ctx, r.db.Rebind(query), string(request.Result), finalizedAt.Format(time.RFC3339Nano), string(request.Status), finalizedAt,
		request.TaskID, request.SessionID, request.RequestID, request.PendingID, request.ClaimID, string(models.PermissionResolutionDispatching))
	if err != nil {
		return nil, fmt.Errorf("finalize permission resolution: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("finalize permission resolution rows: %w", err)
	}
	message, getErr := r.getPermissionMessageByIdentity(ctx, request.TaskID, request.SessionID, request.RequestID, request.PendingID)
	if rows == 1 {
		if getErr != nil {
			return nil, fmt.Errorf("read finalized permission resolution: %w", getErr)
		}
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalized, Message: message}, nil
	}
	if errors.Is(getErr, sql.ErrNoRows) {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeNotFound}, nil
	}
	if getErr != nil {
		return nil, fmt.Errorf("read unfinalized permission resolution: %w", getErr)
	}
	audit, ok := permissionAuditFromMetadata(message.Metadata)
	if !ok || audit.ClaimID != request.ClaimID {
		return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeClaimMismatch, Message: message}, nil
	}
	return &models.PermissionResolutionFinalizeResult{Outcome: models.PermissionFinalizeAlreadyFinal, Message: message}, nil
}

func permissionClaimJSONExpression(driver string) string {
	if dialect.IsPostgres(driver) {
		return `jsonb_set(COALESCE(NULLIF(metadata, ''), '{}')::jsonb, '{permission_resolution}', ?::jsonb, true)::text`
	}
	return `json_set(COALESCE(NULLIF(metadata, ''), '{}'), '$.permission_resolution', json(?))`
}

func permissionFinalizeJSONExpression(driver string) string {
	if dialect.IsPostgres(driver) {
		return `jsonb_set(jsonb_set(jsonb_set(COALESCE(NULLIF(metadata, ''), '{}')::jsonb, '{permission_resolution,result}', to_jsonb(?::text), true), '{permission_resolution,finalized_at}', to_jsonb(?::text), true), '{status}', to_jsonb(?::text), true)::text`
	}
	return `json_set(COALESCE(NULLIF(metadata, ''), '{}'), '$.permission_resolution.result', ?, '$.permission_resolution.finalized_at', ?, '$.status', ?)`
}

func permissionJSONExtract(driver, column string, path ...string) string {
	if dialect.IsPostgres(driver) {
		return fmt.Sprintf("COALESCE(NULLIF(%s, ''), '{}')::jsonb#>>'{%s}'", column, strings.Join(path, ","))
	}
	return fmt.Sprintf("json_extract(CASE WHEN json_valid(%s) THEN %s ELSE '{}' END, '$.%s')", column, column, strings.Join(path, "."))
}

func (r *Repository) getPermissionMessageByIdentity(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.Message, error) {
	driver := r.db.DriverName()
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content,
		       requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages
		WHERE task_id = ? AND task_session_id = ? AND type = 'permission_request'
		  AND %s = ? AND %s = ?
		LIMIT 1
	`, permissionJSONExtract(driver, "metadata", "request_id"), permissionJSONExtract(driver, "metadata", "pending_id"))
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(query), taskID, sessionID, requestID, pendingID).Scan(
		&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID,
		&message.AuthorType, &message.AuthorID, &message.Content, &requestsInput,
		&messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	message.RequestsInput = requestsInput == 1
	message.Type = models.MessageType(messageType)
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &message.Metadata); err != nil {
			return nil, fmt.Errorf("deserialize permission message metadata: %w", err)
		}
	}
	return message, nil
}

func permissionAuditFromMetadata(metadata map[string]any) (models.PermissionResolutionAudit, bool) {
	raw, ok := metadata["permission_resolution"]
	if !ok {
		return models.PermissionResolutionAudit{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return models.PermissionResolutionAudit{}, false
	}
	var audit models.PermissionResolutionAudit
	if err := json.Unmarshal(encoded, &audit); err != nil {
		return models.PermissionResolutionAudit{}, false
	}
	return audit, audit.ClaimID != ""
}

func (r *Repository) GetPermissionResolutionAudit(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.PermissionResolutionAudit, error) {
	message, err := r.getPermissionMessageByIdentity(ctx, taskID, sessionID, requestID, pendingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	audit, ok := permissionAuditFromMetadata(message.Metadata)
	if !ok {
		return nil, nil
	}
	return &audit, nil
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
