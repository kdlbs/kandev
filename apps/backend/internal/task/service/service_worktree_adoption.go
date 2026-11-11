package service

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

// WorktreeAdoptionSource distinguishes an explicit agent report from passive
// observation. Detected candidates are suppressible and never fail a request.
type WorktreeAdoptionSource string

const (
	WorktreeAdoptionSourceReported WorktreeAdoptionSource = "reported"
	WorktreeAdoptionSourceDetected WorktreeAdoptionSource = "detected"
)

// ErrWorktreeNotAdoptable marks a candidate that is not a linked worktree of an
// attached repository inside the task workspace root.
var ErrWorktreeNotAdoptable = errors.New("worktree is not adoptable")

const (
	worktreeAdoptionResultRejected   = "rejected"
	worktreeAdoptionResultSuppressed = "suppressed"
	worktreeAdoptionResultAdopted    = "adopted"
	worktreeAdoptionResultAttached   = "already_attached"
)

var (
	worktreeAdoptionTotal = expvar.NewMap("task_worktree_adoption_total")
	worktreeDetachTotal   = expvar.NewMap("task_worktree_detach_total")
)

func recordWorktreeAdoption(source WorktreeAdoptionSource, result string) {
	worktreeAdoptionTotal.Add(string(source)+"|"+result, 1)
}

func recordWorktreeDetach(reason string) {
	worktreeDetachTotal.Add(reason, 1)
}

// AdoptWorktreeRequest asks the task service to attach an agent-created linked
// worktree as a task branch.
type AdoptWorktreeRequest struct {
	TaskID         string
	Path           string
	BaseBranch     string
	CheckoutBranch string
	Source         WorktreeAdoptionSource
}

// AdoptWorktreeResult reports the adopted attachment and the unchanged paths
// the caller needs to render the result.
type AdoptWorktreeResult struct {
	*models.TaskRepository
	WorktreePath      string
	TaskWorkspacePath string
	WorktreeBranch    string
	Adopted           bool
	AlreadyAttached   bool
}

type gitWorktreeState struct {
	Path         string
	TopLevel     string
	CommonDir    string
	GitDir       string
	Branch       string
	HeadSHA      string
	Superproject string
}

type worktreeAdoptionState struct {
	environment   *models.TaskEnvironment
	workspaceRoot string
	envRepos      []*models.TaskEnvironmentRepo
	candidate     *gitWorktreeState
	repository    *models.Repository
	existingPath  *models.TaskEnvironmentRepo
}

// AdoptWorktree attaches an existing linked worktree of a repository that is
// already part of the task. It never moves, recreates, or writes Git state.
func (s *Service) AdoptWorktree(ctx context.Context, req AdoptWorktreeRequest) (*AdoptWorktreeResult, error) {
	source := req.Source
	if source == "" {
		source = WorktreeAdoptionSourceReported
	}
	req.Source = source
	if req.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	if strings.TrimSpace(req.Path) == "" {
		return nil, fmt.Errorf("%w: worktree_path is required", ErrWorktreeNotAdoptable)
	}
	if source == WorktreeAdoptionSourceReported {
		if err := s.authorizeTaskScope(ctx, req.TaskID, authz.ScopeRepositoryManage); err != nil {
			recordWorktreeAdoption(source, worktreeAdoptionResultRejected)
			return nil, err
		}
	}
	unlock := s.lockWorkspaceSources(req.TaskID)
	defer unlock()

	state, err := s.inspectAdoptableWorktree(ctx, req)
	if err != nil {
		recordWorktreeAdoption(source, rejectionResult(source, err))
		return nil, err
	}
	if state.existingPath != nil {
		return s.reuseAdoptedWorktree(ctx, req, state)
	}
	if state.repository == nil {
		recordWorktreeAdoption(source, rejectionResult(source, ErrWorktreeNotAdoptable))
		return nil, fmt.Errorf("%w: worktree %q does not belong to a repository attached to this task", ErrWorktreeNotAdoptable, req.Path)
	}
	return s.commitAdoptedWorktree(ctx, req, source, state)
}

