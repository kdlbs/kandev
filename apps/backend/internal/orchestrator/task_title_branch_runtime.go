package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// titleBranchRuntime is intentionally narrower than the full agent manager
// interface. Branch renames are workspace operations owned by lifecycle, so
// adding this seam does not force every runtime test double to implement a new
// method.
type titleBranchRuntime interface {
	RenameBranchForSession(ctx context.Context, sessionID, newName, repo string) (*runtimeapi.GitOperationResult, error)
}

type titleBranchPrimaryRuntime interface {
	RenameBranchForSessionWithPrimary(ctx context.Context, sessionID, newName, repo string, primary bool) (*runtimeapi.GitOperationResult, error)
}

// titleBranchStore contains the repository reads and per-repository snapshot
// writes needed by the title handoff. The production SQLite repository
// implements this superset; keeping it optional preserves the smaller
// orchestrator test repositories.
type titleBranchStore interface {
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskEnvironmentRepo, error)
	ListTaskEnvironmentRepos(ctx context.Context, envID string) ([]*models.TaskEnvironmentRepo, error)
	UpdateTaskEnvironmentRepo(ctx context.Context, repo *models.TaskEnvironmentRepo) error
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
}

type titleBranchScopedSnapshotStore interface {
	UpdateTaskSessionWorktreeBranchByRepository(ctx context.Context, sessionID, repositoryID, branch string) error
}

type titleBranchWorktreeSnapshotStore interface {
	UpdateTaskSessionWorktreeBranchByWorktree(ctx context.Context, sessionID, worktreeID, branch string) error
}

type titleBranchWorktreeLister interface {
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskEnvironmentRepo, error)
}

type titleBranchBinding struct {
	sessionID       string
	taskRepository  *models.TaskRepository
	repository      *models.Repository
	worktree        *models.TaskEnvironmentRepo
	environmentRepo *models.TaskEnvironmentRepo
	plan            worktree.BranchIdentityPlan
}

// SetTitleBranchRuntime wires the lifecycle-owned branch operation used by
// RenameGeneratedBranchesForTaskTitle. Nil disables Git side effects while the
// accepted title response remains successful.
func (s *Service) SetTitleBranchRuntime(runtime titleBranchRuntime) {
	s.titleBranchRuntime = runtime
}

// RenameGeneratedBranchesForTaskTitle updates Kandev-owned branches after the
// title-owner session resolves a prompt-first title. The title write has
// already succeeded before this method is called; branch failures therefore
// remain per-repository outcomes and never roll the title back.
func (s *Service) RenameGeneratedBranchesForTaskTitle(
	ctx context.Context,
	taskID string,
	sessionID string,
	title string,
) (TitleBranchRenameResult, error) {
	var result TitleBranchRenameResult

	// This must remain the first operation for a task+session entry point.
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return result, err
	}
	if s.repo == nil {
		return result, fmt.Errorf("orchestrator repository is not configured")
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return result, err
	}
	store, ok := s.repo.(titleBranchStore)
	if !ok {
		return result, fmt.Errorf("title branch repository capabilities are not configured")
	}
	worktrees, err := store.ListTaskSessionWorktrees(ctx, sessionID)
	if err != nil {
		return result, err
	}
	if len(worktrees) == 0 {
		result.Status = TitleBranchStatusNotApplicable
		return result, nil
	}
	taskRepositories, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return result, err
	}
	if len(taskRepositories) == 0 {
		return missingTitleBranchRepositoryResult(worktrees), nil
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return result, err
	}
	env, envRepos, err := s.titleBranchEnvironmentState(ctx, session, taskID, store)
	if err != nil {
		return result, err
	}

	executorType := s.titleBranchExecutorType(ctx, session, env)
	bindings, err := s.buildTitleBranchBindings(ctx, store, sessionID, taskRepositories, worktrees, envRepos)
	if err != nil {
		return result, err
	}
	return s.renameTitleBranches(ctx, task, title, executorType, env, worktrees, bindings), nil
}

func missingTitleBranchRepositoryResult(worktrees []*models.TaskEnvironmentRepo) TitleBranchRenameResult {
	var result TitleBranchRenameResult
	for _, worktree := range worktrees {
		if worktree == nil {
			continue
		}
		result.Failed = append(result.Failed, TitleBranchFailure{
			RepositoryID: worktree.RepositoryID,
			Branch:       worktree.WorktreeBranch,
			Message:      "task repository metadata is unavailable",
		})
	}
	result.Status = aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed)
	return result
}

