package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
)

// ErrPrimaryTaskRepositoryRoot rejects removal of the root that contains the
// agent working directory.
var ErrPrimaryTaskRepositoryRoot = errors.New("the primary repository root cannot be removed from the task")

// DetachTaskRepository removes a non-primary root from the task without
// touching its files, branch, worktree registration, or linked pull requests.
func (s *Service) DetachTaskRepository(ctx context.Context, taskID, taskRepositoryID string) error {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeRepositoryManage); err != nil {
		return err
	}
	unlock := s.lockWorkspaceSources(taskID)
	defer unlock()
	if err := s.detachTaskRepositoryLocked(ctx, taskID, taskRepositoryID, true); err != nil {
		return err
	}
	recordWorktreeDetach("user")
	return nil
}

// DetachTaskRepositoryFromDisk removes a root whose adopted worktree no longer
// exists. It never records a suppression row and never touches task_prs.
func (s *Service) DetachTaskRepositoryFromDisk(ctx context.Context, taskID, taskRepositoryID string) error {
	unlock := s.lockWorkspaceSources(taskID)
	defer unlock()
	if err := s.detachTaskRepositoryLocked(ctx, taskID, taskRepositoryID, false); err != nil {
		return err
	}
	recordWorktreeDetach("removed_from_disk")
	return nil
}

func (s *Service) detachTaskRepositoryLocked(ctx context.Context, taskID, taskRepositoryID string, markDetached bool) error {
	taskRepo, err := s.loadTaskRepositoryForUpdate(ctx, taskID, taskRepositoryID)
	if err != nil {
		return err
	}
	environment, envRepo, rows, err := s.removableDetachmentTarget(ctx, taskID, taskRepo)
	if err != nil {
		return err
	}
	detachPath := taskRepositoryDetachPath(environment, taskRepo, envRepo)
	if err := s.detachAttachmentRows(ctx, taskID, taskRepo.ID, envRepo, detachPath, markDetached); err != nil {
		return err
	}
	if detachPath != "" {
		s.removeNestedWorkspaceExclusion(context.WithoutCancel(ctx), rows, detachPath)
	}
	s.PublishTaskUpdatedByID(context.WithoutCancel(ctx), taskID)
	if environment != nil {
		s.rescanAdoptedWorkspace(context.WithoutCancel(ctx), taskID, environment)
	}
	s.logger.Info("detached repository root from task",
		zap.String("task_id", taskID),
		zap.String("task_repository_id", taskRepositoryID),
		zap.String("path", detachPath),
		zap.Bool("suppressed", markDetached))
	return nil
}

func (s *Service) removableDetachmentTarget(ctx context.Context, taskID string, taskRepo *models.TaskRepository) (*models.TaskEnvironment, *models.TaskEnvironmentRepo, []*models.TaskEnvironmentRepo, error) {
	rows, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list task repositories: %w", err)
	}
	if isPrimaryTaskRepository(rows, taskRepo) {
		return nil, nil, nil, ErrPrimaryTaskRepositoryRoot
	}
	environment, envRepo, envRows, err := s.detachmentTarget(ctx, taskID, taskRepo)
	if err != nil {
		return nil, nil, nil, err
	}
	if environment != nil && envRepo != nil && environment.WorkspacePath != "" &&
		envRepo.WorktreePath != "" && sameResolvedPath(environment.WorkspacePath, envRepo.WorktreePath) {
		return nil, nil, nil, ErrPrimaryTaskRepositoryRoot
	}
	return environment, envRepo, envRows, nil
}

func (s *Service) detachAttachmentRows(ctx context.Context, taskID, taskRepositoryID string, envRepo *models.TaskEnvironmentRepo, path string, markDetached bool) error {
	environmentRepoID := ""
	if envRepo != nil {
		environmentRepoID = envRepo.ID
	}
	store, ok := s.taskEnvironments.(taskrepository.TaskDetachedWorktreeRepository)
	if !ok {
		return fmt.Errorf("detach repository attachment: atomic detachment storage is unavailable")
	}
	if err := store.DetachTaskRepositoryAttachment(ctx, taskID, taskRepositoryID, environmentRepoID, path, markDetached); err != nil {
		return fmt.Errorf("detach repository attachment: %w", err)
	}
	return nil
}

func isPrimaryTaskRepository(rows []*models.TaskRepository, target *models.TaskRepository) bool {
	if target == nil {
		return false
	}
	var primary *models.TaskRepository
	for _, row := range rows {
		if row == nil {
			continue
		}
		if primary == nil || row.Position < primary.Position {
			primary = row
		}
	}
	return primary != nil && primary.ID == target.ID
}

func (s *Service) detachmentTarget(ctx context.Context, taskID string, taskRepo *models.TaskRepository) (*models.TaskEnvironment, *models.TaskEnvironmentRepo, []*models.TaskEnvironmentRepo, error) {
	environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || environment == nil {
		return nil, nil, nil, nil
	}
	rows, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list environment repositories: %w", err)
	}
	return environment, environmentRepoForTaskRepository(rows, taskRepo), rows, nil
}

func environmentRepoForTaskRepository(rows []*models.TaskEnvironmentRepo, taskRepo *models.TaskRepository) *models.TaskEnvironmentRepo {
	var candidates []*models.TaskEnvironmentRepo
	for _, row := range rows {
		if !taskEnvironmentRepoActive(row) || row.RepositoryID != taskRepo.RepositoryID {
			continue
		}
		candidates = append(candidates, row)
		if taskRepo.WorkspaceRelativePath != "" && row.WorkspaceRelativePath == taskRepo.WorkspaceRelativePath {
			return row
		}
		if taskRepo.CheckoutBranch != "" && row.WorktreeBranch == taskRepo.CheckoutBranch {
			return row
		}
	}
	if len(candidates) == 1 && candidates[0].WorktreeOrigin != models.WorktreeOriginAdopted {
		return candidates[0]
	}
	return nil
}

func taskRepositoryDetachPath(environment *models.TaskEnvironment, taskRepo *models.TaskRepository, envRepo *models.TaskEnvironmentRepo) string {
	if envRepo != nil && envRepo.WorktreePath != "" {
		resolved, err := filepath.EvalSymlinks(filepath.Clean(envRepo.WorktreePath))
		if err == nil {
			return resolved
		}
		return filepath.Clean(envRepo.WorktreePath)
	}
	if taskRepo == nil || taskRepo.WorkspaceRelativePath == "" || environment == nil {
		return ""
	}
	root, err := taskWorkspaceRootPath(environment)
	if err != nil {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(taskRepo.WorkspaceRelativePath))
}

func taskWorkspaceRootPath(environment *models.TaskEnvironment) (string, error) {
	workspace := strings.TrimSpace(environment.WorkspacePath)
	if workspace == "" {
		return "", fmt.Errorf("task workspace root is unknown")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(workspace))
	if err != nil {
		return filepath.Clean(workspace), nil
	}
	return resolved, nil
}

// TaskDetachedWorktreeStore exposes the suppression table to callers that
// need to reason about a path without loading the full task service.
func (s *Service) TaskDetachedWorktreeStore() taskrepository.TaskDetachedWorktreeRepository {
	store, _ := s.taskEnvironments.(taskrepository.TaskDetachedWorktreeRepository)
	return store
}
