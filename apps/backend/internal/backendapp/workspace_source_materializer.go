package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
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
	ListTaskEnvironmentRepos(context.Context, string) ([]*models.TaskEnvironmentRepo, error)
	CreateTaskEnvironmentRepo(context.Context, *models.TaskEnvironmentRepo) error
	DeleteTaskEnvironmentRepo(context.Context, string) error
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	ListTaskWorkspaceFolders(context.Context, string) ([]*models.TaskWorkspaceFolder, error)
	GetRepository(context.Context, string) (*models.Repository, error)
}

type workspaceRepositoryMaterialization = lifecycle.WorkspaceRepositoryMaterialization

// workspaceSourceMaterializer is the host-only materialization boundary. It
// creates only Kandev-owned directory entries; each source itself remains a
// live user-owned directory and is never copied, removed, or modified.
type workspaceSourceMaterializer struct {
	repo               workspaceSourceMaterializerRepo
	worktreeMgr        *worktree.Manager
	branches           *branchMaterializer
	rescanner          workspaceSourceSessionRebinder
	remoteMaterializer interface {
		MaterializeRepositoriesForEnvironment(context.Context, string, []lifecycle.WorkspaceRepositoryMaterialization) ([]string, error)
	}
	hostCloner orchestrator.RepositoryHostCloner
	logger     *logger.Logger
}

type workspaceSourceSessionRebinder interface {
	RebindWorkspaceForSession(context.Context, string, string, ...[]string) error
}

type workspaceSourceSessionRescanner interface {
	RescanWorkspaceForSession(context.Context, string, string, ...[]string) error
}

type workspaceSourceMaterializationState struct {
	environment  *models.TaskEnvironment
	sessions     []*models.TaskSession
	repositories []*models.TaskRepository
	folders      []*models.TaskWorkspaceFolder
	entities     map[string]*models.Repository
}

type hostWorkspaceMaterialization struct {
	root            string
	linkRoot        string
	oldPath         string
	oldLayout       string
	rootExisted     bool
	linkRootExisted bool
	knownWorktree   map[string]bool
	newWorktrees    []*worktree.Worktree
	priorRoots      []string
	postRoots       []string
	linkUndo        []ownedDirectoryLinkUndo
}

type ownedDirectoryLinkUndo struct {
	Path        string
	PriorTarget string
}

func materializedLinkOwner(taskID, taskDirName string) worktree.OwnedDirectoryLinkOwner {
	return worktree.OwnedDirectoryLinkOwner{TaskID: taskID, TaskDirName: taskDirName}
}

func newWorkspaceSourceMaterializer(repo workspaceSourceMaterializerRepo, mgr *worktree.Manager, lc *lifecycle.Manager, log *logger.Logger) *workspaceSourceMaterializer {
	var rescanner workspaceSourceSessionRebinder
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
	if state.environment.Status == models.TaskEnvironmentStatusCreating ||
		(isHostWorkspaceExecutor(state.environment.ExecutorType) && state.environment.TaskDirName == "") {
		m.logger.Info("deferring workspace source materialization until the task environment is provisioned",
			zap.String("task_id", taskID), zap.String("task_environment_id", state.environment.ID))
		return &taskservice.WorkspaceSourceMaterializationResult{}, nil
	}
	if !isHostWorkspaceExecutor(state.environment.ExecutorType) {
		if !models.IsRemoteExecutorType(models.ExecutorType(state.environment.ExecutorType)) {
			return nil, fmt.Errorf(
				"%w: executor %q cannot materialize workspace sources",
				taskservice.ErrUnsupportedWorkspaceSource,
				state.environment.ExecutorType,
			)
		}
		return m.materializeRemoteWorkspaceSources(ctx, taskID, state, batch)
	}
	return m.materializeHostWorkspaceSources(ctx, taskID, state, batch)
}

