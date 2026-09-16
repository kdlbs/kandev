package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	conversationTimestampParameter = "CAST(? AS timestamp)"
	conversationAscending          = "ASC"
	conversationDescending         = "DESC"
)

// initConversationSourceSchema installs the bounded revision metadata used by
// current-state conversation readers. Source rows remain authoritative; this
// table never stores message or turn payloads.
func (r *Repository) initConversationSourceSchema() error {
	if _, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS conversation_session_revisions (
			session_id TEXT PRIMARY KEY,
			revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		)`); err != nil {
		return fmt.Errorf("create conversation source revisions: %w", err)
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.initPostgresConversationSourceTriggers()
	}
	return r.initSQLiteConversationSourceTriggers()
}

const sqliteConversationSourceTriggerSQL = `
CREATE TRIGGER IF NOT EXISTS conversation_source_message_insert
AFTER INSERT ON task_session_messages
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT NEW.task_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = NEW.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
END;

CREATE TRIGGER IF NOT EXISTS conversation_source_message_update
AFTER UPDATE ON task_session_messages
WHEN OLD.task_session_id IS NOT NEW.task_session_id
  OR OLD.task_id IS NOT NEW.task_id
  OR OLD.turn_id IS NOT NEW.turn_id
  OR OLD.author_type IS NOT NEW.author_type
  OR OLD.author_id IS NOT NEW.author_id
  OR OLD.content IS NOT NEW.content
  OR OLD.requests_input IS NOT NEW.requests_input
  OR OLD.type IS NOT NEW.type
  OR OLD.metadata IS NOT NEW.metadata
  OR OLD.created_at IS NOT NEW.created_at
  OR OLD.updated_at IS NOT NEW.updated_at
  OR OLD.prompt_seq IS NOT NEW.prompt_seq
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT OLD.task_session_id, 1
	WHERE OLD.task_session_id != NEW.task_session_id
	  AND EXISTS (SELECT 1 FROM task_sessions WHERE id = OLD.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT NEW.task_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = NEW.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
END;

CREATE TRIGGER IF NOT EXISTS conversation_source_message_delete
AFTER DELETE ON task_session_messages
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT OLD.task_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = OLD.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
END;

CREATE TRIGGER IF NOT EXISTS conversation_source_turn_insert
AFTER INSERT ON task_session_turns
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT NEW.task_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = NEW.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
END;

CREATE TRIGGER IF NOT EXISTS conversation_source_turn_update
AFTER UPDATE ON task_session_turns
WHEN OLD.task_session_id IS NOT NEW.task_session_id
  OR OLD.task_id IS NOT NEW.task_id
  OR OLD.execution_profile_id IS NOT NEW.execution_profile_id
  OR OLD.route_generation IS NOT NEW.route_generation
  OR OLD.started_at IS NOT NEW.started_at
  OR OLD.completed_at IS NOT NEW.completed_at
  OR OLD.metadata IS NOT NEW.metadata
  OR OLD.created_at IS NOT NEW.created_at
  OR OLD.updated_at IS NOT NEW.updated_at
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT OLD.task_session_id, 1
	WHERE OLD.task_session_id != NEW.task_session_id
	  AND EXISTS (SELECT 1 FROM task_sessions WHERE id = OLD.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT NEW.task_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = NEW.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
END;

CREATE TRIGGER IF NOT EXISTS conversation_source_turn_delete
AFTER DELETE ON task_session_turns
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT OLD.task_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = OLD.task_session_id)
	ON CONFLICT(session_id) DO UPDATE SET revision = conversation_session_revisions.revision + 1;
END;`

func (r *Repository) initSQLiteConversationSourceTriggers() error {
	if _, err := r.db.ExecContext(r.migrationContext(), sqliteConversationSourceTriggerSQL); err != nil {
		return fmt.Errorf("create SQLite conversation source triggers: %w", err)
	}
	return nil
}

func (r *Repository) initPostgresConversationSourceTriggers() error {
	const triggerSQL = `
CREATE OR REPLACE FUNCTION conversation_source_bump_revision(p_session_id TEXT)
RETURNS VOID AS $$
BEGIN
	INSERT INTO conversation_session_revisions(session_id, revision)
	SELECT p_session_id, 1
	WHERE EXISTS (SELECT 1 FROM task_sessions WHERE id = p_session_id)
	ON CONFLICT(session_id) DO UPDATE
	SET revision = conversation_session_revisions.revision + 1;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION conversation_source_message_revision()
RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM conversation_source_bump_revision(OLD.task_session_id);
		RETURN OLD;
	END IF;
	IF TG_OP = 'UPDATE' AND OLD.task_session_id IS DISTINCT FROM NEW.task_session_id THEN
		PERFORM conversation_source_bump_revision(OLD.task_session_id);
	END IF;
	PERFORM conversation_source_bump_revision(NEW.task_session_id);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS conversation_source_message_revision_trigger ON task_session_messages;
CREATE TRIGGER conversation_source_message_revision_trigger
AFTER INSERT OR UPDATE OR DELETE ON task_session_messages
FOR EACH ROW EXECUTE FUNCTION conversation_source_message_revision();

CREATE OR REPLACE FUNCTION conversation_source_turn_revision()
RETURNS TRIGGER AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM conversation_source_bump_revision(OLD.task_session_id);
		RETURN OLD;
	END IF;
	IF TG_OP = 'UPDATE' AND OLD.task_session_id IS DISTINCT FROM NEW.task_session_id THEN
		PERFORM conversation_source_bump_revision(OLD.task_session_id);
	END IF;
	PERFORM conversation_source_bump_revision(NEW.task_session_id);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS conversation_source_turn_revision_trigger ON task_session_turns;
CREATE TRIGGER conversation_source_turn_revision_trigger
AFTER INSERT OR UPDATE OR DELETE ON task_session_turns
FOR EACH ROW EXECUTE FUNCTION conversation_source_turn_revision();`
	if _, err := r.db.ExecContext(r.migrationContext(), r.db.Rebind(triggerSQL)); err != nil {
		return fmt.Errorf("create PostgreSQL conversation source triggers: %w", err)
	}
	return nil
}

// ReadConversationRevision reads session existence and its current revision in
// one short read transaction. A revision row is optional because revision zero
// is the correct value for an existing session that has not changed since the
// source schema was installed.
func (r *Repository) ReadConversationRevision(ctx context.Context, sessionID string) (models.ConversationRevision, error) {
	tx, err := r.beginConversationSourceRead(ctx)
	if err != nil {
		return models.ConversationRevision{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := readConversationState(ctx, tx, r.db.DriverName(), sessionID)
	if err != nil {
		return models.ConversationRevision{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationRevision{}, fmt.Errorf("commit conversation revision read: %w", err)
	}
	return state, nil
}

// ReadConversationMessagesPage reads source messages and their revision from a
// single consistent transaction. The result is a current-state page; it is
// never a read from the historical journal tables.
func (r *Repository) ReadConversationMessagesPage(
	ctx context.Context,
	req models.ConversationMessagePageRequest,
) (models.ConversationMessagePage, error) {
	tx, err := r.beginConversationSourceRead(ctx)
	if err != nil {
		return models.ConversationMessagePage{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := readConversationState(ctx, tx, r.db.DriverName(), req.SessionID)
	if err != nil {
		return models.ConversationMessagePage{}, err
	}
	if !state.Exists {
		return models.ConversationMessagePage{}, models.ErrTaskSessionNotFound
	}
	limit := conversationSourcePageLimit(req.Limit)
	cursorKey, cursorID, err := readConversationMessageCursor(ctx, tx, r.db.DriverName(), req.SessionID, req.CursorID)
	if err != nil {
		return models.ConversationMessagePage{}, err
	}
	query, args := buildConversationMessagePageQuery(r.db.DriverName(), req, cursorKey, cursorID, limit)
	rows, err := tx.QueryxContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return models.ConversationMessagePage{}, fmt.Errorf("read conversation message page: %w", err)
	}
	messages, hasMore, scanErr := scanPromptIndexedMessageRows(rows, limit)
	_ = rows.Close()
	if scanErr != nil {
		return models.ConversationMessagePage{}, scanErr
	}
	page := models.ConversationMessagePage{Messages: messages, HasMore: hasMore, Revision: state.Revision}
	if hasMore && len(messages) > 0 {
		page.CursorID = messages[len(messages)-1].ID
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationMessagePage{}, fmt.Errorf("commit conversation message page: %w", err)
	}
	return page, nil
}

// ReadConversationTurnsPage is the turn counterpart to
// ReadConversationMessagesPage. Turn metadata remains available to first-party
// callers, while the plugin layer maps it to its safe DTO.
func (r *Repository) ReadConversationTurnsPage(
	ctx context.Context,
	req models.ConversationTurnPageRequest,
) (models.ConversationTurnPage, error) {
	tx, err := r.beginConversationSourceRead(ctx)
	if err != nil {
		return models.ConversationTurnPage{}, err
	}
	defer func() { _ = tx.Rollback() }()
	state, err := readConversationState(ctx, tx, r.db.DriverName(), req.SessionID)
	if err != nil {
		return models.ConversationTurnPage{}, err
	}
	if !state.Exists {
		return models.ConversationTurnPage{}, models.ErrTaskSessionNotFound
	}
	limit := conversationSourcePageLimit(req.Limit)
	cursorStarted, cursorID, err := readConversationTurnCursor(ctx, tx, req.SessionID, req.CursorID)
	if err != nil {
		return models.ConversationTurnPage{}, err
	}
	query, args := buildConversationTurnPageQuery(r.db.DriverName(), req, cursorStarted, cursorID, limit)
	rows, err := tx.QueryxContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return models.ConversationTurnPage{}, fmt.Errorf("read conversation turn page: %w", err)
	}
	defer func() { _ = rows.Close() }()
	turns := make([]*models.Turn, 0, limit)
	for rows.Next() {
		turn, scanErr := scanTurn(rows)
		if scanErr != nil {
			return models.ConversationTurnPage{}, scanErr
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return models.ConversationTurnPage{}, err
	}
	hasMore := len(turns) > limit
	if hasMore {
		turns = turns[:limit]
	}
	page := models.ConversationTurnPage{Turns: turns, HasMore: hasMore, Revision: state.Revision}
	if hasMore && len(turns) > 0 {
		page.CursorID = turns[len(turns)-1].ID
	}
	if err := tx.Commit(); err != nil {
		return models.ConversationTurnPage{}, fmt.Errorf("commit conversation turn page: %w", err)
	}
	return page, nil
}

func (r *Repository) beginConversationSourceRead(ctx context.Context) (*sqlx.Tx, error) {
	options := &sql.TxOptions{ReadOnly: true}
	if dialect.IsPostgres(r.db.DriverName()) {
		options.Isolation = sql.LevelRepeatableRead
	}
	return r.ro.BeginTxx(ctx, options)
}

func readConversationState(ctx context.Context, tx *sqlx.Tx, _ string, sessionID string) (models.ConversationRevision, error) {
	var exists int
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT 1 FROM task_sessions WHERE id = ?`), sessionID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ConversationRevision{SessionID: sessionID}, nil
		}
		return models.ConversationRevision{}, fmt.Errorf("read conversation session: %w", err)
	}
	var revision int64
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT revision FROM conversation_session_revisions WHERE session_id = ?`), sessionID).Scan(&revision); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.ConversationRevision{}, fmt.Errorf("read conversation revision: %w", err)
	}
	return models.ConversationRevision{SessionID: sessionID, Revision: revision, Exists: exists == 1}, nil
}

func conversationSourcePageLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func readConversationMessageCursor(ctx context.Context, tx *sqlx.Tx, driver, sessionID, cursorID string) (string, string, error) {
	if cursorID == "" {
		return "", "", nil
	}
	var cursorSession string
	var cursorCreated time.Time
	query := `SELECT task_session_id, created_at FROM task_session_messages WHERE id = ?`
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), cursorID).Scan(&cursorSession, &cursorCreated); err != nil {
		return "", "", err
	}
	if cursorSession != sessionID {
		return "", "", fmt.Errorf("message cursor not found: %s", cursorID)
	}
	if dialect.IsPostgres(driver) {
		return formatPromptKey(cursorCreated), cursorID, nil
	}
	var key string
	if err := tx.QueryRowxContext(ctx, tx.Rebind(fmt.Sprintf(
		`SELECT %s FROM task_session_messages WHERE id = ?`,
		dialect.NormalizedMicrosecond(driver, "created_at"),
	)), cursorID).Scan(&key); err != nil {
		return "", "", err
	}
	return key, cursorID, nil
}

func readConversationTurnCursor(ctx context.Context, tx *sqlx.Tx, sessionID, cursorID string) (time.Time, string, error) {
	if cursorID == "" {
		return time.Time{}, "", nil
	}
	var cursorSession string
	var cursorStarted time.Time
	if err := tx.QueryRowxContext(ctx, tx.Rebind(
		`SELECT task_session_id, started_at FROM task_session_turns WHERE id = ?`,
	), cursorID).Scan(&cursorSession, &cursorStarted); err != nil {
		return time.Time{}, "", err
	}
	if cursorSession != sessionID {
		return time.Time{}, "", fmt.Errorf("turn cursor not found: %s", cursorID)
	}
	return cursorStarted, cursorID, nil
}

func buildConversationMessagePageQuery(
	driver string,
	req models.ConversationMessagePageRequest,
	cursorKey, cursorID string,
	limit int,
) (string, []interface{}) {
	normalized := dialect.NormalizedMicrosecond(driver, "created_at")
	bound := "?"
	if dialect.IsPostgres(driver) {
		bound = conversationTimestampParameter
	}
	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id, content,
		       requests_input, type, metadata, created_at, updated_at,
		       CASE WHEN author_type = 'user' THEN prompt_seq ELSE 0 END AS prompt_index
		FROM task_session_messages
		WHERE task_session_id = ?`
	args := []interface{}{req.SessionID}
	if req.TaskID != nil {
		query += " AND task_id = ?"
		args = append(args, *req.TaskID)
	}
	if len(req.Authors) > 0 {
		placeholders := make([]string, len(req.Authors))
		for index, author := range req.Authors {
			placeholders[index] = "?"
			args = append(args, author)
		}
		query += " AND author_type IN (" + strings.Join(placeholders, ",") + ")"
	}
	sortOrder := conversationAscending
	compare := ">"
	if strings.EqualFold(req.Sort, "desc") {
		sortOrder = conversationDescending
		compare = "<"
	}
	if cursorID != "" {
		query += fmt.Sprintf(" AND (%s %s %s OR (%s = %s AND id %s ?))", normalized, compare, bound, normalized, bound, compare)
		args = append(args, cursorKey, cursorKey, cursorID)
	}
	query += fmt.Sprintf(" ORDER BY %s %s, id %s", normalized, sortOrder, sortOrder)
	query += sqlLimitClause
	args = append(args, limit+1)
	return query, args
}

func buildConversationTurnPageQuery(driver string, req models.ConversationTurnPageRequest, cursorStarted time.Time, cursorID string, limit int) (string, []interface{}) {
	query := `
		SELECT id, task_session_id, task_id, execution_profile_id, route_generation,
		       started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns turn_row
		WHERE turn_row.task_session_id = ? AND ` + turnHistoryPredicate(driver, "turn_row")
	args := []interface{}{req.SessionID}
	if req.TaskID != nil {
		query += " AND turn_row.task_id = ?"
		args = append(args, *req.TaskID)
	}
	sortOrder := conversationAscending
	if strings.EqualFold(req.Sort, "desc") {
		sortOrder = conversationDescending
	}
	if cursorID != "" {
		compare := ">"
		if sortOrder == conversationDescending {
			compare = "<"
		}
		query += fmt.Sprintf(" AND (turn_row.started_at %s ? OR (turn_row.started_at = ? AND turn_row.id %s ?))", compare, compare)
		args = append(args, cursorStarted, cursorStarted, cursorID)
	}
	query += " ORDER BY turn_row.started_at " + sortOrder + ", turn_row.id " + sortOrder + sqlLimitClause
	args = append(args, limit+1)
	return query, args
}
