package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

func (m *workspaceSourceMaterializer) materializeRemoteWorkspaceSources(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) (*taskservice.WorkspaceSourceMaterializationResult, error) {
	if m.remoteMaterializer == nil {
		return nil, fmt.Errorf("remote workspace materializer is unavailable")
	}
	projection, err := buildRemoteWorkspaceRepositoryBatch(batch, state.entities)
	if err != nil {
		return nil, err
	}
	created, err := m.persistEnvironmentRepositoryInventory(ctx, state.environment.ID, state.environment.ExecutorType, batch, nil)
	if err != nil {
		_ = m.deleteEnvironmentRepositoryInventory(context.WithoutCancel(ctx), created)
		return nil, err
	}
	ids, err := m.remoteMaterializer.MaterializeRepositoriesForEnvironment(ctx, state.environment.ID, projection)
	if err != nil {
		_ = m.deleteEnvironmentRepositoryInventory(context.WithoutCancel(ctx), created)
		return nil, err
	}
	return &taskservice.WorkspaceSourceMaterializationResult{WorkspacePath: state.environment.WorkspacePath, SessionIDs: ids}, nil
}

// persistEnvironmentRepositoryInventory records the repository slots created
// by this attachment after their runtime materialization succeeds. The resume
// path validates task_repositories against this canonical inventory before it
// reuses an environment, so a live workspace-source attachment must publish
// its new slot before the next backend restart.
func (m *workspaceSourceMaterializer) persistEnvironmentRepositoryInventory(
	ctx context.Context,
	environmentID, executorType string,
	batch *models.WorkspaceSourceBatch,
	branchMaterializations []*branchMaterialization,
) ([]*models.TaskEnvironmentRepo, error) {
	if batch == nil || environmentID == "" {
		return nil, nil
	}
	existing, err := m.loadEnvironmentRepositoryInventory(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	created := make([]*models.TaskEnvironmentRepo, 0, len(batch.Sources))
	for _, row := range workspaceSourceInventoryRows(environmentID, executorType, batch, branchMaterializations, existing) {
		if err := m.repo.CreateTaskEnvironmentRepo(ctx, row); err != nil {
			return created, fmt.Errorf("persist task environment repository %q: %w", row.RepositoryID, err)
		}
		created = append(created, row)
	}
	return created, nil
}

func (m *workspaceSourceMaterializer) deleteEnvironmentRepositoryInventory(ctx context.Context, rows []*models.TaskEnvironmentRepo) error {
	var deleteErr error
	for index := len(rows) - 1; index >= 0; index-- {
		row := rows[index]
		if row == nil || row.ID == "" {
			continue
		}
		if err := m.repo.DeleteTaskEnvironmentRepo(ctx, row.ID); err != nil {
			deleteErr = errors.Join(deleteErr, fmt.Errorf("delete task environment repository %q: %w", row.ID, err))
		}
	}
	return deleteErr
}

func (m *workspaceSourceMaterializer) loadEnvironmentRepositoryInventory(ctx context.Context, environmentID string) (map[string]struct{}, error) {
	rows, err := m.repo.ListTaskEnvironmentRepos(ctx, environmentID)
	if err != nil {
		return nil, fmt.Errorf("list task environment repository inventory: %w", err)
	}
	existing := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row != nil && row.RepositoryID != "" {
			existing[environmentRepoInventoryKey(row.RepositoryID, row.BranchSlug)] = struct{}{}
		}
	}
	return existing, nil
}

