package messagequeue

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

func (r *sqliteRepository) SetSessionPolicyAndCompact(ctx context.Context, identity QueueSessionIdentity, change SessionPolicyChange, global GlobalQueuePolicy) (SessionPolicyResult, error) {
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return SessionPolicyResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActiveTaskTx(ctx, tx, identity.TaskID); err != nil {
		return SessionPolicyResult{}, err
	}
	if err := r.lockSessionTx(ctx, tx, identity.SessionID); err != nil {
		return SessionPolicyResult{}, err
	}
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		return SessionPolicyResult{}, err
	}
	before, err := r.resolveSessionPolicyTx(ctx, tx, identity, global)
	if err != nil {
		return SessionPolicyResult{}, err
	}
	if err := r.writeSessionPolicyTx(ctx, tx, identity, change); err != nil {
		return SessionPolicyResult{}, err
	}
	result, err := r.resolveSessionPolicyTx(ctx, tx, identity, global)
	if err != nil {
		return SessionPolicyResult{}, err
	}
	if !before.AutoMerge.Enabled && result.AutoMerge.Enabled {
		result.compactionRan = true
		result.CompactedCount, err = r.compactSessionQueueTx(ctx, tx, identity, change.Leased)
		if err != nil {
			return SessionPolicyResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SessionPolicyResult{}, err
	}
	return result, nil
}

func (r *sqliteRepository) resolveSessionPolicyTx(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity, global GlobalQueuePolicy) (SessionPolicyResult, error) {
	var mergeEnabled sql.NullInt64
	var mergeRevision int64
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT auto_merge_override,auto_merge_revision
 FROM queue_session_state WHERE session_id = ? AND session_incarnation_id = ?`), identity.SessionID, identity.SessionIncarnationID).Scan(&mergeEnabled, &mergeRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SessionPolicyResult{}, err
	}
	var merge *AutoMergeOverride
	if mergeEnabled.Valid {
		merge = &AutoMergeOverride{Enabled: mergeEnabled.Int64 != 0, Revision: mergeRevision}
	}
	return SessionPolicyResult{AutoMerge: resolveAutoMergeOverride(merge, global.AutoMerge)}, nil
}

func (r *sqliteRepository) writeSessionPolicyTx(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity, change SessionPolicyChange) error {
	query := `INSERT INTO queue_session_state (session_id,session_incarnation_id,auto_merge_override,auto_merge_revision) VALUES (?,?,?,1)
 ON CONFLICT(session_id) DO UPDATE SET session_incarnation_id = excluded.session_incarnation_id,
 auto_merge_override = excluded.auto_merge_override,auto_merge_revision = queue_session_state.auto_merge_revision + 1
 WHERE queue_session_state.session_incarnation_id = '' OR queue_session_state.session_incarnation_id = excluded.session_incarnation_id`
	result, err := tx.ExecContext(ctx, tx.Rebind(query), identity.SessionID, identity.SessionIncarnationID, boolToInt(change.Enabled))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrSessionIdentityMismatch
	}
	return nil
}
