//nolint:revive // This file keeps the dialect-specific journal trigger lifecycle atomic.
package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/sysprompt"
)

const conversationJournalTables = `
CREATE TABLE IF NOT EXISTS conversation_session_streams (
	session_id TEXT PRIMARY KEY,
	watermark BIGINT NOT NULL DEFAULT 0,
	terminal BOOLEAN NOT NULL DEFAULT FALSE,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS conversation_session_events (
	session_id TEXT NOT NULL,
	sequence BIGINT NOT NULL,
	event_id TEXT NOT NULL UNIQUE,
	protocol_version INTEGER NOT NULL DEFAULT 1,
	event_type TEXT NOT NULL,
	task_id TEXT,
	payload TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	PRIMARY KEY (session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_events_created
	ON conversation_session_events(created_at);
CREATE TABLE IF NOT EXISTS conversation_message_versions (
	session_id TEXT NOT NULL,
	message_id TEXT NOT NULL,
	row_sequence BIGINT NOT NULL,
	task_id TEXT,
	author_type TEXT,
	created_at TIMESTAMP NOT NULL,
	tombstone BOOLEAN NOT NULL DEFAULT FALSE,
	payload TEXT NOT NULL,
	PRIMARY KEY (session_id, message_id, row_sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_message_snapshot
	ON conversation_message_versions(session_id, row_sequence, created_at, message_id);
CREATE TABLE IF NOT EXISTS conversation_turn_versions (
	session_id TEXT NOT NULL,
	turn_id TEXT NOT NULL,
	row_sequence BIGINT NOT NULL,
	task_id TEXT,
	started_at TIMESTAMP NOT NULL,
	tombstone BOOLEAN NOT NULL DEFAULT FALSE,
	payload TEXT NOT NULL,
	PRIMARY KEY (session_id, turn_id, row_sequence)
);
CREATE INDEX IF NOT EXISTS idx_conversation_turn_snapshot
	ON conversation_turn_versions(session_id, row_sequence, started_at, turn_id);
CREATE TABLE IF NOT EXISTS conversation_journal_meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// journalSchemaVersion is the current conversation journal schema version.
// Boot runs the trigger (re)creation plus its one-time sanitize migration only
// while the stored version is below this constant, then records it, so the
// per-boot full-corpus payload rewrite never repeats.
const journalSchemaVersion = 3

// journalBackfillKey records that the one-time backfill of pre-trigger source
// rows completed, so boot does not re-scan the whole message/turn corpus.
const (
	// Repeated payload/column keys kept as constants for goconst.
	jKeySessionID = "session_id"
	jKeyTaskID    = "task_id"
)

const journalBackfillKey = "backfill.complete"

func (r *Repository) initConversationJournalSchema() error {
	if _, err := r.db.Exec(conversationJournalTables); err != nil {
		return fmt.Errorf("create conversation journal tables: %w", err)
	}
	version, err := r.readConversationJournalMetaInt("schema.version")
	if err != nil {
		return err
	}
	if version < journalSchemaVersion {
		var triggerErr error
		if dialect.IsPostgres(r.db.DriverName()) {
			triggerErr = r.initPostgresConversationJournalTriggers()
		} else {
			triggerErr = r.initSQLiteConversationJournalTriggers()
		}
		if triggerErr != nil {
			return triggerErr
		}
		if err := r.writeConversationJournalMeta("schema.version", journalSchemaVersion); err != nil {
			return err
		}
	}
	backfilled, err := r.readConversationJournalMetaFlag(journalBackfillKey)
	if err != nil {
		return err
	}
	if backfilled {
		return nil
	}
	if err := r.backfillConversationJournal(); err != nil {
		return err
	}
	return r.writeConversationJournalMeta(journalBackfillKey, 1)
}

func (r *Repository) readConversationJournalMetaInt(key string) (int, error) {
	var value string
	err := r.db.Get(&value, r.db.Rebind(`SELECT value FROM conversation_journal_meta WHERE key = ?`), key)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read conversation journal meta %s: %w", key, err)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse conversation journal meta %s: %w", key, err)
	}
	return parsed, nil
}

func (r *Repository) readConversationJournalMetaFlag(key string) (bool, error) {
	value, err := r.readConversationJournalMetaInt(key)
	if err != nil {
		return false, err
	}
	return value > 0, nil
}

func (r *Repository) writeConversationJournalMeta(key string, value int) error {
	_, err := r.db.Exec(r.db.Rebind(`
		INSERT INTO conversation_journal_meta(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`), key, strconv.Itoa(value))
	if err != nil {
		return fmt.Errorf("write conversation journal meta %s: %w", key, err)
	}
	return nil
}

type conversationJournalMessageSeed struct {
	ID            string    `db:"id"`
	TaskSessionID string    `db:"task_session_id"`
	TaskID        string    `db:"task_id"`
	TurnID        string    `db:"turn_id"`
	AuthorType    string    `db:"author_type"`
	AuthorID      string    `db:"author_id"`
	Content       string    `db:"content"`
	RequestsInput bool      `db:"requests_input"`
	MessageType   string    `db:"message_type"`
	Metadata      string    `db:"metadata"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
	PromptIndex   int       `db:"prompt_index"`
}

type conversationJournalTurnSeed struct {
	ID                 string     `db:"id"`
	TaskSessionID      string     `db:"task_session_id"`
	TaskID             string     `db:"task_id"`
	ExecutionProfileID string     `db:"execution_profile_id"`
	RouteGeneration    int64      `db:"route_generation"`
	Metadata           string     `db:"metadata"`
	StartedAt          time.Time  `db:"started_at"`
	CompletedAt        *time.Time `db:"completed_at"`
	CreatedAt          time.Time  `db:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at"`
}

