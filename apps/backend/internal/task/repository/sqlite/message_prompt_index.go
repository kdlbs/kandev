package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// promptKeyLayout is the exact byte form of the normalized-microsecond
// ordering key: `YYYY-MM-DD HH:MM:SS.ffffff` (space separator, zero-padded
// 6-digit fraction, no zone suffix, UTC). SQLite rows compare against this
// text; the same literal is cast to `timestamp` on PostgreSQL.
const promptKeyLayout = "2006-01-02 15:04:05.000000"

// promptIndexLeadWindow bounded how far a live create could advance its
// ordering timestamp; it was removed when prompt ordinals moved to a durable
// per-session sequence that no longer depends on timestamp comparisons. See
// the migration history for the derived-ordinal predecessor design.

// ErrMessageTimestampNotAfterNewest is returned when an explicit user-message
// timestamp is not strictly after the session's newest user message. Explicit
// imports preserve only valid strictly-after-max timestamps; reorderings of
// an existing session's history are rejected.
var ErrMessageTimestampNotAfterNewest = errors.New("explicit message timestamp is not strictly after the session's newest user message")

// messageBoundaryExecer is the subset of *sqlx.DB / *sqlx.Tx the per-session
// user-message create boundary needs: reads and the insert under one
// transaction.
type messageBoundaryExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	Rebind(query string) string
}

// formatPromptKey renders a UTC time in the normalized prompt-order key layout used by the per-session ordering boundary and pagination cursors.
func formatPromptKey(t time.Time) string {
	return t.UTC().Format(promptKeyLayout)
}

// parsePromptKey parses a normalized prompt-order key back into a UTC time.
func parsePromptKey(s string) (time.Time, error) {
	return time.Parse(promptKeyLayout, s)
}

// resolvePromptOrderingTime applies the create-boundary timestamp rules for a
// user message. Live creates (zero timestamp) keep the session's ordering
// monotonic: a colliding or backward key is clamped forward by one microsecond
// tick. Clamping is unbounded on purpose — the prompt ordinal comes from the
// durable per-session sequence, not this timestamp, so a backward clock
// correction can never block new prompts; the timestamp only keeps pagination
// order stable. Explicit timestamps are preserved only when strictly after
// the session's newest user message. maxKey is the session's newest
// user-message normalized key (NULL when the session has no user rows).
func resolvePromptOrderingTime(live bool, created time.Time, maxKey sql.NullString) (time.Time, error) {
	if !maxKey.Valid || maxKey.String == "" {
		return created, nil
	}
	newKey := formatPromptKey(created)
	if live {
		if newKey > maxKey.String {
			return created, nil
		}
		advanced, err := parsePromptKey(maxKey.String)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse session max user-message key %q: %w", maxKey.String, err)
		}
		return advanced.Add(time.Microsecond), nil
	}
	if newKey <= maxKey.String {
		return time.Time{}, fmt.Errorf(
			"%w: %s is not strictly after %s",
			ErrMessageTimestampNotAfterNewest, formatPromptKey(created), maxKey.String,
		)
	}
	return created, nil
}

// allocatePromptSeq advances the session's durable prompt-sequence counter by
// one and returns the new ordinal. It runs inside the per-session create
// boundary, which serializes all user-message creates for the session (the
// advisory session lock on PostgreSQL, the single-writer pool on SQLite), so
// the read-increment-write is atomic. Deletion never decrements the counter:
// published ordinals stay stable, and a deleted prompt's ordinal is never
// reused.
func (r *Repository) allocatePromptSeq(ctx context.Context, execer messageBoundaryExecer, sessionID string) (int, error) {
	var lastSeq int
	err := execer.QueryRowContext(ctx, execer.Rebind(
		`SELECT last_seq FROM task_session_prompt_seq WHERE task_session_id = ?`,
	), sessionID).Scan(&lastSeq)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("read session prompt sequence: %w", err)
	}
	seq := lastSeq + 1
	if _, err := execer.ExecContext(ctx, execer.Rebind(`
		INSERT INTO task_session_prompt_seq (task_session_id, last_seq) VALUES (?, ?)
		ON CONFLICT(task_session_id) DO UPDATE SET last_seq = excluded.last_seq
	`), sessionID, seq); err != nil {
		return 0, fmt.Errorf("advance session prompt sequence: %w", err)
	}
	return seq, nil
}

// HasUserPromptHistory reports whether a session has accepted or reserved a
// user prompt. The durable sequence is not decremented when a message is
// deleted, so this remains true after the transcript row is removed. An empty
// sequence row is the reservation marker written by
// ClaimInitialPromptFallback; it is also history for the one-time fallback
// decision. The single-row lookup is bounded by the session primary key and
// does not scan message history.
func (r *Repository) HasUserPromptHistory(ctx context.Context, sessionID string) (bool, error) {
	var marker int
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT 1 FROM task_session_prompt_seq WHERE task_session_id = ?`,
	), sessionID).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read session prompt history: %w", err)
	}
	return marker == 1, nil
}

// ClaimInitialPromptFallback atomically reserves the first prompt slot for an
// empty workflow-step task-description fallback. User-message creation uses
// the same per-session write boundary, so a direct prompt that commits first
// wins the slot and a concurrent fallback cannot also qualify. A successful
// claim writes a zero-valued reservation marker; the later visible fallback
// message then receives prompt ordinal 1 without making the reservation itself
// an empty transcript row.
func (r *Repository) ClaimInitialPromptFallback(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("session ID is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin initial prompt fallback claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_prompt_seq (task_session_id, last_seq)
		VALUES (?, 0)
		ON CONFLICT(task_session_id) DO NOTHING
	`), sessionID)
	if err != nil {
		return false, fmt.Errorf("claim initial prompt fallback: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit initial prompt fallback claim: %w", err)
	}
	claimed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read initial prompt fallback claim result: %w", err)
	}
	return claimed > 0, nil
}

