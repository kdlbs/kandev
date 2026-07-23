package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

type WorkspaceSourceKind string

const (
	WorkspaceSourceRepository WorkspaceSourceKind = "repository"
	WorkspaceSourceFolder     WorkspaceSourceKind = "folder"
)

type WorkspaceSourceInput struct {
	Kind                                                                                                 WorkspaceSourceKind
	RepositoryID, LocalPath, GitHubURL, RemoteURL, Provider, ProviderRepoID, ProviderOwner, ProviderName string
	BaseBranch, CheckoutBranch, DisplayName                                                              string
}

type AttachWorkspaceSourcesRequest struct {
	TaskID  string
	Sources []WorkspaceSourceInput
}

// AttachWorkspaceSourcesResult is the authoritative post-materialization
// projection shared by HTTP and MCP callers. Every field is loaded before the
// durable commit is declared successful and task.updated is published.
type AttachWorkspaceSourcesResult struct {
	Task          *models.Task
	WorkspacePath string
	SessionIDs    []string
}

// WorkspaceSourceMaterializationResult is returned by the runtime boundary
// after it has successfully adopted the durable source batch. Its values are
// authoritative: callers must not perform fallible hydration after this point.
type WorkspaceSourceMaterializationResult struct {
	WorkspacePath string
	SessionIDs    []string
}

// WorkspaceSourceMaterializer applies durable sources to the active runtime.
type WorkspaceSourceMaterializer interface {
	MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error)
}

func (s *Service) AttachWorkspaceSources(ctx context.Context, req AttachWorkspaceSourcesRequest) (*AttachWorkspaceSourcesResult, error) {
	if req.TaskID == "" || len(req.Sources) == 0 {
		return nil, fmt.Errorf("%w: task_id and at least one source are required", ErrInvalidWorkspaceSource)
	}
	unlock := s.lockWorkspaceSources(req.TaskID)
	defer unlock()
	task, err := s.tasks.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("%w: %s", taskrepository.ErrTaskNotFound, req.TaskID)
	}
	if err := s.workspaceSourcesIdle(ctx, task.ID); err != nil {
		return nil, err
	}
	if err := s.rejectUnsupportedFolderSources(ctx, task.ID, req.Sources); err != nil {
		return nil, err
	}
	existing, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, fmt.Errorf("%w: task must have a repository before attaching workspace sources", ErrInvalidWorkspaceSource)
	}
	store := s.workspaceSourceStore()
	if store == nil {
		return nil, fmt.Errorf("%w: workspace source persistence is unavailable", ErrWorkspaceSourceMaterialize)
	}
	folders, err := store.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	batch, cleanupCreated, err := s.prepareWorkspaceSourceBatch(ctx, task, existing, folders, req.Sources)
	if err != nil {
		return nil, err
	}
	return s.commitWorkspaceSourceBatch(ctx, task, batch, cleanupCreated, s.materializeWorkspaceSources)
}

