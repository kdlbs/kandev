package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) AdvanceTurnChangeSetPromptGeneration(
	ctx context.Context,
	changeSetID, executionID, startupAttemptID, taskEnvironmentID string,
	expectedRevision, generation int64,
) (bool, error) {
	if changeSetID == "" || executionID == "" || startupAttemptID == "" || taskEnvironmentID == "" || generation <= 0 {
		return false, fmt.Errorf("advance turn change generation: complete owner identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changeSet, err := lockTurnChangeSet(ctx, tx, r.db, changeSetID)
	if err != nil {
		return false, turnChangeSetLockError(err)
	}
	if !validTurnChangeGenerationAdvance(changeSet, executionID, startupAttemptID, taskEnvironmentID, expectedRevision, generation) {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets SET prompt_generation = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND revision = ? AND runtime_execution_id = ? AND startup_attempt_id = ?
			AND task_environment_id = ? AND prompt_generation < ? AND terminal_at IS NULL AND start_accepted = ?
	`), generation, time.Now().UTC(), changeSetID, expectedRevision, executionID, startupAttemptID, taskEnvironmentID, generation, true)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validTurnChangeGenerationAdvance(
	changeSet *models.TurnChangeSet,
	executionID, startupAttemptID, taskEnvironmentID string,
	expectedRevision, generation int64,
) bool {
	return changeSet.Revision == expectedRevision && changeSet.StartAccepted && changeSet.TerminalAt == nil &&
		changeSet.CaptureEnabled && changeSet.RuntimeExecutionID == executionID &&
		changeSet.StartupAttemptID == startupAttemptID && changeSet.TaskEnvironmentID == taskEnvironmentID &&
		generation > changeSet.PromptGeneration
}

func (r *Repository) ClaimTurnChangeSetTerminal(
	ctx context.Context,
	changeSetID string,
	expectedRevision int64,
	claim models.TurnChangeSetTerminalClaim,
) (bool, error) {
	claim.At = claim.At.UTC()
	if changeSetID == "" || claim.At.IsZero() || claim.PromptGeneration <= 0 || claim.TaskEnvironmentID == "" {
		return false, fmt.Errorf("claim terminal turn change: terminal identity and timestamp are required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changeSet, err := lockTurnChangeSet(ctx, tx, r.db, changeSetID)
	if err != nil {
		return false, turnChangeSetLockError(err)
	}
	if changeSet.TerminalAt != nil || changeSet.Revision != expectedRevision {
		return false, nil
	}
	if changeSet.TerminalCaptureStartedAt != nil {
		return terminalClaimMatches(changeSet, claim), nil
	}
	if !validTurnChangeTerminalOwner(changeSet, claim) {
		return false, nil
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets SET revision = revision + 1, terminal_capture_started_at = ?,
			terminal_capture_execution_id = ?, terminal_capture_startup_attempt_id = ?,
			terminal_capture_prompt_generation = ?, terminal_capture_environment_id = ?,
			terminal_capture_outcome = ?, terminal_capture_final_message_id = ?, updated_at = ?
		WHERE id = ? AND revision = ? AND terminal_at IS NULL AND terminal_capture_started_at IS NULL
	`), claim.At, claim.ExecutionID, claim.StartupAttemptID, claim.PromptGeneration, claim.TaskEnvironmentID,
		claim.Outcome, claim.FinalAssistantMessageID, time.Now().UTC(), changeSetID, expectedRevision)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validTurnChangeTerminalOwner(changeSet *models.TurnChangeSet, claim models.TurnChangeSetTerminalClaim) bool {
	return changeSet.RuntimeExecutionID == claim.ExecutionID && changeSet.StartupAttemptID == claim.StartupAttemptID &&
		changeSet.PromptGeneration == claim.PromptGeneration && changeSet.TaskEnvironmentID == claim.TaskEnvironmentID &&
		(changeSet.StartAccepted || !changeSet.CaptureEnabled)
}

func terminalClaimMatches(changeSet *models.TurnChangeSet, claim models.TurnChangeSetTerminalClaim) bool {
	return changeSet != nil && changeSet.TerminalCaptureExecutionID == claim.ExecutionID &&
		changeSet.TerminalCaptureStartupAttemptID == claim.StartupAttemptID &&
		changeSet.TerminalCapturePromptGeneration == claim.PromptGeneration &&
		changeSet.TerminalCaptureEnvironmentID == claim.TaskEnvironmentID
}

func (r *Repository) AcceptTurnRepositoryEnd(
	ctx context.Context,
	changeSetID, repositoryChangeID string,
	expectedStartCommitOID, expectedStartTreeOID string,
	end models.TurnRepositoryChangeSet,
) (bool, error) {
	if !validAcceptedTurnRepositoryEnd(changeSetID, repositoryChangeID, expectedStartCommitOID, expectedStartTreeOID, end) {
		return false, fmt.Errorf("accept turn repository end: accepted endpoint identity is required")
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
		UPDATE turn_repository_changes SET end_commit_oid = ?, end_tree_oid = ?, end_captured_at = ?, end_ref = ?,
			cleanup_pending = ?, updated_at = ?
		WHERE id = ? AND change_set_id = ? AND start_commit_oid = ? AND start_tree_oid = ?
			AND (end_commit_oid = '' OR (end_commit_oid = ? AND end_tree_oid = ?))
	`), end.EndCommitOID, end.EndTreeOID, end.EndCapturedAt.UTC(), end.EndReachabilityRef, end.EndReachabilityRef != "", time.Now().UTC(),
		repositoryChangeID, changeSetID, expectedStartCommitOID, expectedStartTreeOID, end.EndCommitOID, end.EndTreeOID)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if updated != 1 {
		matches, matchErr := acceptedTurnRepositoryEndMatches(ctx, tx, r.db, changeSetID, repositoryChangeID, expectedStartCommitOID, expectedStartTreeOID, end)
		if matchErr != nil || !matches {
			return false, matchErr
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validAcceptedTurnRepositoryEnd(
	changeSetID, repositoryChangeID, expectedStartCommitOID, expectedStartTreeOID string,
	end models.TurnRepositoryChangeSet,
) bool {
	return changeSetID != "" && repositoryChangeID != "" && expectedStartCommitOID != "" && expectedStartTreeOID != "" &&
		end.EndCommitOID != "" && end.EndTreeOID != "" && end.HashAlgorithm != "" && end.EndReachabilityRef != "" && end.EndCapturedAt != nil
}

func acceptedTurnRepositoryEndMatches(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSetID, repositoryChangeID, expectedStartCommitOID, expectedStartTreeOID string,
	end models.TurnRepositoryChangeSet,
) (bool, error) {
	var accepted struct {
		Commit string `db:"end_commit_oid"`
		Tree   string `db:"end_tree_oid"`
	}
	err := tx.GetContext(ctx, &accepted, db.Rebind(`
		SELECT end_commit_oid, end_tree_oid FROM turn_repository_changes
		WHERE id = ? AND change_set_id = ? AND start_commit_oid = ? AND start_tree_oid = ?
	`), repositoryChangeID, changeSetID, expectedStartCommitOID, expectedStartTreeOID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return accepted.Commit == end.EndCommitOID && accepted.Tree == end.EndTreeOID, nil
}