func rejectionResult(source WorktreeAdoptionSource, err error) string {
	if source == WorktreeAdoptionSourceDetected && errors.Is(err, errAdoptionSuppressed) {
		return worktreeAdoptionResultSuppressed
	}
	return worktreeAdoptionResultRejected
}

var errAdoptionSuppressed = errors.New("worktree adoption suppressed")

func (s *Service) reuseAdoptedWorktree(ctx context.Context, req AdoptWorktreeRequest, state *worktreeAdoptionState) (*AdoptWorktreeResult, error) {
	if err := s.clearDetachedWorktree(ctx, req.TaskID, state.candidate.Path); err != nil {
		return nil, err
	}
	if _, err := s.ensureNestedWorkspaceExclusion(ctx, state.envRepos, state.candidate.Path); err != nil {
		if req.Source == WorktreeAdoptionSourceReported {
			return nil, err
		}
		s.logger.Warn("detected worktree is nested in a checkout without an effective exclusion",
			zap.String("task_id", req.TaskID), zap.String("path", state.candidate.Path), zap.Error(err))
	}
	taskRepo := s.taskRepositoryForEnvironmentRow(ctx, req.TaskID, state.existingPath)
	if taskRepo == nil {
		// The inventory row survives without its attachment row. Repair the
		// task-side row instead of creating a second physical root.
		if state.repository == nil {
			return nil, fmt.Errorf("%w: repository is unavailable", ErrWorktreeNotAdoptable)
		}
		return s.commitAdoptedWorktree(ctx, req, WorktreeAdoptionSourceReported, state)
	}
	if requestedBase := strings.TrimSpace(req.BaseBranch); requestedBase != "" && requestedBase != taskRepo.BaseBranch {
		updated, updateErr := s.UpdateRepositoryBaseBranch(ctx, UpdateRepositoryBaseBranchRequest{
			TaskID: req.TaskID, TaskRepositoryID: taskRepo.ID, BaseBranch: requestedBase,
		})
		if updateErr != nil {
			return nil, updateErr
		}
		taskRepo = updated
	}
	s.rescanAdoptedWorkspace(ctx, req.TaskID, state.environment)
	recordWorktreeAdoption(req.Source, worktreeAdoptionResultAttached)
	return &AdoptWorktreeResult{
		TaskRepository:    taskRepo,
		WorktreePath:      state.candidate.Path,
		TaskWorkspacePath: state.workspaceRoot,
		WorktreeBranch:    state.candidate.Branch,
		Adopted:           true,
		AlreadyAttached:   true,
	}, nil
}

func (s *Service) inspectAdoptableWorktree(ctx context.Context, req AdoptWorktreeRequest) (*worktreeAdoptionState, error) {
	environment, err := s.loadWorktreeAdoptionEnvironment(ctx, req.TaskID, req.Source)
	if err != nil {
		return nil, err
	}
	if environment == nil {
		return nil, fmt.Errorf("%w: task has no provisioned environment", ErrWorktreeNotAdoptable)
	}
	if !supportsWorktreeAdoption(environment.ExecutorType) {
		if req.Source == WorktreeAdoptionSourceDetected {
			return nil, fmt.Errorf("%w: executor %q does not support worktree adoption", errAdoptionSuppressed, environment.ExecutorType)
		}
		return nil, fmt.Errorf("worktree adoption is only supported on host Worktree and Local executors; this task uses %q", environment.ExecutorType)
	}
	envRepos, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, environment.ID)
	if err != nil {
		return nil, fmt.Errorf("list environment repositories: %w", err)
	}
	root, err := s.taskWorkspaceRoot(environment, envRepos)
	if err != nil {
		return nil, err
	}
	candidate, err := s.resolveAdoptionCandidate(ctx, root, req.Path)
	if err != nil {
		return nil, err
	}
	if req.CheckoutBranch != "" && req.CheckoutBranch != candidate.Branch {
		return nil, fmt.Errorf("%w: checkout_branch %q does not match the worktree branch %q", ErrWorktreeNotAdoptable, req.CheckoutBranch, candidate.Branch)
	}
	if err := s.rejectDetachedAdoptionCandidate(ctx, req, candidate.Path); err != nil {
		return nil, err
	}
	state := &worktreeAdoptionState{environment: environment, workspaceRoot: root, envRepos: envRepos, candidate: candidate}
	if existing := environmentRepoByPath(envRepos, candidate.Path); existing != nil {
		if err := s.loadExistingAdoptionRepository(ctx, state, existing); err != nil {
			return nil, err
		}
		return state, nil
	}
	repository, err := s.repositoryForWorktree(ctx, envRepos, candidate)
	if err != nil {
		return nil, err
	}
	state.repository = repository
	return state, nil
}

