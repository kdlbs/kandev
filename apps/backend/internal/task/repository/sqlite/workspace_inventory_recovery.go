package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	workspaceInventoryRepoStatusFailed  = "failed"
	workspaceInventoryRepoStatusDeleted = "deleted"
)

// RepairWorkspaceInventory atomically repairs one proven canonical slot and
// appends its preservation receipt.
func (r *Repository) RepairWorkspaceInventory(
	ctx context.Context,
	repair *models.WorkspaceInventoryRepair,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	if repair == nil || repair.TaskID == "" || repair.IdempotencyKey == "" || repair.RequestHash == "" {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := r.loadWorkspaceInventoryReceiptTx(ctx, tx, repair.TaskID, repair.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.RequestHash != repair.RequestHash {
			return nil, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict
		}
		existing.ResultCode = models.WorkspaceInventoryRecoveryDeduplicated
		return existing, nil
	}
	if err := r.validateWorkspaceInventoryRepairTx(ctx, tx, repair); err != nil {
		return nil, err
	}
	receipt := newWorkspaceInventoryReceipt(repair)
	if err := r.insertWorkspaceInventoryReceiptTx(ctx, tx, receipt); err != nil {
		return nil, err
	}
	if repair.ExpectedEnvironmentRepoUpdate.IsZero() {
		row := &models.TaskEnvironmentRepo{
			ID: repair.EnvironmentRepoID, TaskEnvironmentID: repair.TaskEnvironmentID,
			RepositoryID: repair.RepositoryID, BranchSlug: repair.BranchSlug,
			WorktreeID: repair.WorktreeID, WorktreePath: repair.WorktreePath,
			WorktreeBranch: repair.WorktreeBranch, Position: repair.Position,
			Status: worktreeRepoStatusActive,
		}
		if err := r.insertTaskEnvironmentRepoTx(ctx, tx, row); err != nil {
			return nil, fmt.Errorf("repair workspace inventory row: %w", err)
		}
	} else if err := r.updateTaskEnvironmentRepoIdentityTx(ctx, tx, repair); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return receipt, nil
}

func newWorkspaceInventoryReceipt(repair *models.WorkspaceInventoryRepair) *models.WorkspaceInventoryRecoveryReceipt {
	return &models.WorkspaceInventoryRecoveryReceipt{
		ID: uuid.NewString(), TaskID: repair.TaskID, WorkspaceID: repair.WorkspaceID,
		SessionID: repair.SessionID, TaskEnvironmentID: repair.TaskEnvironmentID,
		TaskRepositoryID: repair.TaskRepositoryID, EnvironmentRepoID: repair.EnvironmentRepoID,
		RepositoryID: repair.RepositoryID, IdempotencyKey: repair.IdempotencyKey,
		RequestHash: repair.RequestHash, ResultCode: models.WorkspaceInventoryRecoveryRepaired,
		ExpectedEnvironmentUpdatedAt:  repair.ExpectedEnvironmentUpdatedAt,
		ExpectedTaskRepositoryUpdate:  repair.ExpectedTaskRepositoryUpdate,
		ExpectedEnvironmentRepoUpdate: repair.ExpectedEnvironmentRepoUpdate,
		Preservation:                  repair.Preservation, CreatedAt: time.Now().UTC(),
	}
}

func (r *Repository) validateWorkspaceInventoryRepairTx(ctx context.Context, tx *sqlx.Tx, repair *models.WorkspaceInventoryRepair) error {
	if !workspaceInventoryRepairHasRequiredIdentity(repair) {
		return models.ErrWorkspaceInventoryRecoveryInvalid
	}
	if err := r.lockTaskRowInTx(ctx, tx, repair.TaskID); err != nil {
		return err
	}
	if err := r.validateWorkspaceInventoryOwnershipTx(ctx, tx, repair); err != nil {
		return err
	}
	occupied, err := r.workspaceInventorySlotOccupiedTx(ctx, tx, repair)
	if err != nil {
		return err
	}
	if occupied {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	if !repair.ExpectedEnvironmentRepoUpdate.IsZero() {
		if err := r.validateWorkspaceInventoryExistingRowTx(ctx, tx, repair); err != nil {
			return err
		}
	}
	return nil
}

func workspaceInventoryRepairHasRequiredIdentity(repair *models.WorkspaceInventoryRepair) bool {
	return repair.WorkspaceID != "" && repair.SessionID != "" && repair.TaskEnvironmentID != "" &&
		repair.TaskRepositoryID != "" && repair.RepositoryID != "" && repair.EnvironmentRepoID != "" &&
		repair.WorktreeID != "" && repair.WorktreePath != "" && repair.WorktreeBranch != ""
}

func (r *Repository) workspaceInventorySlotOccupiedTx(
	ctx context.Context,
	tx *sqlx.Tx,
	repair *models.WorkspaceInventoryRepair,
) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT COUNT(1) FROM task_environment_repos
		WHERE task_environment_id = ? AND repository_id = ? AND branch_slug = ?
	`), repair.TaskEnvironmentID, repair.RepositoryID, repair.BranchSlug).Scan(&count)
	return count != 0, err
}

func (r *Repository) validateWorkspaceInventoryExistingRowTx(
	ctx context.Context,
	tx *sqlx.Tx,
	repair *models.WorkspaceInventoryRepair,
) error {
	var environmentID, repositoryID, worktreeID, worktreePath, worktreeBranch, status string
	var position int
	var updatedAt time.Time
	var deletedAt sql.NullTime
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_environment_id, repository_id, worktree_id, worktree_path,
		       worktree_branch, position, status, updated_at, deleted_at
		FROM task_environment_repos WHERE id = ?
	`), repair.EnvironmentRepoID).Scan(
		&environmentID, &repositoryID, &worktreeID, &worktreePath,
		&worktreeBranch, &position, &status, &updatedAt, &deletedAt,
	)
	if err != nil || environmentID != repair.TaskEnvironmentID || repositoryID != repair.RepositoryID ||
		worktreeID != repair.WorktreeID || worktreePath != repair.WorktreePath ||
		worktreeBranch != repair.WorktreeBranch || position != repair.Position ||
		status == workspaceInventoryRepoStatusFailed || status == workspaceInventoryRepoStatusDeleted ||
		deletedAt.Valid || !updatedAt.Equal(repair.ExpectedEnvironmentRepoUpdate) {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}

