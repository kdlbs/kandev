//nolint:revive // The journal projection and sanitization contract is intentionally co-located.
package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/sysprompt"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// errConversationCursorGone reports a page cursor whose message was
// deleted/tombstoned before the walk reached it; the page would otherwise
// silently truncate.
var errConversationCursorGone = errors.New("conversation page cursor message no longer exists")

const (
	conversationContentKey    = "content"
	conversationTypeKey       = "type"
	conversationSessionIDKey  = "session_id"
	conversationTaskIDKey     = "task_id"
	conversationMessageIDKey  = "message_id"
	conversationAuthorTypeKey = "author_type"
	conversationTurnCompleted = "session.turn.completed"
	conversationTurnRemoved   = "session.turn.removed"
	conversationTurnStarted   = "session.turn.started"
)

//nolint:goconst // These keys are an explicit privacy allowlist.
var conversationMessageMetadataKeys = map[string]struct{}{
	"action_visibility": {}, "actions": {}, "agent_disconnected": {},
	"attempt": {}, "attachments": {}, "auth_methods": {}, "auto_start": {},
	"base_branch": {}, "context": {}, "context_files": {}, "decision_id": {},
	"effective_model": {}, "entity_references": {}, "error_output": {},
	"failure_code": {}, "failure_details": {}, "failure_kind": {},
	"fallback_model": {}, "has_hidden_prompts": {}, "has_resume_token": {},
	"has_review_comments": {}, "is_auth_error": {}, "kind": {}, "max_attempts": {},
	"message": {}, "missing_branch": {}, "model_id": {}, "new_branch": {},
	"original_branch": {}, "pending_id": {}, "plan_mode": {}, "progress": {},
	"provider_name": {}, "question": {}, "question_id": {}, "question_index": {},
	"question_total": {}, "recovery_actions": {}, "remediation": {},
	"remediation_url": {}, "requested_model": {}, "requests_input": {},
	"response": {}, "reset_at": {}, "retry_at": {}, "retry_in_seconds": {},
	"retrying": {}, "sender_session_id": {}, "sender_session_name": {},
	"sender_task_id": {}, "sender_task_title": {}, "stage": {}, "status": {},
	"task_id": {}, "text": {}, "variant": {}, "workflow_message": {},
	"workflow_step_color": {}, "workflow_step_id": {}, "workflow_step_name": {},
}

// SetConversationJournalDB configures the primary database that owns source
// mutations and their immutable conversation journal rows.
func (s *Service) SetConversationJournalDB(database *sqlx.DB) {
	s.conversationJournal = database
}

func (s *Service) HasConversationJournal() bool {
	return s.conversationJournal != nil
}

func (s *Service) conversationJournalTablesExist(ctx context.Context) (bool, error) {
	return s.conversationTableExists(ctx, "conversation_message_versions")
}

func toAnySlice(values []string) []any {
	out := make([]any, len(values))
	for index, value := range values {
		out[index] = value
	}
	return out
}

func (s *Service) conversationTableExists(ctx context.Context, table string) (bool, error) {
	present, err := db.TableExistsContext(ctx, s.conversationJournal, table)
	if err != nil {
		return false, fmt.Errorf("check conversation journal schema (%s): %w", table, err)
	}
	return present, nil
}