func (s *Service) prepareWorkspaceSourceBatch(ctx context.Context, task *models.Task, existing []*models.TaskRepository, folders []*models.TaskWorkspaceFolder, inputs []WorkspaceSourceInput) (*models.WorkspaceSourceBatch, func(context.Context), error) {
	batch := &models.WorkspaceSourceBatch{TaskID: task.ID}
	created := make([]string, 0)
	cleanup := func(cleanupCtx context.Context) { s.cleanupCreatedWorkspaceRepositories(cleanupCtx, created) }
	keepCreatedRepositories := false
	defer func() {
		if !keepCreatedRepositories {
			cleanup(context.WithoutCancel(ctx))
		}
	}()
	seenPaths, seenNames := map[string]bool{}, map[string]bool{}
	for _, folder := range folders {
		seenPaths[folder.LocalPath], seenNames[folder.DisplayName] = true, true
	}
	prospective := cloneTaskRepositories(existing)
	updates, err := s.deriveLegacyWorkspaceSourceBranches(ctx, task.ID, prospective)
	if err != nil {
		return nil, nil, err
	}
	batch.RepositoryUpdates = updates
	for _, input := range inputs {
		switch input.Kind {
		case WorkspaceSourceFolder:
			folder, err := prepareFolderWorkspaceSource(input, seenPaths, seenNames)
			if err != nil {
				return nil, nil, err
			}
			batch.Sources = append(batch.Sources, models.WorkspaceSource{Folder: folder})
		case WorkspaceSourceRepository:
			taskRepository, createdByUs, err := s.prepareRepositoryWorkspaceSource(ctx, task, input, prospective)
			if err != nil {
				return nil, nil, err
			}
			if createdByUs != "" {
				created = append(created, createdByUs)
			}
			prospective = append(prospective, taskRepository)
			batch.Sources = append(batch.Sources, models.WorkspaceSource{Repository: taskRepository})
		default:
			return nil, nil, fmt.Errorf("%w: unsupported workspace source kind %q", ErrInvalidWorkspaceSource, input.Kind)
		}
	}
	if err := s.rejectRuntimeNameCollisions(ctx, prospective, batch); err != nil {
		return nil, nil, err
	}
	keepCreatedRepositories = true
	return batch, cleanup, nil
}

func cloneTaskRepositories(repositories []*models.TaskRepository) []*models.TaskRepository {
	clones := make([]*models.TaskRepository, 0, len(repositories))
	for _, repository := range repositories {
		if repository == nil {
			clones = append(clones, nil)
			continue
		}
		clone := *repository
		clones = append(clones, &clone)
	}
	return clones
}

func (s *Service) deriveLegacyWorkspaceSourceBranches(ctx context.Context, taskID string, repositories []*models.TaskRepository) ([]models.WorkspaceSourceRepositoryUpdate, error) {
	executorType, err := s.workspaceSourceExecutorType(ctx, taskID)
	if err != nil || isLocalWorkspaceExecutor(executorType) {
		return nil, err
	}
	updates := make([]models.WorkspaceSourceRepositoryUpdate, 0)
	for _, taskRepository := range repositories {
		if taskRepository == nil || taskRepository.BaseBranch != "" || taskRepository.CheckoutBranch != "" {
			continue
		}
		repository, err := s.repoEntities.GetRepository(ctx, taskRepository.RepositoryID)
		if err != nil {
			return nil, classifyWorkspaceRepositoryError(err)
		}
		if repository == nil || repository.LocalPath == "" {
			continue
		}
		branch, err := s.RepositoryCurrentBranch(ctx, repository.ID)
		if err != nil || !securityutil.IsValidBranchName(branch) {
			return nil, fmt.Errorf("%w: repository %q has no safe runtime branch", ErrInvalidWorkspaceSource, repository.Name)
		}
		taskRepository.BaseBranch = branch
		updates = append(updates, models.WorkspaceSourceRepositoryUpdate{TaskRepositoryID: taskRepository.ID, BaseBranch: branch})
	}
	return updates, nil
}

func prepareFolderWorkspaceSource(input WorkspaceSourceInput, seenPaths, seenNames map[string]bool) (*models.TaskWorkspaceFolder, error) {
	path, err := canonicalFolder(input.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkspaceSource, err)
	}
	name := input.DisplayName
	if name == "" {
		name = filepath.Base(path)
	}
	if filepath.Base(name) != name || name == "." || name == ".." || worktree.SanitizeRepoDirName(name) != name {
		return nil, fmt.Errorf("%w: folder display_name %q is not a safe workspace entry", ErrInvalidWorkspaceSource, name)
	}
	if seenPaths[path] || seenNames[name] {
		return nil, fmt.Errorf("%w: workspace folder %q is already attached", ErrWorkspaceSourceConflict, name)
	}
	seenPaths[path], seenNames[name] = true, true
	return &models.TaskWorkspaceFolder{LocalPath: path, DisplayName: name}, nil
}