// readSessionMaxUserKey returns the session's newest user-message normalized
// key in the exact key layout, and whether any user row exists. On SQLite the
// key expression yields the text directly; on PostgreSQL the native TIMESTAMP
// is scanned and formatted (scanning a timestamp into a string yields RFC3339,
// which the key parse and lexicographic comparison do not understand).
func (r *Repository) readSessionMaxUserKey(
	ctx context.Context,
	execer messageBoundaryExecer,
	driver, nm, sessionID string,
) (string, bool, error) {
	query := fmt.Sprintf(
		"SELECT max(%s) FROM task_session_messages WHERE task_session_id = ? AND author_type = ?",
		nm,
	)
	args := []interface{}{sessionID, string(models.MessageAuthorUser)}
	if dialect.IsPostgres(driver) {
		var maxKey sql.NullTime
		if err := execer.QueryRowContext(ctx, execer.Rebind(query), args...).Scan(&maxKey); err != nil {
			return "", false, fmt.Errorf("read session max user-message key: %w", err)
		}
		if !maxKey.Valid {
			return "", false, nil
		}
		return formatPromptKey(maxKey.Time), true, nil
	}
	var maxKey sql.NullString
	if err := execer.QueryRowContext(ctx, execer.Rebind(query), args...).Scan(&maxKey); err != nil {
		return "", false, fmt.Errorf("read session max user-message key: %w", err)
	}
	return maxKey.String, maxKey.Valid && maxKey.String != "", nil
}

// createUserMessageWithBoundary persists a user message with its
// prompt-ordering timestamp and durable prompt ordinal assigned inside the
// per-session write boundary (a transaction on both dialects; PostgreSQL
// additionally takes the session turn-write advisory lock, and SQLite's
// single-writer pool provides the same serialization). The caller-visible
// CreatedAt and PromptIndex fields are only committed on success, so a failed
// attempt (rejected explicit timestamp, busy retry, concurrent winner) leaves
// the retried model untouched.
func (r *Repository) createUserMessageWithBoundary(
	ctx context.Context,
	message *models.Message,
	requestsInput int,
	messageType, metadataJSON string,
) error {
	driver := r.db.DriverName()
	nm := dialect.NormalizedMicrosecond(driver, "created_at")
	return r.executeBoundaryTransaction(ctx, message, requestsInput, messageType, metadataJSON, driver, nm)
}

// executeBoundaryTransaction runs one per-session write boundary: begin a
// transaction, take the session turn-write lock (advisory on PostgreSQL, no-op
// on SQLite), resolve the prompt-ordering timestamp and ordinal, and insert
// the user message row. Any failure restores the message's caller-visible
// CreatedAt/UpdatedAt/PromptIndex so a retry re-resolves from scratch.
func (r *Repository) executeBoundaryTransaction(
	ctx context.Context,
	message *models.Message,
	requestsInput int,
	messageType, metadataJSON, driver, nm string,
) error {
	origCreatedAt := message.CreatedAt
	origUpdatedAt := message.UpdatedAt
	origPromptIndex := message.PromptIndex
	restore := func() {
		message.CreatedAt = origCreatedAt
		message.UpdatedAt = origUpdatedAt
		message.PromptIndex = origPromptIndex
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user message creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, driver, message.TaskSessionID); err != nil {
		return err
	}

	now := r.nowUTC()
	created := message.CreatedAt
	if created.IsZero() {
		created = now
	}

	maxKeyStr, maxKeyValid, err := r.readSessionMaxUserKey(ctx, tx, driver, nm, message.TaskSessionID)
	if err != nil {
		return err
	}

	created, err = resolvePromptOrderingTime(message.CreatedAt.IsZero(), created, sql.NullString{String: maxKeyStr, Valid: maxKeyValid})
	if err != nil {
		return err
	}

	// The ordinal comes from the session's durable sequence counter (inside
	// this serialized boundary), never from the timestamp or the remaining
	// rows: deletion of an earlier prompt cannot renumber this row, and a
	// backward clock correction cannot block allocation.
	seq, err := r.allocatePromptSeq(ctx, tx, message.TaskSessionID)
	if err != nil {
		return err
	}

	message.CreatedAt = created
	message.PromptIndex = seq
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = created
	}
	if err := r.insertMessageRow(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		restore()
		return err
	}
	if err := tx.Commit(); err != nil {
		restore()
		return fmt.Errorf("commit user message creation: %w", err)
	}
	return nil
}

// GetMessageWithPromptIndex retrieves a message by ID with its prompt_index
// read from the durable per-session prompt sequence (prompt_seq), which is
// allocated at creation inside the write boundary and survives message
// deletion. Non-user rows return zero. This is the 13-column read used by the
// idempotent WS replay/response path and user update-event publication;
// hot-path GetMessage stays on the 12-column scan.
func (r *Repository) GetMessageWithPromptIndex(ctx context.Context, id string) (*models.Message, error) {
	message := &models.Message{}
	var requestsInput int
	var messageType string
	var metadataJSON string
	query := `
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id,
		       content, requests_input, type, metadata, created_at, updated_at,
		       CASE WHEN author_type = 'user' THEN prompt_seq ELSE 0 END AS prompt_index
		FROM task_session_messages
		WHERE id = ?`
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), id).Scan(
		&message.ID, &message.TaskSessionID, &message.TaskID, &message.TurnID, &message.AuthorType, &message.AuthorID,
		&message.Content, &requestsInput, &messageType, &metadataJSON, &message.CreatedAt, &message.UpdatedAt,
		&message.PromptIndex,
	)
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