func (r *Repository) updateTaskEnvironmentRepoIdentityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	repair *models.WorkspaceInventoryRepair,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environment_repos
		SET branch_slug = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND updated_at = ?
	`), repair.BranchSlug, repair.EnvironmentRepoID, repair.ExpectedEnvironmentRepoUpdate)
	if err != nil {
		return fmt.Errorf("repair workspace inventory row: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}

func (r *Repository) validateWorkspaceInventoryOwnershipTx(ctx context.Context, tx *sqlx.Tx, repair *models.WorkspaceInventoryRepair) error {
	var workspaceID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM tasks WHERE id = ?`), repair.TaskID).Scan(&workspaceID); err != nil || workspaceID != repair.WorkspaceID {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	var envTaskID, envStatus string
	var envUpdated time.Time
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, status, updated_at FROM task_environments WHERE id = ?`), repair.TaskEnvironmentID).Scan(&envTaskID, &envStatus, &envUpdated); err != nil || envTaskID != repair.TaskID || envStatus == string(models.TaskEnvironmentStatusFailed) || !envUpdated.Equal(repair.ExpectedEnvironmentUpdatedAt) {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	if err := r.validateWorkspaceInventoryRepositoryTx(ctx, tx, repair); err != nil {
		return err
	}
	var sessionTaskID, sessionEnvironmentID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, task_environment_id FROM task_sessions WHERE id = ?`), repair.SessionID).Scan(&sessionTaskID, &sessionEnvironmentID); err != nil || sessionTaskID != repair.TaskID || sessionEnvironmentID != repair.TaskEnvironmentID {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}

func (r *Repository) validateWorkspaceInventoryRepositoryTx(ctx context.Context, tx *sqlx.Tx, repair *models.WorkspaceInventoryRepair) error {
	var taskRepoTaskID, repositoryID string
	var position int
	var taskRepoUpdated time.Time
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, repository_id, position, updated_at FROM task_repositories WHERE id = ?`), repair.TaskRepositoryID).Scan(&taskRepoTaskID, &repositoryID, &position, &taskRepoUpdated); err != nil || taskRepoTaskID != repair.TaskID || repositoryID != repair.RepositoryID || position != repair.Position || !taskRepoUpdated.Equal(repair.ExpectedTaskRepositoryUpdate) {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	var repositoryWorkspace string
	var deletedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id, deleted_at FROM repositories WHERE id = ?`), repair.RepositoryID).Scan(&repositoryWorkspace, &deletedAt); err != nil || repositoryWorkspace != repair.WorkspaceID || deletedAt.Valid {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}