func workspaceSourceInventoryRows(
	environmentID, executorType string,
	batch *models.WorkspaceSourceBatch,
	branchMaterializations []*branchMaterialization,
	existing map[string]struct{},
) []*models.TaskEnvironmentRepo {
	materialized := make(map[string]*branchMaterialization, len(branchMaterializations)*2)
	for _, branch := range branchMaterializations {
		if branch != nil && branch.repositoryID != "" {
			materialized[workspaceSourceMaterializationKey(branch.taskRepositoryID, branch.repositoryID, branch.slug)] = branch
			if branch.taskRepositoryID == "" {
				materialized[environmentRepoInventoryKey(branch.repositoryID, branch.slug)] = branch
			}
		}
	}
	rows := make([]*models.TaskEnvironmentRepo, 0, len(batch.Sources))
	for _, source := range batch.Sources {
		row, ok := workspaceSourceInventoryRow(environmentID, executorType, source.Repository, materialized)
		if !ok {
			continue
		}
		if _, found := existing[environmentRepoInventoryKey(row.RepositoryID, row.BranchSlug)]; found {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func workspaceSourceInventoryRow(
	environmentID, executorType string,
	taskRepository *models.TaskRepository,
	materialized map[string]*branchMaterialization,
) (*models.TaskEnvironmentRepo, bool) {
	if taskRepository == nil || taskRepository.RepositoryID == "" {
		return nil, false
	}
	branchSlug := workspaceSourceBranchSlug(taskRepository)
	branch := materialized[workspaceSourceMaterializationKey(taskRepository.ID, taskRepository.RepositoryID, branchSlug)]
	if branch == nil {
		branch = materialized[environmentRepoInventoryKey(taskRepository.RepositoryID, branchSlug)]
	}
	if executorType == string(models.ExecutorTypeWorktree) && branch == nil {
		// A worktree source that was not materialized has no physical checkout
		// to publish. The next launch must prepare it first.
		return nil, false
	}
	row := &models.TaskEnvironmentRepo{
		TaskEnvironmentID:     environmentID,
		RepositoryID:          taskRepository.RepositoryID,
		WorkspaceRelativePath: taskRepository.WorkspaceRelativePath,
		BranchSlug:            branchSlug,
		Position:              taskRepository.Position,
	}
	if branch != nil && branch.worktree != nil {
		row.WorktreeID = branch.worktree.ID
		row.WorktreePath = branch.worktree.Path
		row.WorktreeBranch = branch.worktree.Branch
		row.BranchSlug = branch.slug
	}
	return row, true
}

func workspaceSourceBranchSlug(taskRepository *models.TaskRepository) string {
	branch := taskRepository.CheckoutBranch
	if branch == "" {
		branch = taskRepository.BaseBranch
	}
	return worktree.SanitizeBranchSlug(branch)
}

func workspaceSourceMaterializationKey(taskRepositoryID, repositoryID, branchSlug string) string {
	if taskRepositoryID != "" {
		return "task-repository\x00" + taskRepositoryID
	}
	return environmentRepoInventoryKey(repositoryID, branchSlug)
}

func environmentRepoInventoryKey(repositoryID, branchSlug string) string {
	return repositoryID + "\x00" + branchSlug
}

// buildRemoteWorkspaceRepositoryBatch projects only the sources created by
// this attachment operation. Existing durable siblings belong in a fresh
// launch/resume projection, but revalidating them in a live execution would
// reject normal agent commits that advance their HEAD after attachment.
func buildRemoteWorkspaceRepositoryBatch(batch *models.WorkspaceSourceBatch, repositoryByID map[string]*models.Repository) ([]workspaceRepositoryMaterialization, error) {
	if batch == nil {
		return nil, nil
	}
	projection := make([]workspaceRepositoryMaterialization, 0, len(batch.Sources))
	for _, source := range batch.Sources {
		if source.Repository == nil {
			continue
		}
		repository := repositoryByID[source.Repository.RepositoryID]
		if repository == nil {
			return nil, fmt.Errorf("%w: repository %q", ErrRemoteRepositoryLocatorUnavailable, source.Repository.RepositoryID)
		}
		locator := remoteRepositoryLocator(repository)
		if locator == "" {
			return nil, fmt.Errorf("%w: repository %q", ErrRemoteRepositoryLocatorUnavailable, repository.Name)
		}
		branch := source.Repository.CheckoutBranch
		if branch == "" {
			branch = source.Repository.BaseBranch
		}
		if branch == "" {
			return nil, fmt.Errorf("%w: repository %q has no checkout ref", ErrRemoteRepositoryLocatorUnavailable, repository.Name)
		}
		name, err := taskservice.WorkspaceSourceRuntimeEntryName("remote", repository, source.Repository)
		if err != nil {
			return nil, fmt.Errorf("%w: repository %q has unsafe runtime name", ErrRemoteRepositoryLocatorUnavailable, repository.Name)
		}
		projection = append(projection, workspaceRepositoryMaterialization{RepositoryURL: locator, Destination: name, BaseBranch: source.Repository.BaseBranch, CheckoutBranch: source.Repository.CheckoutBranch})
	}
	return projection, nil
}

func buildRemoteWorkspaceRepositories(taskRepositories []*models.TaskRepository, repositoryByID map[string]*models.Repository) ([]workspaceRepositoryMaterialization, error) {
	projection := make([]workspaceRepositoryMaterialization, 0, len(taskRepositories))
	// The primary repository is already represented by the executor workspace
	// root. Materialize only durable siblings in their collision-checked names.
	for index, taskRepository := range taskRepositories {
		if index == 0 {
			continue
		}
		repository := repositoryByID[taskRepository.RepositoryID]
		if repository == nil {
			return nil, fmt.Errorf("%w: repository %q", ErrRemoteRepositoryLocatorUnavailable, taskRepository.RepositoryID)
		}
		locator := remoteRepositoryLocator(repository)
		if locator == "" {
			return nil, fmt.Errorf("%w: repository %q", ErrRemoteRepositoryLocatorUnavailable, repository.Name)
		}
		branch := taskRepository.CheckoutBranch
		if branch == "" {
			branch = taskRepository.BaseBranch
		}
		if branch == "" {
			return nil, fmt.Errorf("%w: repository %q has no checkout ref", ErrRemoteRepositoryLocatorUnavailable, repository.Name)
		}
		name, err := taskservice.WorkspaceSourceRuntimeEntryName("remote", repository, taskRepository)
		if err != nil {
			return nil, fmt.Errorf("%w: repository %q has unsafe runtime name", ErrRemoteRepositoryLocatorUnavailable, repository.Name)
		}
		projection = append(projection, workspaceRepositoryMaterialization{RepositoryURL: locator, Destination: name, BaseBranch: taskRepository.BaseBranch, CheckoutBranch: taskRepository.CheckoutBranch})
	}
	return projection, nil
}

func remoteRepositoryLocator(repository *models.Repository) string {
	locator := strings.TrimSpace(repository.RemoteURL)
	if locator == "" {
		locator = providerRepositoryLocator(repository)
	}
	if locator == "" || strings.HasPrefix(locator, "/") || strings.HasPrefix(locator, "file:") {
		return ""
	}
	if strings.HasPrefix(locator, "git@") {
		return locator
	}
	parsed, err := url.Parse(locator)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh" && parsed.Scheme != "git") {
		return ""
	}
	return locator
}

func providerRepositoryLocator(repository *models.Repository) string {
	if repository.ProviderOwner == "" || repository.ProviderName == "" {
		return ""
	}
	if strings.EqualFold(repository.Provider, "gitlab") && strings.TrimSpace(repository.ProviderHost) == "" {
		return ""
	}
	locator, err := repoclone.CloneURLWithHost(repository.Provider, repository.ProviderHost, repository.ProviderOwner, repository.ProviderName, repoclone.ProtocolHTTPS)
	if err != nil {
		return ""
	}
	return locator
}

func isHostWorkspaceExecutor(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocal) || executorType == legacyLocalPCExecutor || executorType == string(models.ExecutorTypeWorktree)
}

func (m *workspaceSourceMaterializer) materializeWorktreeSources(ctx context.Context, taskID, root string, batch *models.WorkspaceSourceBatch, folders []*models.TaskWorkspaceFolder, owner worktree.OwnedDirectoryLinkOwner) ([]ownedDirectoryLinkUndo, []*branchMaterialization, error) {
	materializations := make([]*branchMaterialization, 0, len(batch.Sources))
	created := make([]ownedDirectoryLinkUndo, 0, len(batch.Sources))
	for _, source := range batch.Sources {
		if source.Repository != nil {
			if m.branches == nil {
				return nil, nil, fmt.Errorf("worktree repository materializer is unavailable")
			}
			materialization, err := m.branches.materializeUnfinalized(ctx, taskID, source.Repository.ID)
			if err != nil {
				return created, materializations, err
			}
			if materialization != nil {
				materializations = append(materializations, materialization)
			}
		}
	}
	entries, err := workspaceFolderEntries(folders, batch)
	if err != nil {
		return created, materializations, err
	}
	created = make([]ownedDirectoryLinkUndo, 0, len(entries))
	for entry, target := range entries {
		linkRoot, name, err := workspaceDirectoryLinkLocation(root, entry)
		if err != nil {
			return created, materializations, fmt.Errorf("link worktree folder %q: %w", entry, err)
		}
		result, err := worktree.EnsureOwnedDirectoryLink(linkRoot, name, target, owner)
		if err != nil {
			return created, materializations, fmt.Errorf("link worktree folder %q: %w", entry, err)
		}
		if result.Created {
			created = append(created, ownedDirectoryLinkUndo{Path: result.Path, PriorTarget: result.PriorTarget})
		}
	}
	return created, materializations, nil
}

func localWorkspaceEntries(repos []*models.TaskRepository, folders []*models.TaskWorkspaceFolder, entities map[string]*models.Repository, batch *models.WorkspaceSourceBatch) (map[string]string, error) {
	entries := map[string]string{}
	add := func(name, target string) error {
		if _, exists := entries[name]; exists {
			return fmt.Errorf("workspace runtime entry %q collides", name)
		}
		entries[name] = target
		return nil
	}
	for _, tr := range repos {
		if tr == nil {
			continue
		}
		repository := entities[tr.RepositoryID]
		name, err := taskservice.WorkspaceSourceRuntimeEntryName(string(models.ExecutorTypeLocal), repository, tr)
		if err != nil {
			return nil, err
		}
		entry := name
		if tr.WorkspaceRelativePath != "" {
			entry = tr.WorkspaceRelativePath
		}
		if err := add(entry, repository.LocalPath); err != nil {
			return nil, err
		}
	}
	folderEntries, err := workspaceFolderEntries(folders, batch)
	if err != nil {
		return nil, err
	}
	for name, target := range folderEntries {
		if err := add(name, target); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// workspaceFolderEntries treats durable folder rows as the source of truth.
// The attachment service persists a batch before materializing it, so the
// matching batch row is already represented in folders and must not collide
// with itself. Unpersisted batch folders remain included and collision-checked.
func workspaceFolderEntries(folders []*models.TaskWorkspaceFolder, batch *models.WorkspaceSourceBatch) (map[string]string, error) {
	entries := make(map[string]string, len(folders))
	persistedIDs := make(map[string]struct{}, len(folders))
	add := func(folder *models.TaskWorkspaceFolder) error {
		entry := folder.DisplayName
		if folder.WorkspaceRelativePath != "" {
			entry = folder.WorkspaceRelativePath
		}
		if _, exists := entries[entry]; exists {
			return fmt.Errorf("workspace runtime entry %q collides", folder.DisplayName)
		}
		entries[entry] = folder.LocalPath
		return nil
	}
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		if folder.ID != "" {
			persistedIDs[folder.ID] = struct{}{}
		}
		if err := add(folder); err != nil {
			return nil, err
		}
	}
	if batch == nil {
		return entries, nil
	}
	for _, source := range batch.Sources {
		folder := source.Folder
		if folder == nil {
			continue
		}
		if _, persisted := persistedIDs[folder.ID]; persisted && folder.ID != "" {
			continue
		}
		if err := add(folder); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func pathExists(path string) bool { _, err := os.Lstat(path); return err == nil }

func sameDirectory(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftInfo, err := os.Stat(left)
	if err != nil || !leftInfo.IsDir() {
		return false
	}
	rightInfo, err := os.Stat(right)
	if err != nil || !rightInfo.IsDir() {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}
