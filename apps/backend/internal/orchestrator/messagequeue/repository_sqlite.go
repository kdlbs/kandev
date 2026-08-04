package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// sqliteRepository persists queued messages and pending moves.
type sqliteRepository struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader

	mu           sync.Mutex
	sessionLocks map[string]*sync.Mutex
}

// NewSQLiteRepository creates a SQLite-backed Repository. The supplied writer
// and reader are taken from the shared DB pool. initSchema runs idempotently.
func NewSQLiteRepository(writer, reader *sqlx.DB) (Repository, error) {
	r := &sqliteRepository{db: writer, ro: reader, sessionLocks: make(map[string]*sync.Mutex)}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("messagequeue: init schema: %w", err)
	}
	return r, nil
}

// withSessionLock serializes the queue mutations of one session so a concurrent
// merge and drain cannot interleave between their reads and writes. It is
// per-session (not global), so unrelated sessions proceed in parallel. The
// affected-row checks remain the authoritative guard across processes (e.g.
// multiple backends sharing a Postgres queue); the lock makes in-process
// merge-wins/drain-wins ordering deterministic.
func (r *sqliteRepository) withSessionLock(sessionID string) func() {
	r.mu.Lock()
	lock := r.sessionLocks[sessionID]
	if lock == nil {
		lock = &sync.Mutex{}
		r.sessionLocks[sessionID] = lock
	}
	r.mu.Unlock()
	lock.Lock()
	return func() { lock.Unlock() }
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

	CREATE TABLE IF NOT EXISTS lifecycle_queue_generations (
		task_id    TEXT PRIMARY KEY,
		generation INTEGER NOT NULL DEFAULT 0
	);

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

func (r *sqliteRepository) Restore(ctx context.Context, msg *QueuedMessage, maxPerSession int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restore tx: %w", err)
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
	`), msg.ID, msg.SessionID, msg.TaskID, msg.Position, msg.Content, msg.Model,
		boolToInt(msg.PlanMode), attachmentsJSON, metadataJSON, msg.QueuedAt, msg.QueuedBy,
	); err != nil {
		return fmt.Errorf("restore queued_messages: %w", err)
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

func (r *sqliteRepository) InsertOrReplaceByCoalesceKey(ctx context.Context, msg *QueuedMessage, coalesceKey string, maxPerSession int, allowInsert bool) (*QueuedMessage, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin coalesce tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := r.findCoalesced(ctx, tx, msg.SessionID, msg.QueuedBy, coalesceKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		updated, err := r.replaceCoalesced(ctx, tx, existing, msg)
		if err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return updated, true, nil
	}
	if !allowInsert {
		return nil, false, ErrEntryNotFound
	}
	if err := r.insertCoalesced(ctx, tx, msg, maxPerSession); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}

// InsertOrReplaceLifecycleByCoalesceKey serializes lifecycle acceptance with
// task archival. The no-op UPDATE takes SQLite's writer lock only when the
// referenced task is still active; archive either commits first (this returns
// ErrTaskInactive) or queues first (the archive then follows normal
// cancellation semantics).
func (r *sqliteRepository) InsertOrReplaceLifecycleByCoalesceKey(ctx context.Context, msg *QueuedMessage, coalesceKey string, maxPerSession int, allowInsert bool) (*QueuedMessage, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin lifecycle coalesce tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	guard, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET updated_at = updated_at
		WHERE id = ? AND archived_at IS NULL
	`), msg.TaskID)
	if err != nil {
		return nil, false, fmt.Errorf("guard active lifecycle task: %w", err)
	}
	rows, err := guard.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("active lifecycle task rows affected: %w", err)
	}
	if rows == 0 {
		return nil, false, ErrTaskInactive
	}
	expectedGeneration, ok := lifecycleGenerationFromMetadata(msg.Metadata)
	if !ok {
		return nil, false, ErrLifecycleCancelled
	}
	generation, err := lifecycleGenerationInTx(ctx, tx, r.db, msg.TaskID)
	if err != nil {
		return nil, false, err
	}
	if expectedGeneration != generation {
		return nil, false, ErrLifecycleCancelled
	}

	existing, err := r.findCoalesced(ctx, tx, msg.SessionID, msg.QueuedBy, coalesceKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		updated, err := r.replaceCoalesced(ctx, tx, existing, msg)
		if err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return updated, true, nil
	}
	if !allowInsert {
		return nil, false, ErrEntryNotFound
	}
	if err := r.insertCoalesced(ctx, tx, msg, maxPerSession); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return msg, false, nil
}

