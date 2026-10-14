package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/checkoutadoption"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func (r *Repository) ensureWorkspaceAdoptionsSchema() error {
	if err := r.migrate.Apply("task_environment_checkout_adoptions.table", `CREATE TABLE IF NOT EXISTS task_environment_checkout_adoptions (
 operation_id TEXT PRIMARY KEY, task_environment_id TEXT NOT NULL, slot_id TEXT NOT NULL,
 receipt_json TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
 UNIQUE(task_environment_id, slot_id),
 FOREIGN KEY(task_environment_id) REFERENCES task_environments(id) ON DELETE CASCADE
 )`); err != nil {
		return fmt.Errorf("create workspace adoption table: %w", err)
	}
	return r.migrate.Err()
}

func (r *Repository) GetWorkspaceAdoption(ctx context.Context, operationID string) (*models.WorkspaceAdoptionReceipt, error) {
	return checkoutadoption.Load(ctx, r.ro, operationID)
}

func (r *Repository) ValidateWorkspaceAdoption(ctx context.Context, receipt *models.WorkspaceAdoptionReceipt) error {
	return checkoutadoption.ValidateCurrent(ctx, r.ro, receipt)
}

// CommitWorkspaceAdoption publishes the slot and receipt under the current claim.
// verify retains the caller's pinned objects through the transaction commit.
func (r *Repository) CommitWorkspaceAdoption(ctx context.Context, p models.WorkspaceAdoptionPreview, operationID string, claim *models.TaskEnvironmentRecoveryClaim, verify func() error) (*models.WorkspaceAdoptionReceipt, error) {
	if err := validateWorkspaceAdoptionPublication(p, operationID, claim, verify); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := recoveryclaim.ValidateTx(ctx, r.db, tx, claim); err != nil {
		return nil, err
	}
	if err := recoveryclaim.EnsureQuiescentTx(ctx, r.db, tx, claim); err != nil {
		return nil, err
	}
	if err := r.validateAdoptionSelectionTx(ctx, tx, p, claim); err != nil {
		return nil, err
	}
	if err := r.publishAdoptionTx(ctx, tx, p); err != nil {
		return nil, err
	}
	receipt := &models.WorkspaceAdoptionReceipt{OperationID: operationID, Preview: p, CreatedAt: r.nowUTC()}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_environment_checkout_adoptions(operation_id,task_environment_id,slot_id,receipt_json,created_at) VALUES(?,?,?,?,?)`), operationID, p.Selection.Snapshot.TaskEnvironmentID, p.SlotID, string(encoded), receipt.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := verify(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (r *Repository) validateAdoptionSelectionTx(ctx context.Context, tx *sqlx.Tx, p models.WorkspaceAdoptionPreview, claim *models.TaskEnvironmentRecoveryClaim) error {
	s := p.Selection.Snapshot
	if !adoptionClaimMatchesSelection(s, claim) {
		return models.ErrWorkspaceAdoptionStale
	}
	observation := models.WorkspaceRecoveryErrorObservation{TaskID: s.TaskID, SessionID: s.SessionID, TaskEnvironmentID: s.TaskEnvironmentID, EnvironmentOwnerTaskID: s.EnvironmentOwnerTaskID, OwnershipGeneration: s.OwnershipGeneration, SelectionSnapshot: s}
	matches, err := r.workspaceRecoverySelectionMatches(ctx, tx, observation)
	if err != nil {
		return err
	}
	if !matches {
		return models.ErrWorkspaceAdoptionStale
	}
	var count int
	query := `SELECT COUNT(*) FROM task_sessions ts JOIN tasks t ON t.id=ts.task_id WHERE ts.id=? AND ts.task_id=? AND ts.task_environment_id=? AND ts.queue_incarnation_id=? AND ts.state='WAITING_FOR_INPUT' AND ts.workspace_path=? AND t.workspace_id=? AND t.archived_at IS NULL`
	if err := tx.GetContext(ctx, &count, r.db.Rebind(query), s.SessionID, s.TaskID, s.TaskEnvironmentID, p.Selection.SessionIncarnation, p.Selection.SessionWorkspacePath, p.Selection.WorkspaceID); err != nil {
		return err
	}
	if count != 1 {
		return models.ErrWorkspaceAdoptionStale
	}
	if err := checkoutadoption.ValidateAttachmentsTx(ctx, r.db, tx, p); err != nil {
		return err
	}
	return nil
}

func (r *Repository) publishAdoptionTx(ctx context.Context, tx *sqlx.Tx, p models.WorkspaceAdoptionPreview) error {
	slot, err := p.Slot()
	if err != nil {
		return err
	}
	var count int
	if err := tx.GetContext(ctx, &count, r.db.Rebind(`SELECT COUNT(*) FROM task_environment_repos WHERE worktree_path=? AND deleted_at IS NULL AND (COALESCE(status,'')='' OR status='active')`), p.Proof.CandidatePath); err != nil {
		return err
	}
	if count != 0 {
		return models.ErrWorkspaceAdoptionStale
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_environment_repos SET worktree_path=?,worktree_source_clone_path=?,worktree_source_common_dir=?,updated_at=? WHERE id=? AND task_environment_id=? AND worktree_id=? AND worktree_path=? AND worktree_branch=? AND (COALESCE(status,'')='' OR status='active') AND deleted_at IS NULL`),
		p.Proof.CandidatePath, p.Proof.CandidatePath, filepath.Join(p.Proof.CandidatePath, ".git"), r.nowUTC(), p.SlotID, p.Selection.Snapshot.TaskEnvironmentID, slot.WorktreeID, p.Proof.OriginalPath, p.Proof.Branch)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return models.ErrWorkspaceAdoptionStale
	}
	if p.Selection.Snapshot.WorkspacePath == p.Proof.OriginalPath {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_environments SET workspace_path=?,updated_at=? WHERE id=? AND workspace_path=?`), p.Proof.CandidatePath, r.nowUTC(), p.Selection.Snapshot.TaskEnvironmentID, p.Proof.OriginalPath); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET workspace_path=?,updated_at=? WHERE task_environment_id=? AND workspace_path=?`), p.Proof.CandidatePath, r.nowUTC(), p.Selection.Snapshot.TaskEnvironmentID, p.Proof.OriginalPath); err != nil {
			return err
		}
	}
	return nil
}