func (s *Service) prepareRepositoryWorkspaceSource(ctx context.Context, task *models.Task, input WorkspaceSourceInput, existing []*models.TaskRepository) (*models.TaskRepository, string, error) {
	if err := s.validateRepositoryWorkspaceSourceInput(ctx, task, input); err != nil {
		return nil, "", err
	}
	id, base, createdID, err := s.resolveRepositoryWorkspaceSource(ctx, task, input)
	if err != nil {
		return nil, createdID, err
	}
	repo, base, err := s.resolveWorkspaceSourceBaseBranch(ctx, task, id, base)
	if err != nil {
		return nil, createdID, err
	}
	if err := validateWorkspaceSourceBranches(base, input.CheckoutBranch); err != nil {
		return nil, createdID, err
	}
	if input.LocalPath != "" {
		if err := s.requireCloneableLocalRepository(ctx, task.ID, repo); err != nil {
			return nil, createdID, err
		}
	}
	checkout := input.CheckoutBranch
	if _, duplicate := scanForBranchAddDuplicate(existing, id, base, checkout, repo); duplicate != nil {
		return nil, createdID, fmt.Errorf("%w: %v", ErrWorkspaceSourceConflict, duplicate)
	}
	return &models.TaskRepository{RepositoryID: id, BaseBranch: base, CheckoutBranch: checkout, Metadata: map[string]interface{}{}}, createdID, nil
}

func (s *Service) validateRepositoryWorkspaceSourceInput(ctx context.Context, task *models.Task, input WorkspaceSourceInput) error {
	if repositoryLocatorCount(input) != 1 {
		return fmt.Errorf("%w: repository source requires exactly one locator", ErrInvalidWorkspaceSource)
	}
	if err := validateWorkspaceSourceBranches(input.BaseBranch, input.CheckoutBranch); err != nil {
		return err
	}
	if input.RepositoryID == "" {
		return nil
	}
	if _, err := s.resolveBranchRepo(ctx, task, input.RepositoryID); err != nil {
		return classifyWorkspaceRepositoryError(err)
	}
	return nil
}

func (s *Service) resolveRepositoryWorkspaceSource(ctx context.Context, task *models.Task, input WorkspaceSourceInput) (string, string, string, error) {
	id, base, created, err := s.ResolveRepositoryRef(ctx, task.WorkspaceID, TaskRepositoryInput{RepositoryID: input.RepositoryID, LocalPath: input.LocalPath, GitHubURL: input.GitHubURL, RemoteURL: input.RemoteURL, Provider: input.Provider, ProviderRepoID: input.ProviderRepoID, ProviderOwner: input.ProviderOwner, ProviderName: input.ProviderName, BaseBranch: input.BaseBranch, ResolveProviderDefaults: true})
	if err != nil {
		return "", "", "", classifyWorkspaceRepositoryError(err)
	}
	createdID := ""
	if created {
		createdID = id
	}
	if base == "" {
		base = input.BaseBranch
	}
	return id, base, createdID, nil
}

func (s *Service) resolveWorkspaceSourceBaseBranch(ctx context.Context, task *models.Task, repositoryID, base string) (*models.Repository, string, error) {
	repo, err := s.resolveBranchRepo(ctx, task, repositoryID)
	if err != nil {
		return nil, "", classifyWorkspaceRepositoryError(err)
	}
	if base == "" && repo.LocalPath != "" {
		base, err = s.RepositoryCurrentBranch(ctx, repo.ID)
		if err != nil {
			return nil, "", fmt.Errorf("%w: resolve local repository branch: %v", ErrInvalidWorkspaceSource, err)
		}
	}
	if base == "" {
		return nil, "", fmt.Errorf("%w: repository source requires base_branch", ErrInvalidWorkspaceSource)
	}
	return repo, base, nil
}

