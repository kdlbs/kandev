package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

// AcceptTurnRepositoryComparison stores the accepted comparison once. A retry
// that repeats the same accepted pair reuses it; a different pair is rejected.
func (r *Repository) AcceptTurnRepositoryComparison(
	ctx context.Context,
	changeSetID, repositoryChangeID string,
	comparison models.TurnRepositoryComparison,
) (bool, error) {
	if err := validateTurnRepositoryComparison(changeSetID, repositoryChangeID, comparison); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := lockTurnChangeSet(ctx, tx, r.db, changeSetID); err != nil {
		return false, turnChangeSetLockError(err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes
		   SET comparison_version = ?, comparison_basis = ?, comparison_reason = ?,
		       effective_start_commit_oid = ?, effective_start_tree_oid = ?,
		       effective_end_commit_oid = ?, effective_end_tree_oid = ?, comparison_ref = ?, updated_at = ?
		 WHERE id = ? AND change_set_id = ? AND end_commit_oid <> '' AND comparison_basis = ''
	`), comparison.Version, comparison.Basis, comparison.Reason,
		comparison.EffectiveStartCommitOID, comparison.EffectiveStartTreeOID,
		comparison.EffectiveEndCommitOID, comparison.EffectiveEndTreeOID, comparison.ComparisonRef, time.Now().UTC(),
		repositoryChangeID, changeSetID)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if updated == 1 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	existing, found, err := readAcceptedTurnRepositoryComparison(ctx, tx, r.db, changeSetID, repositoryChangeID)
	if err != nil {
		return false, err
	}
	if !found || !sameTurnRepositoryComparison(existing, comparison) {
		return false, nil
	}
	return true, nil
}

func validateTurnRepositoryComparison(
	changeSetID, repositoryChangeID string,
	comparison models.TurnRepositoryComparison,
) error {
	if changeSetID == "" || repositoryChangeID == "" || !comparison.Basis.Valid() || !comparison.Reason.Valid() {
		return fmt.Errorf("accept turn repository comparison: comparison identity is required")
	}
	if comparison.Basis != turnchanges.ComparisonHistoryUnavailable &&
		hasEmptyValue(comparison.EffectiveStartCommitOID, comparison.EffectiveStartTreeOID,
			comparison.EffectiveEndCommitOID, comparison.EffectiveEndTreeOID) {
		return fmt.Errorf("accept turn repository comparison: effective endpoints are required")
	}
	return nil
}

func readAcceptedTurnRepositoryComparison(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSetID, repositoryChangeID string,
) (models.TurnRepositoryComparison, bool, error) {
	comparison := models.TurnRepositoryComparison{}
	err := tx.QueryRowxContext(ctx, db.Rebind(`
		SELECT comparison_version, comparison_basis, comparison_reason,
			effective_start_commit_oid, effective_start_tree_oid, effective_end_commit_oid,
			effective_end_tree_oid, comparison_ref
		FROM turn_repository_changes WHERE id = ? AND change_set_id = ?
	`), repositoryChangeID, changeSetID).Scan(
		&comparison.Version, &comparison.Basis, &comparison.Reason,
		&comparison.EffectiveStartCommitOID, &comparison.EffectiveStartTreeOID,
		&comparison.EffectiveEndCommitOID, &comparison.EffectiveEndTreeOID, &comparison.ComparisonRef,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return comparison, false, nil
	}
	if err != nil {
		return comparison, false, err
	}
	return comparison, true, nil
}

func sameTurnRepositoryComparison(left, right models.TurnRepositoryComparison) bool {
	return left.Version == right.Version && left.Basis == right.Basis && left.Reason == right.Reason &&
		left.EffectiveStartCommitOID == right.EffectiveStartCommitOID &&
		left.EffectiveStartTreeOID == right.EffectiveStartTreeOID &&
		left.EffectiveEndCommitOID == right.EffectiveEndCommitOID &&
		left.EffectiveEndTreeOID == right.EffectiveEndTreeOID &&
		left.ComparisonRef == right.ComparisonRef
}

// ListTurnChangeWriterIntervals returns canonical, half-open writing intervals
// for one admitted checkout, ordered by a stable cursor.
func (r *Repository) ListTurnChangeWriterIntervals(
	ctx context.Context,
	identity models.TurnChangeCheckoutIdentity,
	after string,
	limit int,
) ([]models.TurnChangeWriterInterval, error) {
	if identity.CheckoutID == "" && identity.EnvironmentRepoID == "" && identity.WorktreeID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	afterChangeSetID, afterCheckoutID := after, ""
	if separator := strings.IndexByte(after, 0); separator >= 0 {
		afterChangeSetID, afterCheckoutID = after[:separator], after[separator+1:]
	}
	var intervals []models.TurnChangeWriterInterval
	err := r.ro.SelectContext(ctx, &intervals, r.ro.Rebind(`
		SELECT rc.id AS repository_change_id, rc.change_set_id, rc.checkout_id, rc.environment_repo_id,
			rc.worktree_id, cs.turn_id, rc.start_captured_at, rc.end_captured_at,
			cs.terminal_capture_started_at, cs.terminal_at, turn_row.completed_at AS turn_completed_at,
			cs.runtime_execution_id, cs.startup_attempt_id, cs.prompt_generation,
			cs.overlap_intervals_json, cs.revision
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		LEFT JOIN task_session_turns turn_row ON turn_row.id = cs.turn_id
		WHERE rc.start_captured_at IS NOT NULL
		  AND (
		    (? <> '' AND rc.worktree_id = ?) OR
		    ((? = '' OR rc.worktree_id = '') AND ? <> '' AND rc.environment_repo_id = ?) OR
		    ((? = '' OR rc.worktree_id = '') AND (? = '' OR rc.environment_repo_id = '') AND rc.checkout_id = ?)
		  )
		  AND (rc.change_set_id > ? OR (rc.change_set_id = ? AND rc.checkout_id > ?))
		ORDER BY rc.change_set_id, rc.checkout_id
		LIMIT ?
	`), identity.WorktreeID, identity.WorktreeID,
		identity.WorktreeID, identity.EnvironmentRepoID, identity.EnvironmentRepoID,
		identity.WorktreeID, identity.EnvironmentRepoID, identity.CheckoutID,
		afterChangeSetID, afterChangeSetID, afterCheckoutID, limit)
	if err != nil {
		return nil, err
	}
	return intervals, nil
}

// MarkTurnRepositoryComparisonReady publishes a comparison only after its file
// inventory is durable, so restart recovery can reuse both.
func (r *Repository) MarkTurnRepositoryComparisonReady(
	ctx context.Context,
	changeSetID, repositoryChangeID string,
	complete bool,
	reason models.TurnChangeReason,
) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes
		   SET availability = ?, reason = ?, enumeration_complete = ?, comparison_complete = ?,
		       export_state = CASE WHEN content_complete THEN export_state ELSE ? END,
		       export_reason = CASE WHEN content_complete THEN export_reason ELSE '' END,
		       updated_at = ?
		 WHERE id = ? AND change_set_id = ? AND end_commit_oid <> '' AND comparison_basis <> ''
	`), models.TurnChangeAvailabilityReady, reason, complete, complete,
		models.TurnChangeExportQueued, time.Now().UTC(), repositoryChangeID, changeSetID)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return updated == 1, nil
}