func (s *Service) loadWorktreeAdoptionEnvironment(ctx context.Context, taskID string, source WorktreeAdoptionSource) (*models.TaskEnvironment, error) {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("%w: %s", sqlite.ErrTaskNotFound, taskID)
	}
	environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || environment == nil {
		if source == WorktreeAdoptionSourceDetected {
			return nil, fmt.Errorf("%w: task has no provisioned environment", errAdoptionSuppressed)
		}
		if err != nil {
			return nil, fmt.Errorf("look up task environment: %w", err)
		}
		return nil, fmt.Errorf("%w: task has no provisioned environment", ErrWorktreeNotAdoptable)
	}
	return environment, nil
}

func supportsWorktreeAdoption(executorType string) bool {
	switch executorType {
	case string(models.ExecutorTypeWorktree), string(models.ExecutorTypeLocal), "local_pc":
		return true
	default:
		return false
	}
}

func isHostLocalAdoptionExecutor(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocal) || executorType == "local_pc"
}

// taskWorkspaceRoot resolves the containment boundary for a task. A single
// worktree checkout uses its parent task folder; a multi-repository or
// local-executor workspace already is the root.
func (s *Service) taskWorkspaceRoot(environment *models.TaskEnvironment, envRepos []*models.TaskEnvironmentRepo) (string, error) {
	workspace := strings.TrimSpace(environment.WorkspacePath)
	primary := primaryEnvironmentRepo(envRepos)
	if isHostLocalAdoptionExecutor(environment.ExecutorType) && workspace != "" {
		return canonicalDirectory(workspace)
	}
	if workspace != "" {
		if primary != nil && primary.WorktreePath != "" && sameResolvedPath(workspace, primary.WorktreePath) {
			return taskContainmentRoot(environment, workspace, primary.WorktreePath)
		}
		return canonicalDirectory(workspace)
	}
	if primary != nil && primary.WorktreePath != "" {
		return taskContainmentRoot(environment, primary.WorktreePath, primary.WorktreePath)
	}
	if primary != nil {
		repository, err := s.repoEntities.GetRepository(context.Background(), primary.RepositoryID)
		if err == nil && repository != nil && repository.LocalPath != "" {
			if isHostLocalAdoptionExecutor(environment.ExecutorType) {
				return canonicalDirectory(repository.LocalPath)
			}
			return canonicalDirectory(filepath.Dir(repository.LocalPath))
		}
	}
	return "", fmt.Errorf("%w: task workspace root could not be resolved", ErrWorktreeNotAdoptable)
}

func taskContainmentRoot(environment *models.TaskEnvironment, workspace, primaryPath string) (string, error) {
	if environment.TaskDirName == "" {
		return canonicalDirectory(workspace)
	}
	return canonicalDirectory(filepath.Dir(primaryPath))
}

func primaryEnvironmentRepo(envRepos []*models.TaskEnvironmentRepo) *models.TaskEnvironmentRepo {
	var primary *models.TaskEnvironmentRepo
	for _, repo := range envRepos {
		if repo == nil || !taskEnvironmentRepoActive(repo) {
			continue
		}
		if primary == nil || repo.Position < primary.Position {
			primary = repo
		}
	}
	return primary
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: workspace path is empty", ErrWorktreeNotAdoptable)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: resolve workspace path %q: %v", ErrWorktreeNotAdoptable, path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: workspace path is not a directory: %s", ErrWorktreeNotAdoptable, path)
	}
	return resolved, nil
}