func validateWorkspaceSourceBranches(baseBranch, checkoutBranch string) error {
	if !securityutil.IsValidBranchName(baseBranch) {
		return fmt.Errorf("%w: base_branch %q is not a safe git branch", ErrInvalidWorkspaceSource, baseBranch)
	}
	if checkoutBranch != "" && !securityutil.IsValidBranchName(checkoutBranch) {
		return fmt.Errorf("%w: checkout_branch %q is not a safe git branch", ErrInvalidWorkspaceSource, checkoutBranch)
	}
	return nil
}

func validateTaskRepositoryBranches(repositories []TaskRepositoryInput) error {
	for _, repository := range repositories {
		if repository.BaseBranch != "" && !securityutil.IsValidBranchName(repository.BaseBranch) {
			return fmt.Errorf("base_branch %q is not a safe git branch", repository.BaseBranch)
		}
		if repository.CheckoutBranch != "" && !securityutil.IsValidBranchName(repository.CheckoutBranch) {
			return fmt.Errorf("checkout_branch %q is not a safe git branch", repository.CheckoutBranch)
		}
	}
	return nil
}

func (s *Service) requireCloneableLocalRepository(ctx context.Context, taskID string, repository *models.Repository) error {
	executorType, err := s.workspaceSourceExecutorType(ctx, taskID)
	if err != nil || executorType == "" || isLocalWorkspaceExecutor(executorType) || executorType == string(models.ExecutorTypeWorktree) {
		return err
	}
	if repository == nil || repository.RemoteURL == "" {
		return fmt.Errorf("%w: local repository has no safe cloneable origin", ErrUnsupportedWorkspaceSource)
	}
	return nil
}

func classifyWorkspaceRepositoryError(err error) error {
	if errors.Is(err, taskrepository.ErrRepositoryNotFound) {
		return fmt.Errorf("repository source: %w", err)
	}
	return fmt.Errorf("%w: %v", ErrInvalidWorkspaceSource, err)
}

func (s *Service) rejectRuntimeNameCollisions(ctx context.Context, repositories []*models.TaskRepository, batch *models.WorkspaceSourceBatch) error {
	executorType, err := s.workspaceSourceExecutorType(ctx, batch.TaskID)
	if err != nil {
		return err
	}
	names := make(map[string]bool, len(repositories))
	for _, taskRepository := range repositories {
		repo, err := s.repoEntities.GetRepository(ctx, taskRepository.RepositoryID)
		if err != nil {
			return classifyWorkspaceRepositoryError(err)
		}
		if repo == nil || repo.WorkspaceID == "" {
			return fmt.Errorf("%w: repository %q", taskrepository.ErrRepositoryNotFound, taskRepository.RepositoryID)
		}
		name, err := WorkspaceSourceRuntimeEntryName(executorType, repo, taskRepository)
		if err != nil {
			return err
		}
		if names[name] {
			return fmt.Errorf("%w: workspace runtime name %q collides", ErrWorkspaceSourceConflict, name)
		}
		names[name] = true
	}
	for _, source := range batch.Sources {
		if source.Folder == nil {
			continue
		}
		if _, exists := names[source.Folder.DisplayName]; exists {
			return fmt.Errorf("%w: folder %q collides with repository runtime name", ErrWorkspaceSourceConflict, source.Folder.DisplayName)
		}
		names[source.Folder.DisplayName] = true
	}
	return nil
}