func (r *sqliteRepository) LifecycleGeneration(ctx context.Context, taskID string) (int64, error) {
	var generation int64
	err := r.ro.GetContext(ctx, &generation, r.ro.Rebind(`
		SELECT generation FROM lifecycle_queue_generations WHERE task_id = ?
	`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get lifecycle queue generation: %w", err)
	}
	return generation, nil
}

func (r *sqliteRepository) PurgeTask(ctx context.Context, taskID string) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin lifecycle queue purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	removed, err := PurgeTaskInTransaction(ctx, tx, r.db, taskID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}

func lifecycleGenerationInTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, taskID string) (int64, error) {
	var generation int64
	err := tx.GetContext(ctx, &generation, db.Rebind(`
		SELECT generation FROM lifecycle_queue_generations WHERE task_id = ?
	`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get lifecycle queue generation: %w", err)
	}
	return generation, nil
}

// PurgeTaskInTransaction lets the task repository make archive/delete and
// durable queue invalidation one SQLite transaction. It is backend-internal:
// user queue handlers must keep using ownership-checked deletion methods.
func PurgeTaskInTransaction(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, taskID string) (int, error) {
	result, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM queued_messages WHERE task_id = ?`), taskID)
	if err != nil {
		return 0, fmt.Errorf("purge queued task entries: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge queued task entries rows affected: %w", err)
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO lifecycle_queue_generations (task_id, generation) VALUES (?, 1)
		ON CONFLICT(task_id) DO UPDATE SET generation = lifecycle_queue_generations.generation + 1
	`), taskID); err != nil {
		return 0, fmt.Errorf("advance lifecycle queue generation: %w", err)
	}
	return int(removed), nil
}

