package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// sqliteRepository persists queued messages and pending moves.
type sqliteRepository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewSQLiteRepository creates a SQLite-backed Repository. The supplied writer
// and reader are taken from the shared DB pool. initSchema runs idempotently.
func NewSQLiteRepository(writer, reader *sqlx.DB) (Repository, error) {
	r := &sqliteRepository{db: writer, ro: reader}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("messagequeue: init schema: %w", err)
	}
	return r, nil
}

func (r *sqliteRepository) initSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS queued_messages (
		id               TEXT PRIMARY KEY,
		session_id       TEXT NOT NULL,
		task_id          TEXT NOT NULL,
		position         INTEGER NOT NULL,
		content          TEXT NOT NULL DEFAULT '',
		model            TEXT NOT NULL DEFAULT '',
		plan_mode        INTEGER NOT NULL DEFAULT 0,
		attachments_json TEXT NOT NULL DEFAULT '[]',
		metadata_json    TEXT NOT NULL DEFAULT '{}',
		queued_at        TIMESTAMP NOT NULL,
		queued_by        TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_queued_messages_session_position ON queued_messages(session_id, position);

	CREATE TABLE IF NOT EXISTS pending_moves (
		id               TEXT PRIMARY KEY,
		session_id       TEXT NOT NULL UNIQUE,
		task_id          TEXT NOT NULL DEFAULT '',
		workflow_id      TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '',
		step_position    INTEGER NOT NULL DEFAULT 0,
		queued_at        TIMESTAMP NOT NULL
	);
	`)
	return err
}

func (r *sqliteRepository) Insert(ctx context.Context, msg *QueuedMessage, maxPerSession int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if maxPerSession > 0 {
		var count int
		if err := tx.GetContext(ctx, &count, r.db.Rebind(`SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`), msg.SessionID); err != nil {
			return fmt.Errorf("count: %w", err)
		}
		if count >= maxPerSession {
			return ErrQueueFull
		}
	}

	var maxPos sql.NullInt64
	if err := tx.GetContext(ctx, &maxPos, r.db.Rebind(`SELECT MAX(position) FROM queued_messages WHERE session_id = ?`), msg.SessionID); err != nil {
		return fmt.Errorf("max position: %w", err)
	}
	msg.Position = maxPos.Int64 + 1
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.QueuedAt.IsZero() {
		msg.QueuedAt = time.Now().UTC()
	}

	attachmentsJSON, err := marshalAttachments(msg.Attachments)
	if err != nil {
		return err
	}
	metadataJSON, err := marshalMetadata(msg.Metadata)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queued_messages
			(id, session_id, task_id, position, content, model, plan_mode, attachments_json, metadata_json, queued_at, queued_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		msg.ID, msg.SessionID, msg.TaskID, msg.Position, msg.Content, msg.Model,
		boolToInt(msg.PlanMode), attachmentsJSON, metadataJSON, msg.QueuedAt, msg.QueuedBy,
	); err != nil {
		return fmt.Errorf("insert queued_messages: %w", err)
	}
	return tx.Commit()
}

func (r *sqliteRepository) AppendOrInsertTail(ctx context.Context, sessionID, taskID, content, model, queuedBy string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin append tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tail, err := r.scanTail(ctx, tx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if tail != nil && tail.QueuedBy == queuedBy {
		newContent := tail.Content + "\n\n---\n\n" + content
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE queued_messages SET content = ? WHERE id = ?`), newContent, tail.ID); err != nil {
			return nil, false, fmt.Errorf("append update: %w", err)
		}
		tail.Content = newContent
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return tail, true, nil
	}

	// No matching tail — insert a fresh entry while still inside the same tx.
	if maxPerSession > 0 {
		var count int
		if err := tx.GetContext(ctx, &count, r.db.Rebind(`SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`), sessionID); err != nil {
			return nil, false, fmt.Errorf("count: %w", err)
		}
		if count >= maxPerSession {
			return nil, false, ErrQueueFull
		}
	}
	var maxPos sql.NullInt64
	if err := tx.GetContext(ctx, &maxPos, r.db.Rebind(`SELECT MAX(position) FROM queued_messages WHERE session_id = ?`), sessionID); err != nil {
		return nil, false, fmt.Errorf("max position: %w", err)
	}

	msg := &QueuedMessage{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		TaskID:      taskID,
		Position:    maxPos.Int64 + 1,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadata,
		QueuedAt:    time.Now().UTC(),
		QueuedBy:    queuedBy,
	}
	attachmentsJSON, err := marshalAttachments(attachments)
	if err != nil {
		return nil, false, err
	}
	metadataJSON, err := marshalMetadata(metadata)
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO queued_messages
			(id, session_id, task_id, position, content, model, plan_mode, attachments_json, metadata_json, queued_at, queued_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		msg.ID, msg.SessionID, msg.TaskID, msg.Position, msg.Content, msg.Model,
		boolToInt(msg.PlanMode), attachmentsJSON, metadataJSON, msg.QueuedAt, msg.QueuedBy,
	); err != nil {
		return nil, false, fmt.Errorf("insert queued_messages: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}

func (r *sqliteRepository) ListBySession(ctx context.Context, sessionID string) ([]QueuedMessage, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE session_id = ?
		ORDER BY position ASC
	`), sessionID)
	if err != nil {
		return nil, fmt.Errorf("list queued: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []QueuedMessage
	for rows.Next() {
		msg, err := scanQueuedRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *msg)
	}
	return out, rows.Err()
}

func (r *sqliteRepository) CountBySession(ctx context.Context, sessionID string) (int, error) {
	var n int
	err := r.ro.GetContext(ctx, &n, r.ro.Rebind(`SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`), sessionID)
	return n, err
}

func (r *sqliteRepository) TakeHead(ctx context.Context, sessionID string) (*QueuedMessage, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin take tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE session_id = ?
		ORDER BY position ASC
		LIMIT 1
	`), sessionID)
	msg, err := scanQueuedRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("take head: %w", err)
	}
	// Two concurrent TakeHead calls can both observe the same head row in
	// their respective DEFERRED transactions; one wins the DELETE, the other
	// finds RowsAffected()==0 after waiting on the writer lock. Returning the
	// already-drained message in that case would let the orchestrator dispatch
	// it twice. Treat the lost race as "queue empty for now" so the caller
	// retries on the next agent.ready instead.
	res, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM queued_messages WHERE id = ?`), msg.ID)
	if err != nil {
		return nil, fmt.Errorf("delete head: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("delete head rows affected: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msg, nil
}

func (r *sqliteRepository) UpdateContent(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	attachmentsJSON, err := marshalAttachments(attachments)
	if err != nil {
		return err
	}
	var query string
	args := []interface{}{content, attachmentsJSON, entryID, sessionID}
	if queuedBy != "" {
		query = `UPDATE queued_messages SET content = ?, attachments_json = ? WHERE id = ? AND session_id = ? AND queued_by = ?`
		args = append(args, queuedBy)
	} else {
		query = `UPDATE queued_messages SET content = ?, attachments_json = ? WHERE id = ? AND session_id = ?`
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("update queued: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *sqliteRepository) DeleteByID(ctx context.Context, sessionID, entryID string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages
		WHERE id = ?
		  AND session_id = ?
		  AND queued_by != ?
	`), entryID, sessionID, QueuedByAgent)
	if err != nil {
		return fmt.Errorf("delete queued: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *sqliteRepository) DeleteAllBySession(ctx context.Context, sessionID string) (int, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM queued_messages WHERE session_id = ?`), sessionID)
	if err != nil {
		return 0, fmt.Errorf("delete all queued: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func (r *sqliteRepository) TransferSession(ctx context.Context, oldSessionID, newSessionID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Shift positions on the destination so transferred entries land at the tail
	// without colliding on the (session_id, position) implicit ordering.
	var destMax sql.NullInt64
	if err := tx.GetContext(ctx, &destMax, r.db.Rebind(`SELECT MAX(position) FROM queued_messages WHERE session_id = ?`), newSessionID); err != nil {
		return fmt.Errorf("transfer max: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queued_messages
		SET session_id = ?, position = position + ?
		WHERE session_id = ?
	`), newSessionID, destMax.Int64, oldSessionID); err != nil {
		return fmt.Errorf("transfer queued: %w", err)
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM pending_moves WHERE session_id = ?
	`), newSessionID); err != nil {
		return fmt.Errorf("clear dest pending move: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE pending_moves SET session_id = ? WHERE session_id = ?
	`), newSessionID, oldSessionID); err != nil {
		return fmt.Errorf("transfer pending move: %w", err)
	}
	return tx.Commit()
}

func (r *sqliteRepository) SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) error {
	if move.QueuedAt.IsZero() {
		move.QueuedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO pending_moves (id, session_id, task_id, workflow_id, workflow_step_id, step_position, queued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			task_id = excluded.task_id,
			workflow_id = excluded.workflow_id,
			workflow_step_id = excluded.workflow_step_id,
			step_position = excluded.step_position,
			queued_at = excluded.queued_at
	`),
		uuid.New().String(), sessionID, move.TaskID, move.WorkflowID, move.WorkflowStepID, move.Position, move.QueuedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert pending move: %w", err)
	}
	return nil
}

func (r *sqliteRepository) TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin take pending tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		taskID, workflowID, workflowStepID string
		position                           int
		queuedAt                           time.Time
	)
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT task_id, workflow_id, workflow_step_id, step_position, queued_at
		FROM pending_moves WHERE session_id = ?
	`), sessionID).Scan(&taskID, &workflowID, &workflowStepID, &position, &queuedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pending move: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM pending_moves WHERE session_id = ?`), sessionID); err != nil {
		return nil, fmt.Errorf("delete pending move: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &PendingMove{
		TaskID:         taskID,
		WorkflowID:     workflowID,
		WorkflowStepID: workflowStepID,
		Position:       position,
		QueuedAt:       queuedAt,
	}, nil
}