// WorkspaceSourceRuntimeEntryName derives the owned workspace entry name used
// by both service collision checks and runtime materialization. Local runtimes
// link repositories under bare repository names; branch-capable runtimes use
// the repository and effective ref so independent checkouts cannot collide.
func WorkspaceSourceRuntimeEntryName(executorType string, repository *models.Repository, taskRepository *models.TaskRepository) (string, error) {
	if repository == nil || taskRepository == nil {
		return "", fmt.Errorf("%w: missing repository runtime source", ErrInvalidWorkspaceSource)
	}
	name := worktree.SanitizeRepoDirName(repository.Name)
	if name == "" {
		return "", fmt.Errorf("%w: repository %q has unsafe runtime name", ErrInvalidWorkspaceSource, repository.Name)
	}
	if isLocalWorkspaceExecutor(executorType) {
		if taskRepository.CheckoutBranch != "" {
			return "", fmt.Errorf("%w: checkout_branch is unsupported by local executor", ErrInvalidWorkspaceSource)
		}
		return name, nil
	}
	branch := worktree.SanitizeBranchSlug(taskRepository.CheckoutBranch)
	if branch == "" {
		branch = worktree.SanitizeBranchSlug(taskRepository.BaseBranch)
	}
	if branch == "" {
		return "", fmt.Errorf("%w: repository %q has no safe runtime branch", ErrInvalidWorkspaceSource, repository.Name)
	}
	return name + "-" + branch, nil
}

func isLocalWorkspaceExecutor(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocal) || executorType == "local_pc"
}

func (s *Service) workspaceSourceExecutorType(ctx context.Context, taskID string) (string, error) {
	if s.taskEnvironments == nil {
		return "", nil
	}
	environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) || environment == nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return environment.ExecutorType, nil
}

func (s *Service) commitWorkspaceSourceBatch(ctx context.Context, task *models.Task, batch *models.WorkspaceSourceBatch, cleanupCreated func(context.Context), materialize func(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error)) (*AttachWorkspaceSourcesResult, error) {
	store := s.workspaceSourceStore()
	if store == nil {
		return nil, fmt.Errorf("%w: workspace source persistence is unavailable", ErrWorkspaceSourceMaterialize)
	}
	if err := store.CreateWorkspaceSourceBatch(ctx, batch); err != nil {
		cleanupCreated(context.WithoutCancel(ctx))
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			s.compensateWorkspaceSources(ctx, batch, nil)
			cleanupCreated(context.WithoutCancel(ctx))
		}
	}()
	result, err := s.hydrateWorkspaceSourceResult(ctx, task, store)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("workspace source request canceled: %w", err)
	}
	var materialized *WorkspaceSourceMaterializationResult
	if materialize != nil {
		var err error
		materialized, err = materialize(ctx, task.ID, batch)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrWorkspaceSourceMaterialize, err)
		}
		if s.workspaceSourceMaterializer != nil && materialized == nil {
			return nil, fmt.Errorf("%w: empty materialization result", ErrWorkspaceSourceMaterialize)
		}
	}
	if materialized != nil {
		result.WorkspacePath = materialized.WorkspacePath
		result.SessionIDs = materialized.SessionIDs
	}
	s.publishTaskEvent(context.WithoutCancel(ctx), events.TaskUpdated, result.Task, nil)
	if materialized != nil {
		s.PublishWorkspaceSourcesAdopted(context.WithoutCancel(ctx), task.ID, result.WorkspacePath, result.SessionIDs)
	}
	succeeded = true
	return result, nil
}

func (s *Service) hydrateWorkspaceSourceResult(ctx context.Context, task *models.Task, store taskrepository.TaskWorkspaceFolderRepository) (*AttachWorkspaceSourcesResult, error) {
	projected := *task
	var err error
	if projected.Repositories, err = s.taskRepos.ListTaskRepositories(ctx, task.ID); err != nil {
		return nil, fmt.Errorf("hydrate repositories: %w", err)
	}
	if projected.WorkspaceFolders, err = store.ListTaskWorkspaceFolders(ctx, task.ID); err != nil {
		return nil, fmt.Errorf("hydrate workspace folders: %w", err)
	}
	result := &AttachWorkspaceSourcesResult{Task: &projected, SessionIDs: []string{}}
	if s.taskEnvironments != nil {
		env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, task.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("hydrate task environment: %w", err)
		}
		if env != nil {
			result.WorkspacePath = env.WorkspacePath
		}
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("hydrate task sessions: %w", err)
	}
	result.SessionIDs = make([]string, 0, len(sessions))
	for _, session := range sessions {
		result.SessionIDs = append(result.SessionIDs, session.ID)
	}
	return result, nil
}

