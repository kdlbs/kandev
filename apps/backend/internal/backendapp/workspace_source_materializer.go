package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// ErrRemoteRepositoryLocatorUnavailable prevents host-local repository paths
// from crossing an executor boundary that can only clone remote locators.
var ErrRemoteRepositoryLocatorUnavailable = errors.New("remote repository locator unavailable")

const legacyLocalPCExecutor = "local_pc"

// workspaceSourceMaterializerRepo is intentionally limited to the durable
// state needed after the service has transactionally persisted a batch.
type workspaceSourceMaterializerRepo interface {
	GetTaskEnvironmentByTaskID(context.Context, string) (*models.TaskEnvironment, error)
	UpdateTaskEnvironment(context.Context, *models.TaskEnvironment) error
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	ListTaskWorkspaceFolders(context.Context, string) ([]*models.TaskWorkspaceFolder, error)
	GetRepository(context.Context, string) (*models.Repository, error)
}

// workspaceSourceMaterializer is the host-only materialization boundary. It
// creates only Kandev-owned directory entries; each source itself remains a
// live user-owned directory and is never copied, removed, or modified.
type workspaceSourceMaterializer struct {
	repo        workspaceSourceMaterializerRepo
	worktreeMgr *worktree.Manager
	branches    *branchMaterializer
	rescanner   interface {
		RebindWorkspaceForSession(context.Context, string, string, ...[]string) error
	}
	remoteMaterializer interface {
		MaterializeRepositoriesForEnvironment(context.Context, string, []lifecycle.WorkspaceRepositoryMaterialization) ([]string, error)
	}
	hostCloner orchestrator.RepositoryHostCloner
	logger     *logger.Logger
}

type workspaceSourceMaterializationState struct {
	environment  *models.TaskEnvironment
	sessions     []*models.TaskSession
	repositories []*models.TaskRepository
	folders      []*models.TaskWorkspaceFolder
	entities     map[string]*models.Repository
}

type hostWorkspaceMaterialization struct {
	root          string
	oldPath       string
	rootExisted   bool
	knownWorktree map[string]bool
	priorRoots    []string
	postRoots     []string
}

func newWorkspaceSourceMaterializer(repo workspaceSourceMaterializerRepo, mgr *worktree.Manager, lc *lifecycle.Manager, log *logger.Logger) *workspaceSourceMaterializer {
	var rescanner interface {
		RebindWorkspaceForSession(context.Context, string, string, ...[]string) error
	}
	if lc != nil {
		rescanner = lc
	}
	materializer := &workspaceSourceMaterializer{repo: repo, worktreeMgr: mgr, rescanner: rescanner, remoteMaterializer: lc, logger: log.WithFields(zap.String("component", "workspace_source_materializer"))}
	if branchRepo, ok := repo.(branchMaterializerRepo); ok {
		materializer.branches = newBranchMaterializer(branchRepo, mgr, lc, log)
	}
	return materializer
}

// SetHostRepositoryCloner wires the authenticated orchestrator clone seam
// after its repository and workspace credential dependencies are configured.
func (m *workspaceSourceMaterializer) SetHostRepositoryCloner(cloner orchestrator.RepositoryHostCloner) {
	if m != nil {
		m.hostCloner = cloner
	}
}