func (m *workspaceSourceMaterializer) materializeHostWorkspaceSources(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) (_ *taskservice.WorkspaceSourceMaterializationResult, err error) {
	if err := m.ensureHostRepositoryPaths(ctx, taskID, state); err != nil {
		return nil, err
	}
	materialization, err := m.prepareHostWorkspaceMaterialization(ctx, taskID, state, batch)
	if err != nil {
		return nil, err
	}
	adoptedSessions := make([]*models.TaskSession, 0, len(state.sessions))
	rescannedSessions := make([]*models.TaskSession, 0, len(state.sessions))
	createdInventory := make([]*models.TaskEnvironmentRepo, 0, len(batch.Sources))
	defer func() {
		if err == nil {
			return
		}
		if rollbackErr := m.rollbackHostWorkspaceMaterialization(ctx, taskID, state, materialization, adoptedSessions, rescannedSessions, createdInventory); rollbackErr != nil {
			err = fmt.Errorf("%w; restore workspace materialization: %v", err, rollbackErr)
		}
	}()
	branchMaterializations, materialized, materializeErr := m.materializeHostRuntime(ctx, taskID, materialization.root, state, batch)
	materialization.linkUndo = append(materialization.linkUndo, materialized...)
	for _, branch := range branchMaterializations {
		if branch != nil && branch.worktree != nil {
			materialization.newWorktrees = append(materialization.newWorktrees, branch.worktree)
		}
	}
	if materializeErr != nil {
		return nil, materializeErr
	}
	if workspaceSourcePlacementKeepsRoot(batch) || sameWorkspacePath(materialization.root, materialization.oldPath) {
		ids, rescanned, inventory, nestedErr := m.materializeNestedHostWorkspaceSources(ctx, state, batch, materialization, branchMaterializations)
		rescannedSessions = append(rescannedSessions, rescanned...)
		createdInventory = append(createdInventory, inventory...)
		if nestedErr != nil {
			return nil, nestedErr
		}
		return &taskservice.WorkspaceSourceMaterializationResult{WorkspacePath: materialization.oldPath, SessionIDs: ids}, nil
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
	createdInventory, err = m.persistEnvironmentRepositoryInventory(ctx, state.environment.ID, state.environment.ExecutorType, batch, branchMaterializations)
	if err != nil {
		return nil, err
	}
	for _, branch := range branchMaterializations {
		if branch != nil && m.branches != nil {
			m.branches.finalize(branch, ctx)
		}
	}
	return &taskservice.WorkspaceSourceMaterializationResult{WorkspacePath: materialization.root, SessionIDs: ids}, nil
}

func workspaceSourcePlacementKeepsRoot(batch *models.WorkspaceSourceBatch) bool {
	if batch == nil {
		return false
	}
	return batch.RepositoryPlacement == string(taskservice.WorkspacePlacementKandevDirectory) || batch.RepositoryPlacement == string(taskservice.WorkspacePlacementCurrentRoot)
}

func (m *workspaceSourceMaterializer) materializeNestedHostWorkspaceSources(ctx context.Context, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch, materialization *hostWorkspaceMaterialization, branches []*branchMaterialization) ([]string, []*models.TaskSession, []*models.TaskEnvironmentRepo, error) {
	for _, branch := range branches {
		if branch != nil && branch.worktree != nil {
			materialization.postRoots = appendWorkspaceRoot(materialization.postRoots, branch.worktree.Path)
		}
	}
	ids, rescanned, err := m.rescanSessionWorkspaces(ctx, state.sessions, materialization.root, materialization.postRoots)
	if err != nil {
		return nil, rescanned, nil, err
	}
	createdInventory, err := m.persistEnvironmentRepositoryInventory(ctx, state.environment.ID, state.environment.ExecutorType, batch, branches)
	if err != nil {
		return nil, rescanned, createdInventory, err
	}
	desiredLayout := workspaceSourceMaterializationLayout(state.environment, batch)
	if desiredLayout != "" && state.environment.WorkspaceLayout != desiredLayout {
		state.environment.WorkspaceLayout = desiredLayout
		state.environment.UpdatedAt = time.Now().UTC()
		if err := m.repo.UpdateTaskEnvironment(ctx, state.environment); err != nil {
			return nil, rescanned, createdInventory, fmt.Errorf("persist task workspace layout: %w", err)
		}
	}
	for _, branch := range branches {
		if branch != nil && m.branches != nil {
			m.branches.notifyMaterialized(branch, materialization.oldPath, ctx)
		}
	}
	return ids, rescanned, createdInventory, nil
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
	root, err := m.resolveHostWorkspaceRoot(state, batch)
	if err != nil {
		return nil, err
	}
	linkRoot := root
	if isLocalWorkspaceExecutor(state.environment.ExecutorType) && workspaceSourceMaterializationLayout(state.environment, batch) == taskservice.WorkspaceLayoutKandevDirectory {
		linkRoot = filepath.Join(root, "kandev")
	}
	worktrees, err := m.worktreeMgr.GetAllByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("snapshot task worktrees: %w", err)
	}
	return &hostWorkspaceMaterialization{
		root: root, linkRoot: linkRoot, oldPath: state.environment.WorkspacePath, oldLayout: state.environment.WorkspaceLayout,
		rootExisted: pathExists(root), linkRootExisted: pathExists(linkRoot), knownWorktree: worktreeIDs(worktrees),
		priorRoots: priorRoots, postRoots: postRoots, linkUndo: make([]ownedDirectoryLinkUndo, 0, len(batch.Sources)),
	}, nil
}