func (s *Service) renameTitleBranches(
	ctx context.Context,
	task *models.Task,
	title string,
	executorType models.ExecutorType,
	env *models.TaskEnvironment,
	worktrees []*models.TaskEnvironmentRepo,
	bindings []titleBranchBinding,
) TitleBranchRenameResult {
	var result TitleBranchRenameResult
	multiRepo := len(bindings) > 1 || len(worktrees) > 1
	for _, binding := range bindings {
		s.renameTitleBranchBinding(ctx, task, title, executorType, env, multiRepo, binding, &result)
	}
	result.Status = aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed)
	return result
}

func (s *Service) titleBranchEnvironment(ctx context.Context, session *models.TaskSession, taskID string) (*models.TaskEnvironment, error) {
	if session != nil && session.TaskEnvironmentID != "" {
		return s.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	}
	return s.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
}

func (s *Service) titleBranchEnvironmentState(
	ctx context.Context,
	session *models.TaskSession,
	taskID string,
	store titleBranchStore,
) (*models.TaskEnvironment, map[string][]*models.TaskEnvironmentRepo, error) {
	env, err := s.titleBranchEnvironment(ctx, session, taskID)
	if err != nil || env == nil {
		return env, nil, err
	}
	rows := env.Repos
	if len(rows) == 0 && env.ID != "" {
		rows, err = store.ListTaskEnvironmentRepos(ctx, env.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	envRepos := make(map[string][]*models.TaskEnvironmentRepo, len(rows))
	for _, envRepo := range rows {
		if envRepo == nil {
			continue
		}
		envRepos[envRepo.RepositoryID] = append(envRepos[envRepo.RepositoryID], envRepo)
	}
	return env, envRepos, nil
}

func (s *Service) titleBranchExecutorType(ctx context.Context, session *models.TaskSession, env *models.TaskEnvironment) models.ExecutorType {
	if env != nil && env.ExecutorType != "" {
		return models.ExecutorType(env.ExecutorType)
	}
	if session != nil && session.ExecutorID != "" {
		if executor, err := s.repo.GetExecutor(ctx, session.ExecutorID); err == nil && executor != nil && executor.Type != "" {
			return executor.Type
		}
	}
	// Historical rows without an environment were host worktrees. Retain that
	// naming strategy rather than inventing a deterministic remote suffix.
	return models.ExecutorTypeWorktree
}

func (s *Service) buildTitleBranchBindings(
	ctx context.Context,
	store titleBranchStore,
	sessionID string,
	taskRepositories []*models.TaskRepository,
	worktrees []*models.TaskEnvironmentRepo,
	envRepos map[string][]*models.TaskEnvironmentRepo,
) ([]titleBranchBinding, error) {
	inputs := make([]worktree.BranchIdentityInput, 0, len(taskRepositories))
	bindings := make([]titleBranchBinding, 0, len(taskRepositories))
	for _, taskRepository := range taskRepositories {
		if taskRepository == nil {
			continue
		}
		repository, err := store.GetRepository(ctx, taskRepository.RepositoryID)
		if err != nil {
			return nil, err
		}
		if repository == nil {
			return nil, fmt.Errorf("repository %q is unavailable", taskRepository.RepositoryID)
		}
		bindings = append(bindings, titleBranchBinding{
			sessionID:      sessionID,
			taskRepository: taskRepository,
			repository:     repository,
		})
		inputs = append(inputs, worktree.BranchIdentityInput{
			RepositoryID:   taskRepository.RepositoryID,
			BaseBranch:     taskRepository.BaseBranch,
			CheckoutBranch: taskRepository.CheckoutBranch,
			DefaultBranch:  repository.DefaultBranch,
			PRNumber:       titleBranchPRNumber(taskRepository.Metadata),
			Position:       taskRepository.Position,
		})
	}
	plans := worktree.BuildBranchIdentityPlans(inputs)
	for i := range bindings {
		repositoryID := bindings[i].taskRepository.RepositoryID
		bindings[i].plan = plans[i]
		bindings[i].worktree = titleBranchWorktree(worktrees, repositoryID, plans[i].IdentitySlug)
		bindings[i].environmentRepo = titleBranchEnvironmentRepo(envRepos[repositoryID], plans[i].IdentitySlug)
	}
	return bindings, nil
}

func titleBranchWorktree(worktrees []*models.TaskEnvironmentRepo, repositoryID, branchSlug string) *models.TaskEnvironmentRepo {
	var fallback *models.TaskEnvironmentRepo
	for _, sessionWorktree := range worktrees {
		if sessionWorktree == nil || sessionWorktree.RepositoryID != repositoryID {
			continue
		}
		if fallback == nil {
			fallback = sessionWorktree
		}
		if worktree.SanitizeBranchSlug(sessionWorktree.BranchSlug) == branchSlug {
			return sessionWorktree
		}
	}
	return fallback
}

func titleBranchEnvironmentRepo(repos []*models.TaskEnvironmentRepo, branchSlug string) *models.TaskEnvironmentRepo {
	var fallback *models.TaskEnvironmentRepo
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		if fallback == nil {
			fallback = repo
		}
		if worktree.SanitizeBranchSlug(repo.BranchSlug) == branchSlug {
			return repo
		}
	}
	return fallback
}

func addTitleBranchPreservation(result *TitleBranchRenameResult, repositoryID, branch, reason string) {
	result.Preserved = append(result.Preserved, TitleBranchPreservation{
		RepositoryID: repositoryID,
		Branch:       branch,
		Reason:       reason,
	})
}

func addTitleBranchFailure(result *TitleBranchRenameResult, repositoryID, branch, message string) {
	result.Failed = append(result.Failed, TitleBranchFailure{
		RepositoryID: repositoryID,
		Branch:       branch,
		Message:      message,
	})
}

func (s *Service) renameTitleBranchBinding(
	ctx context.Context,
	task *models.Task,
	title string,
	executorType models.ExecutorType,
	env *models.TaskEnvironment,
	multiRepo bool,
	binding titleBranchBinding,
	result *TitleBranchRenameResult,
) {
	if binding.taskRepository == nil || binding.repository == nil {
		return
	}
	branch := titleBranchCurrentBranch(binding, env)
	if reason := titleBranchPreservationReason(binding.taskRepository, executorType); reason != "" {
		addTitleBranchPreservation(result, binding.taskRepository.RepositoryID, branch, reason)
		return
	}
	if binding.worktree == nil || branch == "" {
		addTitleBranchFailure(result, binding.taskRepository.RepositoryID, branch, "current generated branch is unavailable")
		return
	}
	generatedBranch, generatedBranchKnown := titleBranchGeneratedSnapshot(binding, env)
	if !generatedBranchKnown {
		addTitleBranchPreservation(result, binding.taskRepository.RepositoryID, branch, "branch_unverified")
		return
	}
	if generatedBranch != branch {
		addTitleBranchPreservation(result, binding.taskRepository.RepositoryID, branch, "switched_branch")
		return
	}
	if s.titleBranchRuntime == nil {
		addTitleBranchPreservation(result, binding.taskRepository.RepositoryID, branch, "runtime_unavailable")
		return
	}

	primary := titleBranchIsPrimaryEnvironment(env, binding, multiRepo)
	newName, err := s.performTitleBranchRename(ctx, task, title, executorType, env, multiRepo, primary, binding, branch)
	if err != nil {
		addTitleBranchFailure(result, binding.taskRepository.RepositoryID, branch, err.Error())
		return
	}
	if branch == newName {
		result.Renamed = append(result.Renamed, TitleBranchRename{
			RepositoryID: binding.taskRepository.RepositoryID,
			From:         branch,
			To:           newName,
		})
		return
	}
	if err := s.persistTitleBranchSnapshots(ctx, env, binding, newName, multiRepo); err != nil {
		addTitleBranchFailure(result, binding.taskRepository.RepositoryID, newName, err.Error())
		return
	}
	result.Renamed = append(result.Renamed, TitleBranchRename{
		RepositoryID: binding.taskRepository.RepositoryID,
		From:         branch,
		To:           newName,
	})
}

func (s *Service) performTitleBranchRename(
	ctx context.Context,
	task *models.Task,
	title string,
	executorType models.ExecutorType,
	env *models.TaskEnvironment,
	multiRepo bool,
	primary bool,
	binding titleBranchBinding,
	branch string,
) (string, error) {
	suffix := titleBranchSuffix(executorType, task.ID)
	newName, err := renderTitleBranchNameForTaskRepository(title, task, binding.repository, binding.taskRepository, suffix)
	if err != nil {
		return "", err
	}
	if branch == newName {
		return newName, nil
	}
	repoPath := titleBranchAgentctlRepoPath(env, binding, multiRepo)
	operation, err := s.renameTitleBranchRuntime(ctx, binding.sessionID, newName, repoPath, primary)
	if err != nil {
		return "", err
	}
	if operation == nil || !operation.Success {
		if operation != nil && operation.Error != "" {
			return "", fmt.Errorf("%s", operation.Error)
		}
		return "", fmt.Errorf("agentctl rejected branch rename")
	}
	return newName, nil
}

func (s *Service) renameTitleBranchRuntime(ctx context.Context, sessionID, newName, repoPath string, primary bool) (*runtimeapi.GitOperationResult, error) {
	if scoped, ok := s.titleBranchRuntime.(titleBranchPrimaryRuntime); ok {
		return scoped.RenameBranchForSessionWithPrimary(ctx, sessionID, newName, repoPath, primary)
	}
	return s.titleBranchRuntime.RenameBranchForSession(ctx, sessionID, newName, repoPath)
}

func titleBranchPreservationReason(taskRepository *models.TaskRepository, executorType models.ExecutorType) string {
	if taskRepository != nil && taskRepository.CheckoutBranch != "" {
		return "remote_checkout"
	}
	if executorType == models.ExecutorTypeLocal || string(executorType) == "local_pc" {
		return "local_executor"
	}
	return ""
}

func titleBranchCurrentBranch(binding titleBranchBinding, env *models.TaskEnvironment) string {
	if binding.worktree != nil && binding.worktree.WorktreeBranch != "" {
		return binding.worktree.WorktreeBranch
	}
	if binding.environmentRepo != nil && binding.environmentRepo.WorktreeBranch != "" {
		return binding.environmentRepo.WorktreeBranch
	}
	return ""
}

func titleBranchSuffix(executorType models.ExecutorType, taskID string) string {
	if executorType == models.ExecutorTypeWorktree {
		// The initial worktree suffix is random, but title handoffs can be
		// retried. Derive a stable replacement from task data so repeated
		// renders of the same title remain idempotent.
		digest := sha256.Sum256([]byte(taskID))
		return hex.EncodeToString(digest[:])[:3]
	}
	suffix := taskID
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return suffix
}

func titleBranchAgentctlRepoPath(env *models.TaskEnvironment, binding titleBranchBinding, multiRepo bool) string {
	if !multiRepo || binding.worktree == nil {
		return ""
	}
	if env != nil && env.WorkspacePath != "" && binding.worktree.WorktreePath != "" {
		if relative, err := filepath.Rel(env.WorkspacePath, binding.worktree.WorktreePath); err == nil && relative != "." && !strings.HasPrefix(relative, "..") {
			return filepath.ToSlash(relative)
		}
	}
	repoName := worktree.SanitizeRepoDirName(binding.repository.Name)
	if repoName == "" {
		repoName = worktree.SanitizeRepoDirName(binding.repository.ID)
	}
	if slug := worktree.SanitizeBranchSlug(binding.plan.PathSlug); slug != "" {
		repoName += "-" + slug
	}
	return repoName
}

func (s *Service) persistTitleBranchSnapshots(
	ctx context.Context,
	env *models.TaskEnvironment,
	binding titleBranchBinding,
	branch string,
	multiRepo bool,
) error {
	return s.persistTitleBranchEnvironmentRepoSnapshot(ctx, binding, branch)
}

func (s *Service) persistTitleBranchEnvironmentRepoSnapshot(ctx context.Context, binding titleBranchBinding, branch string) error {
	if binding.environmentRepo == nil {
		return nil
	}
	binding.environmentRepo.WorktreeBranch = branch
	store, ok := s.repo.(titleBranchStore)
	if !ok {
		return nil
	}
	if err := store.UpdateTaskEnvironmentRepo(ctx, binding.environmentRepo); err != nil {
		return fmt.Errorf("failed to persist environment repository branch: %w", err)
	}
	return nil
}

func titleBranchIsPrimaryEnvironment(env *models.TaskEnvironment, binding titleBranchBinding, multiRepo bool) bool {
	if !multiRepo {
		return true
	}
	if binding.worktree == nil {
		return binding.taskRepository != nil && binding.taskRepository.Position == 0
	}
	if binding.plan.PathSlug == "" && binding.taskRepository != nil {
		return binding.worktree.RepositoryID == binding.taskRepository.RepositoryID
	}
	return binding.taskRepository != nil && binding.taskRepository.Position == 0
}

func titleBranchGeneratedSnapshot(binding titleBranchBinding, env *models.TaskEnvironment) (string, bool) {
	if binding.environmentRepo != nil && binding.environmentRepo.WorktreeBranch != "" {
		return binding.environmentRepo.WorktreeBranch, true
	}
	return "", false
}

func titleBranchPRNumber(metadata map[string]interface{}) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata["pr_number"].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return 0
}