func sameResolvedPath(left, right string) bool {
	leftResolved, leftErr := filepath.EvalSymlinks(filepath.Clean(left))
	rightResolved, rightErr := filepath.EvalSymlinks(filepath.Clean(right))
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return leftResolved == rightResolved
}

func (s *Service) resolveAdoptionCandidate(ctx context.Context, root, requested string) (*gitWorktreeState, error) {
	path := strings.TrimSpace(requested)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("%w: worktree path %q does not exist", ErrWorktreeNotAdoptable, requested)
	}
	if !pathWithinDirectory(resolved, root) {
		return nil, fmt.Errorf("%w: worktree path %q is outside the task workspace root", ErrWorktreeNotAdoptable, requested)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: worktree path %q is not a directory", ErrWorktreeNotAdoptable, requested)
	}
	state, err := inspectGitWorktree(ctx, resolved)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorktreeNotAdoptable, err)
	}
	if state.TopLevel == "" {
		return nil, fmt.Errorf("%w: cannot resolve the worktree root for %q", ErrWorktreeNotAdoptable, requested)
	}
	if !sameResolvedPath(state.TopLevel, resolved) {
		return nil, fmt.Errorf("%w: %q is inside a worktree; report the worktree root %q", ErrWorktreeNotAdoptable, requested, state.TopLevel)
	}
	if state.GitDir == state.CommonDir {
		return nil, fmt.Errorf("%w: %q is a primary checkout, not a linked worktree", ErrWorktreeNotAdoptable, requested)
	}
	if state.Superproject != "" {
		return nil, fmt.Errorf("%w: %q is a submodule checkout", ErrWorktreeNotAdoptable, requested)
	}
	if state.Branch == "" {
		return nil, fmt.Errorf("%w: worktree %q has a detached HEAD; check out a named branch first", ErrWorktreeNotAdoptable, requested)
	}
	return state, nil
}

func inspectGitWorktree(ctx context.Context, path string) (*gitWorktreeState, error) {
	gitDir, err := runGitPath(ctx, path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("inspect git directory: %w", err)
	}
	commonDir, err := runGitPath(ctx, path, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("inspect git common directory: %w", err)
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(path, commonDir)
	}
	commonDir, err = filepath.EvalSymlinks(filepath.Clean(commonDir))
	if err != nil {
		return nil, fmt.Errorf("resolve git common directory: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(filepath.Clean(gitDir)); resolveErr == nil {
		gitDir = resolved
	}
	branch, branchErr := runGitValue(ctx, path, "symbolic-ref", "--short", "--quiet", "HEAD")
	if branchErr != nil {
		branch = ""
	}
	head, _ := runGitValue(ctx, path, "rev-parse", "--verify", "HEAD")
	superproject, _ := runGitValue(ctx, path, "rev-parse", "--show-superproject-working-tree")
	topLevel, topLevelErr := runGitPath(ctx, path, "rev-parse", "--show-toplevel")
	if topLevelErr == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(filepath.Clean(topLevel)); resolveErr == nil {
			topLevel = resolved
		}
	} else {
		topLevel = ""
	}
	return &gitWorktreeState{
		Path: path, TopLevel: topLevel, CommonDir: commonDir, GitDir: filepath.Clean(gitDir),
		Branch: strings.TrimSpace(branch), HeadSHA: strings.TrimSpace(head),
		Superproject: strings.TrimSpace(superproject),
	}, nil
}