func (m *workspaceSourceMaterializer) resolveHostWorkspaceRoot(state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) (string, error) {
	if isLocalWorkspaceExecutor(state.environment.ExecutorType) && state.environment.WorkspacePath != "" {
		root, err := filepath.Abs(filepath.Clean(state.environment.WorkspacePath))
		if err != nil {
			return "", fmt.Errorf("resolve established workspace root: %w", err)
		}
		return root, nil
	}
	root, err := m.worktreeMgr.TaskRoot(state.environment.TaskDirName)
	if err != nil {
		return "", fmt.Errorf("resolve owned task root: %w", err)
	}
	if !workspaceSourcePlacementKeepsRoot(batch) || taskservice.EffectiveTaskEnvironmentWorkspaceLayout(state.environment) == taskservice.WorkspaceLayoutTaskRoot {
		return root, nil
	}
	return nestedWorkspaceMaterializationRoot(root, state.environment.WorkspacePath)
}

func isLocalWorkspaceExecutor(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocal) || executorType == legacyLocalPCExecutor
}

func workspaceSourceMaterializationLayout(environment *models.TaskEnvironment, batch *models.WorkspaceSourceBatch) string {
	if environment == nil {
		return ""
	}
	if batch != nil {
		switch batch.RepositoryPlacement {
		case string(taskservice.WorkspacePlacementKandevDirectory):
			return taskservice.WorkspaceLayoutKandevDirectory
		case string(taskservice.WorkspacePlacementCurrentRoot):
			return taskservice.WorkspaceLayoutCurrentRoot
		}
	}
	if environment.WorkspaceLayout != "" {
		return environment.WorkspaceLayout
	}
	if isLocalWorkspaceExecutor(environment.ExecutorType) && environment.WorkspacePath != "" {
		return taskservice.WorkspaceLayoutCurrentRoot
	}
	return taskservice.EffectiveTaskEnvironmentWorkspaceLayout(environment)
}

func sameWorkspacePath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && leftAbs == rightAbs
}

func nestedWorkspaceMaterializationRoot(taskRoot, workspacePath string) (string, error) {
	if workspacePath == "" {
		return "", fmt.Errorf("%w: current workspace root is empty", taskservice.ErrInvalidWorkspaceRepositoryPlacement)
	}
	taskRoot, err := filepath.Abs(filepath.Clean(taskRoot))
	if err != nil {
		return "", fmt.Errorf("%w: resolve task root: %v", taskservice.ErrInvalidWorkspaceRepositoryPlacement, err)
	}
	workspacePath, err = filepath.Abs(filepath.Clean(workspacePath))
	if err != nil {
		return "", fmt.Errorf("%w: resolve current workspace root: %v", taskservice.ErrInvalidWorkspaceRepositoryPlacement, err)
	}
	relative, err := filepath.Rel(taskRoot, workspacePath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: current workspace root must be inside the task root", taskservice.ErrInvalidWorkspaceRepositoryPlacement)
	}
	if !pathExists(workspacePath) {
		return "", fmt.Errorf("%w: current workspace root does not exist", taskservice.ErrInvalidWorkspaceRepositoryPlacement)
	}
	return workspacePath, nil
}