func (r *sqliteRepository) replaceCoalesced(ctx context.Context, tx *sqlx.Tx, existing, msg *QueuedMessage) (*QueuedMessage, error) {
	if msg.QueuedAt.IsZero() {
		msg.QueuedAt = time.Now().UTC()
	}
	attachmentsJSON, err := marshalAttachments(msg.Attachments)
	if err != nil {
		return nil, err
	}
	metadataJSON, err := marshalMetadata(msg.Metadata)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queued_messages
		SET task_id = ?, content = ?, model = ?, plan_mode = ?,
		    attachments_json = ?, metadata_json = ?, queued_at = ?
		WHERE id = ? AND session_id = ? AND queued_by = ?
	`),
		msg.TaskID, msg.Content, msg.Model, boolToInt(msg.PlanMode),
		attachmentsJSON, metadataJSON, msg.QueuedAt,
		existing.ID, msg.SessionID, msg.QueuedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("replace coalesced queued: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("replace coalesced rows affected: %w", err)
	}
	if rows == 0 {
		return nil, ErrEntryNotFound
	}
	existing.TaskID = msg.TaskID
	existing.Content = msg.Content
	existing.Model = msg.Model
	existing.PlanMode = msg.PlanMode
	existing.Attachments = msg.Attachments
	existing.Metadata = msg.Metadata
	existing.QueuedAt = msg.QueuedAt
	return existing, nil
}

func (r *sqliteRepository) insertCoalesced(ctx context.Context, tx *sqlx.Tx, msg *QueuedMessage, maxPerSession int) error {
	if err := r.ensureQueueCapacity(ctx, tx, msg.SessionID, maxPerSession); err != nil {
		return err
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
		return fmt.Errorf("insert coalesced queued: %w", err)
	}
	return nil
}

func (r *sqliteRepository) ensureQueueCapacity(ctx context.Context, tx *sqlx.Tx, sessionID string, maxPerSession int) error {
	if maxPerSession <= 0 {
		return nil
	}
	var count int
	if err := tx.GetContext(ctx, &count, r.db.Rebind(`SELECT COUNT(*) FROM queued_messages WHERE session_id = ?`), sessionID); err != nil {
		return fmt.Errorf("count: %w", err)
	}
	if count >= maxPerSession {
		return ErrQueueFull
	}
	return nil
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
	// Share the per-session lock with MergeIntoAbove so a drain and a merge on
	// the same queue are serialized in-process, not just at the DB layer.
	unlock := r.withSessionLock(sessionID)
	defer unlock()

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

func (r *sqliteRepository) ReserveHead(ctx context.Context, sessionID string) (*QueuedMessage, error) {
	unlock := r.withSessionLock(sessionID)
	defer unlock()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin reserve tx: %w", err)
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
	msg, storedMetadataJSON, err := scanQueuedRowWithMetadataJSON(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reserve head: %w", err)
	}
	if msg.IsDurableLifecycle() {
		// Keep the row for crash recovery but stop reporting it as pending.
		// Strip a marker persisted by an interrupted prior process from the
		// returned copy so a failed retry becomes visible again.
		msg.Metadata = clearReservedMetadata(msg.Metadata)
		reservedJSON, err := marshalMetadata(markReservedMetadata(msg.Metadata))
		if err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE queued_messages SET metadata_json = ?
			WHERE id = ? AND session_id = ? AND metadata_json = ?
		`), reservedJSON, msg.ID, sessionID, storedMetadataJSON)
		if err != nil {
			return nil, fmt.Errorf("mark lifecycle reservation in flight: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("mark lifecycle reservation rows affected: %w", err)
		}
		if affected == 0 {
			return nil, nil
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		msg.reservedLifecycleDelivery = true
		return msg, nil
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages WHERE id = ? AND session_id = ?
	`), msg.ID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("delete reserved ordinary head: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("delete reserved ordinary head rows affected: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msg, nil
}

func (r *sqliteRepository) AcknowledgeByID(ctx context.Context, sessionID, entryID string) error {
	unlock := r.withSessionLock(sessionID)
	defer unlock()

	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages WHERE id = ? AND session_id = ?
	`), entryID, sessionID)
	if err != nil {
		return fmt.Errorf("acknowledge queued: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrEntryNotFound
	}
	return nil
}

// TakeByID atomically returns and deletes the entry identified by entryID,
// regardless of its FIFO position. Mirrors TakeHead's race handling: if a
// concurrent take already removed the row between the SELECT and DELETE,
// RowsAffected()==0 is treated as "already taken" (nil, nil) rather than an
// error, so two racing takers can never both dispatch the same entry. The
// DELETE is also scoped by session_id (not just id) so a concurrent
// TransferSession that reassigns this row to a different session between
// the SELECT and DELETE can't have it removed out from under the new
// session — same session-scope invariant DeleteByID/UpdateContent enforce.
func (r *sqliteRepository) TakeByID(ctx context.Context, sessionID, entryID string) (*QueuedMessage, error) {
	unlock := r.withSessionLock(sessionID)
	defer unlock()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin take tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE id = ? AND session_id = ?
	`), entryID, sessionID)
	msg, err := scanQueuedRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("take by id: %w", err)
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM queued_messages WHERE id = ? AND session_id = ?`), msg.ID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("delete by id: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("delete by id rows affected: %w", err)
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
	return r.UpdateContentAndMetadata(ctx, sessionID, entryID, content, attachments, nil, queuedBy)
}

