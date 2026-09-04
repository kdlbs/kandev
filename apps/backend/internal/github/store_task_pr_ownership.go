package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// TaskPROwnershipReconciler moves or detaches an old watch-sourced association
// when a session now resolves to a different workspace-group owner. The
// operation is kept in the store so the TaskPR row and its task-scoped
// automation state change together, without publishing an intermediate event.
type TaskPROwnershipReconciler interface {
	ReconcileTaskPROwnership(
		ctx context.Context, memberTaskID, ownerTaskID, repositoryID string, prNumber int,
	) (bool, error)
}

type taskPROwnerState int

const (
	taskPROwnerAbsent taskPROwnerState = iota
	taskPROwnerActive
	taskPROwnerDetached
)

// ReconcileTaskPROwnership repairs a legacy member-owned TaskPR for one exact
// watch-derived PR. If an active owner row exists, the member row becomes a
// detached tombstone. If no owner row exists, the row and its automation state
// move to the owner in one transaction. Explicit URL links are not changed.
// The bool reports whether an active member row was reconciled.
func (s *Store) ReconcileTaskPROwnership(
	ctx context.Context, memberTaskID, ownerTaskID, repositoryID string, prNumber int,
) (bool, error) {
	if !validTaskPROwnershipArgs(s, memberTaskID, ownerTaskID, prNumber) {
		return false, nil
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	memberID, source, found, err := findTaskPRMemberTx(ctx, tx, memberTaskID, repositoryID, prNumber)
	if err != nil {
		return false, err
	}
	if !found || source == TaskPRSourceURLLink {
		return false, nil
	}

	ownerState, err := findTaskPROwnerStateTx(ctx, tx, ownerTaskID, repositoryID, prNumber)
	if err != nil {
		return false, err
	}
	if ownerState == taskPROwnerActive {
		if err := reconcileExistingOwnerTaskPRTx(ctx, tx, memberID, memberTaskID, ownerTaskID, repositoryID, prNumber); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if ownerState == taskPROwnerDetached {
		// A detached owner tombstone is an explicit opt-out. Do not resurrect
		// it through a watch, but still remove the active duplicate member row.
		if err := detachTaskPROwnershipRowTx(ctx, tx, memberID, memberTaskID, repositoryID, prNumber); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}

	if err := moveTaskPROwnershipTx(ctx, tx, memberID, memberTaskID, ownerTaskID, repositoryID, prNumber); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func validTaskPROwnershipArgs(s *Store, memberTaskID, ownerTaskID string, prNumber int) bool {
	return s != nil && memberTaskID != "" && ownerTaskID != "" && memberTaskID != ownerTaskID && prNumber != 0
}

func findTaskPRMemberTx(
	ctx context.Context, tx *sqlx.Tx, memberTaskID, repositoryID string, prNumber int,
) (string, string, bool, error) {
	var memberID, source string
	err := tx.QueryRowxContext(ctx, `
		SELECT id, source
		FROM github_task_prs
		WHERE task_id = ? AND repository_id = ? AND pr_number = ? AND detached_at IS NULL
		LIMIT 1`, memberTaskID, repositoryID, prNumber).Scan(&memberID, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if source == TaskPRSourceURLLink {
		return memberID, source, true, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load member task PR: %w", err)
	}
	return memberID, source, true, nil
}

func reconcileExistingOwnerTaskPRTx(
	ctx context.Context, tx *sqlx.Tx, memberID, memberTaskID, ownerTaskID, repositoryID string, prNumber int,
) error {
	if err := copyTaskPROwnershipStateTx(ctx, tx, memberTaskID, ownerTaskID, repositoryID, prNumber); err != nil {
		return err
	}
	return detachTaskPROwnershipRowTx(ctx, tx, memberID, memberTaskID, repositoryID, prNumber)
}

func findTaskPROwnerStateTx(
	ctx context.Context, tx *sqlx.Tx, ownerTaskID, repositoryID string, prNumber int,
) (taskPROwnerState, error) {
	var ownerID string
	err := tx.QueryRowxContext(ctx, `
		SELECT id
		FROM github_task_prs
		WHERE task_id = ? AND repository_id = ? AND pr_number = ? AND detached_at IS NULL
		LIMIT 1`, ownerTaskID, repositoryID, prNumber).Scan(&ownerID)
	if err == nil {
		return taskPROwnerActive, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return taskPROwnerAbsent, fmt.Errorf("load owner task PR: %w", err)
	}
	err = tx.QueryRowxContext(ctx, `
		SELECT id
		FROM github_task_prs
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?
		LIMIT 1`, ownerTaskID, repositoryID, prNumber).Scan(&ownerID)
	if err == nil {
		return taskPROwnerDetached, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return taskPROwnerAbsent, nil
	}
	return taskPROwnerAbsent, fmt.Errorf("load owner task PR tombstone: %w", err)
}

func moveTaskPROwnershipTx(
	ctx context.Context, tx *sqlx.Tx, memberID, memberTaskID, ownerTaskID, repositoryID string, prNumber int,
) error {
	if err := copyTaskPROwnershipStateTx(ctx, tx, memberTaskID, ownerTaskID, repositoryID, prNumber); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE github_task_prs
		SET task_id = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND repository_id = ? AND pr_number = ? AND detached_at IS NULL`,
		ownerTaskID, time.Now().UTC(), memberID, memberTaskID, repositoryID, prNumber)
	if err != nil {
		return fmt.Errorf("move task PR to owner: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return fmt.Errorf("inspect moved task PR: %w", rowsErr)
	} else if rows != 1 {
		return fmt.Errorf("move task PR to owner: affected %d rows", rows)
	}
	return moveTaskPROwnershipStateTx(ctx, tx, memberTaskID, ownerTaskID, repositoryID, prNumber)
}

func detachTaskPROwnershipRowTx(
	ctx context.Context, tx *sqlx.Tx, memberID, memberTaskID, repositoryID string, prNumber int,
) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE github_task_prs
		SET detached_at = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND repository_id = ? AND pr_number = ? AND detached_at IS NULL`,
		time.Now().UTC(), time.Now().UTC(), memberID, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("detach duplicate task PR: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM github_task_pr_automation_options
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("remove duplicate task PR automation options: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM github_task_ci_pr_state
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("remove duplicate task PR automation state: %w", err)
	}
	return nil
}

func copyTaskPROwnershipStateTx(
	ctx context.Context, tx *sqlx.Tx, memberTaskID, ownerTaskID, repositoryID string, prNumber int,
) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO github_task_pr_automation_options (
			task_id, repository_id, pr_number, auto_fix_enabled, auto_merge_enabled,
			prompt_on_review_requested, prompt_on_merged, prompt_on_closed, created_at, updated_at
		)
		SELECT ?, repository_id, pr_number, auto_fix_enabled, auto_merge_enabled,
			prompt_on_review_requested, prompt_on_merged, prompt_on_closed, created_at, updated_at
		FROM github_task_pr_automation_options
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
		ownerTaskID, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("copy task PR automation options: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, last_fix_signature, last_fix_checkpoint_json,
			last_fix_enqueued_at, last_fix_session_id, auto_fix_round_count, auto_fix_exhausted_at,
			last_merge_signature, last_merge_attempt_at, last_merge_result, merge_retry_pending,
			last_queue_attempt_head_sha, last_queue_fix_event_id, last_queue_removal_cause,
			review_request_initialized, last_review_requested, last_observed_pr_state,
			last_lifecycle_event, last_lifecycle_prompt_at, last_lifecycle_session_id,
			last_error, last_error_kind, created_at, updated_at
		)
		SELECT ?, repository_id, pr_number, last_fix_signature, last_fix_checkpoint_json,
			last_fix_enqueued_at, last_fix_session_id, auto_fix_round_count, auto_fix_exhausted_at,
			last_merge_signature, last_merge_attempt_at, last_merge_result, merge_retry_pending,
			last_queue_attempt_head_sha, last_queue_fix_event_id, last_queue_removal_cause,
			review_request_initialized, last_review_requested, last_observed_pr_state,
			last_lifecycle_event, last_lifecycle_prompt_at, last_lifecycle_session_id,
			last_error, last_error_kind, created_at, updated_at
		FROM github_task_ci_pr_state
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
		ownerTaskID, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("copy task PR automation state: %w", err)
	}
	return nil
}

func moveTaskPROwnershipStateTx(
	ctx context.Context, tx *sqlx.Tx, memberTaskID, ownerTaskID, repositoryID string, prNumber int,
) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM github_task_pr_automation_options
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("remove moved task PR automation options: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM github_task_ci_pr_state
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`, memberTaskID, repositoryID, prNumber); err != nil {
		return fmt.Errorf("remove moved task PR automation state: %w", err)
	}
	return nil
}