// SyncCommittedSessionEvents mirrors primary-journal rows into the durable
// delivery log without allocating a second identity or sequence.
func (s *Service) SyncCommittedSessionEvents(ctx context.Context, sessionID string) ([]SessionEvent, error) {
	if s.conversationJournal == nil || s.sessionEvents == nil {
		return nil, nil
	}
	watermark := s.sessionEvents.Watermark(sessionID)
	query := s.conversationJournal.Rebind(`
		SELECT session_id, event_id, sequence, protocol_version, event_type, task_id, payload, created_at
		FROM conversation_session_events
		WHERE session_id = ? AND sequence > ?
		ORDER BY sequence ASC`)
	rows, err := s.conversationJournal.QueryxContext(ctx, query, sessionID, watermark)
	if err != nil {
		return nil, fmt.Errorf("query committed conversation events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	mirrored := make([]SessionEvent, 0)
	expected := watermark
	for rows.Next() {
		var event SessionEvent
		var taskID sql.NullString
		var payload string
		if err := rows.Scan(&event.SessionID, &event.ID, &event.Sequence, &event.ProtocolVersion, &event.EventType, &taskID, &payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan committed conversation event: %w", err)
		}
		event.Payload = sanitizeConversationEventPayload(event.EventType, json.RawMessage(payload))
		if taskID.Valid {
			event.TaskID = &taskID.String
		}
		appended, err := s.sessionEvents.AppendCommitted(event)
		if err != nil {
			return nil, fmt.Errorf("mirror committed conversation event: %w", err)
		}
		if !appended {
			continue
		}
		mirrored = append(mirrored, event)
		if event.Sequence > expected+1 && s.log != nil {
			s.log.Warn("journal mirror healed a retention gap",
				zap.String("session_id", sessionID),
				zap.Uint64("expected_sequence", expected+1),
				zap.Uint64("sequence", event.Sequence),
			)
		}
		expected = event.Sequence
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate committed conversation events: %w", err)
	}
	return mirrored, nil
}

// emptyConversationPayload is the durable payload persisted for a
// primary-journal row whose JSON is malformed. It carries no raw bytes yet is
// non-nil, so the BLOB NOT NULL session_events column accepts the write and
// the mirrored row is retained as a poison record instead of failing the
// append (the projection rejects it because no type/identity fields exist).
var emptyConversationPayload = json.RawMessage("{}")

func sanitizeConversationEventPayload(eventType string, raw json.RawMessage) json.RawMessage {
	var source map[string]any
	if err := json.Unmarshal(raw, &source); err != nil {
		return emptyConversationPayload
	}
	payload := map[string]any{conversationTypeKey: eventType}
	copyConversationPayloadFields(payload, source, conversationSessionIDKey, conversationTaskIDKey)
	switch eventType {
	case events.MessageAdded, events.MessageUpdated:
		copyConversationPayloadFields(payload, source,
			conversationMessageIDKey, "turn_id", conversationAuthorTypeKey,
			"created_at", "updated_at", "prompt_index", "requests_input",
			"message_type")
		sanitizeConversationMessagePayload(payload, source)
	case events.MessageDeleted:
		copyConversationPayloadFields(payload, source, conversationMessageIDKey)
	case conversationTurnStarted, conversationTurnCompleted, conversationTurnRemoved:
		copyConversationPayloadFields(payload, source, "id", "started_at", "completed_at", "updated_at", "had_output", "metadata")
	case events.SessionRemoved:
		// The common identity fields above are the complete public removal DTO.
	}
	sanitized, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return sanitized
}

func copyConversationPayloadFields(target, source map[string]any, keys ...string) {
	for _, key := range keys {
		if value, exists := source[key]; exists {
			target[key] = value
		}
	}
}

// SanitizeConversationMessageMetadata returns the presentation metadata that
// the web client needs for a conversation message. Internal agent/tool
// metadata is deliberately excluded from ordered session events.
func SanitizeConversationMessageMetadata(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string]any)
	for key := range conversationMessageMetadataKeys {
		value, exists := source[key]
		if !exists || value == nil {
			continue
		}
		target[key] = sanitizeConversationMetadataValue(value)
	}
	return target
}

func sanitizeConversationMetadataValue(value any) any {
	switch typed := value.(type) {
	case string:
		return sysprompt.StripSystemContent(typed)
	case map[string]any:
		target := make(map[string]any, len(typed))
		for key, nested := range typed {
			target[key] = sanitizeConversationMetadataValue(nested)
		}
		return target
	case []any:
		target := make([]any, len(typed))
		for index, nested := range typed {
			target[index] = sanitizeConversationMetadataValue(nested)
		}
		return target
	default:
		return value
	}
}