func (r *sqliteRepository) UpdateContentAndMetadata(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
	if queuedBy == "" || IsReservedQueuedBy(queuedBy) {
		return ErrEntryNotFound
	}
	attachmentsJSON, err := marshalAttachments(attachments)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update queued tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var metadataJSON string
	query := `SELECT metadata_json FROM queued_messages WHERE id = ? AND session_id = ? AND queued_by = ? AND queued_by NOT IN (?, ?, ?)`
	if err := tx.GetContext(ctx, &metadataJSON, r.db.Rebind(query), entryID, sessionID, queuedBy, QueuedByAgent, QueuedByWorkflow, QueuedByServer); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("read queued metadata: %w", err)
	}
	var metadata map[string]interface{}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return fmt.Errorf("unmarshal queued metadata: %w", err)
		}
	}
	metadataJSON, err = marshalMetadata(applyMetadataUpdates(metadata, metadataUpdates))
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE queued_messages SET content = ?, attachments_json = ?, metadata_json = ?
		WHERE id = ? AND session_id = ? AND queued_by = ? AND queued_by NOT IN (?, ?, ?)`),
		content, attachmentsJSON, metadataJSON, entryID, sessionID, queuedBy, QueuedByAgent, QueuedByWorkflow, QueuedByServer)
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
	return tx.Commit()
}

// MergeIntoAbove folds the source entry into the entry directly above it within
// the same session, updating the target row and deleting the source row in one
// transaction so a concurrent drain can never observe a half-merged queue.
// The per-session lock serializes against drains (TakeHead/ReserveHead/
// TakeByID) and the affected-row checks turn a lost write into an
// ErrEntryNotFound rollback rather than a silent content loss. See
// Repository.MergeIntoAbove for the merge rules and error mapping.
func (r *sqliteRepository) MergeIntoAbove(ctx context.Context, sessionID, sourceID, queuedBy string) (*QueuedMessage, error) {
	unlock := r.withSessionLock(sessionID)
	defer unlock()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin merge tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	source, err := readMergeSource(ctx, r, tx, sessionID, sourceID)
	if err != nil {
		return nil, err
	}

	target, err := readMergeTarget(ctx, r, tx, sessionID, source.Position)
	if err != nil {
		return nil, err
	}

	if !mergeAllowed(source, target, queuedBy) {
		return nil, ErrNoMergeTarget
	}

	content, attachments, metadata, err := buildMergedEntry(target, source)
	if err != nil {
		return nil, err
	}
	if err := applyMergeWrites(ctx, r, tx, target, source, content, attachments, metadata, sessionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	merged := *target
	merged.Content = content
	merged.Attachments = attachments
	merged.Metadata = metadata
	return &merged, nil
}

// readMergeSource loads the source entry by id, mapping a missing row to
// ErrEntryNotFound so the caller can report it without wrapping.
func readMergeSource(ctx context.Context, r *sqliteRepository, tx *sqlx.Tx, sessionID, sourceID string) (*QueuedMessage, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE id = ? AND session_id = ?
	`), sourceID, sessionID)
	source, err := scanQueuedRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEntryNotFound
		}
		return nil, fmt.Errorf("read merge source: %w", err)
	}
	return source, nil
}

