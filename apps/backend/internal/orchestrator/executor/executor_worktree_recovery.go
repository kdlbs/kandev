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
	// A fresh launch may have reserved a creating environment before its
	// session incarnation and worktree identities are durable. It has no
	// reusable checkout to recover; defer admission until materialization.
	if env.Status == models.TaskEnvironmentStatusCreating && env.MaterializationSessionID == session.ID &&
		!environmentHasMaterializedWorktree(env) {
		return nil, nil
	}
	if environmentHasRecoveryRepositoryWithoutWorktreeIdentity(env) {
		return nil, fmt.Errorf("worktree recovery admission: selected environment has a repository without a durable worktree identity")
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
		repository, err := e.repo.GetRepository(ctx, row.RepositoryID)
		if err != nil {
			return nil, err
		}
		if repository == nil || strings.TrimSpace(repository.LocalPath) == "" {
			return nil, fmt.Errorf("load repository %q for worktree recovery: local path is missing", row.RepositoryID)
		}
		repositoryPath := repository.LocalPath
		cloneRelocation := managedCloneRelocationProof(e.repoCloner, repository, row.WorktreeSourceClonePath, row.WorktreeSourceCommonDir)
		slots = append(slots, worktree.RecoverySlot{
			WorktreeID:      row.WorktreeID,
			RepositoryID:    row.RepositoryID,
			BranchSlug:      row.BranchSlug,
			RepositoryPath:  repositoryPath,
			CloneRelocation: cloneRelocation,
			Worktree: &worktree.Worktree{
				ID:                row.WorktreeID,
				TaskID:            env.TaskID,
				TaskEnvironmentID: env.ID,
				TaskDirName:       env.TaskDirName,
				RepositoryID:      row.RepositoryID,
				BranchSlug:        row.BranchSlug,
				RepositoryPath:    repositoryPath,
				SourceClonePath:   row.WorktreeSourceClonePath,
				SourceCommonDir:   row.WorktreeSourceCommonDir,
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
		TaskID:               taskID,
		SessionID:            session.ID,
		SessionIncarnationID: session.QueueIncarnationID,
		TaskEnvironmentID:    env.ID,
		OwnerTaskID:          env.TaskID,
		OwnershipGeneration:  env.OwnershipGeneration,
		ExecutorType:         executorType,
		Slots:                slots,
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
			row.WorktreeSourceClonePath = slot.Worktree.SourceClonePath
			row.WorktreeSourceCommonDir = slot.Worktree.SourceCommonDir
			break
		}
	}
	return admission, nil
}

func environmentHasMaterializedWorktree(env *models.TaskEnvironment) bool {
	if env == nil {
		return false
	}
	for _, repo := range env.Repos {
		if repo != nil && (repo.WorktreeID != "" || repo.WorktreePath != "" || repo.WorktreeBranch != "") {
			return true
		}
	}
	return false
}

func environmentHasRecoveryRepositoryWithoutWorktreeIdentity(env *models.TaskEnvironment) bool {
	if env == nil {
		return false
	}
	for _, repo := range env.Repos {
		if repo == nil || repo.RepositoryID == "" || repo.DeletedAt != nil || (repo.Status != "" && repo.Status != "active") {
			continue
		}
		if repo.WorktreeID == "" || repo.WorktreePath == "" || repo.WorktreeBranch == "" {
			return true
		}
	}
	return false
}

// PreflightSessionWorktreeRecovery performs selected-environment recovery
// before a recovery action mutates provider state such as its resume token.
// The returned admission must remain held through LaunchSession.
func (e *Executor) PreflightSessionWorktreeRecovery(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (*worktree.RecoveryAdmission, error) {
	if e == nil || e.repo == nil || session == nil || taskID == "" || session.TaskID != taskID {
		return nil, nil
	}
	env, err := e.resolveEnvironmentForAdmission(ctx, taskID, session.TaskEnvironmentID)
	if err != nil || env == nil {
		return nil, err
	}
	return e.admitSelectedWorktreeRecovery(ctx, taskID, session, env, env.ExecutorType)
}

type managedClonePathResolver interface {
	ManagedCloneRelocationPaths(
		*models.Repository,
	) (root, providerSource, ownerNameSource, destination string, ok bool, err error)
}

func managedCloneRelocationProof(cloner any, repository *models.Repository, recordedPath, recordedCommonDir string) *worktree.ManagedCloneRelocationProof {
	paths, ok := cloner.(managedClonePathResolver)
	if !ok || repository == nil {
		return nil
	}
	root, source, ownerNameSource, destination, managed, err := paths.ManagedCloneRelocationPaths(repository)
	if err != nil || !managed {
		return nil
	}
	return &worktree.ManagedCloneRelocationProof{
		ManagedRoot: root, ExpectedSourcePath: source, LegacyOwnerNameSourcePath: ownerNameSource,
		ExpectedDestinationPath: destination,
		RecordedSourcePath:      recordedPath, RecordedSourceCommonDir: recordedCommonDir,
		Identity: worktree.ManagedRepositoryIdentity{
			Provider: repository.Provider, Host: repository.ProviderHost,
			Owner: repository.ProviderOwner, Name: repository.ProviderName,
		},
	}
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
