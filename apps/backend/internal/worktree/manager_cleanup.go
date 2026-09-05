package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/system/storage"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func (m *Manager) PruneQuarantinedWorkspace(ctx context.Context, entry storage.QuarantineEntry) error {
	worktrees, err := m.GetAllByTaskID(ctx, entry.TaskID)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{})
	var errs []error
	for _, wt := range worktrees {
		if wt == nil || wt.RepositoryPath == "" || wt.Path == "" {
			continue
		}
		key := wt.RepositoryPath + "\x00" + filepath.Clean(wt.Path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := m.pruneQuarantinedWorktree(ctx, wt); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) pruneQuarantinedWorktree(ctx context.Context, wt *Worktree) error {
	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(wt.RepositoryPath)
	}()

	cmd := newGitCommand(ctx, "worktree", "remove", "--force", wt.Path)
	cmd.Dir = wt.RepositoryPath
	if _, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		present, inspectErr := worktreeRegistrationExists(ctx, wt.RepositoryPath, wt.Path)
		if inspectErr != nil {
			return fmt.Errorf("verify worktree registration for %s: %w", wt.Path, inspectErr)
		}
		if present {
			return fmt.Errorf("remove worktree registration for %s: %w", wt.Path, err)
		}
	}
	return nil
}

func worktreeRegistrationExists(ctx context.Context, repoPath, worktreePath string) (bool, error) {
	cmd := newGitCommand(ctx, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return false, err
	}
	want := filepath.Clean(worktreePath)
	for _, field := range strings.Split(string(output), "\x00") {
		if path, ok := strings.CutPrefix(field, "worktree "); ok && filepath.Clean(path) == want {
			return true, nil
		}
	}
	return false, nil
}

type worktreeRegistrationOwnership uint8

const (
	worktreeRegistrationAbsent worktreeRegistrationOwnership = iota
	worktreeRegistrationOwned
	worktreeRegistrationCompeting
)

type worktreeRegistration struct {
	path   string
	head   string
	branch string
}

type worktreeRegistrationOwnershipOptions struct {
	allowAnyBranchAtPath bool
}

func inspectWorktreeRegistrationOwnership(
	ctx context.Context, repoPath, worktreePath, branchRef, headOID string,
) (worktreeRegistrationOwnership, error) {
	return inspectWorktreeRegistrationOwnershipWithOptions(
		ctx, repoPath, worktreePath, branchRef, headOID,
		worktreeRegistrationOwnershipOptions{},
	)
}

func inspectWorktreeRegistrationOwnershipWithOptions(
	ctx context.Context, repoPath, worktreePath, branchRef, headOID string,
	options worktreeRegistrationOwnershipOptions,
) (worktreeRegistrationOwnership, error) {
	cmd := newGitCommand(ctx, "worktree", "list", "--porcelain", "-z")
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return worktreeRegistrationAbsent, err
	}
	wantPath, err := normalizedWorktreeTargetPath(worktreePath)
	if err != nil {
		return worktreeRegistrationAbsent, err
	}
	return classifyWorktreeRegistrationOwnership(
		parseWorktreeRegistrations(string(output)), wantPath, branchRef, headOID, options,
	)
}

func parseWorktreeRegistrations(output string) []worktreeRegistration {
	var registrations []worktreeRegistration
	current := -1
	for _, field := range strings.Split(output, "\x00") {
		switch {
		case strings.HasPrefix(field, "worktree "):
			registrations = append(registrations, worktreeRegistration{
				path: strings.TrimPrefix(field, "worktree "),
			})
			current = len(registrations) - 1
		case current >= 0 && strings.HasPrefix(field, "HEAD "):
			registrations[current].head = strings.TrimPrefix(field, "HEAD ")
		case current >= 0 && strings.HasPrefix(field, "branch "):
			registrations[current].branch = strings.TrimPrefix(field, "branch ")
		}
	}
	return registrations
}