func adoptionClaimMatchesSelection(s models.WorkspaceRecoverySelectionSnapshot, claim *models.TaskEnvironmentRecoveryClaim) bool {
	return claim != nil && claim.OperationID != "" && claim.TaskEnvironmentID == s.TaskEnvironmentID && claim.OwnerTaskID == s.TaskID && claim.SessionID == s.SessionID && claim.OwnershipGeneration == s.OwnershipGeneration && s.TaskID == s.EnvironmentOwnerTaskID && s.ExecutorType == string(models.ExecutorTypeWorktree) && claim.ExecutorType == string(models.ExecutorTypeWorktree) && s.EnvironmentStatus == string(models.TaskEnvironmentStatusReady) && s.Complete()
}

// FinalizeWorkspaceAdoption releases only a claim proven by a committed receipt.
// This closes the crash window between publication and normal claim release.
func (r *Repository) FinalizeWorkspaceAdoption(ctx context.Context, receipt *models.WorkspaceAdoptionReceipt) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	claim, err := recoveryclaim.GetTx(ctx, r.db, tx, receipt.Preview.Selection.Snapshot.TaskEnvironmentID)
	if err != nil {
		return err
	}
	if claim == nil {
		return nil
	}
	s := receipt.Preview.Selection.Snapshot
	if claim.OperationID != receipt.OperationID || claim.OwnerTaskID != s.EnvironmentOwnerTaskID || claim.SessionID != s.SessionID || claim.OwnershipGeneration != s.OwnershipGeneration {
		return models.ErrWorkspaceAdoptionStale
	}
	if err := recoveryclaim.ValidateTx(ctx, r.db, tx, claim); err != nil {
		return err
	}
	if err := checkoutadoption.ValidateCurrentTx(ctx, r.db, tx, receipt); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_environment_recovery_claims WHERE task_environment_id=? AND operation_id=? AND owner_task_id=? AND ownership_generation=? AND session_id=?`), claim.TaskEnvironmentID, claim.OperationID, claim.OwnerTaskID, claim.OwnershipGeneration, claim.SessionID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return models.ErrWorkspaceAdoptionStale
	}
	return tx.Commit()
}

func validateWorkspaceAdoptionPublication(p models.WorkspaceAdoptionPreview, operationID string, claim *models.TaskEnvironmentRecoveryClaim, verify func() error) error {
	if _, err := uuid.Parse(operationID); err != nil {
		return models.ErrWorkspaceAdoptionStale
	}
	if claim == nil || claim.OperationID != operationID || p.Token == "" || p.Token != p.Digest() || verify == nil {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}
