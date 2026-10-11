package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *sqliteRepository) RecoveryRead(ctx context.Context, identity QueueSessionIdentity) (RepositorySnapshot, error) {
	snapshots, err := r.RecoveryReadBatch(ctx, []QueueSessionIdentity{identity})
	if err != nil {
		return RepositorySnapshot{}, err
	}
	return snapshots[0], nil
}

func (r *sqliteRepository) RecoveryReadBatch(ctx context.Context, identities []QueueSessionIdentity) ([]RepositorySnapshot, error) {
	if !r.tasksTablePresent {
		return nil, ErrSessionIdentityMismatch
	}
	if len(identities) > 256 {
		return nil, ErrRecoveryLimit
	}
	options := &sql.TxOptions{ReadOnly: true}
	if r.ro.DriverName() == postgresDriverName {
		options.Isolation = sql.LevelRepeatableRead
	}
	tx, err := r.ro.BeginTxx(ctx, options)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result := make([]RepositorySnapshot, 0, len(identities))
	total := 0
	for _, identity := range identities {
		if err := r.recoveryIdentity(ctx, tx, identity); err != nil {
			return nil, err
		}
		size, err := r.recoveryPreflight(ctx, tx, identity.SessionID)
		if err != nil {
			return nil, err
		}
		total += size
		if total > 32*1024*1024 {
			return nil, ErrRecoveryLimit
		}
	}
	for _, identity := range identities {
		snapshot, err := r.recoveryReadTx(ctx, tx, identity)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *sqliteRepository) recoveryReadTx(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity) (RepositorySnapshot, error) {
	if identity.TaskID == "" || identity.SessionID == "" || identity.SessionIncarnationID == "" {
		return RepositorySnapshot{}, ErrSessionIdentityMismatch
	}
	if err := r.recoveryIdentity(ctx, tx, identity); err != nil {
		return RepositorySnapshot{}, err
	}
	entries, err := r.recoveryEntries(ctx, tx, identity)
	if err != nil {
		return RepositorySnapshot{}, err
	}
	if _, err := recoveryEntriesBytes(entries); err != nil {
		return RepositorySnapshot{}, err
	}
	autoRun := 1
	err = tx.GetContext(ctx, &autoRun, r.ro.Rebind(`SELECT auto_run FROM queue_session_state WHERE session_id = ? AND (session_incarnation_id = ? OR session_incarnation_id = '')`), identity.SessionID, identity.SessionIncarnationID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RepositorySnapshot{}, err
	}
	return RepositorySnapshot{Entries: entries, AutoRun: autoRun != 0}, nil
}

func (r *sqliteRepository) recoveryIdentity(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity) error {
	var incarnation string
	err := tx.GetContext(ctx, &incarnation, r.ro.Rebind(`
		SELECT s.queue_incarnation_id FROM task_sessions s
		JOIN tasks t ON t.id = s.task_id
		WHERE s.id = ? AND s.task_id = ? AND t.archived_at IS NULL
	`), identity.SessionID, identity.TaskID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && incarnation != identity.SessionIncarnationID {
		return ErrSessionIdentityMismatch
	}
	return err
}

func (r *sqliteRepository) recoveryPreflight(ctx context.Context, tx *sqlx.Tx, sessionID string) (int, error) {
	columns := []string{"id", "task_id", "session_id", "content", "model", "queued_by", "attachments_json", "metadata_json"}
	lengths := make([]string, 0, len(columns))
	for _, column := range columns {
		lengths = append(lengths, "COALESCE("+dialect.ByteLength(r.ro.DriverName(), column)+", 0)")
	}
	query := fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(payload_bytes), 0) FROM
		(SELECT %s AS payload_bytes FROM queued_messages WHERE session_id = ? ORDER BY position LIMIT %d) bounded`, strings.Join(lengths, "+"), MaxRecoveryEntries+1)
	var count, size int64
	if err := tx.QueryRowxContext(ctx, r.ro.Rebind(query), sessionID).Scan(&count, &size); err != nil {
		return 0, err
	}
	if count > MaxRecoveryEntries || size > MaxRecoverySourceBytes {
		return 0, ErrRecoveryLimit
	}
	return int(size), nil
}

func (r *sqliteRepository) recoveryEntries(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity) ([]QueuedMessage, error) {
	rows, err := tx.QueryxContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, task_id, position, content, model, plan_mode,
		attachments_json, metadata_json, queued_at, queued_by
		FROM queued_messages WHERE session_id = ? ORDER BY position ASC LIMIT ?
	`), identity.SessionID, MaxRecoveryEntries+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	entries := make([]QueuedMessage, 0)
	for rows.Next() {
		entry, err := scanQueuedRow(rows)
		if err != nil {
			return nil, err
		}
		if entry.TaskID != identity.TaskID || entry.SessionID != identity.SessionID {
			return nil, ErrSessionIdentityMismatch
		}
		entries = append(entries, *entry)
	}
	return entries, rows.Err()
}
