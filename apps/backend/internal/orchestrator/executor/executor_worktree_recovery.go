package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// admitSelectedWorktreeRecovery builds admission from the effective executor
// and environment selected by the launch path. It never inventories a task's
// unrelated environments or asks a remote executor to use host authority.
//
//nolint:cyclop,funlen // The boundary independently validates each selected repository slot.
func (e *Executor) admitSelectedWorktreeRecovery(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	env *models.TaskEnvironment,
	executorType string,
) (*worktree.RecoveryAdmission, error) {
	if e.selectedWorktreeRecoveryAdmission == nil || taskID == "" || session == nil || env == nil ||
		env.ID == "" || executorType != string(models.ExecutorTypeWorktree) ||
		env.ExecutorType != string(models.ExecutorTypeWorktree) {
		return nil, nil
	}
	if env.TaskID == "" || env.OwnershipGeneration <= 0 {
		return nil, fmt.Errorf("worktree recovery admission: selected environment identity is incomplete")
	}

	slots := make([]worktree.RecoverySlot, 0, len(env.Repos))
	for _, row := range env.Repos {
		if row == nil || row.RepositoryID == "" || row.DeletedAt != nil ||
			(row.Status != "" && row.Status != "active") {
			continue
		}
		if row.WorktreeID == "" {
			continue
		}
		repositoryPath, err := e.repositoryLocalPath(ctx, row.RepositoryID)
		if err != nil {
			return nil, err
		}
		slots = append(slots, worktree.RecoverySlot{
			WorktreeID:     row.WorktreeID,
			RepositoryID:   row.RepositoryID,
			BranchSlug:     row.BranchSlug,
			RepositoryPath: repositoryPath,
			Worktree: &worktree.Worktree{
				ID:                row.WorktreeID,
				TaskID:            env.TaskID,
				TaskEnvironmentID: env.ID,
				TaskDirName:       env.TaskDirName,
				RepositoryID:      row.RepositoryID,
				BranchSlug:        row.BranchSlug,
				RepositoryPath:    repositoryPath,
				Path:              row.WorktreePath,
				Branch:            row.WorktreeBranch,
				BaseBranch:        session.BaseBranch,
				Status:            worktree.StatusActive,
			},
		})
	}
	if len(slots) == 0 {
		return nil, nil
	}

	admission, err := e.selectedWorktreeRecoveryAdmission(ctx, worktree.RecoveryAdmissionRequest{
		TaskID:              taskID,
		SessionID:           session.ID,
		TaskEnvironmentID:   env.ID,
		OwnerTaskID:         env.TaskID,
		OwnershipGeneration: env.OwnershipGeneration,
		ExecutorType:        executorType,
		Slots:               slots,
	})
	if err != nil {
		return nil, err
	}
	// Admission may publish a replacement Worktree in each slot. Refresh the
	// canonical environment rows immediately so the same launch and the nested
	// lifecycle request cannot continue with the stale worktree identity.
	for _, slot := range slots {
		if slot.Worktree == nil {
			continue
		}
		for _, row := range env.Repos {
			if row == nil || row.RepositoryID != slot.RepositoryID || row.BranchSlug != slot.BranchSlug {
				continue
			}
			if slot.WorktreeID != "" && row.WorktreeID != slot.WorktreeID {
				continue
			}
			row.WorktreeID = slot.Worktree.ID
			row.WorktreePath = slot.Worktree.Path
			row.WorktreeBranch = slot.Worktree.Branch
			break
		}
	}
	return admission, nil
}

func (e *Executor) repositoryLocalPath(ctx context.Context, repositoryID string) (string, error) {
	repository, err := e.repo.GetRepository(ctx, repositoryID)
	if err != nil {
		return "", fmt.Errorf("load repository %q for worktree recovery: %w", repositoryID, err)
	}
	if repository == nil || strings.TrimSpace(repository.LocalPath) == "" {
		return "", fmt.Errorf("load repository %q for worktree recovery: local path is missing", repositoryID)
	}
	return repository.LocalPath, nil
}

func (e *Executor) resolveEnvironmentForAdmission(
	ctx context.Context,
	taskID, environmentID string,
) (*models.TaskEnvironment, error) {
	if environmentID != "" {
		env, err := e.repo.GetTaskEnvironment(ctx, environmentID)
		if err != nil {
			return nil, fmt.Errorf("load selected task environment %q: %w", environmentID, err)
		}
		return env, nil
	}
	if taskID == "" {
		return nil, nil
	}
	return e.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
}

func releaseSelectedWorktreeRecovery(ctx context.Context, admission **worktree.RecoveryAdmission) error {
	if admission == nil || *admission == nil {
		return nil
	}
	err := (*admission).Release(ctx)
	*admission = nil
	return err
}