func classifyWorktreeRegistrationOwnership(
	registrations []worktreeRegistration, wantPath, branchRef, headOID string,
	options ...worktreeRegistrationOwnershipOptions,
) (worktreeRegistrationOwnership, error) {
	ownershipOptions := worktreeRegistrationOwnershipOptions{}
	if len(options) > 0 {
		ownershipOptions = options[0]
	}
	var exactTarget, branchElsewhere, targetClaimed bool
	for _, registration := range registrations {
		if registration.path == "" {
			continue
		}
		currentPath, err := normalizedWorktreeTargetPath(registration.path)
		if err != nil {
			return worktreeRegistrationAbsent, err
		}
		if currentPath != wantPath {
			if branchRef != "" && registration.branch == branchRef {
				branchElsewhere = true
			}
			continue
		}
		matchesBranch := registration.branch == branchRef
		if branchRef == "" && ownershipOptions.allowAnyBranchAtPath {
			matchesBranch = true
		}
		matchesHead := registration.head == headOID
		if matchesBranch && matchesHead && !exactTarget {
			exactTarget = true
			continue
		}
		targetClaimed = true
	}
	if exactTarget && !branchElsewhere && !targetClaimed {
		return worktreeRegistrationOwned, nil
	}
	if branchElsewhere || targetClaimed {
		return worktreeRegistrationCompeting, nil
	}
	return worktreeRegistrationAbsent, nil
}

// RemoveByID removes a specific worktree by its ID and optionally its branch.
func (m *Manager) RemoveByID(ctx context.Context, worktreeID string, removeBranch bool) error {
	wt, err := m.GetByID(ctx, worktreeID)
	if err != nil {
		return err
	}
	m.enrichCleanupWorktreeFromCache(wt)
	return m.removeWorktree(ctx, wt, removeBranch)
}

func (m *Manager) enrichCleanupWorktreeFromCache(wt *Worktree) {
	if wt == nil || (wt.RepositoryPath != "" && wt.BaseBranch != "" && wt.CleanupHeadOID != "") {
		return
	}
	m.mu.RLock()
	cached := m.worktrees[cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug)]
	if cached == nil {
		for _, candidate := range m.worktrees {
			if candidate != nil && candidate.ID == wt.ID {
				cached = candidate
				break
			}
		}
	}
	m.mu.RUnlock()
	if cached == nil || cached.ID != wt.ID {
		return
	}
	if wt.RepositoryPath == "" {
		wt.RepositoryPath = cached.RepositoryPath
	}
	if wt.BaseBranch == "" {
		wt.BaseBranch = cached.BaseBranch
	}
	if wt.CleanupHeadOID == "" {
		wt.CleanupHeadOID = cached.CleanupHeadOID
	}
}

// CaptureCleanupHeadOIDs records the checkout commit for each worktree before
// a durable task cleanup job is persisted. Cleanup later compares this value
// with the live branch and registration so a replacement checkout cannot be
// mistaken for the original task workspace.
func (m *Manager) CaptureCleanupHeadOIDs(ctx context.Context, worktrees []*Worktree) (map[string]string, error) {
	identities := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil || wt.ID == "" {
			continue
		}
		m.enrichCleanupWorktreeFromCache(wt)
		if strings.TrimSpace(wt.RepositoryPath) == "" || strings.TrimSpace(wt.Path) == "" {
			// Older environment rows can outlive their repository/session rows.
			// Keep them in the durable snapshot, but let the later cleanup audit
			// fail closed instead of rejecting the task mutation itself.
			continue
		}
		identityPath := wt.Path
		info, statErr := os.Stat(wt.Path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, statErr)
		}
		if statErr != nil || !info.IsDir() {
			if wt.Branch == "" {
				continue
			}
			identityPath = wt.RepositoryPath
		}
		args := []string{"rev-parse", "--verify", "HEAD^{commit}"}
		if identityPath == wt.RepositoryPath {
			args = []string{"rev-parse", "--verify", "refs/heads/" + wt.Branch + "^{commit}"}
		}
		output, err := m.runBoundedGitInspect(ctx, identityPath, args...)
		if err != nil {
			return nil, fmt.Errorf("capture cleanup identity for %s: %w", wt.ID, err)
		}
		oid := strings.TrimSpace(output)
		if oid == "" {
			return nil, fmt.Errorf("capture cleanup identity for %s returned an empty commit", wt.ID)
		}
		identities[wt.ID] = oid
	}
	return identities, nil
}