func runGitPath(ctx context.Context, dir string, args ...string) (string, error) {
	value, err := runGitValue(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func runGitValue(ctx context.Context, dir string, args ...string) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmdArgs := append([]string{"-C", dir}, args...)
	out, err := subproc.RunGitOutputClass(execCtx, subproc.GitBackground, subproc.NewGitCommand(execCtx, cmdArgs...))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pathWithinDirectory(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func taskEnvironmentRepoActive(repo *models.TaskEnvironmentRepo) bool {
	return repo != nil && repo.DeletedAt == nil && repo.Status == models.TaskEnvironmentRepoStatusActive
}

func environmentRepoByPath(envRepos []*models.TaskEnvironmentRepo, path string) *models.TaskEnvironmentRepo {
	for _, repo := range envRepos {
		if !taskEnvironmentRepoActive(repo) || repo.WorktreePath == "" {
			continue
		}
		if sameResolvedPath(repo.WorktreePath, path) {
			return repo
		}
	}
	return nil
}

func environmentRepoByPathIncludingDetached(envRepos []*models.TaskEnvironmentRepo, path string) *models.TaskEnvironmentRepo {
	for _, repo := range envRepos {
		if repo == nil || repo.WorktreePath == "" {
			continue
		}
		if sameResolvedPath(repo.WorktreePath, path) {
			return repo
		}
	}
	return nil
}

func (s *Service) repositoryForWorktree(ctx context.Context, envRepos []*models.TaskEnvironmentRepo, candidate *gitWorktreeState) (*models.Repository, error) {
	seen := map[string]bool{}
	for _, envRepo := range envRepos {
		if envRepo == nil || envRepo.RepositoryID == "" || seen[envRepo.RepositoryID] {
			continue
		}
		seen[envRepo.RepositoryID] = true
		if envRepo.WorktreeSourceCommonDir != "" && sameResolvedPath(envRepo.WorktreeSourceCommonDir, candidate.CommonDir) {
			return s.repoEntities.GetRepository(ctx, envRepo.RepositoryID)
		}
		repository, err := s.repoEntities.GetRepository(ctx, envRepo.RepositoryID)
		if err != nil || repository == nil {
			continue
		}
		commonDir, err := repositoryCommonDir(ctx, repository, envRepo)
		if err == nil && sameResolvedPath(commonDir, candidate.CommonDir) {
			return repository, nil
		}
	}
	return nil, nil
}

func repositoryCommonDir(ctx context.Context, repository *models.Repository, envRepo *models.TaskEnvironmentRepo) (string, error) {
	candidates := []string{envRepo.WorktreeSourceClonePath, repository.LocalPath}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		commonDir, err := runGitPath(ctx, path, "rev-parse", "--git-common-dir")
		if err != nil {
			continue
		}
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(path, commonDir)
		}
		return filepath.EvalSymlinks(filepath.Clean(commonDir))
	}
	return "", fmt.Errorf("git common directory is unknown")
}

func (s *Service) taskRepositoryForEnvironmentRow(ctx context.Context, taskID string, envRepo *models.TaskEnvironmentRepo) *models.TaskRepository {
	if envRepo == nil {
		return nil
	}
	rows, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil
	}
	var candidates []*models.TaskRepository
	for _, row := range rows {
		if row.RepositoryID != envRepo.RepositoryID {
			continue
		}
		candidates = append(candidates, row)
		if envRepo.WorkspaceRelativePath != "" && row.WorkspaceRelativePath == envRepo.WorkspaceRelativePath {
			return row
		}
		if envRepo.WorktreeBranch != "" && row.CheckoutBranch == envRepo.WorktreeBranch {
			return row
		}
	}
	if envRepo.WorktreeOrigin == models.WorktreeOriginAdopted || len(candidates) != 1 {
		return nil
	}
	return candidates[0]
}