func (s *Service) materializeWorkspaceSources(ctx context.Context, taskID string, batch *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	if s.workspaceSourceMaterializer == nil {
		return nil, nil
	}
	return s.workspaceSourceMaterializer.MaterializeWorkspaceSources(ctx, taskID, batch)
}

func repositoryLocatorCount(in WorkspaceSourceInput) int {
	n := 0
	if in.RepositoryID != "" {
		n++
	}
	if in.LocalPath != "" {
		n++
	}
	if in.GitHubURL != "" || in.RemoteURL != "" {
		n++
	}
	return n
}

func canonicalFolder(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("folder source requires local_path")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	// codeql[go/path-injection] Explicit local-folder selection resolves to a canonical directory for the trusted local-user model; remote executors are rejected before this path is reached.
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("folder source %q is not a directory", p)
	}
	return p, nil
}

func (s *Service) workspaceSourcesIdle(ctx context.Context, taskID string) error {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		turn, err := s.turns.GetActiveTurnBySessionID(ctx, session.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if turn != nil {
			return fmt.Errorf("%w: task has an active turn", ErrWorkspaceSourceActive)
		}
	}
	return nil
}

func (s *Service) rejectUnsupportedFolderSources(ctx context.Context, taskID string, sources []WorkspaceSourceInput) error {
	for _, source := range sources {
		if source.Kind == WorkspaceSourceFolder {
			return s.requireFolderSourceExecutor(ctx, taskID)
		}
	}
	return nil
}

func (s *Service) requireFolderSourceExecutor(ctx context.Context, taskID string) error {
	if s.taskEnvironments == nil {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if env == nil || env.ExecutorType == "" || env.ExecutorType == string(models.ExecutorTypeLocal) || env.ExecutorType == "local_pc" || env.ExecutorType == string(models.ExecutorTypeWorktree) {
		return nil
	}
	return fmt.Errorf("%w: folder sources are not supported by executor %q", ErrUnsupportedWorkspaceSource, env.ExecutorType)
}

func (s *Service) compensateWorkspaceSources(ctx context.Context, batch *models.WorkspaceSourceBatch, created []string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if store := s.workspaceSourceStore(); store != nil {
		_ = store.CompensateWorkspaceSourceBatch(cleanupCtx, batch)
	}
	s.cleanupCreatedWorkspaceRepositories(cleanupCtx, created)
}

// workspaceSourceStore keeps the shared commit path compatible with service
// tests and older construction sites that supplied the SQLite repository only
// through TaskRepos before folder attachments existed.
func (s *Service) workspaceSourceStore() taskrepository.TaskWorkspaceFolderRepository {
	if s.workspaceFolders != nil {
		return s.workspaceFolders
	}
	store, _ := s.taskRepos.(taskrepository.TaskWorkspaceFolderRepository)
	return store
}

func (s *Service) cleanupCreatedWorkspaceRepositories(ctx context.Context, ids []string) {
	if s.repositoryCleanup == nil {
		return
	}
	for _, id := range ids {
		repository, err := s.repoEntities.GetRepository(ctx, id)
		if err != nil {
			continue
		}
		deleted, err := s.repositoryCleanup.DeleteRepositoryIfUnreferenced(ctx, id)
		if err != nil || !deleted {
			continue
		}
		s.publishRepositoryEvent(ctx, events.RepositoryDeleted, repository)
	}
}

func (s *Service) lockWorkspaceSources(taskID string) func() {
	s.workspaceSourceLocksMu.Lock()
	if s.workspaceSourceLocks == nil {
		s.workspaceSourceLocks = map[string]*sync.Mutex{}
	}
	lock := s.workspaceSourceLocks[taskID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.workspaceSourceLocks[taskID] = lock
	}
	s.workspaceSourceLocksMu.Unlock()
	lock.Lock()
	return lock.Unlock
}
