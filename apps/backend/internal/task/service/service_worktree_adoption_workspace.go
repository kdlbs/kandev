package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func (s *Service) materializeAdoptedWorktree(ctx context.Context, taskID string, batch *models.WorkspaceSourceBatch, state *worktreeAdoptionState) (*WorkspaceSourceMaterializationResult, error) {
	if len(batch.Sources) == 0 || batch.Sources[0].Repository == nil {
		return nil, fmt.Errorf("adoption materialization requires one repository source")
	}
	taskRepo := batch.Sources[0].Repository
	row, err := s.persistAdoptedInventory(ctx, state, taskRepo)
	if err != nil {
		return nil, err
	}
	sessionIDs := s.rescanAdoptedWorkspace(ctx, taskID, state.environment)
	return &WorkspaceSourceMaterializationResult{
		WorkspacePath: state.workspaceRoot,
		WorktreePath:  row.WorktreePath,
		SessionIDs:    sessionIDs,
	}, nil
}

func (s *Service) persistAdoptedInventory(ctx context.Context, state *worktreeAdoptionState, taskRepo *models.TaskRepository) (*models.TaskEnvironmentRepo, error) {
	rows, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, state.environment.ID)
	if err != nil {
		return nil, fmt.Errorf("list environment repositories: %w", err)
	}
	existing := environmentRepoByPathIncludingDetached(rows, state.candidate.Path)
	if existing == nil {
		existing = &models.TaskEnvironmentRepo{
			TaskEnvironmentID: state.environment.ID,
			RepositoryID:      taskRepo.RepositoryID,
			WorktreeID:        uuid.NewString(),
			Position:          taskRepo.Position,
		}
	}
	existing.RepositoryID = taskRepo.RepositoryID
	existing.WorkspaceRelativePath = taskRepo.WorkspaceRelativePath
	existing.BranchSlug = uniqueAdoptionBranchSlug(rows, taskRepo.RepositoryID, state.candidate.Branch, state.candidate.Path)
	existing.WorktreePath = state.candidate.Path
	existing.WorktreeBranch = state.candidate.Branch
	existing.WorktreeBranchOwner = worktree.BranchOwnerExternal
	existing.WorktreeIntegrationRef = taskRepo.BaseBranch
	existing.WorktreeSourceCommonDir = state.candidate.CommonDir
	existing.WorktreeSourceClonePath = sourceClonePath(state.candidate.CommonDir)
	existing.WorktreeOrigin = models.WorktreeOriginAdopted
	existing.Status = "active"
	existing.DeletedAt = nil
	existing.ErrorMessage = ""
	if existing.ID == "" {
		if err := s.taskEnvironments.CreateTaskEnvironmentRepo(ctx, existing); err != nil {
			return nil, fmt.Errorf("persist adopted worktree inventory: %w", err)
		}
		return existing, nil
	}
	if err := s.taskEnvironments.UpdateTaskEnvironmentRepo(ctx, existing); err != nil {
		return nil, fmt.Errorf("update adopted worktree inventory: %w", err)
	}
	return existing, nil
}

func uniqueAdoptionBranchSlug(rows []*models.TaskEnvironmentRepo, repositoryID, branch, path string) string {
	base := worktree.SanitizeBranchSlug(branch)
	if base == "" {
		base = "adopted"
	}
	used := map[string]bool{}
	for _, row := range rows {
		if row == nil || row.RepositoryID != repositoryID {
			continue
		}
		if sameResolvedPath(row.WorktreePath, path) {
			return row.BranchSlug
		}
		used[row.BranchSlug] = true
	}
	if !used[base] {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func sourceClonePath(commonDir string) string {
	commonDir = filepath.Clean(commonDir)
	base := filepath.Base(commonDir)
	if base == ".git" {
		return filepath.Dir(commonDir)
	}
	if strings.HasSuffix(base, ".git") {
		return commonDir
	}
	return filepath.Dir(commonDir)
}

// rescanAdoptedWorkspace adds or retires the adopted path on every live session
// of the environment. Failure never rolls back the durable attachment.
func (s *Service) rescanAdoptedWorkspace(ctx context.Context, taskID string, environment *models.TaskEnvironment) []string {
	if s.workspaceSourceRescanner == nil || s.sessions == nil || environment == nil {
		return nil
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("list sessions for workspace rescan", zap.String("task_id", taskID), zap.Error(err))
		return nil
	}
	roots, err := s.workspaceSourceRootsForTask(ctx, taskID, environment)
	if err != nil {
		s.logger.Warn("resolve workspace source roots for rescan", zap.String("task_id", taskID), zap.Error(err))
		return nil
	}
	workDir := environment.WorkspacePath
	if rows, listErr := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, environment.ID); listErr == nil {
		if root, rootErr := s.taskWorkspaceRoot(environment, rows); rootErr == nil {
			workDir = root
		}
	}
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session == nil || session.TaskEnvironmentID != environment.ID || !branchMaterializationSessionEligible(session) {
			continue
		}
		ids = append(ids, session.ID)
		if err := s.workspaceSourceRescanner.RescanWorkspaceForSession(ctx, session.ID, workDir, roots); err != nil {
			s.logger.Warn("workspace rescan after adoption failed",
				zap.String("task_id", taskID),
				zap.String("session_id", session.ID),
				zap.Error(err))
		}
	}
	return ids
}

func (s *Service) workspaceSourceRootsForTask(ctx context.Context, taskID string, environment *models.TaskEnvironment) ([]string, error) {
	rows, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil {
		return nil, err
	}
	taskRepos, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	roots := environmentRepoWorktreePaths(rows)
	repositoryRoots, err := s.attachedRepositoryRoots(ctx, taskRepos, rows)
	if err != nil {
		return nil, err
	}
	roots = append(roots, repositoryRoots...)
	if s.workspaceFolders != nil {
		folders, err := s.workspaceFolders.ListTaskWorkspaceFolders(ctx, taskID)
		if err == nil {
			for _, folder := range folders {
				if folder != nil {
					roots = append(roots, folder.LocalPath)
				}
			}
		}
	}
	return canonicalUniqueRoots(roots), nil
}

func environmentRepoWorktreePaths(rows []*models.TaskEnvironmentRepo) []string {
	paths := make([]string, 0, len(rows))
	for _, row := range rows {
		if taskEnvironmentRepoActive(row) && row.WorktreePath != "" {
			paths = append(paths, row.WorktreePath)
		}
	}
	return paths
}

func (s *Service) attachedRepositoryRoots(ctx context.Context, taskRepos []*models.TaskRepository, rows []*models.TaskEnvironmentRepo) ([]string, error) {
	covered := make(map[string]bool, len(rows))
	for _, row := range rows {
		if taskEnvironmentRepoActive(row) && row.WorktreePath != "" {
			covered[row.RepositoryID] = true
		}
	}
	paths := make([]string, 0, len(taskRepos))
	for _, taskRepo := range taskRepos {
		if covered[taskRepo.RepositoryID] {
			continue
		}
		repository, err := s.repoEntities.GetRepository(ctx, taskRepo.RepositoryID)
		if err != nil {
			return nil, err
		}
		if repository != nil && repository.LocalPath != "" {
			paths = append(paths, repository.LocalPath)
		}
	}
	return paths, nil
}

func canonicalUniqueRoots(roots []string) []string {
	seen := make(map[string]bool, len(roots))
	out := make([]string, 0, len(roots))
	for _, path := range roots {
		if path == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
		if err != nil || seen[resolved] {
			continue
		}
		seen[resolved] = true
		out = append(out, resolved)
	}
	sort.Strings(out)
	return out
}
