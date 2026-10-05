package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (e *Executor) admitResumeWorkspaceInventory(ctx context.Context, task *v1.Task, session *models.TaskSession,
	req *LaunchAgentRequest, env *models.TaskEnvironment, repositories []*repoInfo, options ResumeOptions,
) error {
	inventoryErr := e.validateReuseEnvironmentInventory(ctx, req, env)
	// Explicit retries must validate the key's identity even after repair made
	// the inventory valid. Ordinary resumes are gated by the physical row.
	if options.RepairWorkspaceInventory {
		receipt, err := e.resumeInventoryRepairReceipt(ctx, task, session, req, env, repositories, options.WorkspaceInventoryIdempotencyKey, inventoryErr == nil)
		if err != nil {
			return err
		}
		req.WorkspaceInventoryRecoveryReceipt = receipt
	}
	if options.RepairWorkspaceInventory {
		inventoryErr = e.validateReuseEnvironmentInventory(ctx, req, env)
	}
	if inventoryErr != nil {
		return inventoryErr
	}
	if !req.WorkspaceReuseRequired || !req.UseWorktree {
		return nil
	}
	receipt, err := e.attestedWorkspaceInventoryRowsReceipt(ctx, task, session, req, env, repositories)
	if err != nil {
		return err
	}
	if req.WorkspaceInventoryRecoveryReceipt == nil {
		req.WorkspaceInventoryRecoveryReceipt = receipt
	}
	return nil
}

// A new key on already-valid inventory has no repair to replay. Existing
// keys still pass the full session/checkout identity and attestation checks.
func (e *Executor) resumeInventoryRepairReceipt(ctx context.Context, task *v1.Task, session *models.TaskSession,
	req *LaunchAgentRequest, env *models.TaskEnvironment, repositories []*repoInfo, key string, inventoryValid bool,
) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	repairer, err := e.workspaceInventoryRepairer(task, session, req, env, key)
	if err != nil {
		return nil, err
	}
	if inventoryValid {
		existing, err := repairer.GetWorkspaceInventoryRepairReceipt(ctx, task.ID, key)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, nil
		}
	}
	return e.repairReuseEnvironmentInventory(ctx, task, session, req, env, repositories, key)
}

// Borrowers are authorized by environment resolution. Receipt identity stays
// bound to the environment owner's repository slot, not the borrower's slot.
func (e *Executor) workspaceInventoryReceiptOwner(ctx context.Context, receipt *models.WorkspaceInventoryRecoveryReceipt,
	task *v1.Task, env *models.TaskEnvironment, info *repoInfo,
) (*v1.Task, *repoInfo, error) {
	if env.TaskID == task.ID {
		return task, info, nil
	}
	owner, err := e.repo.GetTask(ctx, env.TaskID)
	if err != nil || owner == nil || owner.WorkspaceID != task.WorkspaceID || receipt.TaskID != owner.ID {
		return nil, nil, fmt.Errorf("%w: repaired environment owner does not match", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	repos, err := e.repo.ListTaskRepositories(ctx, owner.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, repo := range repos {
		if repo.ID == receipt.TaskRepositoryID && repo.TaskID == owner.ID && repo.RepositoryID == info.RepositoryID {
			ownerInfo := *info
			ownerInfo.TaskRepositoryID = repo.ID
			ownerInfo.Position = repo.Position
			return owner.ToAPI(), &ownerInfo, nil
		}
	}
	return nil, nil, fmt.Errorf("%w: repaired owner repository slot does not match", models.ErrWorkspaceInventoryRecoveryConflict)
}