// scanTail reads the highest-position entry for a session within an active
// transaction. Returns nil, nil when the queue is empty.
func (r *sqliteRepository) scanTail(ctx context.Context, tx *sqlx.Tx, sessionID string) (*QueuedMessage, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE session_id = ?
		ORDER BY position DESC
		LIMIT 1
	`), sessionID)
	msg, err := scanQueuedRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan tail: %w", err)
	}
	return msg, nil
}

// scanQueuedRow scans a single queued_messages row from any sqlx-compatible row source.
func scanQueuedRow(scanner interface{ Scan(dest ...any) error }) (*QueuedMessage, error) {
	var (
		msg                       QueuedMessage
		planModeInt               int
		attachmentsJSON, metaJSON string
	)
	if err := scanner.Scan(
		&msg.ID, &msg.SessionID, &msg.TaskID, &msg.Position, &msg.Content, &msg.Model,
		&planModeInt, &attachmentsJSON, &metaJSON, &msg.QueuedAt, &msg.QueuedBy,
	); err != nil {
		return nil, err
	}
	msg.PlanMode = planModeInt != 0
	if attachmentsJSON != "" && attachmentsJSON != "[]" {
		if err := json.Unmarshal([]byte(attachmentsJSON), &msg.Attachments); err != nil {
			return nil, fmt.Errorf("unmarshal attachments: %w", err)
		}
	}
	if metaJSON != "" && metaJSON != "{}" {
		if err := json.Unmarshal([]byte(metaJSON), &msg.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &msg, nil
}

func marshalAttachments(att []MessageAttachment) (string, error) {
	if len(att) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(att)
	if err != nil {
		return "", fmt.Errorf("marshal attachments: %w", err)
	}
	return string(b), nil
}

func marshalMetadata(meta map[string]interface{}) (string, error) {
	if len(meta) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}
	return string(b), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
