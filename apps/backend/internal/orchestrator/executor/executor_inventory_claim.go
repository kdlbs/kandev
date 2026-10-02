package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type inventoryRecoveryClaimStore interface {
	AcquireTaskEnvironmentRecoveryClaim(context.Context, models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error)
	ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error
}

// The durable environment claim excludes every borrower and coordinates with
// session/runtime admission, ownership changes, and cleanup transactions.
func (e *Executor) claimInventoryEnvironment(ctx context.Context, env *models.TaskEnvironment, sessionID string) (context.Context, func() error, error) {
	store, ok := e.repo.(inventoryRecoveryClaimStore)
	if !ok || env == nil {
		return ctx, nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	claim, err := store.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: env.ID, OwnerTaskID: env.TaskID, OwnershipGeneration: env.OwnershipGeneration,
		SessionID: sessionID, OperationID: uuid.NewString(), ExecutorType: env.ExecutorType, AllowCurrentSessionRuntime: true,
	})
	if err != nil {
		return ctx, nil, fmt.Errorf("%w: acquire environment preservation claim: %w", models.ErrWorkspaceInventoryRecoveryConflict, err)
	}
	release := func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := store.ReleaseTaskEnvironmentRecoveryClaim(releaseCtx, claim); err != nil {
			return fmt.Errorf("%w: release environment preservation claim: %w", models.ErrWorkspaceInventoryRecoveryConflict, err)
		}
		return nil
	}
	return recoveryclaim.WithClaim(ctx, claim), release, nil
}

func (e *Executor) repairReuseEnvironmentInventory(ctx context.Context, task *v1.Task, session *models.TaskSession,
	req *LaunchAgentRequest, env *models.TaskEnvironment, repositories []*repoInfo, key string,
) (receipt *models.WorkspaceInventoryRecoveryReceipt, err error) {
	if _, err = e.workspaceInventoryRepairer(task, session, req, env, key); err != nil {
		return nil, err
	}
	// Serialize local retries; the durable claim supplies cross-task exclusion.
	writerLock := e.taskEnvLock(task.ID)
	writerLock.Lock()
	defer writerLock.Unlock()
	claimedCtx, release, err := e.claimInventoryEnvironment(ctx, env, session.ID)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, release())
		if err != nil {
			receipt = nil
		}
	}()
	return e.repairClaimedWorkspaceInventory(claimedCtx, task, session, req, env, repositories, key)
}

func (e *Executor) attestInventoryRow(ctx context.Context, repairer workspaceInventoryRepairRepository,
	existing *models.WorkspaceInventoryRecoveryReceipt, spec RepoSpec, session *models.TaskSession,
	env *models.TaskEnvironment, info *repoInfo, candidate *models.TaskEnvironmentRepo,
) (receipt *models.WorkspaceInventoryRecoveryReceipt, err error) {
	if existing.PostRepairVerifiedAt != nil {
		return e.attestedExistingWorkspaceInventoryReceipt(ctx, repairer, existing, spec, session, info, candidate)
	}
	claimedCtx, release, err := e.claimInventoryEnvironment(ctx, env, session.ID)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, release())
		if err != nil {
			receipt = nil
		}
	}()
	return e.attestedExistingWorkspaceInventoryReceipt(claimedCtx, repairer, existing, spec, session, info, candidate)
}