func (m *workspaceSourceMaterializer) MaterializeWorkspaceSources(ctx context.Context, taskID string, batch *models.WorkspaceSourceBatch) (*taskservice.WorkspaceSourceMaterializationResult, error) {
	if m == nil || m.repo == nil || m.worktreeMgr == nil || batch == nil {
		return &taskservice.WorkspaceSourceMaterializationResult{}, nil
	}
	state, err := m.loadMaterializationState(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if state == nil || state.environment == nil {
		return &taskservice.WorkspaceSourceMaterializationResult{}, nil
	}
	if !isHostWorkspaceExecutor(state.environment.ExecutorType) {
		return m.materializeRemoteWorkspaceSources(ctx, taskID, state, batch)
	}
	return m.materializeHostWorkspaceSources(ctx, taskID, state, batch)
}

func (m *workspaceSourceMaterializer) materializeHostWorkspaceSources(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) (_ *taskservice.WorkspaceSourceMaterializationResult, err error) {
	if err := m.ensureHostRepositoryPaths(ctx, state); err != nil {
		return nil, err
	}
	materialization, err := m.prepareHostWorkspaceMaterialization(ctx, taskID, state, batch)
	if err != nil {
		return nil, err
	}
	created := make([]string, 0, len(batch.Sources))
	adoptedSessions := make([]*models.TaskSession, 0, len(state.sessions))
	defer func() {
		if err == nil {
			return
		}
		if rollbackErr := m.rollbackHostWorkspaceMaterialization(ctx, taskID, state, materialization, adoptedSessions, created); rollbackErr != nil {
			err = fmt.Errorf("%w; restore adopted session workspaces: %v", err, rollbackErr)
		}
	}()
	branchMaterializations, materialized, materializeErr := m.materializeHostRuntime(ctx, taskID, materialization.root, state, batch)
	created = append(created, materialized...)
	if materializeErr != nil {
		return nil, materializeErr
	}
	if state.environment.WorkspacePath != materialization.root {
		state.environment.WorkspacePath, state.environment.UpdatedAt = materialization.root, time.Now().UTC()
		if err = m.repo.UpdateTaskEnvironment(ctx, state.environment); err != nil {
			return nil, fmt.Errorf("persist task workspace path: %w", err)
		}
	}
	ids, adopted, adoptErr := m.adoptSessionWorkspaces(ctx, state.sessions, materialization.root, materialization.postRoots)
	adoptedSessions = append(adoptedSessions, adopted...)
	if adoptErr != nil {
		return nil, adoptErr
	}
	for _, materialization := range branchMaterializations {
		m.branches.finalize(materialization, ctx)
	}
	return &taskservice.WorkspaceSourceMaterializationResult{WorkspacePath: materialization.root, SessionIDs: ids}, nil
}

func (m *workspaceSourceMaterializer) prepareHostWorkspaceMaterialization(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) (*hostWorkspaceMaterialization, error) {
	postRoots, err := canonicalWorkspaceSourceRoots(state, batch, true)
	if err != nil {
		return nil, err
	}
	priorRoots, err := canonicalWorkspaceSourceRoots(state, batch, false)
	if err != nil {
		return nil, err
	}
	root, err := m.worktreeMgr.TaskRoot(state.environment.TaskDirName)
	if err != nil {
		return nil, fmt.Errorf("resolve owned task root: %w", err)
	}
	worktrees, err := m.worktreeMgr.GetAllByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("snapshot task worktrees: %w", err)
	}
	return &hostWorkspaceMaterialization{
		root: root, oldPath: state.environment.WorkspacePath, rootExisted: pathExists(root),
		knownWorktree: worktreeIDs(worktrees), priorRoots: priorRoots, postRoots: postRoots,
	}, nil
}

func worktreeIDs(worktrees []*worktree.Worktree) map[string]bool {
	ids := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		if wt != nil {
			ids[wt.ID] = true
		}
	}
	return ids
}

func (m *workspaceSourceMaterializer) materializeHostRuntime(ctx context.Context, taskID, root string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) ([]*branchMaterialization, []string, error) {
	if state.environment.ExecutorType == string(models.ExecutorTypeWorktree) {
		created, materializations, err := m.materializeWorktreeSources(ctx, taskID, root, batch, state.folders)
		return materializations, created, err
	}
	entries, err := localWorkspaceEntries(state.repositories, state.folders, state.entities, batch)
	if err != nil {
		return nil, nil, err
	}
	created, err := materializeDirectoryLinks(root, entries, "workspace source")
	return nil, created, err
}