// readMergeTarget loads the entry with the greatest position strictly below
// the source, mapping a missing row to ErrNoMergeTarget.
func readMergeTarget(ctx context.Context, r *sqliteRepository, tx *sqlx.Tx, sessionID string, sourcePosition int64) (*QueuedMessage, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE session_id = ? AND position < ?
		ORDER BY position DESC
		LIMIT 1
	`), sessionID, sourcePosition)
	target, err := scanQueuedRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoMergeTarget
		}
		return nil, fmt.Errorf("read merge target: %w", err)
	}
	return target, nil
}

// buildMergedEntry computes the merged content, attachments, and metadata for
// a source folded into its target, returning the metadata marshalled to JSON
// so the caller can write both rows in the same transaction.
func buildMergedEntry(target, source *QueuedMessage) (string, []MessageAttachment, map[string]interface{}, error) {
	content := joinMergeContent(target.Content, source.Content)
	attachments := append(append([]MessageAttachment{}, target.Attachments...), source.Attachments...)
	metadata, err := mergeEntryMetadata(target.Metadata, source.Metadata)
	if err != nil {
		return "", nil, nil, err
	}
	return content, attachments, metadata, nil
}

// applyMergeWrites updates the target row and deletes the source row, turning
// a lost write into ErrEntryNotFound so the transaction rolls back cleanly
// instead of silently dropping a queued message.
func applyMergeWrites(ctx context.Context, r *sqliteRepository, tx *sqlx.Tx, target, source *QueuedMessage, content string, attachments []MessageAttachment, metadata map[string]interface{}, sessionID string) error {
	attachmentsJSON, err := marshalAttachments(attachments)
	if err != nil {
		return err
	}
	metadataJSON, err := marshalMetadata(metadata)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queued_messages
		SET content = ?, attachments_json = ?, metadata_json = ?
		WHERE id = ? AND session_id = ?
	`), content, attachmentsJSON, metadataJSON, target.ID, sessionID)
	if err != nil {
		return fmt.Errorf("update merge target: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update merge target rows affected: %w", err)
	}
	if affected == 0 {
		// The target was drained concurrently. Rolling back leaves the source
		// untouched instead of folding into a row that no longer exists.
		return ErrEntryNotFound
	}
	res, err = tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages
		WHERE id = ? AND session_id = ?
	`), source.ID, sessionID)
	if err != nil {
		return fmt.Errorf("delete merge source: %w", err)
	}
	affected, err = res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete merge source rows affected: %w", err)
	}
	if affected == 0 {
		// The source was drained concurrently; rollback restores the target
		// update so the merge is all-or-nothing.
		return ErrEntryNotFound
	}
	return nil
}

func (r *sqliteRepository) DeleteByID(ctx context.Context, sessionID, entryID string) error {
	unlock := r.withSessionLock(sessionID)
	defer unlock()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete queued tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var metadataJSON string
	err = tx.GetContext(ctx, &metadataJSON, r.db.Rebind(`
		SELECT metadata_json FROM queued_messages WHERE id = ? AND session_id = ?
	`), entryID, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("read queued cancellation candidate: %w", err)
	}
	reserved, err := isReservedMetadataJSON(metadataJSON)
	if err != nil {
		return err
	}
	if reserved {
		return ErrEntryNotFound
	}

	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages
		WHERE id = ? AND session_id = ? AND metadata_json = ?
	`), entryID, sessionID, metadataJSON)
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
	return tx.Commit()
}