func (s *Service) commitAdoptedWorktree(ctx context.Context, req AdoptWorktreeRequest, source WorktreeAdoptionSource, state *worktreeAdoptionState) (*AdoptWorktreeResult, error) {
	task, err := s.tasks.GetTask(ctx, req.TaskID)
	if err != nil || task == nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	existing, err := s.taskRepos.ListTaskRepositories(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("list existing branches: %w", err)
	}
	baseBranch, selectionSource, err := adoptionBaseBranch(req.BaseBranch, state.repository)
	if err != nil {
		return nil, err
	}
	checkoutBranch := state.candidate.Branch
	if err := validateWorkspaceSourceBranches(baseBranch, checkoutBranch); err != nil {
		return nil, err
	}
	if err := s.rejectAdoptionDuplicate(existing, state.repository.ID, baseBranch, checkoutBranch, state.candidate.Path); err != nil {
		return nil, err
	}
	metadata := map[string]interface{}{}
	if err := models.PutBaseBranchSelection(metadata, &models.BaseBranchSelection{
		Version: models.BaseBranchSelectionVersion, Branch: baseBranch, Source: selectionSource,
	}); err != nil {
		return nil, err
	}
	relativePath, err := relativeWorkspacePath(state.workspaceRoot, state.candidate.Path)
	if err != nil {
		return nil, err
	}
	taskRepo := &models.TaskRepository{
		RepositoryID: state.repository.ID, BaseBranch: baseBranch,
		CheckoutBranch: checkoutBranch, WorkspaceRelativePath: relativePath, Metadata: metadata,
	}
	batch := &models.WorkspaceSourceBatch{TaskID: task.ID, Sources: []models.WorkspaceSource{{Repository: taskRepo}}}
	if err := s.rejectRuntimeNameCollisions(ctx, append(existing, taskRepo), batch); err != nil {
		return nil, err
	}
	exclusionAdded, err := s.ensureNestedWorkspaceExclusion(ctx, state.envRepos, state.candidate.Path)
	if err != nil {
		return nil, err
	}
	materialize := func(ctx context.Context, taskID string, batch *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
		return s.materializeAdoptedWorktree(ctx, taskID, batch, state)
	}
	if _, err := s.commitWorkspaceSourceBatch(ctx, task, batch, func(context.Context) {}, materialize); err != nil {
		if exclusionAdded {
			s.removeNestedWorkspaceExclusion(context.WithoutCancel(ctx), state.envRepos, state.candidate.Path)
		}
		return nil, err
	}
	if err := s.clearDetachedWorktree(ctx, req.TaskID, state.candidate.Path); err != nil {
		return nil, err
	}
	recordWorktreeAdoption(source, worktreeAdoptionResultAdopted)
	s.logger.Info("adopted agent-created worktree",
		zap.String("task_id", req.TaskID),
		zap.String("task_environment_id", state.environment.ID),
		zap.String("path", state.candidate.Path),
		zap.String("branch", state.candidate.Branch),
		zap.String("source", string(source)))
	return &AdoptWorktreeResult{
		TaskRepository: taskRepo, WorktreePath: state.candidate.Path,
		TaskWorkspacePath: state.workspaceRoot, WorktreeBranch: state.candidate.Branch,
		Adopted: true,
	}, nil
}

func adoptionBaseBranch(requested string, repository *models.Repository) (string, models.BaseBranchSelectionSource, error) {
	if branch := strings.TrimSpace(requested); branch != "" {
		return branch, models.BaseBranchSelectionSourceExplicit, nil
	}
	if repository == nil || repository.DefaultBranch == "" {
		return "", "", fmt.Errorf("cannot resolve base_branch for repository %q: pass base_branch explicitly", repositoryNameOrID(repository))
	}
	return repository.DefaultBranch, models.BaseBranchSelectionSourceRepositoryDefault, nil
}

func repositoryNameOrID(repository *models.Repository) string {
	if repository == nil {
		return ""
	}
	if repository.Name != "" {
		return repository.Name
	}
	return repository.ID
}

func (s *Service) rejectDetachedAdoptionCandidate(ctx context.Context, req AdoptWorktreeRequest, path string) error {
	detached, err := s.isWorktreeDetached(ctx, req.TaskID, path)
	if err != nil {
		return err
	}
	if detached && req.Source == WorktreeAdoptionSourceDetected {
		return fmt.Errorf("%w: worktree %q was removed from this task", errAdoptionSuppressed, path)
	}
	return nil
}

func (s *Service) loadExistingAdoptionRepository(ctx context.Context, state *worktreeAdoptionState, existing *models.TaskEnvironmentRepo) error {
	state.existingPath = existing
	repository, err := s.repoEntities.GetRepository(ctx, existing.RepositoryID)
	if err != nil {
		return fmt.Errorf("load repository %q: %w", existing.RepositoryID, err)
	}
	if repository == nil {
		return fmt.Errorf("%w: repository %q is no longer available", ErrWorktreeNotAdoptable, existing.RepositoryID)
	}
	state.repository = repository
	return nil
}