const conversationJournalBackfillBatchSize = 100

//nolint:cyclop,dupl,gocognit,funlen // Turn and message backfills intentionally share bounded keyset control flow.
func (r *Repository) backfillConversationJournal() error {
	var lastSessionID string
	var lastStartedAt time.Time
	var lastTurnID string
	//nolint:dupl // Turn and message batches use the same transaction boundaries.
	for {
		tx, err := r.db.Beginx()
		if err != nil {
			return fmt.Errorf("begin conversation turn journal backfill: %w", err)
		}
		var turns []conversationJournalTurnSeed
		query := `
			SELECT t.id, t.task_session_id, t.task_id, t.execution_profile_id, t.route_generation,
				t.metadata, t.started_at, t.completed_at, t.created_at, t.updated_at
			FROM task_session_turns t
			WHERE NOT EXISTS (
				SELECT 1 FROM conversation_turn_versions v
				WHERE v.session_id = t.task_session_id AND v.turn_id = t.id
			)`
		args := []any{}
		if lastTurnID != "" {
			query += `
				AND (t.task_session_id > ? OR
					(t.task_session_id = ? AND
						(t.started_at > ? OR (t.started_at = ? AND t.id > ?))))`
			args = append(args, lastSessionID, lastSessionID, lastStartedAt, lastStartedAt, lastTurnID)
		}
		query += ` ORDER BY t.task_session_id, t.started_at, t.id LIMIT ?`
		args = append(args, conversationJournalBackfillBatchSize)
		if err := tx.Select(&turns, r.db.Rebind(query), args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("list conversation turn backfill: %w", err)
		}
		if len(turns) == 0 {
			_ = tx.Rollback()
			break
		}
		for _, turn := range turns {
			if err := r.backfillConversationTurn(tx, turn); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit conversation turn journal backfill: %w", err)
		}
		lastTurn := turns[len(turns)-1]
		lastSessionID, lastStartedAt, lastTurnID = lastTurn.TaskSessionID, lastTurn.StartedAt, lastTurn.ID
	}

	lastSessionID = ""
	var lastCreatedAt time.Time
	var lastMessageID string
	for {
		tx, err := r.db.Beginx()
		if err != nil {
			return fmt.Errorf("begin conversation message journal backfill: %w", err)
		}
		var messages []conversationJournalMessageSeed
		query := `
			SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
				m.content, m.requests_input, m.type AS message_type, m.metadata,
				m.created_at, m.updated_at, m.prompt_seq AS prompt_index
			FROM task_session_messages m
			WHERE NOT EXISTS (
				SELECT 1 FROM conversation_message_versions v
				WHERE v.session_id = m.task_session_id AND v.message_id = m.id
			)`
		args := []any{}
		if lastMessageID != "" {
			query += `
				AND (m.task_session_id > ? OR
					(m.task_session_id = ? AND
						(m.created_at > ? OR (m.created_at = ? AND m.id > ?))))`
			args = append(args, lastSessionID, lastSessionID, lastCreatedAt, lastCreatedAt, lastMessageID)
		}
		query += ` ORDER BY m.task_session_id, m.created_at, m.id LIMIT ?`
		args = append(args, conversationJournalBackfillBatchSize)
		if err := tx.Select(&messages, r.db.Rebind(query), args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("list conversation message backfill: %w", err)
		}
		if len(messages) == 0 {
			_ = tx.Rollback()
			break
		}
		for _, message := range messages {
			if err := r.backfillConversationMessage(tx, message); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit conversation message journal backfill: %w", err)
		}
		lastMessage := messages[len(messages)-1]
		lastSessionID, lastCreatedAt, lastMessageID = lastMessage.TaskSessionID, lastMessage.CreatedAt, lastMessage.ID
	}
	return nil
}

func (r *Repository) nextConversationJournalSequence(tx *sqlx.Tx, sessionID string) (uint64, error) {
	var sequence uint64
	query := r.db.Rebind(`UPDATE conversation_session_streams
		SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP
		WHERE session_id = ? RETURNING watermark`)
	err := tx.Get(&sequence, query, sessionID)
	if err == nil {
		return sequence, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("advance conversation journal sequence: %w", err)
	}
	query = r.db.Rebind(`INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
		VALUES (?, 1, FALSE, CURRENT_TIMESTAMP)`)
	if _, err := tx.Exec(query, sessionID); err != nil {
		return 0, fmt.Errorf("initialize conversation journal sequence: %w", err)
	}
	return 1, nil
}

// sameTurnSourceImage reports whether the freshly read source row still
// matches the backfill seed for every payload-bound field.
//
//nolint:goconst // Backfill payload values mirror the public event contract.
func sameTurnSourceImage(current struct {
	TaskID             string     `db:"task_id"`
	ExecutionProfileID string     `db:"execution_profile_id"`
	RouteGeneration    int64      `db:"route_generation"`
	Metadata           string     `db:"metadata"`
	StartedAt          time.Time  `db:"started_at"`
	CompletedAt        *time.Time `db:"completed_at"`
	CreatedAt          time.Time  `db:"created_at"`
	UpdatedAt          time.Time  `db:"updated_at"`
}, turn conversationJournalTurnSeed) bool {
	completedAtEqual := (current.CompletedAt == nil) == (turn.CompletedAt == nil)
	if current.CompletedAt != nil && turn.CompletedAt != nil && !current.CompletedAt.Equal(*turn.CompletedAt) {
		return false
	}
	return completedAtEqual &&
		current.TaskID == turn.TaskID &&
		current.ExecutionProfileID == turn.ExecutionProfileID &&
		current.RouteGeneration == turn.RouteGeneration &&
		current.Metadata == turn.Metadata &&
		current.StartedAt.Equal(turn.StartedAt) &&
		current.CreatedAt.Equal(turn.CreatedAt) &&
		current.UpdatedAt.Equal(turn.UpdatedAt)
}

