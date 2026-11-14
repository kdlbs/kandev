package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/usagehistory"
)

// The ledger's task and event identity select the accounting copy. Ownership
// remains the copy's captured scope, even after the live workspace moves.
const retainedNativePreservationCopy = `SELECT 1 FROM session_usage_buckets b
	JOIN usage_scopes s ON s.scope_id = b.scope_id AND s.workspace_id = b.workspace_id
	JOIN usage_subjects u ON u.subject_id = b.subject_id AND u.scope_id = b.scope_id AND u.task_id = b.task_id
	JOIN usage_execution_bindings x ON x.binding_id = b.usage_binding_id
		AND x.scope_id = b.scope_id AND x.subject_id = b.subject_id AND x.workspace_id = b.workspace_id
		AND x.task_id = u.task_id AND x.task_incarnation_id = u.task_incarnation_id
		AND x.session_id = u.session_id AND x.session_incarnation_id = u.session_incarnation_id
	WHERE b.source = 'native' AND b.source_record_id = e.usage_event_id AND b.task_id = e.task_id
		AND u.task_incarnation_id = ? AND COALESCE(b.session_id, '') = u.session_id
		AND (COALESCE(e.session_id, '') = '' OR u.session_id = e.session_id)
		AND (? = '' OR u.session_incarnation_id = ?)`

func (r *Repository) nativeUsageGapAcceptedTx(ctx context.Context, tx *sqlx.Tx, usageEventID, taskID string) (bool, error) {
	complete, err := r.usageSourceAdoptionCompleteTx(ctx, tx)
	if err != nil || !complete {
		return false, err
	}
	var accepted bool
	err = tx.GetContext(ctx, &accepted, r.db.Rebind(`SELECT EXISTS (
		SELECT 1 FROM usage_history_migration_gaps g WHERE g.source_key = 'native' AND g.task_id = ?
		AND (g.record_key = ? OR g.record_key = (
			SELECT CAST(id AS TEXT) FROM task_usage_events WHERE usage_event_id = ? AND task_id = ?)))`),
		taskID, usageEventID, usageEventID, taskID)
	return accepted, err
}

func (r *Repository) preserveNativeUsageRowTx(ctx context.Context, tx *sqlx.Tx, target usagePreservationTarget, native usagehistory.NativeUsage) error {
	if err := r.RetainNativeUsageTx(ctx, tx, native); err != nil {
		return err
	}
	// Migration can record a scope gap without copying amounts. Destructive
	// preservation can advance only after an accepted copy exists in this Tx.
	var retained bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
		SELECT 1 FROM task_usage_events e WHERE e.usage_event_id = ? AND e.task_id = ?
		AND EXISTS (`+retainedNativePreservationCopy+`))`), native.UsageEventID, target.TaskID,
		target.TaskIncarnationID, target.SessionID, target.SessionIncarnationID).Scan(&retained)
	if err != nil {
		return fmt.Errorf("verify retained native usage copy: %w", err)
	}
	if !retained {
		acceptedGap, err := r.nativeUsageGapAcceptedTx(ctx, tx, native.UsageEventID, target.TaskID)
		if err != nil {
			return fmt.Errorf("verify accepted native usage gap: %w", err)
		}
		if acceptedGap {
			return nil
		}
		return unpreservedNativeUsageError(native.UsageEventID)
	}
	return nil
}

func (r *Repository) requireNativeUsagePreservedTx(ctx context.Context, tx *sqlx.Tx, target usagePreservationTarget) error {
	if !r.usageHistoryIsEnabled() {
		return nil
	}
	query := `SELECT e.usage_event_id FROM task_usage_events e WHERE e.task_id = ?` +
		sessionPredicateFor("e.session_id", target.SessionID) + ` AND NOT EXISTS (` + retainedNativePreservationCopy + `)`
	args := []any{target.TaskID}
	if target.SessionID != "" {
		args = append(args, target.SessionID)
	}
	args = append(args, target.TaskIncarnationID, target.SessionID, target.SessionIncarnationID)
	var eventID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query+` LIMIT 1`), args...).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("verify native usage before destructive cleanup: %w", err)
	}
	acceptedGap, err := r.nativeUsageGapAcceptedTx(ctx, tx, eventID, target.TaskID)
	if err != nil {
		return fmt.Errorf("verify accepted native usage gap: %w", err)
	}
	if !acceptedGap {
		return unpreservedNativeUsageError(eventID)
	}
	query += ` AND NOT EXISTS (SELECT 1 FROM usage_history_migration_gaps g
		WHERE g.source_key = 'native' AND g.task_id = e.task_id
		AND (g.record_key = e.usage_event_id OR g.record_key = CAST(e.id AS TEXT))) LIMIT 1`
	var unacceptedEventID string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(query), args...).Scan(&unacceptedEventID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("verify native usage gaps before destructive cleanup: %w", err)
	}
	return unpreservedNativeUsageError(unacceptedEventID)
}

func unpreservedNativeUsageError(eventID string) error {
	return fmt.Errorf("native usage event %q has no accepted retained copy; destructive cleanup remains fenced", eventID)
}