// removeWorktree performs the actual removal of a worktree.
func (m *Manager) removeWorktree(ctx context.Context, wt *Worktree, removeBranch bool) error {
	if wt == nil {
		return errors.New("worktree cleanup requires a worktree")
	}
	// Cleanup runs after the inventory snapshot has been selected. Keep a
	// private copy so a later session/path projection update cannot redirect
	// the destructive operation or its reference release.
	worktreeSnapshot := *wt
	wt = &worktreeSnapshot
	if strings.TrimSpace(wt.RepositoryPath) == "" || strings.TrimSpace(wt.Path) == "" {
		return errors.New("worktree cleanup requires repository and worktree paths")
	}
	// Serialize cleanup with create/recreate operations that target the same
	// path. The path lock is acquired before the repository lock to match the
	// creation order and avoid a lock-order deadlock.
	releasePath, err := acquireWorktreeTargetPath(ctx, wt.Path)
	if err != nil {
		return fmt.Errorf("lock worktree cleanup path: %w", err)
	}
	defer releasePath()

	// Get repository lock
	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(wt.RepositoryPath)
	}()
	// CountActiveWorktreeReferences already counts only sessions of OTHER
	// tasks referencing the owning environment. No exclusions are passed:
	// the worktree record returned by GetWorktreeByID carries an arbitrary
	// session of that environment, and excluding it could hide a borrower
	// and authorize deletion of a workspace another task still holds.
	activeReferences, err := m.CountActiveWorktreeReferences(ctx, wt.ID, nil)
	if err != nil {
		return fmt.Errorf("count active references for worktree %s: %w", wt.ID, err)
	}
	if activeReferences > 0 {
		// The worktree is owned by a task environment that another task's
		// session still references. The single environment-repository row is
		// the only record, so there is no per-session reference to release —
		// leave the row and directory untouched.
		m.logger.Info("preserved worktree still referenced by another task session",
			zap.String("worktree_id", wt.ID),
			zap.String("session_id", wt.SessionID),
			zap.Int("non_deleted_references", activeReferences))
		return nil
	}

	audit, err := m.auditWorktreeCleanup(ctx, wt, removeBranch)
	if err != nil {
		return fmt.Errorf("audit worktree cleanup %s: %w", wt.ID, err)
	}
	defer func() {
		if audit.pathHandle != nil {
			_ = audit.pathHandle.Close()
		}
	}()

	// Run the repository script only after the path, Git registration, checkout
	// contents, and branch identity pass the audit. A script such as `git clean`
	// must never erase changes before the audit can reject the cleanup.
	m.runWorktreeCleanupScript(ctx, wt)

	if err := m.completeAuditedWorktreeCleanup(ctx, wt, audit, removeBranch); err != nil {
		return fmt.Errorf("complete worktree cleanup %s: %w", wt.ID, err)
	}

	// Update store
	if m.store != nil && wt.SessionID != "" {
		if err := m.ReleaseWorktreeReference(ctx, wt); err != nil {
			return fmt.Errorf("release worktree reference %s: %w", wt.ID, err)
		}
	}

	// Update cache: delete the (session, repo) entry. Removing a worktree only
	// affects its own repo; siblings on other repos must remain cached.
	m.mu.Lock()
	if wt.SessionID != "" {
		delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug))
	}
	m.mu.Unlock()

	m.logger.Info("removed worktree",
		zap.String("task_id", wt.TaskID),
		zap.String("worktree_id", wt.ID),
		zap.String("path", wt.Path),
		zap.Bool("branch_removed", removeBranch))

	return nil
}

// ReleaseWorktreeReference marks one session's association deleted without
// removing the shared directory or branch.
func (m *Manager) ReleaseWorktreeReference(ctx context.Context, wt *Worktree) error {
	if wt == nil || wt.SessionID == "" {
		return fmt.Errorf("session ID is required to release worktree reference")
	}
	now := time.Now().UTC()
	wt.Status = StatusDeleted
	wt.DeletedAt = &now
	wt.UpdatedAt = now
	if err := m.store.UpdateWorktree(ctx, wt); err != nil && !errors.Is(err, ErrWorktreeNotFound) {
		return err
	}
	m.mu.Lock()
	delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID, wt.BranchSlug))
	m.mu.Unlock()
	return nil
}

