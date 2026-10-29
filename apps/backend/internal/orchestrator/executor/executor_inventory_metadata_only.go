package executor

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type inventoryMetadataKey struct{}
type inventoryFenceStore interface {
	ReadWorkspaceInventoryRepairFence(context.Context, string, string) (*models.WorkspaceInventoryRepairFence, error)
}

func inventoryMetadataFence(ctx context.Context) *models.WorkspaceInventoryRepairFence {
	fence, _ := ctx.Value(inventoryMetadataKey{}).(*models.WorkspaceInventoryRepairFence)
	return fence
}

// RepairWorkspaceInventoryOnly never resolves launch settings or starts an agent.
func (e *Executor) RepairWorkspaceInventoryOnly(ctx context.Context, taskID, sessionID, key string) (*models.WorkspaceInventoryRecoveryReceipt, error) {
	if key == "" {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	task, session, err := e.metadataInventoryTaskSession(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	env, err := e.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !metadataInventoryEnvironmentMatches(taskID, session, env) {
		return nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	store, ok := e.repo.(inventoryFenceStore)
	if !ok {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	fence, err := store.ReadWorkspaceInventoryRepairFence(ctx, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	req, infos, err := e.metadataInventoryInputs(ctx, task.ToAPI(), env)
	if err != nil {
		return nil, err
	}
	scoped := context.WithValue(ctx, inventoryMetadataKey{}, fence)
	receipt, err := e.repairReuseEnvironmentInventory(scoped, task.ToAPI(), session, req, env, infos, key)
	if err != nil {
		return nil, err
	}
	after, err := store.ReadWorkspaceInventoryRepairFence(ctx, taskID, sessionID)
	if err != nil || !fence.Equal(after) {
		return nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	return receipt, nil
}

func (e *Executor) metadataInventoryTaskSession(ctx context.Context, taskID, sessionID string) (*models.Task, *models.TaskSession, error) {
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if task == nil || session == nil || session.TaskID != taskID || !isTerminalSessionState(session.State) {
		return nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	return task, session, nil
}

func metadataInventoryEnvironmentMatches(taskID string, session *models.TaskSession, env *models.TaskEnvironment) bool {
	return env != nil && env.ID != "" && env.TaskID == taskID && session.TaskEnvironmentID == env.ID &&
		env.ExecutorType == string(models.ExecutorTypeWorktree) && env.Status == models.TaskEnvironmentStatusReady
}

func (e *Executor) metadataInventoryInputs(ctx context.Context, task *v1.Task, env *models.TaskEnvironment) (*LaunchAgentRequest, []*repoInfo, error) {
	attached, err := e.repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return nil, nil, err
	}
	if len(attached) != 1 || attached[0].TaskID != task.ID {
		return nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	tr := attached[0]
	repository, err := e.repo.GetRepository(ctx, tr.RepositoryID)
	if err != nil {
		return nil, nil, err
	}
	if repository == nil || repository.WorkspaceID != task.WorkspaceID || repository.SourceType != sourceTypeLocal || repository.LocalPath == "" {
		return nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	info := &repoInfo{TaskRepositoryID: tr.ID, TaskRepositoryUpdatedAt: tr.UpdatedAt, RepositoryID: tr.RepositoryID,
		RepositoryPath: repository.LocalPath, Repository: repository, Position: tr.Position,
		BaseBranch: tr.BaseBranch, CheckoutBranch: tr.CheckoutBranch}
	infos := []*repoInfo{info}
	req := &LaunchAgentRequest{TaskID: task.ID, WorkspaceID: task.WorkspaceID, TaskEnvironmentID: env.ID,
		ExecutorType: env.ExecutorType, UseWorktree: true, WorkspaceReuseRequired: true, Repositories: buildRepoSpecs(infos)}
	if launchRepoBranchIdentitySlug(req.Repositories[0]) == "" {
		return nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	return req, infos, nil
}

func validateScopedInventoryCandidate(ctx context.Context, env *models.TaskEnvironment, info *repoInfo, candidate *models.TaskEnvironmentRepo) error {
	if info.Repository == nil || info.Repository.SourceType != sourceTypeLocal || inventoryMetadataFence(ctx) == nil {
		return validateWorkspaceInventoryRepairCandidate(env, info, candidate)
	}
	if !repairPathIsTaskScoped(env, candidate.WorktreePath) || !isLocalGitRepo(info.RepositoryPath) {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	configured, err := worktree.CanonicalDirectory(info.Repository.LocalPath)
	if err != nil {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	actual, err := worktree.CanonicalDirectory(info.RepositoryPath)
	if err != nil || configured != actual {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	checkout, err := worktree.CanonicalDirectory(candidate.WorktreePath)
	if err != nil || checkout == actual {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}
