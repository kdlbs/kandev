// Package checkoutadoption reads backend-owned adoption provenance.
package checkoutadoption

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

func Load(ctx context.Context, db *sqlx.DB, operationID string) (*models.WorkspaceAdoptionReceipt, error) {
	var data string
	err := db.GetContext(ctx, &data, db.Rebind(`SELECT receipt_json FROM task_environment_checkout_adoptions WHERE operation_id=?`), operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt models.WorkspaceAdoptionReceipt
	if err := json.Unmarshal([]byte(data), &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

// ValidateCurrent checks the original session incarnation and current slot owner.
type receiptQuery interface {
	GetContext(context.Context, any, string, ...any) error
	attachmentQuery
}

func ValidateCurrent(ctx context.Context, db *sqlx.DB, receipt *models.WorkspaceAdoptionReceipt) error {
	return validateCurrent(ctx, db, db, receipt)
}

func ValidateCurrentTx(ctx context.Context, db *sqlx.DB, tx *sqlx.Tx, receipt *models.WorkspaceAdoptionReceipt) error {
	return validateCurrent(ctx, db, tx, receipt)
}

func validateCurrent(ctx context.Context, db *sqlx.DB, q receiptQuery, receipt *models.WorkspaceAdoptionReceipt) error {
	return validateBinding(ctx, db, q, receipt, " AND t.archived_at IS NULL")
}

func validateBinding(ctx context.Context, db *sqlx.DB, q receiptQuery, receipt *models.WorkspaceAdoptionReceipt, taskStateGuard string) error {
	p := receipt.Preview
	s := p.Selection.Snapshot
	slot, err := p.Slot()
	if err != nil {
		return err
	}
	var count int
	err = q.GetContext(ctx, &count, db.Rebind(`SELECT COUNT(*) FROM task_environment_repos er
 JOIN task_environments te ON te.id=er.task_environment_id
 JOIN tasks t ON t.id=te.task_id
 JOIN repositories r ON r.id=er.repository_id
 JOIN task_sessions ts ON ts.id=? AND ts.task_environment_id=te.id
 JOIN task_repositories tr ON tr.id=? AND tr.task_id=t.id AND tr.repository_id=r.id
 WHERE te.id=? AND te.task_id=? AND te.ownership_generation=? AND te.executor_type='worktree'
 AND te.task_dir_name=? AND te.workspace_path=? AND te.status='ready'
 AND t.workspace_id=?`+taskStateGuard+` AND r.workspace_id=t.workspace_id AND r.deleted_at IS NULL
 AND ts.task_id=t.id AND ts.queue_incarnation_id=? AND ts.workspace_path=?
 AND er.id=? AND er.worktree_id=? AND er.repository_id=? AND er.worktree_path=? AND er.worktree_branch=?
 AND er.branch_slug=? AND COALESCE(er.position,0)=?
 AND er.worktree_source_clone_path=? AND er.worktree_source_common_dir=?
 AND (COALESCE(er.status,'')='' OR er.status='active') AND er.deleted_at IS NULL AND COALESCE(tr.checkout_branch,'')=? AND COALESCE(tr.base_branch,'')=?`),
		s.SessionID, p.Selection.AttachmentID, s.TaskEnvironmentID, s.EnvironmentOwnerTaskID, s.OwnershipGeneration,
		s.TaskDirName, p.PublishedWorkspacePath(s.WorkspacePath), p.Selection.WorkspaceID, p.Selection.SessionIncarnation, p.PublishedWorkspacePath(p.Selection.SessionWorkspacePath),
		p.SlotID, slot.WorktreeID, slot.RepositoryID, p.Proof.CandidatePath, p.Proof.Branch,
		slot.BranchSlug, slot.Position,
		p.Proof.CandidatePath, filepath.Join(p.Proof.CandidatePath, ".git"), p.Selection.AttachmentCheckoutBranch, p.Selection.AttachmentBaseBranch)
	if err != nil {
		return err
	}
	if count != 1 {
		return models.ErrWorkspaceAdoptionStale
	}
	return validateAttachments(ctx, db, q, p, false)
}

func ForWorktree(ctx context.Context, db *sqlx.DB, worktreeID string) (*models.WorkspaceAdoptionReceipt, error) {
	return forWorktree(ctx, db, worktreeID, ValidateCurrent)
}

// ForWorktreeCleanupCapture retains unchanged binding authority for retirement.
// Archived tasks remain ineligible for runtime reuse through ForWorktree.
func ForWorktreeCleanupCapture(ctx context.Context, db *sqlx.DB, worktreeID string) (*models.WorkspaceAdoptionReceipt, error) {
	return forWorktree(ctx, db, worktreeID, func(ctx context.Context, db *sqlx.DB, receipt *models.WorkspaceAdoptionReceipt) error {
		return validateBinding(ctx, db, db, receipt, "")
	})
}

func forWorktree(ctx context.Context, db *sqlx.DB, worktreeID string, validate func(context.Context, *sqlx.DB, *models.WorkspaceAdoptionReceipt) error) (*models.WorkspaceAdoptionReceipt, error) {
	var ids []string
	if err := db.SelectContext(ctx, &ids, db.Rebind(`SELECT a.operation_id FROM task_environment_checkout_adoptions a
 JOIN task_environment_repos er ON er.id=a.slot_id AND er.task_environment_id=a.task_environment_id
 WHERE er.worktree_id=? ORDER BY a.operation_id`), worktreeID); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) != 1 {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	receipt, err := Load(ctx, db, ids[0])
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	return receipt, validate(ctx, db, receipt)
}