func appendWorkspaceRoot(roots []string, root string) []string {
	for _, existing := range roots {
		if existing == root {
			return roots
		}
	}
	return append(roots, root)
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

func (m *workspaceSourceMaterializer) materializeHostRuntime(ctx context.Context, taskID, root string, state *workspaceSourceMaterializationState, batch *models.WorkspaceSourceBatch) ([]*branchMaterialization, []ownedDirectoryLinkUndo, error) {
	owner := materializedLinkOwner(taskID, state.environment.TaskDirName)
	if state.environment.ExecutorType == string(models.ExecutorTypeWorktree) {
		created, materializations, err := m.materializeWorktreeSources(ctx, taskID, root, batch, state.folders, owner)
		return materializations, created, err
	}
	entries, err := localWorkspaceEntries(state.repositories, state.folders, state.entities, batch)
	if err != nil {
		return nil, nil, err
	}
	created, err := materializeDirectoryLinks(root, entries, "workspace source", owner)
	return nil, created, err
}

func materializeDirectoryLinks(root string, entries map[string]string, description string, owner worktree.OwnedDirectoryLinkOwner) ([]ownedDirectoryLinkUndo, error) {
	created := make([]ownedDirectoryLinkUndo, 0, len(entries))
	for entry, target := range entries {
		if sameDirectory(root, target) {
			continue
		}
		linkRoot, name, err := workspaceDirectoryLinkLocation(root, entry)
		if err != nil {
			return created, fmt.Errorf("link %s %q: %w", description, entry, err)
		}
		result, err := worktree.EnsureOwnedDirectoryLink(linkRoot, name, target, owner)
		if err != nil {
			return created, fmt.Errorf("link %s %q: %w", description, entry, err)
		}
		if result.Created {
			created = append(created, ownedDirectoryLinkUndo{Path: result.Path, PriorTarget: result.PriorTarget})
		}
	}
	return created, nil
}

func workspaceDirectoryLinkLocation(root, entry string) (string, string, error) {
	if root == "" || !filepath.IsAbs(root) || entry == "" {
		return "", "", errors.New("workspace link root and entry are required")
	}
	relative := filepath.Clean(filepath.FromSlash(entry))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("unsafe workspace link entry %q", entry)
	}
	target := filepath.Join(root, relative)
	contained, err := filepath.Rel(root, target)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("workspace link entry %q escapes root", entry)
	}
	name := filepath.Base(target)
	if name == "." || name == ".." || name == "" || filepath.Base(name) != name {
		return "", "", fmt.Errorf("unsafe workspace link entry %q", entry)
	}
	return filepath.Dir(target), name, nil
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

func (m *workspaceSourceMaterializer) rescanSessionWorkspaces(ctx context.Context, sessions []*models.TaskSession, root string, sourceRoots []string) ([]string, []*models.TaskSession, error) {
	ids := make([]string, 0, len(sessions))
	rescanned := make([]*models.TaskSession, 0, len(sessions))
	rescanner, supported := m.rescanner.(workspaceSourceSessionRescanner)
	if len(sessions) > 0 && !supported {
		return nil, nil, fmt.Errorf("workspace source materialization requires live workspace rescan support")
	}
	for _, session := range sessions {
		if supported {
			if err := rescanner.RescanWorkspaceForSession(ctx, session.ID, root, sourceRoots); err != nil {
				return ids, rescanned, fmt.Errorf("rescan workspace for session %s: %w", session.ID, err)
			}
			rescanned = append(rescanned, session)
		}
		ids = append(ids, session.ID)
	}
	return ids, rescanned, nil
}