func (s *Service) rejectAdoptionDuplicate(existing []*models.TaskRepository, repositoryID, baseBranch, checkoutBranch, path string) error {
	newSlug := worktree.SanitizeBranchSlug(checkoutBranch)
	for _, row := range existing {
		if row.RepositoryID == repositoryID && row.BaseBranch == baseBranch && row.CheckoutBranch == checkoutBranch {
			return branchAddDuplicateError(nil, repositoryID, baseBranch, checkoutBranch)
		}
		if row.RepositoryID == repositoryID && checkoutBranch != "" && row.CheckoutBranch != "" &&
			row.CheckoutBranch != checkoutBranch && worktree.SanitizeBranchSlug(row.CheckoutBranch) == newSlug && newSlug != "" {
			return fmt.Errorf("checkout branch %q conflicts with existing branch %q on this task (both resolve to the same worktree path)", checkoutBranch, row.CheckoutBranch)
		}
	}
	return nil
}

func relativeWorkspacePath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path relative to task workspace: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: worktree path %q is outside the task workspace root", ErrWorktreeNotAdoptable, path)
	}
	return filepath.ToSlash(rel), nil
}

func (s *Service) ensureNestedWorkspaceExclusion(ctx context.Context, rows []*models.TaskEnvironmentRepo, path string) (bool, error) {
	outer := outerWorkspaceCheckout(rows, path)
	if outer == "" || s.nestedWorkspaceExclusioner == nil {
		return false, nil
	}
	changed, err := s.nestedWorkspaceExclusioner.AddNestedWorkspaceExclusion(ctx, outer, path)
	if err != nil {
		return false, fmt.Errorf("protect containing checkout from nested worktree: %w", err)
	}
	return changed, nil
}

func (s *Service) removeNestedWorkspaceExclusion(ctx context.Context, rows []*models.TaskEnvironmentRepo, path string) {
	outer := outerWorkspaceCheckout(rows, path)
	if outer == "" || s.nestedWorkspaceExclusioner == nil {
		return
	}
	if err := s.nestedWorkspaceExclusioner.RemoveNestedWorkspaceExclusion(ctx, outer, path); err != nil {
		s.logger.Warn("failed to remove nested worktree exclusion",
			zap.String("outer_root", outer), zap.String("path", path), zap.Error(err))
	}
}

func outerWorkspaceCheckout(rows []*models.TaskEnvironmentRepo, path string) string {
	resolvedPath := resolvePathAllowingMissing(path)
	longest := ""
	for _, row := range rows {
		if row == nil || row.WorktreePath == "" || row.Status != "active" || row.DeletedAt != nil {
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(row.WorktreePath))
		if err != nil || !pathWithinDirectory(resolvedPath, resolvedRoot) {
			continue
		}
		if len(resolvedRoot) > len(longest) {
			longest = resolvedRoot
		}
	}
	return longest
}

func resolvePathAllowingMissing(path string) string {
	if resolved, err := filepath.EvalSymlinks(filepath.Clean(path)); err == nil {
		return resolved
	}
	parent := filepath.Dir(filepath.Clean(path))
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(resolvedParent, filepath.Base(path))
	}
	return filepath.Clean(path)
}

func (s *Service) isWorktreeDetached(ctx context.Context, taskID, path string) (bool, error) {
	store, ok := s.taskEnvironments.(taskrepository.TaskDetachedWorktreeRepository)
	if !ok {
		return false, nil
	}
	return store.IsTaskWorktreeDetached(ctx, taskID, path)
}

func (s *Service) clearDetachedWorktree(ctx context.Context, taskID, path string) error {
	store, ok := s.taskEnvironments.(taskrepository.TaskDetachedWorktreeRepository)
	if !ok {
		return nil
	}
	return store.ClearTaskWorktreeDetached(ctx, taskID, path)
}