func (r *Repository) insertWorkspaceInventoryReceiptTx(ctx context.Context, tx *sqlx.Tx, receipt *models.WorkspaceInventoryRecoveryReceipt) error {
	payload, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspace_inventory_recovery_receipts (
			id, task_id, workspace_id, session_id, task_environment_id,
			task_repository_id, environment_repo_id, repository_id,
			idempotency_key, request_hash, result_code, receipt_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), receipt.ID, receipt.TaskID, receipt.WorkspaceID, receipt.SessionID,
		receipt.TaskEnvironmentID, receipt.TaskRepositoryID, receipt.EnvironmentRepoID,
		receipt.RepositoryID, receipt.IdempotencyKey, receipt.RequestHash,
		receipt.ResultCode, string(payload), receipt.CreatedAt)
	return err
}

func (r *Repository) loadWorkspaceInventoryReceiptTx(ctx context.Context, tx *sqlx.Tx, taskID, idempotencyKey string) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	var payload, requestHash string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT receipt_json, request_hash FROM workspace_inventory_recovery_receipts
		WHERE task_id = ? AND idempotency_key = ?
	`), taskID, idempotencyKey).Scan(&payload, &requestHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	receipt := &models.WorkspaceInventoryRecoveryReceipt{}
	if err := json.Unmarshal([]byte(payload), receipt); err != nil {
		return nil, err
	}
	receipt.RequestHash = requestHash
	return receipt, nil
}

// GetWorkspaceInventoryRepairReceipt returns the previously committed receipt
// for a task-scoped idempotency key, or nil if none exists. Callers use it to
// short-circuit a retry once the canonical inventory already matches, which
// would otherwise leave no provable mismatch for candidate selection to find.
func (r *Repository) GetWorkspaceInventoryRepairReceipt(
	ctx context.Context,
	taskID string,
	idempotencyKey string,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	if taskID == "" || idempotencyKey == "" {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	var payload, requestHash string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT receipt_json, request_hash FROM workspace_inventory_recovery_receipts
		WHERE task_id = ? AND idempotency_key = ?
	`), taskID, idempotencyKey).Scan(&payload, &requestHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	receipt := &models.WorkspaceInventoryRecoveryReceipt{}
	if err := json.Unmarshal([]byte(payload), receipt); err != nil {
		return nil, err
	}
	receipt.RequestHash = requestHash
	receipt.ResultCode = models.WorkspaceInventoryRecoveryDeduplicated
	return receipt, nil
}

// RecordWorkspaceInventoryPostRepairAttestation persists the post-repair
// checkout evidence onto an already-committed receipt. The inventory repair
// transaction itself has already committed, but lifecycle callers treat a
// failure here as retryable and must not launch from the repaired row until
// positive matching evidence is durably recorded.
func (r *Repository) RecordWorkspaceInventoryPostRepairAttestation(
	ctx context.Context,
	taskID string,
	idempotencyKey string,
	evidence *models.WorkspaceInventoryPreservation,
	matched bool,
	verifiedAt time.Time,
) error {
	if taskID == "" || idempotencyKey == "" {
		return models.ErrWorkspaceInventoryRecoveryInvalid
	}
	var payload, requestHash string
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT receipt_json, request_hash FROM workspace_inventory_recovery_receipts
		WHERE task_id = ? AND idempotency_key = ?
	`), taskID, idempotencyKey).Scan(&payload, &requestHash)
	if err != nil {
		return fmt.Errorf("load receipt for post-repair attestation: %w", err)
	}
	receipt := &models.WorkspaceInventoryRecoveryReceipt{}
	if err := json.Unmarshal([]byte(payload), receipt); err != nil {
		return fmt.Errorf("decode receipt for post-repair attestation: %w", err)
	}
	receipt.RequestHash = requestHash
	receipt.PostRepairEvidence = evidence
	receipt.PostRepairMatched = matched
	verified := verifiedAt
	receipt.PostRepairVerifiedAt = &verified
	updated, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode receipt for post-repair attestation: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workspace_inventory_recovery_receipts
		SET receipt_json = ?, post_repair_matched = ?, post_repair_verified_at = ?
		WHERE task_id = ? AND idempotency_key = ?
		  AND (post_repair_verified_at IS NULL OR post_repair_matched = TRUE OR ? = FALSE)
	`), string(updated), matched, verifiedAt, taskID, idempotencyKey, matched)
	if err != nil {
		return fmt.Errorf("persist post-repair attestation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("persist post-repair attestation: %w", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return nil
}