func (m *workspaceSourceMaterializer) rollbackHostWorkspaceMaterialization(ctx context.Context, taskID string, state *workspaceSourceMaterializationState, materialization *hostWorkspaceMaterialization, adopted, rescanned []*models.TaskSession, createdInventory []*models.TaskEnvironmentRepo) error {
	rollbackErr := m.restoreRescannedSessionWorkspaces(ctx, rescanned, materialization.oldPath, materialization.priorRoots)
	rollbackErr = errors.Join(rollbackErr, m.restoreSessionWorkspaces(ctx, adopted, materialization.oldPath, materialization.priorRoots))
	if err := m.deleteEnvironmentRepositoryInventory(context.WithoutCancel(ctx), createdInventory); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := rollbackOwnedDirectoryLinks(materialization.linkUndo); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	m.cleanupNewWorktrees(ctx, taskID, materialization.knownWorktree, materialization.newWorktrees...)
	if !materialization.linkRootExisted && materialization.linkRoot != materialization.root {
		_ = os.Remove(materialization.linkRoot)
	}
	if !materialization.rootExisted {
		_ = os.Remove(materialization.root)
	}
	if state.environment.WorkspacePath != materialization.oldPath || state.environment.WorkspaceLayout != materialization.oldLayout {
		state.environment.WorkspacePath = materialization.oldPath
		state.environment.WorkspaceLayout = materialization.oldLayout
		state.environment.UpdatedAt = time.Now().UTC()
		_ = m.repo.UpdateTaskEnvironment(context.WithoutCancel(ctx), state.environment)
	}
	return rollbackErr
}

func rollbackOwnedDirectoryLinks(undo []ownedDirectoryLinkUndo) error {
	var rollbackErr error
	for index := len(undo) - 1; index >= 0; index-- {
		if err := rollbackOwnedDirectoryLink(undo[index]); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func rollbackOwnedDirectoryLink(undo ownedDirectoryLinkUndo) error {
	if undo.PriorTarget == "" {
		if err := worktree.RemoveOwnedDirectoryLink(filepath.Dir(undo.Path), filepath.Base(undo.Path)); err != nil {
			return fmt.Errorf("remove materialized link %q: %w", undo.Path, err)
		}
		return nil
	}
	if err := worktree.RestoreOwnedDirectoryLink(filepath.Dir(undo.Path), filepath.Base(undo.Path), undo.PriorTarget); err != nil {
		return fmt.Errorf("restore repointed link %q: %w", undo.Path, err)
	}
	return nil
}

func (m *workspaceSourceMaterializer) cleanupNewWorktrees(ctx context.Context, taskID string, known map[string]bool, additional ...*worktree.Worktree) {
	worktrees, err := m.worktreeMgr.GetAllByTaskID(context.WithoutCancel(ctx), taskID)
	if err != nil {
		m.logger.Warn("list worktrees for workspace rollback", zap.String("task_id", taskID), zap.Error(err))
		worktrees = nil
	}
	worktrees = append(worktrees, additional...)
	created := make([]*worktree.Worktree, 0, len(worktrees))
	seen := make(map[string]struct{}, len(worktrees))
	for _, wt := range worktrees {
		if wt != nil && !known[wt.ID] {
			if wt.ID != "" {
				if _, found := seen[wt.ID]; found {
					continue
				}
				seen[wt.ID] = struct{}{}
			}
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

func (m *workspaceSourceMaterializer) restoreRescannedSessionWorkspaces(ctx context.Context, sessions []*models.TaskSession, workspacePath string, sourceRoots []string) error {
	if len(sessions) == 0 {
		return nil
	}
	rescanner, supported := m.rescanner.(workspaceSourceSessionRescanner)
	if !supported {
		return errors.New("restore of rescanned workspaces requires live workspace rescan support")
	}
	rollbackCtx := context.WithoutCancel(ctx)
	var rollbackErr error
	for index := len(sessions) - 1; index >= 0; index-- {
		session := sessions[index]
		if err := rescanner.RescanWorkspaceForSession(rollbackCtx, session.ID, workspacePath, sourceRoots); err != nil {
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
		if taskRepository == nil {
			continue
		}
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
		if folder == nil {
			continue
		}
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

func (m *workspaceSourceMaterializer) ensureHostRepositoryPaths(
	ctx context.Context, taskID string, state *workspaceSourceMaterializationState,
) error {
	sessionID := workspaceSourceCredentialSessionID(state.sessions)
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
		if sessionID == "" {
			return fmt.Errorf("active task session is unavailable for repository %q", repository.Name)
		}
		path, err := m.hostCloner.EnsureRepositoryClonedForSession(ctx, taskID, sessionID, repository)
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

func workspaceSourceCredentialSessionID(sessions []*models.TaskSession) string {
	for _, session := range sessions {
		if session != nil && session.IsPrimary && session.ID != "" {
			return session.ID
		}
	}
	for _, session := range sessions {
		if session != nil && session.ID != "" {
			return session.ID
		}
	}
	return ""
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
