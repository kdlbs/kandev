package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

func (r *memoryRepository) GetAutoSortMergeOverride(_ context.Context, identity QueueSessionIdentity) (*AutoSortMergeOverride, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateIdentityLocked(identity); err != nil {
		return nil, err
	}
	override, ok := r.autoSortMergeOverrides[identity]
	if !ok {
		return nil, nil
	}
	return &override, nil
}

func (r *memoryRepository) SetAutoSortMergeOverride(_ context.Context, identity QueueSessionIdentity, enabled bool) (AutoSortMergeOverride, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return AutoSortMergeOverride{}, err
	}
	override := r.autoSortMergeOverrides[identity]
	override.Enabled = enabled
	override.Revision++
	r.autoSortMergeOverrides[identity] = override
	return override, nil
}

func (r *sqliteRepository) getAutoSortMergeOverrideTx(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity) (*AutoSortMergeOverride, error) {
	var enabled sql.NullInt64
	var revision int64
	err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT auto_sort_merge_override, auto_sort_merge_revision FROM queue_session_state WHERE session_id = ? AND session_incarnation_id = ?`), identity.SessionID, identity.SessionIncarnationID).Scan(&enabled, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get queue Auto-sort-merge override: %w", err)
	}
	if !enabled.Valid {
		return nil, nil
	}
	return &AutoSortMergeOverride{Enabled: enabled.Int64 != 0, Revision: revision}, nil
}

func (r *sqliteRepository) GetAutoSortMergeOverride(ctx context.Context, identity QueueSessionIdentity) (*AutoSortMergeOverride, error) {
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		return nil, err
	}
	return r.getAutoSortMergeOverrideTx(ctx, tx, identity)
}

func (r *sqliteRepository) SetAutoSortMergeOverride(ctx context.Context, identity QueueSessionIdentity, enabled bool) (AutoSortMergeOverride, error) {
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return AutoSortMergeOverride{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActiveTaskTx(ctx, tx, identity.TaskID); err != nil {
		return AutoSortMergeOverride{}, err
	}
	if err := r.lockSessionTx(ctx, tx, identity.SessionID); err != nil {
		return AutoSortMergeOverride{}, err
	}
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		return AutoSortMergeOverride{}, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
 INSERT INTO queue_session_state (session_id,session_incarnation_id,auto_sort_merge_override,auto_sort_merge_revision)
 VALUES (?,?,?,1)
 ON CONFLICT(session_id) DO UPDATE SET
 session_incarnation_id = excluded.session_incarnation_id,
 auto_sort_merge_override = excluded.auto_sort_merge_override,
 auto_sort_merge_revision = queue_session_state.auto_sort_merge_revision + 1
 WHERE queue_session_state.session_incarnation_id = '' OR queue_session_state.session_incarnation_id = excluded.session_incarnation_id
 `), identity.SessionID, identity.SessionIncarnationID, boolToInt(enabled)); err != nil {
		return AutoSortMergeOverride{}, err
	}
	override, err := r.getAutoSortMergeOverrideTx(ctx, tx, identity)
	if err != nil {
		return AutoSortMergeOverride{}, err
	}
	if override == nil {
		return AutoSortMergeOverride{}, ErrSessionIdentityMismatch
	}
	if err := tx.Commit(); err != nil {
		return AutoSortMergeOverride{}, err
	}
	return *override, nil
}

func (r *sqliteRepository) validateAutoSortMergePolicyTx(ctx context.Context, tx *sqlx.Tx, identity *QueueSessionIdentity, policy *AutoSortMergePolicy) error {
	if identity == nil || policy == nil {
		return nil
	}
	override, err := r.getAutoSortMergeOverrideTx(ctx, tx, *identity)
	if err != nil {
		return err
	}
	if policy.Source == AutoMergeSourceGlobal && override == nil {
		return nil
	}
	if policy.Source == AutoMergeSourceSession && override != nil && override.Enabled == policy.Enabled && override.Revision == policy.Revision {
		return nil
	}
	return ErrAutoMergePolicyChanged
}

func (r *memoryRepository) clearSortPolicyLocked(sessionID string) {
	for identity := range r.autoSortMergeOverrides {
		if identity.SessionID == sessionID {
			delete(r.autoSortMergeOverrides, identity)
		}
	}
}