func materializeDirectoryLinks(root string, entries map[string]string, description string) ([]string, error) {
	created := make([]string, 0, len(entries))
	for name, target := range entries {
		entry, wasCreated, err := worktree.EnsureOwnedDirectoryLink(root, name, target)
		if err != nil {
			return created, fmt.Errorf("link %s %q: %w", description, name, err)
		}
		if wasCreated {
			created = append(created, entry)
		}
	}
	return created, nil
}

func (m *workspaceSourceMaterializer) adoptSessionWorkspaces(ctx context.Context, sessions []*models.TaskSession, root string, sourceRoots []string) ([]string, []*models.TaskSession, error) {
	ids := make([]string, 0, len(sessions))
	adopted := make([]*models.TaskSession, 0, len(sessions))
	for _, session := range sessions {
		if m.rescanner != nil {
			if err := m.rescanner.RebindWorkspaceForSession(ctx, session.ID, root, sourceRoots); err != nil {
				return nil, adopted, fmt.Errorf("adopt workspace for session %s: %w", session.ID, err)
			}
			adopted = append(adopted, session)
		}
		ids = append(ids, session.ID)
	}
	return ids, adopted, nil
}

func (m *workspaceSourceMaterializer) rollbackHostWorkspaceMaterialization(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, materialization *hostWorkspaceMaterialization, adopted []*models.TaskSession, created []string) error {
	rollbackErr := m.restoreSessionWorkspaces(ctx, adopted, materialization.oldPath, materialization.priorRoots)
	for index := len(created) - 1; index >= 0; index-- {
		_ = os.Remove(created[index])
	}
	m.cleanupNewWorktrees(ctx, taskID, materialization.knownWorktree)
	if !materialization.rootExisted {
		_ = os.Remove(materialization.root)
	}
	if state.environment.WorkspacePath != materialization.oldPath {
		state.environment.WorkspacePath = materialization.oldPath
		state.environment.UpdatedAt = time.Now().UTC()
		_ = m.repo.UpdateTaskEnvironment(context.WithoutCancel(ctx), state.environment)
	}
	return rollbackErr
}

func (m *workspaceSourceMaterializer) cleanupNewWorktrees(ctx context.Context, taskID string, known map[string]bool) {
	worktrees, err := m.worktreeMgr.GetAllByTaskID(context.WithoutCancel(ctx), taskID)
	if err != nil {
		return
	}
	created := make([]*worktree.Worktree, 0)
	for _, wt := range worktrees {
		if wt != nil && !known[wt.ID] {
			created = append(created, wt)
		}
	}
	if len(created) > 0 {
		_ = m.worktreeMgr.CleanupWorktrees(context.WithoutCancel(ctx), created)
	}
}

func (m *workspaceSourceMaterializer) restoreSessionWorkspaces(ctx context.Context, sessions []*models.TaskSession, workspacePath string, sourceRoots []string) error {
	if m.rescanner == nil || len(sessions) == 0 {
		return nil
	}
	rollbackCtx := context.WithoutCancel(ctx)
	var rollbackErr error
	for index := len(sessions) - 1; index >= 0; index-- {
		session := sessions[index]
		if err := m.rescanner.RebindWorkspaceForSession(rollbackCtx, session.ID, workspacePath, sourceRoots); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("session %s: %w", session.ID, err))
		}
	}
	return rollbackErr
}

func canonicalWorkspaceSourceRoots(state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch, includeBatch bool) ([]string, error) {
	batchRepositories, batchFolders := workspaceSourceBatchIDs(batch)
	collector := newWorkspaceSourceRootCollector(len(state.repositories) + len(state.folders))
	if err := collector.addRepositories(state.repositories, state.entities, batchRepositories, includeBatch); err != nil {
		return nil, err
	}
	if err := collector.addFolders(state.folders, batchFolders, includeBatch); err != nil {
		return nil, err
	}
	if includeBatch {
		if err := collector.addUnpersistedBatchFolders(batch, batchFolders); err != nil {
			return nil, err
		}
	}
	return collector.roots, nil
}

type workspaceSourceRootCollector struct {
	roots []string
	seen  map[string]struct{}
}