// runWorktreeSetupScript runs the repository's setup script in the freshly
// created worktree. Setup script failures are non-fatal: the worktree is kept
// and the failure is recorded on wt as a warning (surfaced by the env preparer)
// so the agent can still launch. This mirrors the task-level setup script
// behavior in runSetupScriptStep — a broken setup script must not block the task.
func (m *Manager) runWorktreeSetupScript(ctx context.Context, wt *Worktree, profileEnv map[string]string) {
	if m.scriptMsgHandler == nil || m.repoProvider == nil {
		return
	}
	if wt.RepositoryID == "" {
		// Nothing to set up without a linked repository; upstream may not
		// always populate this field.
		return
	}
	repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		m.logger.Warn("failed to fetch repository for setup script",
			zap.String("repository_id", wt.RepositoryID),
			zap.Error(err))
		return
	}
	if strings.TrimSpace(repo.SetupScript) == "" {
		return
	}
	m.logger.Info("executing setup script for worktree",
		zap.String("worktree_id", wt.ID),
		zap.String("repository_id", wt.RepositoryID))
	scriptReq := ScriptExecutionRequest{
		SessionID:    wt.SessionID,
		TaskID:       wt.TaskID,
		RepositoryID: wt.RepositoryID,
		Script:       repo.SetupScript,
		WorkingDir:   wt.Path,
		ScriptType:   "setup",
		Env:          mergeScriptEnv(profileEnv, m.managedScriptEnvironment(ctx)),
	}
	if err := m.scriptMsgHandler.ExecuteSetupScript(ctx, scriptReq); err != nil {
		// Non-fatal: keep the worktree and surface a warning. The detailed
		// script output is already streamed to the script_execution chat
		// message; here we only record a concise, user-facing warning.
		m.logger.Warn("setup script failed, continuing without it",
			zap.String("worktree_id", wt.ID),
			zap.Error(err))
		wt.SetupScriptWarning = "Repository setup script failed; the worktree was created without it. Fix the setup script and re-run if needed."
		wt.SetupScriptWarningDetail = err.Error()
		return
	}
	m.logger.Info("setup script completed successfully", zap.String("worktree_id", wt.ID))
}

// runWorktreeCleanupScript executes the repository cleanup script for a worktree before removal.
func (m *Manager) runWorktreeCleanupScript(ctx context.Context, wt *Worktree) {
	if m.scriptMsgHandler == nil || m.repoProvider == nil {
		return
	}
	repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		m.logger.Warn("failed to fetch repository for cleanup script",
			zap.String("repository_id", wt.RepositoryID),
			zap.Error(err))
		return
	}
	if strings.TrimSpace(repo.CleanupScript) == "" {
		return
	}
	m.logger.Info("executing cleanup script for worktree",
		zap.String("worktree_id", wt.ID),
		zap.String("repository_id", wt.RepositoryID))
	scriptReq := ScriptExecutionRequest{
		SessionID:    wt.SessionID,
		TaskID:       wt.TaskID,
		RepositoryID: wt.RepositoryID,
		Script:       repo.CleanupScript,
		WorkingDir:   wt.Path,
		ScriptType:   "cleanup",
		Env:          m.managedScriptEnvironment(ctx),
	}
	if err := m.scriptMsgHandler.ExecuteCleanupScript(ctx, scriptReq); err != nil {
		m.logger.Warn("cleanup script failed, proceeding with deletion",
			zap.String("worktree_id", wt.ID),
			zap.Error(err))
	} else {
		m.logger.Info("cleanup script completed successfully",
			zap.String("worktree_id", wt.ID))
	}
}

// mergeScriptEnv combines resolved executor-profile env vars with the
// install-managed script environment. Managed values (e.g. GOCACHE) take
// precedence so the managed build cache is never clobbered by a profile entry.
// Returns nil when both inputs are empty so callers preserve the "inherit the
// full process environment" behavior of scriptProcessEnvironment(nil).
func mergeScriptEnv(profileEnv, managed map[string]string) map[string]string {
	if len(profileEnv) == 0 && len(managed) == 0 {
		return nil
	}
	merged := make(map[string]string, len(profileEnv))
	for k, v := range profileEnv {
		merged[k] = v
	}
	for k, v := range managed {
		merged[k] = v
	}
	return merged
}

func (m *Manager) managedScriptEnvironment(ctx context.Context) map[string]string {
	if m.scriptEnvProvider == nil {
		return nil
	}
	env, err := m.scriptEnvProvider.ExecutionEnvironment(ctx)
	if err != nil {
		m.logger.Warn("failed to resolve managed script environment", zap.Error(err))
		return nil
	}
	cachePath := env["GOCACHE"]
	if cachePath == "" || !filepath.IsAbs(cachePath) {
		return nil
	}
	return map[string]string{"GOCACHE": filepath.Clean(cachePath)}
}