func (r *Repository) backfillConversationTurn(tx *sqlx.Tx, turn conversationJournalTurnSeed) error {
	// Optimistic re-check of the complete source image: if any payload-bound
	// field changed since the backfill SELECT, its trigger already journaled
	// the current image, and a stale pre-update image must not win the
	// snapshot with a later sequence. Comparing completion state alone would
	// let a same-state update (metadata, profile, timestamps) pass.
	var current struct {
		TaskID             string     `db:"task_id"`
		ExecutionProfileID string     `db:"execution_profile_id"`
		RouteGeneration    int64      `db:"route_generation"`
		Metadata           string     `db:"metadata"`
		StartedAt          time.Time  `db:"started_at"`
		CompletedAt        *time.Time `db:"completed_at"`
		CreatedAt          time.Time  `db:"created_at"`
		UpdatedAt          time.Time  `db:"updated_at"`
	}
	err := tx.Get(&current, r.db.Rebind(`
		SELECT task_id, execution_profile_id, route_generation, metadata,
			started_at, completed_at, created_at, updated_at
		FROM task_session_turns WHERE id = ?`), turn.ID)
	if err == sql.ErrNoRows {
		return nil // deleted concurrently; its history is gone
	}
	if err != nil {
		return fmt.Errorf("re-check conversation turn backfill %s: %w", turn.ID, err)
	}
	if !sameTurnSourceImage(current, turn) {
		return nil // source image changed concurrently; trigger journaled it
	}
	sequence, err := r.nextConversationJournalSequence(tx, turn.TaskSessionID)
	if err != nil {
		return err
	}
	eventType := "session.turn.started"
	if turn.CompletedAt != nil {
		eventType = "session.turn.completed"
	}
	payload, err := json.Marshal(map[string]any{
		"type": eventType, jKeySessionID: turn.TaskSessionID, jKeyTaskID: journalTaskID(turn.TaskID),
		"id": turn.ID, "execution_profile_id": turn.ExecutionProfileID, "route_generation": turn.RouteGeneration,
		"started_at": turn.StartedAt.UTC().Format(time.RFC3339Nano), "completed_at": journalTime(turn.CompletedAt),
		"metadata_json": turn.Metadata, columnCreatedAt: turn.CreatedAt.UTC().Format(time.RFC3339Nano),
		columnUpdatedAt: turn.UpdatedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("encode conversation turn backfill: %w", err)
	}
	query := r.db.Rebind(`INSERT INTO conversation_turn_versions(
		session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload
	) VALUES (?, ?, ?, ?, ?, FALSE, ?)`)
	if _, err := tx.Exec(query, turn.TaskSessionID, turn.ID, sequence, journalTaskID(turn.TaskID), turn.StartedAt, payload); err != nil {
		return fmt.Errorf("insert conversation turn backfill version: %w", err)
	}
	return r.insertConversationBackfillEvent(tx, turn.TaskSessionID, turn.TaskID, sequence, eventType, payload)
}

//nolint:goconst // Backfill payload values mirror the public event contract.
func (r *Repository) backfillConversationMessage(tx *sqlx.Tx, message conversationJournalMessageSeed) error {
	// Optimistic re-check of the complete source image: if any payload-bound
	// field changed since the backfill SELECT, its trigger already journaled
	// the current image, and a stale pre-update image must not win the
	// snapshot with a later sequence. Comparing content alone would let a
	// same-content update (metadata, task/turn/type/author, timestamps) pass.
	var current struct {
		TaskID     string    `db:"task_id"`
		TurnID     string    `db:"turn_id"`
		AuthorType string    `db:"author_type"`
		Content    string    `db:"content"`
		Type       string    `db:"type"`
		Metadata   string    `db:"metadata"`
		CreatedAt  time.Time `db:"created_at"`
		UpdatedAt  time.Time `db:"updated_at"`
	}
	err := tx.Get(&current, r.db.Rebind(`
		SELECT task_id, turn_id, author_type, content, type, metadata, created_at, updated_at
		FROM task_session_messages WHERE id = ?`), message.ID)
	if err == sql.ErrNoRows {
		return nil // deleted concurrently; its tombstone was journaled
	}
	if err != nil {
		return fmt.Errorf("re-check conversation message backfill %s: %w", message.ID, err)
	}
	if current.TaskID != message.TaskID ||
		current.TurnID != message.TurnID ||
		current.AuthorType != message.AuthorType ||
		current.Content != message.Content ||
		current.Type != message.MessageType ||
		current.Metadata != message.Metadata ||
		!current.CreatedAt.Equal(message.CreatedAt) ||
		!current.UpdatedAt.Equal(message.UpdatedAt) {
		return nil // source image changed concurrently; trigger journaled it
	}
	sequence, err := r.nextConversationJournalSequence(tx, message.TaskSessionID)
	if err != nil {
		return err
	}
	var metadata map[string]any
	_ = json.Unmarshal([]byte(message.Metadata), &metadata)
	senderTaskID, _ := metadata["sender_task_id"].(string)
	payload, err := json.Marshal(map[string]any{
		"type": "message.added", jKeySessionID: message.TaskSessionID, jKeyTaskID: journalTaskID(message.TaskID),
		"message_id": message.ID, "turn_id": message.TurnID, "author_type": message.AuthorType,
		"content":      sysprompt.StripSystemContent(message.Content),
		"message_type": message.MessageType, columnCreatedAt: message.CreatedAt.UTC().Format(time.RFC3339Nano),
		columnUpdatedAt: message.UpdatedAt.UTC().Format(time.RFC3339Nano), "prompt_index": message.PromptIndex,
		"sender_task_id": senderTaskID,
	})
	if err != nil {
		return fmt.Errorf("encode conversation message backfill: %w", err)
	}
	query := r.db.Rebind(`INSERT INTO conversation_message_versions(
		session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload
	) VALUES (?, ?, ?, ?, ?, ?, FALSE, ?)`)
	if _, err := tx.Exec(query, message.TaskSessionID, message.ID, sequence, journalTaskID(message.TaskID), message.AuthorType, message.CreatedAt, payload); err != nil {
		return fmt.Errorf("insert conversation message backfill version: %w", err)
	}
	return r.insertConversationBackfillEvent(tx, message.TaskSessionID, message.TaskID, sequence, "message.added", payload)
}

func (r *Repository) insertConversationBackfillEvent(
	tx *sqlx.Tx,
	sessionID string,
	taskID string,
	sequence uint64,
	eventType string,
	payload []byte,
) error {
	query := r.db.Rebind(`INSERT INTO conversation_session_events(
		session_id, sequence, event_id, protocol_version, event_type, task_id, payload, created_at
	) VALUES (?, ?, ?, 1, ?, ?, ?, CURRENT_TIMESTAMP)`)
	if _, err := tx.Exec(query, sessionID, sequence, sessionID+":"+strconv.FormatUint(sequence, 10), eventType, journalTaskID(taskID), payload); err != nil {
		return fmt.Errorf("insert conversation journal backfill event: %w", err)
	}
	return nil
}

// sqliteStripSystemContentExpr returns a scalar subquery that removes every
// well-formed <kandev-system>...</kandev-system> block from expr - including
// multi-line blocks, multiple blocks, and nested evasion - and trims the
// result, mirroring sysprompt.StripSystemContent. SQLite has no scalar regex
// replace, so a bounded recursive CTE reapplies first-block removal until no
// well-formed block remains. Depth 64 caps adversarial input. If another full
// block remains at that bound, the expression keeps only the prefix before it;
// dropping the unexamined suffix fails closed rather than persisting system
// content. The final row is always non-NULL for text input.
func sqliteStripSystemContentExpr(expr string) string {
	// Go's helper also consumes the whitespace run after each closing tag
	// (`\s*` in the pattern); ltrim the tail so junction spacing matches.
	whitespace := "char(32) || char(9) || char(10) || char(13)"
	return `(SELECT trim(x) FROM (
		WITH RECURSIVE strip(x, depth) AS (
			SELECT ` + expr + `, 0
			UNION ALL
			SELECT substr(x, 1, instr(x, '<kandev-system>') - 1)
				|| ltrim(substr(x,
					instr(x, '<kandev-system>') + length('<kandev-system>')
						+ instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') - 1
						+ length('</kandev-system>')),
					` + whitespace + `), depth + 1
			FROM strip
			WHERE depth < 64
				AND instr(x, '<kandev-system>') > 0
				AND instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') > 0
		)
		SELECT CASE WHEN depth = 64 AND instr(x, '<kandev-system>') > 0 AND instr(substr(x, instr(x, '<kandev-system>') + length('<kandev-system>')), '</kandev-system>') > 0 THEN substr(x, 1, instr(x, '<kandev-system>') - 1) ELSE x END AS x FROM strip ORDER BY depth DESC LIMIT 1
	))`
}

// sqliteStripSystemToken marks where a trigger or migration must insert the
// strip expression; the raw SQL literal keeps strftime %-formats untouched.
const sqliteStripSystemToken = "__SQLITE_STRIP_SYSTEM__"
const sqliteConversationMetadataToken = "__SQLITE_CONVERSATION_METADATA__"
const postgresConversationMetadataToken = "__POSTGRES_CONVERSATION_METADATA__"

//nolint:goconst // These keys are an explicit privacy allowlist.
var conversationMessageMetadataKeys = []string{
	"action_visibility", "actions", "agent_disconnected", "attempt", "attachments",
	"auth_methods", "auto_start", "base_branch", "context", "context_files",
	"decision_id", "effective_model", "entity_references", "error_output",
	"failure_code", "failure_details", "failure_kind", "fallback_model",
	"has_hidden_prompts", "has_resume_token", "has_review_comments", "is_auth_error",
	"kind", "max_attempts", "message", "missing_branch", "model_id", "new_branch",
	"original_branch", "pending_id", "plan_mode", "progress", "provider_name",
	"question", "question_id", "question_index", "question_total", "recovery_actions",
	"remediation", "remediation_url", "requested_model", "response", "reset_at",
	"retry_at", "retry_in_seconds", "retrying", "sender_session_id",
	"sender_session_name", "sender_task_id", "sender_task_title", "stage", "status",
	"task_id", "text", "variant", "workflow_message", "workflow_step_color",
	"workflow_step_id", "workflow_step_name",
}

func sqliteConversationMetadataExpr(metadataExpr string) string {
	arguments := make([]string, 0, len(conversationMessageMetadataKeys)*2)
	for _, key := range conversationMessageMetadataKeys {
		arguments = append(arguments, "'"+key+"'", "json_extract("+metadataExpr+", '$."+key+"')")
	}
	return "CASE WHEN json_valid(" + metadataExpr + ") THEN json_object(" + strings.Join(arguments, ",") + ") ELSE json_object() END"
}

func postgresConversationMetadataExpr(metadataExpr string) string {
	source := "conversation_safe_jsonb(" + metadataExpr + ")"
	arguments := make([]string, 0, len(conversationMessageMetadataKeys)*2)
	for _, key := range conversationMessageMetadataKeys {
		arguments = append(arguments, "'"+key+"'", source+" -> '"+key+"'")
	}
	return "jsonb_strip_nulls(jsonb_build_object(" + strings.Join(arguments, ",") + "))"
}

func journalTaskID(taskID string) any {
	if taskID == "" {
		return nil
	}
	return taskID
}
func journalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

//nolint:funlen // Trigger definitions are kept together so schema initialization is atomic.
func (r *Repository) initSQLiteConversationJournalTriggers() error {
	triggerSQL := strings.ReplaceAll(`
DROP TRIGGER IF EXISTS conversation_message_insert;
CREATE TRIGGER IF NOT EXISTS conversation_message_insert
AFTER INSERT ON task_session_messages
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_message_versions(session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.author_type, NEW.created_at, FALSE,
		json_object('type','message.added','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'message_id',NEW.id,'turn_id',NULLIF(NEW.turn_id,''),'author_type',NEW.author_type,
			'content',__SQLITE_STRIP_SYSTEM__,'message_type',NEW.type,'requests_input',NEW.requests_input,
			'created_at',__SQLITE_RFC3339_MILLIS__(NEW.created_at),
		'updated_at',__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.created_at)),'prompt_index',NEW.prompt_seq,
			'metadata',__SQLITE_CONVERSATION_METADATA__(NEW.metadata),
			'sender_task_id',CASE WHEN json_valid(NEW.metadata) AND typeof(json_extract(NEW.metadata,'$.sender_task_id')) = 'text' THEN json_extract(NEW.metadata,'$.sender_task_id') END)
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'message.added', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_message_versions
		ON conversation_message_versions.session_id = conversation_session_streams.session_id
		AND conversation_message_versions.message_id = NEW.id
		AND conversation_message_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;

DROP TRIGGER IF EXISTS conversation_message_update;
CREATE TRIGGER IF NOT EXISTS conversation_message_update
AFTER UPDATE ON task_session_messages
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_message_versions(session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.author_type, NEW.created_at, FALSE,
		json_object('type','message.updated','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'message_id',NEW.id,'turn_id',NULLIF(NEW.turn_id,''),'author_type',NEW.author_type,
			'content',__SQLITE_STRIP_SYSTEM__,'message_type',NEW.type,'requests_input',NEW.requests_input,
			'created_at',__SQLITE_RFC3339_MILLIS__(NEW.created_at),
			'updated_at',__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.created_at)),'prompt_index',NEW.prompt_seq,
			'metadata',__SQLITE_CONVERSATION_METADATA__(NEW.metadata),
			'sender_task_id',CASE WHEN json_valid(NEW.metadata) AND typeof(json_extract(NEW.metadata,'$.sender_task_id')) = 'text' THEN json_extract(NEW.metadata,'$.sender_task_id') END)
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'message.updated', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_message_versions
		ON conversation_message_versions.session_id = conversation_session_streams.session_id
		AND conversation_message_versions.message_id = NEW.id
		AND conversation_message_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;
DROP TRIGGER IF EXISTS conversation_message_delete;

CREATE TRIGGER IF NOT EXISTS conversation_message_delete
BEFORE DELETE ON task_session_messages
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (OLD.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_message_versions(session_id, message_id, row_sequence, task_id, author_type, created_at, tombstone, payload)
	SELECT OLD.task_session_id, OLD.id, watermark, NULLIF(OLD.task_id, ''), OLD.author_type, OLD.created_at, TRUE,
		json_object('type','message.deleted','session_id',OLD.task_session_id,'task_id',NULLIF(OLD.task_id,''),'message_id',OLD.id)
	FROM conversation_session_streams WHERE session_id = OLD.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT OLD.task_session_id, watermark, OLD.task_session_id || ':' || watermark, 'message.deleted', NULLIF(OLD.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_message_versions
		ON conversation_message_versions.session_id = conversation_session_streams.session_id
		AND conversation_message_versions.message_id = OLD.id
		AND conversation_message_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = OLD.task_session_id;
END;
DROP TRIGGER IF EXISTS conversation_turn_insert;

CREATE TRIGGER IF NOT EXISTS conversation_turn_insert
AFTER INSERT ON task_session_turns
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_turn_versions(session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.started_at, FALSE,
		json_object('type','session.turn.started','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'id',NEW.id,'started_at',__SQLITE_RFC3339_MILLIS__(NEW.started_at),
			'completed_at',__SQLITE_RFC3339_MILLIS__(NEW.completed_at),
			'created_at',__SQLITE_RFC3339_MILLIS__(NEW.created_at),
			'updated_at',__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.started_at)))
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'session.turn.started', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_turn_versions
		ON conversation_turn_versions.session_id = conversation_session_streams.session_id
		AND conversation_turn_versions.turn_id = NEW.id
		AND conversation_turn_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;
DROP TRIGGER IF EXISTS conversation_turn_complete;

CREATE TRIGGER IF NOT EXISTS conversation_turn_complete
AFTER UPDATE OF completed_at ON task_session_turns
WHEN OLD.completed_at IS NULL AND NEW.completed_at IS NOT NULL
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (NEW.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_turn_versions(session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload)
	SELECT NEW.task_session_id, NEW.id, watermark, NULLIF(NEW.task_id, ''), NEW.started_at, FALSE,
		json_object('type','session.turn.completed','session_id',NEW.task_session_id,'task_id',NULLIF(NEW.task_id,''),
			'id',NEW.id,'started_at',__SQLITE_RFC3339_MILLIS__(NEW.started_at),
			'completed_at',__SQLITE_RFC3339_MILLIS__(NEW.completed_at),
			'created_at',__SQLITE_RFC3339_MILLIS__(NEW.created_at),
			'updated_at',__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.completed_at,NEW.started_at)),
			'metadata',CASE WHEN json_valid(NEW.metadata) THEN json(NEW.metadata) ELSE json('{}') END,
			'had_output',json(CASE WHEN EXISTS (
				SELECT 1 FROM task_session_messages output
				WHERE output.turn_id = NEW.id AND output.author_type = 'agent'
				AND (
					output.type IN ('tool_call','tool_edit','tool_read','tool_search','tool_execute',
						'agent_plan','todo','permission_request','clarification_request')
					OR (output.type IN ('message','content','') AND trim(output.content) <> '')
				)) THEN 'true' ELSE 'false' END))
	FROM conversation_session_streams WHERE session_id = NEW.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT NEW.task_session_id, watermark, NEW.task_session_id || ':' || watermark, 'session.turn.completed', NULLIF(NEW.task_id,''), payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_turn_versions
		ON conversation_turn_versions.session_id = conversation_session_streams.session_id
		AND conversation_turn_versions.turn_id = NEW.id
		AND conversation_turn_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = NEW.task_session_id;
END;

DROP TRIGGER IF EXISTS conversation_turn_delete;
CREATE TRIGGER IF NOT EXISTS conversation_turn_delete
AFTER DELETE ON task_session_turns
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (OLD.task_session_id, 1, FALSE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_turn_versions(session_id, turn_id, row_sequence, task_id, started_at, tombstone, payload)
	SELECT OLD.task_session_id, OLD.id, watermark, NULLIF(OLD.task_id, ''), OLD.started_at, TRUE,
		json_object('type','session.turn.removed','session_id',OLD.task_session_id,'task_id',NULLIF(OLD.task_id,''),'id',OLD.id)
	FROM conversation_session_streams WHERE session_id = OLD.task_session_id;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT OLD.task_session_id, watermark, OLD.task_session_id || ':' || watermark, 'session.turn.removed', NULLIF(OLD.task_id,''),
		payload, CURRENT_TIMESTAMP
	FROM conversation_session_streams JOIN conversation_turn_versions
		ON conversation_turn_versions.session_id = conversation_session_streams.session_id
		AND conversation_turn_versions.turn_id = OLD.id
		AND conversation_turn_versions.row_sequence = conversation_session_streams.watermark
	WHERE conversation_session_streams.session_id = OLD.task_session_id;
END;

DROP TRIGGER IF EXISTS conversation_session_delete;
CREATE TRIGGER IF NOT EXISTS conversation_session_delete
AFTER DELETE ON task_sessions
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (OLD.id, 1, TRUE, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET watermark = watermark + 1, terminal = TRUE, updated_at = CURRENT_TIMESTAMP;
	INSERT INTO conversation_session_events(session_id, sequence, event_id, event_type, task_id, payload, created_at)
	SELECT OLD.id, watermark, OLD.id || ':' || watermark, 'session.removed', NULLIF(OLD.task_id,''),
		json_object('type','session.removed','session_id',OLD.id,'task_id',NULLIF(OLD.task_id,'')), CURRENT_TIMESTAMP
	FROM conversation_session_streams WHERE session_id = OLD.id;
END;
`, sqliteStripSystemToken, sqliteStripSystemContentExpr("NEW.content"))
	triggerSQL = strings.ReplaceAll(triggerSQL,
		sqliteConversationMetadataToken+"(NEW.metadata)", sqliteConversationMetadataExpr("NEW.metadata"))
	triggerSQL = strings.NewReplacer(
		"__SQLITE_RFC3339_MILLIS__(NEW.created_at)", dialect.RFC3339Millis(dialect.SQLite3, "NEW.created_at"),
		"__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.created_at))", dialect.RFC3339Millis(dialect.SQLite3, "COALESCE(NEW.updated_at,NEW.created_at)"),
		"__SQLITE_RFC3339_MILLIS__(NEW.started_at)", dialect.RFC3339Millis(dialect.SQLite3, "NEW.started_at"),
		"__SQLITE_RFC3339_MILLIS__(NEW.completed_at)", dialect.RFC3339Millis(dialect.SQLite3, "NEW.completed_at"),
		"__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.started_at))", dialect.RFC3339Millis(dialect.SQLite3, "COALESCE(NEW.updated_at,NEW.started_at)"),
		"__SQLITE_RFC3339_MILLIS__(COALESCE(NEW.updated_at,NEW.completed_at,NEW.started_at))", dialect.RFC3339Millis(dialect.SQLite3, "COALESCE(NEW.updated_at,NEW.completed_at,NEW.started_at)"),
	).Replace(triggerSQL)
	if _, err := r.db.Exec(triggerSQL); err != nil {
		return fmt.Errorf("create SQLite conversation journal triggers: %w", err)
	}
	messageMigrate := `
		UPDATE conversation_message_versions
		SET payload = json_set(payload, '$.content', __SQLITE_STRIP_SYSTEM__)
		WHERE json_valid(payload)
		  AND json_extract(payload, '$.content') LIKE '%<kandev-system>%'
	`
	contentExpr := sqliteStripSystemContentExpr("json_extract(payload, '$.content')")
	if _, err := r.db.Exec(strings.ReplaceAll(messageMigrate, sqliteStripSystemToken, contentExpr)); err != nil {
		return fmt.Errorf("sanitize SQLite conversation message journal: %w", err)
	}
	eventMigrate := `
		UPDATE conversation_session_events
		SET payload = json_set(payload, '$.content', __SQLITE_STRIP_SYSTEM__)
		WHERE json_valid(payload)
		  AND json_extract(payload, '$.content') LIKE '%<kandev-system>%'
	`
	if _, err := r.db.Exec(strings.ReplaceAll(eventMigrate, sqliteStripSystemToken, contentExpr)); err != nil {
		return fmt.Errorf("sanitize SQLite conversation event journal: %w", err)
	}
	return nil
}

//nolint:funlen // Trigger definitions are kept together so schema initialization is atomic.
func (r *Repository) initPostgresConversationJournalTriggers() error {
	triggerSQL := strings.ReplaceAll(`
CREATE OR REPLACE FUNCTION conversation_next_sequence(p_session_id TEXT, p_terminal BOOLEAN DEFAULT FALSE)
RETURNS BIGINT AS $$
DECLARE next_value BIGINT;
BEGIN
	INSERT INTO conversation_session_streams(session_id, watermark, terminal, updated_at)
	VALUES (p_session_id, 1, p_terminal, CURRENT_TIMESTAMP)
	ON CONFLICT(session_id) DO UPDATE SET
		watermark = conversation_session_streams.watermark + 1,
		terminal = conversation_session_streams.terminal OR EXCLUDED.terminal,
		updated_at = CURRENT_TIMESTAMP
	RETURNING watermark INTO next_value;
	RETURN next_value;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION conversation_safe_jsonb(value TEXT) RETURNS JSONB AS $$
BEGIN
	RETURN COALESCE(NULLIF(value, ''), '{}')::jsonb;
EXCEPTION WHEN others THEN
	RETURN '{}'::jsonb;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION conversation_visible_content(value TEXT) RETURNS TEXT AS $$
DECLARE
	result TEXT := value;
	open_pos INTEGER;
	close_offset INTEGER;
	close_pos INTEGER;
BEGIN
	LOOP
		open_pos := strpos(result, '<kandev-system>');
		IF open_pos = 0 THEN
			EXIT;
		END IF;
		close_offset := strpos(
			substr(result, open_pos + length('<kandev-system>')),
			'</kandev-system>'
		);
		IF close_offset = 0 THEN
			EXIT;
		END IF;
		close_pos := open_pos + length('<kandev-system>') + close_offset - 1;
		result := substr(result, 1, open_pos - 1)
			|| ltrim(substr(result, close_pos + length('</kandev-system>')), E' \t\n\r');
	END LOOP;
	RETURN btrim(result);
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION conversation_message_journal() RETURNS TRIGGER AS $$
DECLARE seq BIGINT; event_name TEXT; source_row task_session_messages%ROWTYPE; deleted BOOLEAN;
BEGIN
	deleted := TG_OP = 'DELETE';
	IF deleted THEN source_row := OLD; event_name := 'message.deleted';
	ELSIF TG_OP = 'INSERT' THEN source_row := NEW; event_name := 'message.added';
	ELSE source_row := NEW; event_name := 'message.updated'; END IF;
	seq := conversation_next_sequence(source_row.task_session_id);
	INSERT INTO conversation_message_versions(session_id,message_id,row_sequence,task_id,author_type,created_at,tombstone,payload)
	VALUES (source_row.task_session_id,source_row.id,seq,NULLIF(source_row.task_id,''),source_row.author_type,source_row.created_at,deleted,
		CASE WHEN deleted THEN json_build_object('type',event_name,'session_id',source_row.task_session_id,'task_id',NULLIF(source_row.task_id,''),'message_id',source_row.id)::text
		ELSE json_build_object('type',event_name,'session_id',source_row.task_session_id,'task_id',NULLIF(source_row.task_id,''),
			'message_id',source_row.id,'turn_id',NULLIF(source_row.turn_id,''),'author_type',source_row.author_type,
			'content',conversation_visible_content(source_row.content),'message_type',source_row.type,
			'requests_input',source_row.requests_input,
			'created_at',to_char(source_row.created_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
			'updated_at',to_char(COALESCE(source_row.updated_at,source_row.created_at),'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'prompt_index',source_row.prompt_seq,
			'metadata',__POSTGRES_CONVERSATION_METADATA__(source_row.metadata),
			'sender_task_id',CASE WHEN jsonb_typeof(conversation_safe_jsonb(source_row.metadata) -> 'sender_task_id') = 'string' THEN conversation_safe_jsonb(source_row.metadata) ->> 'sender_task_id' END)::text END);
	INSERT INTO conversation_session_events(session_id,sequence,event_id,event_type,task_id,payload,created_at)
	SELECT source_row.task_session_id,seq,source_row.task_session_id || ':' || seq,event_name,NULLIF(source_row.task_id,''),payload,CURRENT_TIMESTAMP
	FROM conversation_message_versions WHERE session_id=source_row.task_session_id AND message_id=source_row.id AND row_sequence=seq;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS conversation_message_journal_trigger ON task_session_messages;
CREATE TRIGGER conversation_message_journal_trigger AFTER INSERT OR UPDATE OR DELETE ON task_session_messages
FOR EACH ROW EXECUTE FUNCTION conversation_message_journal();

CREATE OR REPLACE FUNCTION conversation_turn_journal() RETURNS TRIGGER AS $$
DECLARE seq BIGINT; event_name TEXT; source_row task_session_turns%ROWTYPE; deleted BOOLEAN;
BEGIN
	deleted := TG_OP = 'DELETE';
	IF deleted THEN source_row := OLD; event_name := 'session.turn.removed';
	ELSIF TG_OP = 'UPDATE' AND NOT (OLD.completed_at IS NULL AND NEW.completed_at IS NOT NULL) THEN RETURN NEW;
	ELSIF TG_OP = 'INSERT' THEN source_row := NEW; event_name := 'session.turn.started';
	ELSE source_row := NEW; event_name := 'session.turn.completed'; END IF;
	seq := conversation_next_sequence(source_row.task_session_id);
	INSERT INTO conversation_turn_versions(session_id,turn_id,row_sequence,task_id,started_at,tombstone,payload)
	VALUES (source_row.task_session_id,source_row.id,seq,NULLIF(source_row.task_id,''),source_row.started_at,deleted,
		CASE WHEN deleted THEN
			json_build_object('type',event_name,'session_id',source_row.task_session_id,'task_id',NULLIF(source_row.task_id,''),'id',source_row.id)::text
		ELSE
			json_build_object('type',event_name,'session_id',source_row.task_session_id,'task_id',NULLIF(source_row.task_id,''),
				'id',source_row.id,'started_at',to_char(source_row.started_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'completed_at',CASE WHEN source_row.completed_at IS NULL THEN NULL ELSE to_char(source_row.completed_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
				'created_at',to_char(source_row.created_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'updated_at',to_char(COALESCE(source_row.updated_at,source_row.completed_at,source_row.started_at),'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'metadata',CASE WHEN source_row.completed_at IS NULL THEN NULL ELSE conversation_safe_jsonb(source_row.metadata) END,
				'had_output',CASE WHEN source_row.completed_at IS NULL THEN NULL ELSE EXISTS (
					SELECT 1 FROM task_session_messages output
					WHERE output.turn_id = source_row.id AND output.author_type = 'agent'
					AND (
						output.type IN ('tool_call','tool_edit','tool_read','tool_search','tool_execute',
							'agent_plan','todo','permission_request','clarification_request')
						OR (output.type IN ('message','content','') AND btrim(output.content) <> '')
					)) END)::text
		END);
	INSERT INTO conversation_session_events(session_id,sequence,event_id,event_type,task_id,payload,created_at)
	SELECT source_row.task_session_id,seq,source_row.task_session_id || ':' || seq,event_name,NULLIF(source_row.task_id,''),payload,CURRENT_TIMESTAMP
	FROM conversation_turn_versions
	WHERE session_id=source_row.task_session_id AND turn_id=source_row.id AND row_sequence=seq;
	IF deleted THEN RETURN OLD; END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS conversation_turn_journal_trigger ON task_session_turns;
CREATE TRIGGER conversation_turn_journal_trigger AFTER INSERT OR UPDATE OR DELETE ON task_session_turns
FOR EACH ROW EXECUTE FUNCTION conversation_turn_journal();

CREATE OR REPLACE FUNCTION conversation_session_delete_journal() RETURNS TRIGGER AS $$
DECLARE seq BIGINT;
BEGIN
	seq := conversation_next_sequence(OLD.id, TRUE);
	INSERT INTO conversation_session_events(session_id,sequence,event_id,event_type,task_id,payload,created_at)
	VALUES (OLD.id,seq,OLD.id || ':' || seq,'session.removed',NULLIF(OLD.task_id,''),
		json_build_object('type','session.removed','session_id',OLD.id,'task_id',NULLIF(OLD.task_id,''))::text,CURRENT_TIMESTAMP);
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS conversation_session_delete_trigger ON task_sessions;
CREATE TRIGGER conversation_session_delete_trigger AFTER DELETE ON task_sessions
FOR EACH ROW EXECUTE FUNCTION conversation_session_delete_journal();
	`, postgresConversationMetadataToken+"(source_row.metadata)", postgresConversationMetadataExpr("source_row.metadata"))
	_, err := r.db.Exec(r.db.Rebind(triggerSQL))
	if err != nil {
		return fmt.Errorf("create PostgreSQL conversation journal triggers: %w", err)
	}
	if _, err := r.db.Exec(`
		UPDATE conversation_message_versions
		SET payload = jsonb_set(payload::jsonb, '{content}',
			to_jsonb(conversation_visible_content(payload::jsonb ->> 'content')))::text
		WHERE conversation_safe_jsonb(payload) ->> 'content' LIKE '%<kandev-system>%'
	`); err != nil {
		return fmt.Errorf("sanitize PostgreSQL conversation message journal: %w", err)
	}
	if _, err := r.db.Exec(`
		UPDATE conversation_session_events
		SET payload = jsonb_set(payload::jsonb, '{content}',
			to_jsonb(conversation_visible_content(payload::jsonb ->> 'content')))::text
		WHERE conversation_safe_jsonb(payload) ->> 'content' LIKE '%<kandev-system>%'
	`); err != nil {
		return fmt.Errorf("sanitize PostgreSQL conversation event journal: %w", err)
	}
	return nil
}