func newWorkspaceSourceRootCollector(capacity int) *workspaceSourceRootCollector {
	return &workspaceSourceRootCollector{roots: make([]string, 0, capacity), seen: make(map[string]struct{}, capacity)}
}

func (c *workspaceSourceRootCollector) addRepositories(repositories []*models.TaskRepository, entities map[string]*models.Repository, batchIDs map[string]bool, includeBatch bool) error {
	for _, taskRepository := range repositories {
		if !includeBatch && batchIDs[taskRepository.ID] {
			continue
		}
		if repository := entities[taskRepository.RepositoryID]; repository != nil {
			if err := c.add(repository.LocalPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *workspaceSourceRootCollector) addFolders(folders []*models.TaskWorkspaceFolder, batchIDs map[string]bool, includeBatch bool) error {
	for _, folder := range folders {
		if !includeBatch && batchIDs[folder.ID] {
			continue
		}
		if err := c.add(folder.LocalPath); err != nil {
			return err
		}
	}
	return nil
}

func (c *workspaceSourceRootCollector) addUnpersistedBatchFolders(batch *models.WorkspaceSourceBatch, batchIDs map[string]bool) error {
	for _, source := range batch.Sources {
		if source.Folder != nil && !batchIDs[source.Folder.ID] {
			if err := c.add(source.Folder.LocalPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *workspaceSourceRootCollector) add(path string) error {
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("resolve workspace source root %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("workspace source root is not a directory: %s", path)
	}
	if _, exists := c.seen[resolved]; !exists {
		c.seen[resolved] = struct{}{}
		c.roots = append(c.roots, resolved)
	}
	return nil
}

func workspaceSourceBatchIDs(batch *models.WorkspaceSourceBatch) (map[string]bool, map[string]bool) {
	repositories, folders := map[string]bool{}, map[string]bool{}
	if batch == nil {
		return repositories, folders
	}
	for _, source := range batch.Sources {
		if source.Repository != nil && source.Repository.ID != "" {
			repositories[source.Repository.ID] = true
		}
		if source.Folder != nil && source.Folder.ID != "" {
			folders[source.Folder.ID] = true
		}
	}
	return repositories, folders
}

func (m *workspaceSourceMaterializer) ensureHostRepositoryPaths(ctx context.Context, state *workspaceSourceMaterializationState) error {
	for _, taskRepository := range state.repositories {
		repository := state.entities[taskRepository.RepositoryID]
		if repository == nil || repository.LocalPath != "" {
			continue
		}
		if repository.ProviderOwner == "" || repository.ProviderName == "" {
			return fmt.Errorf("repository %q source path is missing", taskRepository.RepositoryID)
		}
		if m.hostCloner == nil {
			return fmt.Errorf("host repository cloner is unavailable for %q", repository.Name)
		}
		path, err := m.hostCloner.EnsureRepositoryCloned(ctx, repository)
		if err != nil {
			return fmt.Errorf("clone repository %q: %w", repository.Name, err)
		}
		if path == "" {
			return fmt.Errorf("repository %q clone produced no local path", repository.Name)
		}
		repository.LocalPath = path
	}
	return nil
}

func (m *workspaceSourceMaterializer) loadMaterializationState(ctx context.Context, taskID string) (*workspaceSourceMaterializationState, error) {
	environment, err := m.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || environment == nil {
		return nil, err
	}
	sessions, err := m.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	repositories, err := m.repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	folders, err := m.repo.ListTaskWorkspaceFolders(ctx, taskID)
	if err != nil {
		return nil, err
	}
	entities := make(map[string]*models.Repository, len(repositories))
	for _, taskRepository := range repositories {
		entity, err := m.repo.GetRepository(ctx, taskRepository.RepositoryID)
		if err != nil {
			return nil, fmt.Errorf("resolve repository source: %w", err)
		}
		if entity == nil {
			return nil, fmt.Errorf("resolve repository source: repository %q not found", taskRepository.RepositoryID)
		}
		entities[taskRepository.RepositoryID] = entity
	}
	return &workspaceSourceMaterializationState{environment: environment, sessions: sessions, repositories: repositories, folders: folders, entities: entities}, nil
}

func (m *workspaceSourceMaterializer) materializeRemoteWorkspaceSources(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) (*taskservice.WorkspaceSourceMaterializationResult, error) {
	if m.remoteMaterializer == nil {
		return nil, fmt.Errorf("remote workspace materializer is unavailable")
	}
	projection, err := buildRemoteWorkspaceRepositoryBatch(batch, state.entities)
	if err != nil {
		return nil, err
	}
	ids, err := m.remoteMaterializer.MaterializeRepositoriesForEnvironment(ctx, state.environment.ID, projection)
	if err != nil {
		return nil, err
	}
	return &taskservice.WorkspaceSourceMaterializationResult{WorkspacePath: state.environment.WorkspacePath, SessionIDs: ids}, nil
}

// buildRemoteWorkspaceRepositoryBatch projects only the sources created by
// this attachment operation. Existing durable siblings belong in a fresh
// launch/resume projection, but revalidating them in a live execution would
// reject normal agent commits that advance their HEAD after attachment.
func buildRemoteWorkspaceRepositoryBatch(batch *models.WorkspaceSourceBatch, repositoryByID map[string]*models.Repository) ([]lifecycle.WorkspaceRepositoryMaterialization, error) {
	if batch == nil {
		return nil, nil
	}
	projection := make([]lifecycle.WorkspaceRepositoryMaterialization, 0, len(batch.Sources))
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
		projection = append(projection, lifecycle.WorkspaceRepositoryMaterialization{RepositoryURL: locator, Destination: name, BaseBranch: source.Repository.BaseBranch, CheckoutBranch: source.Repository.CheckoutBranch})
	}
	return projection, nil
}

func buildRemoteWorkspaceRepositories(taskRepositories []*models.TaskRepository, repositoryByID map[string]*models.Repository) ([]lifecycle.WorkspaceRepositoryMaterialization, error) {
	projection := make([]lifecycle.WorkspaceRepositoryMaterialization, 0, len(taskRepositories))
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
		projection = append(projection, lifecycle.WorkspaceRepositoryMaterialization{RepositoryURL: locator, Destination: name, BaseBranch: taskRepository.BaseBranch, CheckoutBranch: taskRepository.CheckoutBranch})
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

func (m *workspaceSourceMaterializer) materializeWorktreeSources(ctx context.Context, taskID, root string, batch *models.WorkspaceSourceBatch, folders []*models.TaskWorkspaceFolder) ([]string, []*branchMaterialization, error) {
	materializations := make([]*branchMaterialization, 0, len(batch.Sources))
	for _, source := range batch.Sources {
		if source.Repository != nil {
			if m.branches == nil {
				return nil, nil, fmt.Errorf("worktree repository materializer is unavailable")
			}
			materialization, err := m.branches.materializeUnfinalized(ctx, taskID, source.Repository.ID)
			if err != nil {
				return nil, nil, err
			}
			if materialization != nil {
				materializations = append(materializations, materialization)
			}
		}
	}
	entries, err := workspaceFolderEntries(folders, batch)
	if err != nil {
		return nil, nil, err
	}
	created := make([]string, 0, len(entries))
	for name, target := range entries {
		entry, wasCreated, err := worktree.EnsureOwnedDirectoryLink(root, name, target)
		if err != nil {
			return created, nil, fmt.Errorf("link worktree folder %q: %w", name, err)
		}
		if wasCreated {
			created = append(created, entry)
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
		repository := entities[tr.RepositoryID]
		name, err := taskservice.WorkspaceSourceRuntimeEntryName(string(models.ExecutorTypeLocal), repository, tr)
		if err != nil {
			return nil, err
		}
		if err := add(name, repository.LocalPath); err != nil {
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
		if _, exists := entries[folder.DisplayName]; exists {
			return fmt.Errorf("workspace runtime entry %q collides", folder.DisplayName)
		}
		entries[folder.DisplayName] = folder.LocalPath
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