func (r *sqliteRepository) DeleteAllBySession(ctx context.Context, sessionID string) (int, error) {
	unlock := r.withSessionLock(sessionID)
	defer unlock()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delete all queued tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	candidates, err := cancellationCandidates(ctx, tx, sessionID)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, candidate := range candidates {
		res, err := tx.ExecContext(ctx, r.db.Rebind(`
			DELETE FROM queued_messages
			WHERE id = ? AND session_id = ? AND metadata_json = ?
		`), candidate.id, sessionID, candidate.metadataJSON)
		if err != nil {
			return 0, fmt.Errorf("delete queued candidate: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("delete queued candidate rows affected: %w", err)
		}
		removed += int(affected)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return removed, nil
}

type cancellationCandidate struct {
	id           string
	metadataJSON string
}

func cancellationCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) ([]cancellationCandidate, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`
		SELECT id, metadata_json FROM queued_messages WHERE session_id = ?
	`), sessionID)
	if err != nil {
		return nil, fmt.Errorf("list queued cancellation candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	candidates := make([]cancellationCandidate, 0)
	for rows.Next() {
		var candidate cancellationCandidate
		if err := rows.Scan(&candidate.id, &candidate.metadataJSON); err != nil {
			return nil, fmt.Errorf("scan queued cancellation candidate: %w", err)
		}
		reserved, err := isReservedMetadataJSON(candidate.metadataJSON)
		if err != nil {
			return nil, err
		}
		if !reserved {
			candidates = append(candidates, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued cancellation candidates: %w", err)
	}
	return candidates, nil
}

func isReservedMetadataJSON(metadataJSON string) (bool, error) {
	metadata := make(map[string]interface{})
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return false, fmt.Errorf("unmarshal cancellation metadata: %w", err)
		}
	}
	return (&QueuedMessage{Metadata: metadata}).IsReservedInFlight(), nil
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

func (r *sqliteRepository) ReplaceSession(ctx context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace session tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM queued_messages WHERE session_id = ?`), sessionID); err != nil {
		return fmt.Errorf("clear queued messages: %w", err)
	}
	for _, entry := range entries {
		attachmentsJSON, err := marshalAttachments(entry.Attachments)
		if err != nil {
			return err
		}
		metadataJSON, err := marshalMetadata(entry.Metadata)
		if err != nil {
			return err
		}
		queuedAt := entry.QueuedAt
		if queuedAt.IsZero() {
			queuedAt = time.Now().UTC()
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO queued_messages
				(id, session_id, task_id, position, content, model, plan_mode, attachments_json, metadata_json, queued_at, queued_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`),
			entry.ID, sessionID, entry.TaskID, entry.Position, entry.Content, entry.Model,
			boolToInt(entry.PlanMode), attachmentsJSON, metadataJSON, queuedAt, entry.QueuedBy,
		); err != nil {
			return fmt.Errorf("restore queued message: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM pending_moves WHERE session_id = ?`), sessionID); err != nil {
		return fmt.Errorf("clear pending move: %w", err)
	}
	if pendingMove != nil {
		queuedAt := pendingMove.QueuedAt
		if queuedAt.IsZero() {
			queuedAt = time.Now().UTC()
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO pending_moves (id, session_id, task_id, workflow_id, workflow_step_id, step_position, queued_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`),
			uuid.New().String(), sessionID, pendingMove.TaskID, pendingMove.WorkflowID,
			pendingMove.WorkflowStepID, pendingMove.Position, queuedAt,
		); err != nil {
			return fmt.Errorf("restore pending move: %w", err)
		}
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

func (r *sqliteRepository) GetPendingMove(ctx context.Context, sessionID string) (*PendingMove, error) {
	var (
		taskID, workflowID, workflowStepID string
		position                           int
		queuedAt                           time.Time
	)
	if err := r.ro.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT task_id, workflow_id, workflow_step_id, step_position, queued_at
		FROM pending_moves WHERE session_id = ?
	`), sessionID).Scan(&taskID, &workflowID, &workflowStepID, &position, &queuedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pending move: %w", err)
	}
	return &PendingMove{
		TaskID:         taskID,
		WorkflowID:     workflowID,
		WorkflowStepID: workflowStepID,
		Position:       position,
		QueuedAt:       queuedAt,
	}, nil
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

func (r *sqliteRepository) findCoalesced(ctx context.Context, tx *sqlx.Tx, sessionID, queuedBy, coalesceKey string) (*QueuedMessage, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		       attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages
		WHERE session_id = ? AND queued_by = ?
		ORDER BY position ASC
	`), sessionID, queuedBy)
	if err != nil {
		return nil, fmt.Errorf("scan coalesced queued: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		msg, err := scanQueuedRow(rows)
		if err != nil {
			return nil, err
		}
		if metadataString(msg.Metadata, MetadataCoalesceKey) == coalesceKey {
			return msg, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

// scanQueuedRow scans a single queued_messages row from any sqlx-compatible row source.
func scanQueuedRow(scanner interface{ Scan(dest ...any) error }) (*QueuedMessage, error) {
	msg, _, err := scanQueuedRowWithMetadataJSON(scanner)
	return msg, err
}

func scanQueuedRowWithMetadataJSON(
	scanner interface{ Scan(dest ...any) error },
) (*QueuedMessage, string, error) {
	var (
		msg                       QueuedMessage
		planModeInt               int
		attachmentsJSON, metaJSON string
	)
	if err := scanner.Scan(
		&msg.ID, &msg.SessionID, &msg.TaskID, &msg.Position, &msg.Content, &msg.Model,
		&planModeInt, &attachmentsJSON, &metaJSON, &msg.QueuedAt, &msg.QueuedBy,
	); err != nil {
		return nil, "", err
	}
	msg.PlanMode = planModeInt != 0
	if attachmentsJSON != "" && attachmentsJSON != "[]" {
		if err := json.Unmarshal([]byte(attachmentsJSON), &msg.Attachments); err != nil {
			return nil, "", fmt.Errorf("unmarshal attachments: %w", err)
		}
	}
	if metaJSON != "" && metaJSON != "{}" {
		if err := json.Unmarshal([]byte(metaJSON), &msg.Metadata); err != nil {
			return nil, "", fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &msg, metaJSON, nil
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

func metadataString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	value, _ := meta[key].(string)
	return value
}