func sanitizeConversationMessagePayload(target, source map[string]any) {
	if content, ok := source[conversationContentKey].(string); ok {
		target[conversationContentKey] = sysprompt.StripSystemContent(content)
	}
	metadata, _ := source["metadata"].(map[string]any)
	sanitizedMetadata := SanitizeConversationMessageMetadata(metadata)
	if senderTaskID, ok := source["sender_task_id"].(string); ok && senderTaskID != "" {
		target["sender_task_id"] = senderTaskID
		if sanitizedMetadata == nil {
			sanitizedMetadata = map[string]any{}
		}
		sanitizedMetadata["sender_task_id"] = senderTaskID
	}
	if len(sanitizedMetadata) > 0 {
		target["metadata"] = sanitizedMetadata
		if senderTaskID, ok := sanitizedMetadata["sender_task_id"].(string); ok && senderTaskID != "" {
			target["sender_task_id"] = senderTaskID
		}
	}
}

func (s *Service) syncAllCommittedSessionEvents(ctx context.Context) ([]SessionEvent, error) {
	if s.conversationJournal == nil || s.sessionEvents == nil {
		return nil, nil
	}
	rows, err := s.conversationJournal.QueryxContext(ctx, `SELECT session_id FROM conversation_session_streams ORDER BY session_id`)
	if err != nil {
		return nil, fmt.Errorf("query conversation stream partitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sessionIDs []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, fmt.Errorf("scan conversation stream partition: %w", err)
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation stream partitions: %w", err)
	}
	var mirrored []SessionEvent
	var failures []string
	for _, sessionID := range sessionIDs {
		events, err := s.SyncCommittedSessionEvents(ctx, sessionID)
		if err != nil {
			// Isolate one unhealthy partition: it must not freeze retention
			// for every other session. The combined error is still returned
			// so the failure stays loud (it is only reachable through state
			// tampering, since the maintenance tick prunes both sides).
			failures = append(failures, sessionID+": "+err.Error())
			continue
		}
		mirrored = append(mirrored, events...)
	}
	if len(failures) > 0 {
		return mirrored, fmt.Errorf("mirror %d session partition(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return mirrored, nil
}

func (s *Service) SetSessionEventSink(sink func(SessionEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionEventSink = sink
}

//nolint:cyclop // Retention orchestration: schema gates plus four prune passes over three tables.
func (s *Service) pruneConversationJournal(
	ctx context.Context,
	cutoff time.Time,
	retained map[string]struct{},
) error {
	if s.conversationJournal == nil {
		return nil
	}
	exists, err := s.conversationJournalTablesExist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	purgeable, err := s.deadSessionPurgeEnabled(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(retained))
	for sessionID := range retained {
		ids = append(ids, sessionID)
	}
	sort.Strings(ids)
	exclusion, extraArgs := retainedSessionExclusion(ids)
	args := append([]any{cutoff.UTC()}, extraArgs...)
	tx, err := s.conversationJournal.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin conversation journal retention: %w", err)
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	eventQuery := s.conversationJournal.Rebind(
		`DELETE FROM conversation_session_events
		 WHERE created_at < ? AND event_type <> 'session.removed' AND ` + exclusion,
	)
	if _, err := tx.ExecContext(ctx, eventQuery, args...); err != nil {
		return rollback(fmt.Errorf("prune conversation events: %w", err))
	}
	messageQuery := s.conversationJournal.Rebind(
		`DELETE FROM conversation_message_versions AS current_version
		 WHERE current_version.created_at < ? AND ` + exclusion + `
		   AND EXISTS (
		     SELECT 1 FROM conversation_message_versions AS newer
		     WHERE newer.session_id = current_version.session_id
		       AND newer.message_id = current_version.message_id
		       AND newer.row_sequence > current_version.row_sequence
		   )`,
	)
	if _, err := tx.ExecContext(ctx, messageQuery, args...); err != nil {
		return rollback(fmt.Errorf("prune conversation message versions: %w", err))
	}
	turnQuery := s.conversationJournal.Rebind(
		`DELETE FROM conversation_turn_versions AS current_version
		 WHERE current_version.started_at < ? AND ` + exclusion + `
		   AND EXISTS (
		     SELECT 1 FROM conversation_turn_versions AS newer
		     WHERE newer.session_id = current_version.session_id
		       AND newer.turn_id = current_version.turn_id
		       AND newer.row_sequence > current_version.row_sequence
		   )`,
	)
	if _, err := tx.ExecContext(ctx, turnQuery, args...); err != nil {
		return rollback(fmt.Errorf("prune conversation turn versions: %w", err))
	}
	if purgeable {
		if err := s.purgeDeadSessionPartitions(ctx, tx, exclusion, args); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conversation journal retention: %w", err)
	}
	return nil
}

// deadSessionPurgeEnabled reports whether the retention pass can purge whole
// dead-session partitions: both the journal stream table and the source
// session table must be present. Checked before the retention tx opens so a
// single-connection pool (in-memory test DBs) cannot deadlock on the catalog
// lookups.
func (s *Service) deadSessionPurgeEnabled(ctx context.Context) (bool, error) {
	streams, err := s.conversationTableExists(ctx, "conversation_session_streams")
	if err != nil {
		return false, err
	}
	if !streams {
		return false, nil
	}
	return s.conversationTableExists(ctx, "task_sessions")
}

// retainedSessionExclusion builds the SQL exclusion clause and its bound
// arguments for the retained session ids; with none retained it returns the
// no-op "1 = 1" clause so every row is eligible for age-based pruning.
func retainedSessionExclusion(ids []string) (string, []any) {
	if len(ids) == 0 {
		return "1 = 1", nil
	}
	placeholders := make([]string, len(ids))
	extra := make([]any, len(ids))
	for index, sessionID := range ids {
		placeholders[index] = "?"
		extra[index] = sessionID
	}
	return "session_id NOT IN (" + strings.Join(placeholders, ",") + ")", extra
}

// purgeDeadSessionPartitions deletes every journal row (message and turn
// versions, events, and the stream row) for sessions whose source task_sessions
// row no longer exists and whose partition is older than the retention cutoff
// (args[0]) and not protected by a live cursor/poison (remaining args). The
// dead ids are materialized first because a CTE whose DELETE targets the same
// table the CTE reads can stall SQLite.
//
// The session.removed terminal marker intentionally lives exactly as long as
// the rest of the partition: this pass fires only after updated_at has aged
// past SessionEventRetention, so a subscriber arriving within that window
// still receives session_removed (the durable-log mirror keeps the terminal
// tombstone through the same window and only then drops it). After both
// stores forget the partition, the session is gone for good and later
// snapshot reads correctly 404.
func (s *Service) purgeDeadSessionPartitions(ctx context.Context, tx *sqlx.Tx, exclusion string, args []any) error {
	deadQuery := s.conversationJournal.Rebind(`
		SELECT st.session_id
		FROM conversation_session_streams st
		WHERE st.updated_at < ? AND ` + exclusion + `
		  AND NOT EXISTS (
			SELECT 1 FROM task_sessions ts WHERE ts.id = st.session_id
		  )`)
	var deadIDs []string
	if err := tx.SelectContext(ctx, &deadIDs, deadQuery, args...); err != nil {
		return fmt.Errorf("select dead session journal partitions: %w", err)
	}
	if len(deadIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(deadIDs))
	for index := range deadIDs {
		placeholders[index] = "?"
	}
	inClause := strings.Join(placeholders, ",")
	for _, child := range []string{"conversation_message_versions", "conversation_turn_versions", "conversation_session_events"} {
		query := s.conversationJournal.Rebind(
			`DELETE FROM ` + child + ` WHERE session_id IN (` + inClause + `)`)
		if _, err := tx.ExecContext(ctx, query, toAnySlice(deadIDs)...); err != nil {
			return fmt.Errorf("purge dead session journal rows (%s): %w", child, err)
		}
	}
	streamQuery := s.conversationJournal.Rebind(
		`DELETE FROM conversation_session_streams WHERE session_id IN (` + inClause + `)`)
	if _, err := tx.ExecContext(ctx, streamQuery, toAnySlice(deadIDs)...); err != nil {
		return fmt.Errorf("purge dead session stream rows: %w", err)
	}
	return nil
}

type journalMessagePayload struct {
	MessageID     string         `json:"message_id"`
	TurnID        string         `json:"turn_id"`
	TaskID        string         `json:"task_id"`
	AuthorType    string         `json:"author_type"`
	Content       string         `json:"content"`
	MessageType   string         `json:"message_type"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	PromptIndex   int            `json:"prompt_index"`
	RequestsInput any            `json:"requests_input"`
	SenderTaskID  string         `json:"sender_task_id"`
	Metadata      map[string]any `json:"metadata"`
}

//nolint:goconst // SQL projection values are protocol literals.
func (s *Service) conversationMessagesAt(
	ctx context.Context,
	sessionID string,
	cutoff uint64,
	taskID *string,
	authors []string,
	sortOrder string,
	cursorID string,
	limit int,
) ([]*taskmodels.Message, bool, error) {
	if s.conversationJournal == nil {
		return nil, false, nil
	}
	args := []any{sessionID, cutoff}
	where := []string{"version_rank = 1", "tombstone = FALSE"}
	if taskID != nil {
		where = append(where, "task_id = ?")
		args = append(args, *taskID)
	}
	if len(authors) > 0 {
		placeholders := make([]string, len(authors))
		for index, author := range authors {
			placeholders[index] = "?"
			args = append(args, author)
		}
		where = append(where, "author_type IN ("+strings.Join(placeholders, ",")+")")
	}
	orderDirection := "ASC"
	comparison := ">"
	if sortOrder == "desc" {
		orderDirection = "DESC"
		comparison = "<"
	}
	if cursorID != "" {
		// A cursor whose message was deleted/tombstoned mid-walk is gone from
		// the live projection; the cursor predicate would otherwise match
		// nothing and the client would read a silent empty terminal page.
		// Surface an error (parity with the source-table fallback) instead.
		live, err := s.liveConversationMessageAtCutoff(ctx, sessionID, cutoff, cursorID)
		if err != nil {
			return nil, false, err
		}
		if !live {
			return nil, false, fmt.Errorf("%w: %s", errConversationCursorGone, cursorID)
		}
		where = append(where, fmt.Sprintf("(created_at %s (SELECT created_at FROM live WHERE message_id = ?) OR (created_at = (SELECT created_at FROM live WHERE message_id = ?) AND message_id %s ?))", comparison, comparison))
		args = append(args, cursorID, cursorID, cursorID)
	}
	args = append(args, limit+1)
	query := fmt.Sprintf(`
		WITH ranked AS (
			SELECT message_id, task_id, author_type, created_at, tombstone, payload,
				ROW_NUMBER() OVER (PARTITION BY message_id ORDER BY row_sequence DESC) AS version_rank
			FROM conversation_message_versions
			WHERE session_id = ? AND row_sequence <= ?
		), live AS (
			SELECT * FROM ranked WHERE version_rank = 1 AND tombstone = FALSE
		)
		SELECT payload FROM live
		WHERE %s
		ORDER BY created_at %s, message_id %s
		LIMIT ?`, strings.Join(where, " AND "), orderDirection, orderDirection)
	query = s.conversationJournal.Rebind(query)
	rows, err := s.conversationJournal.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("query conversation message snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return decodeConversationSnapshotRows(rows, sessionID, limit)
}

type conversationRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// decodeConversationSnapshotRows scans message-version payload rows into
// messages, applying the limit+1 page bound, and reports whether another page
// remains.
func decodeConversationSnapshotRows(rows conversationRowScanner, sessionID string, limit int) ([]*taskmodels.Message, bool, error) {
	messages := make([]*taskmodels.Message, 0, limit+1)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, false, fmt.Errorf("scan conversation message snapshot: %w", err)
		}
		message, err := decodeJournalMessage(raw, sessionID)
		if err != nil {
			return nil, false, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate conversation message snapshot: %w", err)
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	return messages, hasMore, nil
}

// liveConversationMessageAtCutoff reports whether the message cursor still has
// a live (non-tombstoned) version at or before the snapshot cutoff.
func (s *Service) liveConversationMessageAtCutoff(ctx context.Context, sessionID string, cutoff uint64, cursorID string) (bool, error) {
	query := s.conversationJournal.Rebind(`
		WITH ranked AS (
			SELECT message_id, tombstone,
				ROW_NUMBER() OVER (PARTITION BY message_id ORDER BY row_sequence DESC) AS version_rank
			FROM conversation_message_versions
			WHERE session_id = ? AND row_sequence <= ?
		)
		SELECT COUNT(*) FROM ranked WHERE message_id = ? AND version_rank = 1 AND tombstone = FALSE`)
	var live int
	if err := s.conversationJournal.GetContext(ctx, &live, query, sessionID, cutoff, cursorID); err != nil {
		return false, fmt.Errorf("resolve conversation page cursor: %w", err)
	}
	return live > 0, nil
}

func conversationRequestsInput(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	default:
		return false
	}
}

func decodeJournalMessage(raw []byte, sessionID string) (*taskmodels.Message, error) {
	var payload journalMessagePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode conversation message version: %w", err)
	}
	createdAt, err := parseJournalTime(payload.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("decode conversation message created_at: %w", err)
	}
	updatedAt, err := parseJournalTime(payload.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("decode conversation message updated_at: %w", err)
	}
	metadata := payload.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if payload.SenderTaskID != "" {
		metadata["sender_task_id"] = payload.SenderTaskID
	}
	return &taskmodels.Message{
		ID: payload.MessageID, TaskSessionID: sessionID, TurnID: payload.TurnID, TaskID: payload.TaskID,
		AuthorType: taskmodels.MessageAuthorType(payload.AuthorType),
		Content:    payload.Content, Type: taskmodels.MessageType(payload.MessageType),
		RequestsInput: conversationRequestsInput(payload.RequestsInput),
		CreatedAt:     createdAt, UpdatedAt: updatedAt, PromptIndex: payload.PromptIndex, Metadata: metadata,
	}, nil
}

type journalTurnPayload struct {
	TurnID      string         `json:"id"`
	TaskID      string         `json:"task_id"`
	StartedAt   string         `json:"started_at"`
	CompletedAt *string        `json:"completed_at"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
	Metadata    map[string]any `json:"metadata"`
}

func (s *Service) conversationTurnsAt(ctx context.Context, sessionID string, cutoff uint64, taskID *string) ([]*taskmodels.Turn, error) {
	if s.conversationJournal == nil {
		return nil, nil
	}
	args := []any{sessionID, cutoff}
	where := "version_rank = 1 AND tombstone = FALSE"
	if taskID != nil {
		where += " AND task_id = ?"
		args = append(args, *taskID)
	}
	query := fmt.Sprintf(`
		WITH ranked AS (
			SELECT turn_id, task_id, started_at, tombstone, payload,
				ROW_NUMBER() OVER (PARTITION BY turn_id ORDER BY row_sequence DESC) AS version_rank
			FROM conversation_turn_versions
			WHERE session_id = ? AND row_sequence <= ?
		)
		SELECT payload FROM ranked WHERE %s ORDER BY started_at ASC, turn_id ASC`, where)
	query = s.conversationJournal.Rebind(query)
	rows, err := s.conversationJournal.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query conversation turn snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var turns []*taskmodels.Turn
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan conversation turn snapshot: %w", err)
		}
		var payload journalTurnPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("decode conversation turn version: %w", err)
		}
		startedAt, err := parseJournalTime(payload.StartedAt)
		if err != nil {
			return nil, fmt.Errorf("decode conversation turn started_at: %w", err)
		}
		createdAt, err := parseJournalTime(payload.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("decode conversation turn created_at: %w", err)
		}
		updatedAt, err := parseJournalTime(payload.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("decode conversation turn updated_at: %w", err)
		}
		metadata := payload.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		turn := &taskmodels.Turn{
			ID: payload.TurnID, TaskSessionID: sessionID, TaskID: payload.TaskID,
			StartedAt: startedAt, Metadata: metadata, CreatedAt: createdAt, UpdatedAt: updatedAt,
		}
		if payload.CompletedAt != nil {
			completedAt, err := parseJournalTime(*payload.CompletedAt)
			if err != nil {
				return nil, fmt.Errorf("decode conversation turn completed_at: %w", err)
			}
			turn.CompletedAt = &completedAt
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation turn snapshot: %w", err)
	}
	return turns, nil
}

func parseJournalTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05Z07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}