// CleanupWorktrees removes provided worktrees without re-fetching from the store.
func (m *Manager) CleanupWorktrees(ctx context.Context, worktrees []*Worktree) error {
	return m.cleanupWorktrees(ctx, worktrees, true)
}

// CleanupWorktreesPreservingBranches removes provided worktrees while retaining
// their local branch refs for later archive recovery.
func (m *Manager) CleanupWorktreesPreservingBranches(ctx context.Context, worktrees []*Worktree) error {
	return m.cleanupWorktrees(ctx, worktrees, false)
}

func (m *Manager) cleanupWorktrees(ctx context.Context, worktrees []*Worktree, removeBranch bool) error {
	if len(worktrees) == 0 {
		return nil
	}

	var lastErr error
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		// Task-environment inventory rows can describe a repository slot
		// without a materialized physical worktree. Never pass such a row to
		// filesystem cleanup, because its path can be the source checkout.
		if strings.TrimSpace(wt.ID) == "" {
			continue
		}
		m.enrichCleanupWorktreeFromCache(wt)
		if err := m.removeWorktree(ctx, wt, removeBranch); err != nil {
			m.logger.Warn("failed to remove worktree during batch cleanup",
				zap.String("task_id", wt.TaskID),
				zap.String("worktree_id", wt.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

// OnTaskDeleted cleans up all worktrees for a task when it is deleted.
func (m *Manager) OnTaskDeleted(ctx context.Context, taskID string) error {
	// Get all worktrees for this task
	worktrees, err := m.GetAllByTaskID(ctx, taskID)
	if err != nil {
		return err
	}

	return m.CleanupWorktrees(ctx, worktrees)
}

// removeWorktreeDir removes a worktree directory. Audited paths are removed
// through their pinned no-follow handle; un-audited callers use git worktree
// remove with a safe filesystem fallback. After the inner directory is gone it
// tries to rmdir the parent task directory left behind by the nested
// {tasksBase}/{taskDirName}/{repoName} layout (see issue #1266). The rmdir is
// a best-effort no-op if the parent still has siblings or contains
// workspace-scoped content.
func (m *Manager) removeWorktreeDir(
	ctx context.Context,
	worktreePath, repoPath string,
	pathHandles ...storageworkspaces.DirectoryHandle,
) error {
	if len(pathHandles) > 0 && pathHandles[0] != nil {
		// An audited handle pins the original directory identity. Remove through
		// that handle instead of issuing a path-based Git command, which could
		// follow a replacement after an external rename.
		if verifyErr := pathHandles[0].VerifyPath(worktreePath); verifyErr != nil && !errors.Is(verifyErr, os.ErrNotExist) {
			return fmt.Errorf("worktree path changed during cleanup: %w", verifyErr)
		}
		if err := pathHandles[0].RemoveDirectory(ctx); err != nil {
			return fmt.Errorf("remove audited worktree directory: %w", err)
		}
		// The parent cleanup is a separate path-based fallback. Revalidate the
		// audited target immediately before it so a replacement cannot turn an
		// otherwise safe cleanup into recursive deletion of another directory.
		if err := pathHandles[0].VerifyPath(worktreePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("worktree path changed before parent cleanup: %w", err)
		}
		pruneCmd := newGitCommand(ctx, "worktree", "prune")
		pruneCmd.Dir = repoPath
		if err := runGitCmd(ctx, pruneCmd); err != nil {
			m.logger.Debug("git worktree prune failed", zap.Error(err))
		}
		m.tryRemoveEmptyTaskDir(worktreePath)
		return nil
	}

	// First try git worktree remove
	cmd := newGitCommand(ctx, "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoPath
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		m.logger.Debug("git worktree remove failed, falling back to filesystem removal",
			zap.String("output", string(output)),
			zap.Error(err))

		var removeErr error
		if tasksBase, managed := m.cleanupWorktreeTasksBase(worktreePath); managed {
			removeErr = storageworkspaces.RemoveDirectoryNoFollow(ctx, tasksBase, worktreePath)
			if errors.Is(removeErr, os.ErrNotExist) {
				removeErr = nil
			}
		} else {
			removeErr = m.forceRemoveDir(ctx, worktreePath)
		}
		if removeErr != nil {
			return removeErr
		}

		// Prune stale worktree entries
		pruneCmd := newGitCommand(ctx, "worktree", "prune")
		pruneCmd.Dir = repoPath
		if err := runGitCmd(ctx, pruneCmd); err != nil {
			m.logger.Debug("git worktree prune failed", zap.Error(err))
		}
	}
	m.tryRemoveEmptyTaskDir(worktreePath)
	return nil
}

func (m *Manager) cleanupWorktreeTasksBase(worktreePath string) (string, bool) {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil || tasksBase == "" {
		return "", false
	}
	tasksBase = filepath.Clean(tasksBase)
	relativePath, err := filepath.Rel(tasksBase, filepath.Clean(worktreePath))
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", false
	}
	return tasksBase, true
}

// tryRemoveEmptyTaskDir rmdirs the parent of a removed worktree if that
// parent is an immediate child of TasksBasePath (i.e. a per-task container
// directory) and is now empty. Silently skips otherwise.
func (m *Manager) tryRemoveEmptyTaskDir(worktreePath string) {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil || tasksBase == "" {
		return
	}
	// Normalize both sides: ExpandedTasksBasePath returns the configured
	// value verbatim for absolute paths (incl. trailing slashes or doubled
	// separators), while filepath.Dir always yields a cleaned form.
	tasksBase = filepath.Clean(tasksBase)
	parent := filepath.Dir(worktreePath)
	// Guard: only act on direct children of tasksBase. Never touch
	// tasksBase itself or any deeper / unrelated location.
	if filepath.Dir(parent) != tasksBase {
		return
	}
	entries, readErr := os.ReadDir(parent)
	if readErr == nil && len(entries) == 1 && entries[0].Name() == storageworkspaces.OwnershipMarkerFilename && entries[0].Type().IsRegular() {
		if err := os.Remove(filepath.Join(parent, storageworkspaces.OwnershipMarkerFilename)); err != nil && !os.IsNotExist(err) {
			m.logger.Debug("task ownership marker not removed", zap.String("path", parent), zap.Error(err))
			return
		}
	}
	if err := os.Remove(parent); err != nil && !os.IsNotExist(err) {
		m.logger.Debug("task dir not removed (likely non-empty)",
			zap.String("path", parent),
			zap.Error(err))
	}
}

// forceRemoveDir removes a directory, retrying transient filesystem failures.
// Native Windows cleanup intentionally stays within Go's portable filesystem
// APIs. Unix hosts get a final non-shell rm fallback for persistent removal
// failures caused by filesystems that reject os.RemoveAll while allowing rm.
func (m *Manager) forceRemoveDir(ctx context.Context, dir string) error {
	const maxRetries = 3
	const retryDelay = 200 * time.Millisecond
	return m.removeDirWithRetriesAndFallback(
		ctx, dir, maxRetries, retryDelay, os.RemoveAll, forceRemoveDirUnix, isUnixLikeOS(runtime.GOOS),
	)
}

func (m *Manager) removeDirWithRetries(
	ctx context.Context, dir string, maxRetries int, retryDelay time.Duration, removeAll func(string) error,
) error {
	var lastErr error

	for i := range maxRetries {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = removeAll(dir)
		if lastErr == nil {
			return nil
		}
		if i < maxRetries-1 {
			m.logger.Debug("os.RemoveAll failed, retrying",
				zap.String("path", dir),
				zap.Int("attempt", i+1),
				zap.Error(lastErr))
			if err := waitForRetry(ctx, retryDelay); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("remove directory %s after %d attempts: %w", dir, maxRetries, lastErr)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *Manager) removeDirWithRetriesAndFallback(
	ctx context.Context, dir string, maxRetries int, retryDelay time.Duration,
	removeAll func(string) error, fallback func(context.Context, string) error,
	useFallback bool,
) error {
	err := m.removeDirWithRetries(ctx, dir, maxRetries, retryDelay, removeAll)
	if err == nil || !useFallback || ctx.Err() != nil {
		return err
	}
	if fallbackErr := fallback(ctx, dir); fallbackErr != nil {
		return fmt.Errorf("remove directory %s with Unix fallback: %w", dir, errors.Join(err, fallbackErr))
	}
	return nil
}

func forceRemoveDirUnix(ctx context.Context, dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("refusing to remove an empty directory path")
	}
	cmd := exec.CommandContext(ctx, "rm", "-rf", "--", dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rm -rf -- %q: %w: %s", dir, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func isUnixLikeOS(goos string) bool {
	switch goos {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
		return true
	default:
		return false
	}
}
