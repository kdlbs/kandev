package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/system/storage"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/gitbootstrap"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

// ResolveRemoteDefaultBranch returns the default branch advertised by the
// origin remote. It reads the local symref first, then refreshes it once with
// non-interactive Git when the local ref is absent.
func (m *Manager) ResolveRemoteDefaultBranch(ctx context.Context, repoPath string) (string, error) {
	branch, err := readRemoteDefaultBranch(repoPath)
	if err != nil {
		return "", err
	}
	if branch != "" {
		return branch, nil
	}

	output, runErr := m.runBoundedGitInspect(ctx, repoPath, "remote", "set-head", "origin", "--auto")
	if runErr != nil {
		return "", classifyRemoteDefaultError(output, runErr)
	}
	branch, err = readRemoteDefaultBranch(repoPath)
	if err != nil {
		return "", err
	}
	if branch == "" {
		return "", ErrRemoteDefaultUnresolved
	}
	return branch, nil
}

func readRemoteDefaultBranch(repoPath string) (string, error) {
	gitDir, err := gitref.ResolveGitDir(repoPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("resolve git directory: %w", err)
	}
	commonDir := gitref.ResolveCommonGitDir(gitDir)
	path := filepath.Join(commonDir, "refs", "remotes", "origin", "HEAD")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read origin default branch: %w", err)
	}
	line := strings.TrimSpace(string(data))
	const prefix = "ref: refs/remotes/origin/"
	if !strings.HasPrefix(line, prefix) {
		return "", nil
	}
	branch := strings.TrimPrefix(line, prefix)
	if branch == "" || strings.ContainsAny(branch, "\r\n") {
		return "", nil
	}
	return branch, nil
}

func classifyRemoteDefaultError(output string, runErr error) error {
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		return runErr
	}
	lowerOutput := strings.ToLower(output)
	switch {
	case containsAuthFailure(lowerOutput):
		return fmt.Errorf("%w: %s", ErrAuthFailed, strings.TrimSpace(output))
	case strings.Contains(lowerOutput, "could not resolve host"),
		strings.Contains(lowerOutput, "connection timed out"),
		strings.Contains(lowerOutput, "network is unreachable"),
		strings.Contains(lowerOutput, "could not connect"):
		return fmt.Errorf("%w: %s", ErrRemoteDefaultNetwork, strings.TrimSpace(output))
	case isRemoteRefMissingError(fmt.Errorf("%s", output)):
		return fmt.Errorf("%w: %s", ErrRemoteDefaultUnresolved, strings.TrimSpace(output))
	default:
		return ClassifyGitError(output, runErr)
	}
}

// Create creates a new worktree for a session, or returns an existing one.
// Each session gets its own worktree for isolation. Checks by SessionID first,
// then by WorktreeID if provided (for session resumption).
// Only creates a new worktree if none exists for the session.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*Worktree, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.ReuseRequired {
		return m.reuseRequiredWorktree(ctx, req)
	}

	// Reject invalid explicit slugs up-front. If either slug is non-empty but
	// normalizes to "", reuse/path lookup would silently target the wrong
	// worktree or fail later with a less obvious error.
	if req.BranchSlug != "" && SanitizeBranchSlug(req.BranchSlug) == "" {
		return nil, ErrInvalidBranchSlug
	}
	if req.BranchIdentitySlug != "" && SanitizeBranchSlug(req.BranchIdentitySlug) == "" {
		return nil, ErrInvalidBranchSlug
	}

	if wt, handled, err := m.tryReuseExisting(ctx, req); handled {
		if err == nil && wt != nil {
			err = m.configureContributionDestination(ctx, req.RepositoryPath, wt.Path, wt.Branch, req.ContributionDestination)
		}
		return wt, err
	}
	if err := m.refreshRepositoryForMaterialization(ctx, &req); err != nil {
		return nil, err
	}

	// Check repository is a git repo
	if !m.isGitRepo(req.RepositoryPath) {
		return nil, ErrRepoNotGit
	}

	// Get repository lock for safe concurrent access
	repoLock := m.getRepoLock(req.RepositoryPath)
	repoLock.Lock()
	lockAcquired := time.Now()
	defer func() {
		repoLock.Unlock()
		m.logger.Debug("released worktree repo lock",
			zap.String("repository_path", req.RepositoryPath),
			zap.Duration("held", time.Since(lockAcquired)))
		m.releaseRepoLock(req.RepositoryPath)
	}()

	baseRef, fallbackWarning, fallbackDetail, err := m.resolveBaseRefWithFallback(ctx, &req)
	if err != nil {
		return nil, err
	}

	// Worktrees are always placed under ~/.kandev/tasks/{taskDir}/{repo}/.
	// Callers must populate TaskDirName and RepoName; the legacy flat layout
	// has been removed, so a missing field is a programming error.
	if req.TaskDirName == "" || req.RepoName == "" {
		return nil, ErrTaskDirRequired
	}
	wt, err := m.createInTaskDir(ctx, req, baseRef, fallbackWarning, fallbackDetail)
	if err != nil {
		return nil, err
	}
	if err := m.configureContributionDestination(ctx, req.RepositoryPath, wt.Path, wt.Branch, req.ContributionDestination); err != nil {
		return nil, err
	}
	return wt, nil
}

// refreshRepositoryForMaterialization runs a provider-authenticated refresh
// only when Create is going to materialize or recreate a worktree. Reuse is
// checked by the caller first, so a valid existing worktree never refreshes
// origin or incurs provider authentication work.
func (m *Manager) refreshRepositoryForMaterialization(ctx context.Context, req *CreateRequest) error {
	if req == nil || req.RemoteSyncHandled {
		return nil
	}
	if req.RefreshRepositoryWithState != nil {
		state, err := req.RefreshRepositoryWithState(ctx)
		if err != nil {
			return err
		}
		req.RemoteRefState = state
		req.RemoteSyncHandled = true
		req.PullBeforeWorktree = false
		return nil
	}
	if req.RefreshRepository == nil {
		return nil
	}
	if err := req.RefreshRepository(ctx); err != nil {
		return err
	}
	req.RemoteSyncHandled = true
	req.PullBeforeWorktree = false
	return nil
}

func (m *Manager) ensureEmptyRemoteBaseline(ctx context.Context, req *CreateRequest) (string, error) {
	if req == nil || req.RemoteRefState != repoclone.RemoteRefStateEmpty {
		return "", nil
	}
	baseline, err := gitbootstrap.Ensure(ctx, req.RepositoryPath, req.BaseBranch)
	if errors.Is(err, gitbootstrap.ErrBaselineConflict) {
		req.RemoteRefState = repoclone.RemoteRefStateHasRefs
		req.RemoteSyncHandled = false
		baseBranch := strings.TrimPrefix(strings.TrimSpace(req.BaseBranch), "origin/")
		return strings.TrimPrefix(baseBranch, "refs/heads/"), nil
	}
	if err != nil {
		return "", fmt.Errorf("seed empty-remote local baseline: %w", err)
	}
	req.RemoteSyncHandled = true
	req.PullBeforeWorktree = false
	return baseline.BaseRef, nil
}

// reuseRequiredWorktree resolves the exact canonical worktree for an
// additional session. It intentionally performs no Git operation and does
// not run contribution setup or repository scripts: another active session
// may be using this checkout with uncommitted changes.
func (m *Manager) reuseRequiredWorktree(ctx context.Context, req CreateRequest) (*Worktree, error) {
	if req.WorktreeID == "" || req.TaskEnvironmentID == "" {
		return nil, ErrReuseWorktreeUnavailable
	}
	requestedBranchSlug := requestBranchIdentitySlug(req)
	if (req.BranchSlug != "" || req.BranchIdentitySlug != "") && requestedBranchSlug == "" {
		return nil, ErrReuseWorktreeUnavailable
	}
	wt, err := m.GetByID(ctx, req.WorktreeID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReuseWorktreeUnavailable, err)
	}
	if wt == nil || wt.Status != StatusActive ||
		wt.RepositoryID != req.RepositoryID ||
		wt.TaskEnvironmentID != req.TaskEnvironmentID ||
		(requestedBranchSlug != "" && SanitizeBranchSlug(wt.BranchSlug) != requestedBranchSlug) {
		return nil, ErrReuseWorktreeUnavailable
	}
	handle, valid, err := m.openReusableWorktreePath(wt.Path, wt)
	if err != nil {
		return nil, err
	}
	if handle != nil {
		defer func() { _ = handle.Close() }()
	}
	if !valid {
		return nil, ErrReuseWorktreeUnavailable
	}
	return wt, nil
}

// tryReuseExisting looks for an existing worktree to reuse, recreating it if
// the on-disk directory is missing. Returns (wt, true, err) when fully handled
// (caller should return immediately). Returns (nil, false, nil) when no reuse
// candidate matched and the caller should proceed to create a new worktree.
func (m *Manager) tryReuseExisting(ctx context.Context, req CreateRequest) (*Worktree, bool, error) {
	// First, check if a worktree already exists for this
	// (session, repository, branchSlug) triple. Multi-repo + multi-branch
	// sessions can host multiple worktrees concurrently, so we must scope the
	// lookup by RepositoryID AND BranchSlug — otherwise the second branch's
	// Create would return the first branch's worktree, silently collapsing
	// two distinct worktrees into one on-disk directory.
	reuseSlug := requestBranchIdentitySlug(req)
	if req.SessionID != "" {
		existing, err := m.GetBySessionAndRepo(ctx, req.SessionID, req.RepositoryID, reuseSlug)
		if err == nil && existing != nil {
			handle, valid, err := m.openReusableWorktreePath(existing.Path, existing)
			if err != nil {
				return nil, true, err
			}
			if handle != nil {
				defer func() { _ = handle.Close() }()
			}
			if valid {
				if err := m.validateReusableContribution(ctx, req, existing); err != nil {
					return nil, true, err
				}
				m.logger.Debug("reusing existing worktree by session+repo",
					zap.String("worktree_id", existing.ID),
					zap.String("session_id", req.SessionID),
					zap.String("repository_id", req.RepositoryID),
					zap.String("task_id", req.TaskID),
					zap.String("path", existing.Path))
				return existing, true, nil
			}
			m.logger.Warn("worktree directory invalid, recreating",
				zap.String("worktree_id", existing.ID),
				zap.String("session_id", req.SessionID),
				zap.String("repository_id", req.RepositoryID),
				zap.String("task_id", req.TaskID))
			wt, err := m.recreate(ctx, existing, req)
			return wt, true, err
		}
	}

	// If WorktreeID is provided, try to reuse that specific worktree (session resumption)
	if req.WorktreeID != "" {
		existing, err := m.GetByID(ctx, req.WorktreeID)
		if err == nil && existing != nil {
			handle, valid, err := m.openReusableWorktreePath(existing.Path, existing)
			if err != nil {
				return nil, true, err
			}
			if handle != nil {
				defer func() { _ = handle.Close() }()
			}
			if valid {
				if err := m.validateReusableContribution(ctx, req, existing); err != nil {
					return nil, true, err
				}
				m.logger.Info("reusing existing worktree by ID",
					zap.String("worktree_id", req.WorktreeID),
					zap.String("session_id", req.SessionID),
					zap.String("task_id", req.TaskID),
					zap.String("path", existing.Path))
				return existing, true, nil
			}
			m.logger.Warn("worktree directory invalid, recreating",
				zap.String("worktree_id", req.WorktreeID),
				zap.String("session_id", req.SessionID),
				zap.String("task_id", req.TaskID))
			wt, err := m.recreate(ctx, existing, req)
			return wt, true, err
		}
		// WorktreeID provided but not found - fall through to create new
		m.logger.Warn("worktree ID not found, creating new worktree",
			zap.String("worktree_id", req.WorktreeID),
			zap.String("session_id", req.SessionID),
			zap.String("task_id", req.TaskID))
	}

	return nil, false, nil
}

// validateWorktreePathSafe opens a persisted worktree path through no-follow
// directory handles. The handle is returned to the caller so the same
// directory identity is used for ownership and reuse decisions. Missing paths
// remain eligible for the normal recreate flow, while symlinked components
// fail closed.
func (m *Manager) validateWorktreePathSafe(worktreePath string) (storageworkspaces.DirectoryHandle, error) {
	if worktreePath == "" {
		return nil, nil
	}
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return nil, fmt.Errorf("resolve tasks base path: %w", err)
	}
	tasksBase = filepath.Clean(tasksBase)
	// Paths outside the tasks base use the legacy flat layout. They do not have
	// a task-root ownership marker, but still get a no-follow handle so reuse
	// cannot follow a replacement link while checking the checkout.
	relativePath, err := filepath.Rel(tasksBase, worktreePath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		// Legacy paths are not below tasksBase, but they still get a pinned
		// no-follow handle so IsValid cannot be redirected by a symlink race.
		return m.openNoFollowWorktreePath(worktreePath)
	}
	if _, err := os.Lstat(tasksBase); errors.Is(err, os.ErrNotExist) {
		// A removed task base has no components to follow. Normal reuse will
		// recreate the persisted managed root before adding the worktree;
		// attach-only reuse still rejects the missing checkout via IsValid.
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect tasks base path: %w", err)
	}
	return m.openNoFollowWorktreePath(worktreePath)
}

func (m *Manager) openNoFollowWorktreePath(worktreePath string) (storageworkspaces.DirectoryHandle, error) {
	cleanPath := filepath.Clean(worktreePath)
	if !filepath.IsAbs(cleanPath) {
		return nil, nil
	}
	handle, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(cleanPath), cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("unsafe worktree path: %w", err)
	}
	if err := handle.VerifyPath(cleanPath); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("worktree path changed during validation: %w", err)
	}
	return handle, nil
}

func (m *Manager) openReusableWorktreePath(worktreePath string, wt *Worktree) (storageworkspaces.DirectoryHandle, bool, error) {
	handle, err := m.validateWorktreePathSafe(worktreePath)
	if err != nil {
		return nil, false, err
	}
	if err := m.validateExistingWorktreePathOwner(worktreePath, wt); err != nil {
		if handle != nil {
			_ = handle.Close()
		}
		return nil, false, err
	}
	if handle == nil || !handle.IsValidWorktree() {
		if handle != nil {
			_ = handle.Close()
		}
		return nil, false, nil
	}
	if err := handle.VerifyPath(filepath.Clean(worktreePath)); err != nil {
		_ = handle.Close()
		return nil, false, fmt.Errorf("worktree path changed during reuse validation: %w", err)
	}
	return handle, true, nil
}

func worktreeOwnershipIdentity(wt *Worktree) (string, bool) {
	if wt != nil && wt.TaskDirName != "" {
		return wt.TaskDirName, true
	}
	if wt != nil {
		// Older in-memory and persisted records may not project TaskDirName.
		// Keep their existing ownership check until they are refreshed.
		return wt.TaskID, false
	}
	return "", false
}

// validateExistingWorktreePathOwner rejects a stale record whose stored path
// now lies under another task's marked root. It deliberately runs before both
// reuse and recreate: recreate removes the recorded path before adding a new
// worktree, so checking only the reusable path could delete a live checkout.
// Paths outside Kandev's task root and legacy unmarked task roots remain
// eligible for their existing compatibility behavior.
func (m *Manager) validateExistingWorktreePathOwner(worktreePath string, wt *Worktree) error {
	if worktreePath == "" {
		return nil
	}
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return fmt.Errorf("resolve tasks base path: %w", err)
	}
	// Normalize: ExpandedTasksBasePath returns the configured value verbatim
	// (incl. trailing slashes or doubled separators), while filepath.Dir
	// always yields a cleaned form. Without this, the path computation below
	// may miscompute the task-root segment.
	tasksBase = filepath.Clean(tasksBase)
	relativePath, err := filepath.Rel(tasksBase, worktreePath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return nil
	}
	recordIdentity, stableTaskDirName := worktreeOwnershipIdentity(wt)
	if recordIdentity == "" {
		return nil
	}
	// The task-root ownership marker always lives at the first path segment
	// below tasksBase (i.e. <tasksBase>/<taskDirName>/). Walking all levels
	// up from worktreePath is wrong: a descendant repository that legitimately
	// contains its own .kandev-workspace.json (e.g. a nested project) would be
	// mistaken for the task-root marker. Derive the task root from the
	// relative path and inspect only that one directory.
	taskRoot := filepath.Join(tasksBase, strings.SplitN(relativePath, string(filepath.Separator), 2)[0])
	owner, found, err := storageworkspaces.ReadOwnershipMarker(taskRoot)
	if err != nil {
		return fmt.Errorf("inspect worktree task root ownership: %w", err)
	}
	// Validate against the environment's stable task directory name, not the
	// mutable task ID. Shared environments can transfer ownership while their
	// physical task root and marker remain unchanged. Older records without the
	// projected directory name retain the task-ID compatibility check.
	ownerMatches := owner.TaskDirName == recordIdentity
	if !stableTaskDirName {
		ownerMatches = owner.TaskID == recordIdentity
	}
	if found && !ownerMatches {
		return fmt.Errorf("%w: path %q belongs to task %q", ErrWorktreePathOwnedByAnotherTask, worktreePath, owner.TaskID)
	}
	return nil
}

func (m *Manager) validateReusableContribution(ctx context.Context, req CreateRequest, existing *Worktree) error {
	if req.RemoteContribution == nil {
		return nil
	}
	remoteName, remoteRef, err := m.materializeRemoteContribution(ctx, req.RepositoryPath, req.RemoteContribution)
	if err != nil {
		return err
	}
	if err := m.validateContributionAncestor(ctx, existing.Path, req.RemoteContribution.HeadSHA, "HEAD"); err != nil {
		return fmt.Errorf("existing contribution worktree is not based on the validated source: %w", err)
	}
	if err := m.setUpstreamIfExistsRemote(ctx, existing.Path, existing.Branch, remoteName, req.RemoteContribution.HeadBranch); err != nil {
		return err
	}
	_ = remoteRef
	return nil
}

func requestBranchIdentitySlug(req CreateRequest) string {
	if req.BranchIdentitySlug != "" {
		return SanitizeBranchSlug(req.BranchIdentitySlug)
	}
	return SanitizeBranchSlug(req.BranchSlug)
}

// resolveBaseRefWithFallback resolves the base ref for a new worktree, optionally
// pulling from origin first, and falling back to req.FallbackBaseBranch when the
// requested base branch is missing. When the fallback path is taken, req.BaseBranch
// is updated to reflect the resolved name and a non-empty warning/detail pair is
// returned for surfacing on the resulting worktree record.
func (m *Manager) resolveBaseRefWithFallback(ctx context.Context, req *CreateRequest) (baseRef, warning, detail string, err error) {
	baseRef = req.BaseBranch
	switch {
	case req.RemoteRefState == repoclone.RemoteRefStateEmpty:
		baselineRef, baselineErr := m.ensureEmptyRemoteBaseline(ctx, req)
		if baselineErr != nil {
			return "", "", "", baselineErr
		}
		switch {
		case baselineRef != "":
			baseRef = baselineRef
		case req.RemoteSyncHandled:
			baseRef, err = m.preferRefreshedRemoteRef(ctx, req.RepositoryPath, req.BaseBranch)
		case req.PullBeforeWorktree:
			baseRef, err = m.pullBaseBranch(ctx, req.RepositoryPath, req.BaseBranch, req.OnSyncProgress)
		}
	case req.RemoteSyncHandled:
		baseRef, err = m.preferRefreshedRemoteRef(ctx, req.RepositoryPath, req.BaseBranch)
	case req.PullBeforeWorktree:
		baseRef, err = m.pullBaseBranch(ctx, req.RepositoryPath, req.BaseBranch, req.OnSyncProgress)
	}
	if err != nil {
		return "", "", "", err
	}

	baseExists, baseErr := m.branchExists(ctx, req.RepositoryPath, baseRef)
	if baseErr != nil {
		// Could not determine existence (timeout / fs stall). Surface the
		// real cause instead of pretending the branch is missing.
		return "", "", "", fmt.Errorf("could not verify base branch %q: %w", baseRef, baseErr)
	}
	if baseExists {
		return baseRef, "", "", nil
	}

	fallback := strings.TrimSpace(req.FallbackBaseBranch)
	resolvedFallback := ""
	if fallback != "" && fallback != baseRef {
		resolvedFallback, err = m.resolveFallbackRef(ctx, req, fallback)
		if err != nil {
			return "", "", "", err
		}
		fallbackExists, fallbackErr := m.branchExists(ctx, req.RepositoryPath, resolvedFallback)
		if fallbackErr != nil {
			return "", "", "", fmt.Errorf("could not verify fallback base branch %q: %w", resolvedFallback, fallbackErr)
		}
		if fallbackExists {
			return m.finishBaseFallback(req, baseRef, fallback, resolvedFallback)
		}
	}

	// The persisted repository default can be stale or absent. Resolve the
	// live remote default only after the requested branch and stored fallback
	// have both failed.
	liveFallback, liveErr := m.ResolveRemoteDefaultBranch(ctx, req.RepositoryPath)
	if liveErr != nil {
		if errors.Is(liveErr, ErrRemoteDefaultUnresolved) {
			return "", "", "", fmt.Errorf("%w: %s (fallback %q also not found)", ErrInvalidBaseBranch, baseRef, fallback)
		}
		return "", "", "", liveErr
	}
	if liveFallback == baseRef {
		return "", "", "", fmt.Errorf("%w: %s", ErrInvalidBaseBranch, baseRef)
	}
	resolvedFallback, err = m.resolveFallbackRef(ctx, req, liveFallback)
	if err != nil {
		return "", "", "", err
	}
	fallbackExists, fallbackErr := m.branchExists(ctx, req.RepositoryPath, resolvedFallback)
	if fallbackErr != nil {
		return "", "", "", fmt.Errorf("could not verify live fallback base branch %q: %w", resolvedFallback, fallbackErr)
	}
	if !fallbackExists {
		return "", "", "", fmt.Errorf("%w: %s (fallback %q also not found)", ErrInvalidBaseBranch, baseRef, liveFallback)
	}
	fallback = liveFallback
	return m.finishBaseFallback(req, baseRef, fallback, resolvedFallback)
}

func (m *Manager) resolveFallbackRef(ctx context.Context, req *CreateRequest, fallback string) (string, error) {
	if req.RemoteSyncHandled {
		return m.preferRefreshedRemoteRef(ctx, req.RepositoryPath, fallback)
	}
	if req.PullBeforeWorktree {
		return m.pullBaseBranch(ctx, req.RepositoryPath, fallback, nil)
	}
	return fallback, nil
}

func (m *Manager) finishBaseFallback(req *CreateRequest, baseRef, fallback, resolvedFallback string) (string, string, string, error) {
	m.logger.Warn("requested base branch not found, falling back",
		zap.String("repository_path", req.RepositoryPath),
		zap.String("requested_branch", baseRef),
		zap.String("fallback_branch", fallback))
	// Use req.BaseBranch (the user-supplied name) in the user-facing warning
	// rather than baseRef, which may carry an internal "origin/<x>" form
	// produced by pullBaseBranch when PullBeforeWorktree is set.
	warning := fmt.Sprintf("Requested base branch %q not found, used %q instead", req.BaseBranch, fallback)
	detail := fmt.Sprintf("git rev-parse --verify %s failed; recovered using fallback branch %q (typically the repository's default_branch)", baseRef, fallback)
	// Reflect the resolved branch in the persisted worktree record so
	// downstream consumers (UI, queries, debug logs) see the actual base
	// rather than the requested-but-missing one.
	req.BaseBranch = fallback
	return resolvedFallback, warning, detail, nil
}

// createInTaskDir creates a worktree inside the task directory structure:
// ~/.kandev/tasks/{taskDirName}/{repoName}/
//
// RepoName is sanitized to a single path segment so display names like
// "owner/repo" don't produce a nested subdirectory — that would push the
// worktree one level below the task root and break agentctl's sibling-based
// multi-repo detection.
func (m *Manager) createInTaskDir(ctx context.Context, req CreateRequest, baseRef, fallbackWarning, fallbackDetail string) (*Worktree, error) {
	worktreePath, err := m.prepareTaskWorktreePath(req)
	if err != nil {
		return nil, err
	}

	_, branchName := m.buildWorktreeNames(req)
	startPoint := baseRef

	var fetchResult *FetchBranchResult
	checkoutMode := req
	if req.RemoteContribution != nil {
		return m.createContributionInTaskDir(ctx, req, worktreePath, fallbackWarning, fallbackDetail)
	}
	if req.CheckoutBranch != "" {
		if req.RemoteSyncHandled {
			selectedRef, prepareErr := m.prepareBranchFromRefreshedOrigin(
				ctx, req.RepositoryPath, req.CheckoutBranch, req.CheckoutBranch, req.PRNumber,
			)
			if prepareErr != nil {
				return nil, prepareErr
			}
			if selectedRef == "" {
				if req.PRNumber > 0 {
					return nil, fmt.Errorf("%w: refreshed pull request head %d was not materialized", ErrBranchUnrecoverable, req.PRNumber)
				}
				branchName = req.CheckoutBranch
				checkoutMode.CheckoutBranch = ""
			} else {
				// A refreshed remote that contains the old local checkout branch
				// must be the actual worktree start point. Passing it only to the
				// preparation probe would still let git worktree add check out the
				// stale local branch.
				startPoint = selectedRef
			}
		} else {
			// PRNumber != 0 means the caller wants the refs/pull/<N>/head ref;
			// fork PR branches don't exist as plain refs locally or under
			// origin/<branch>, so the existence probe must be skipped and the
			// fetch path always runs.
			//
			// When PRNumber == 0 and the named branch is absent locally and on
			// origin, the caller's intent is "create a new branch with this
			// name" rather than "fetch this existing ref" — the historical
			// fetch-then-check-out path errored ("not found locally or on
			// remote") in that case and rolled back. We drop CheckoutBranch
			// from the request copy and pass the desired name as the fallback
			// (new) branch name so gitAddWorktree creates it from baseRef.
			if req.PRNumber == 0 && !m.checkoutBranchExistsAnywhere(ctx, req.RepositoryPath, req.CheckoutBranch) {
				m.logger.Info("checkout branch missing locally and on origin; creating new branch with this name",
					zap.String("repository_path", req.RepositoryPath),
					zap.String("requested_branch", req.CheckoutBranch),
					zap.String("base_ref", baseRef))
				branchName = req.CheckoutBranch
				checkoutMode.CheckoutBranch = ""
			} else {
				fetchResult, err = m.fetchBranchToLocalWithPolicy(
					ctx, req.RepositoryPath, req.CheckoutBranch, req.PRNumber, req.PullBeforeWorktree,
				)
				if err != nil {
					return nil, err
				}
				if fetchResult.StartPoint != "" {
					startPoint = fetchResult.StartPoint
				} else {
					startPoint = req.CheckoutBranch
				}
			}
		}
	}

	worktreeID, branchName, err := m.addWorktreeForBranch(ctx, checkoutMode, worktreePath, branchName, startPoint, baseRef)
	if err != nil {
		return nil, err
	}

	wt := m.buildWorktreeRecord(worktreeID, req, worktreePath, branchName)
	if fetchResult != nil {
		wt.FetchWarning = fetchResult.Warning
		wt.FetchWarningDetail = fetchResult.WarningDetail
	}

	if err := m.persistAndCacheWorktree(ctx, wt, req, worktreePath); err != nil {
		return nil, err
	}

	// Surface any base-branch fallback before signaling readiness so the
	// "Create worktree" step can show the warning when completed early.
	if fallbackWarning != "" {
		wt.BaseBranchFallbackWarning = fallbackWarning
		wt.BaseBranchFallbackDetail = fallbackDetail
	}

	// The worktree directory now exists and is persisted. Signal readiness
	// before running the per-repo setup script so the env preparer can complete
	// the "Create worktree" UI step and render the setup script as a distinct,
	// subsequent step rather than overlapping it.
	if req.OnWorktreeCreated != nil {
		req.OnWorktreeCreated(wt)
	}

	m.copyConfiguredFiles(ctx, req, wt)

	// Setup script failures are non-fatal — runWorktreeSetupScript records a
	// warning on wt and keeps the worktree so the agent can still launch.
	m.runWorktreeSetupScript(ctx, wt, req.ScriptEnv)

	m.logger.Info("created worktree in task directory",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("task_dir", req.TaskDirName),
		zap.String("repo_name", req.RepoName),
		zap.String("path", worktreePath),
		zap.String("branch", wt.Branch))

	return wt, nil
}

func (m *Manager) createContributionInTaskDir(
	ctx context.Context,
	req CreateRequest,
	worktreePath, fallbackWarning, fallbackDetail string,
) (*Worktree, error) {
	remoteName, contributionRef, err := m.materializeRemoteContribution(ctx, req.RepositoryPath, req.RemoteContribution)
	if err != nil {
		return nil, err
	}
	worktreeID, branchName, err := m.addContributionWorktree(ctx, req, worktreePath, contributionRef, remoteName)
	if err != nil {
		return nil, err
	}
	wt := m.buildWorktreeRecord(worktreeID, req, worktreePath, branchName)
	if err := m.persistAndCacheWorktree(ctx, wt, req, worktreePath); err != nil {
		return nil, err
	}
	if fallbackWarning != "" {
		wt.BaseBranchFallbackWarning = fallbackWarning
		wt.BaseBranchFallbackDetail = fallbackDetail
	}
	if req.OnWorktreeCreated != nil {
		req.OnWorktreeCreated(wt)
	}
	m.copyConfiguredFiles(ctx, req, wt)
	m.runWorktreeSetupScript(ctx, wt, req.ScriptEnv)
	return wt, nil
}

func (m *Manager) prepareTaskWorktreePath(req CreateRequest) (string, error) {
	repoDir := SanitizeRepoDirName(req.RepoName)
	if repoDir == "" {
		return "", ErrInvalidRepoName
	}
	branchSlug := SanitizeBranchSlug(req.BranchSlug)
	if req.BranchSlug != "" && branchSlug == "" {
		return "", ErrInvalidBranchSlug
	}
	worktreePath, err := m.config.TaskWorktreePath(req.TaskDirName, repoDir, branchSlug)
	if err != nil {
		return "", fmt.Errorf("failed to get task worktree path: %w", err)
	}

	taskDir := filepath.Dir(worktreePath)
	if err := m.validateTaskDir(taskDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create task directory: %w", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskDir, storageworkspaces.OwnershipMarker{
		TaskID: req.TaskID, WorkspaceID: req.WorkspaceID, TaskDirName: req.TaskDirName,
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		return "", fmt.Errorf("mark task directory ownership: %w", err)
	}
	return worktreePath, nil
}

func (m *Manager) validateTaskDir(taskDir string) error {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return fmt.Errorf("failed to resolve tasks base path: %w", err)
	}
	absTasksBase, err := filepath.Abs(tasksBase)
	if err != nil {
		return fmt.Errorf("failed to resolve tasks base path: %w", err)
	}
	absTaskDir, err := filepath.Abs(taskDir)
	if err != nil {
		return fmt.Errorf("failed to resolve task directory: %w", err)
	}
	if err := storage.ValidateNoSymlinkPath(absTasksBase, absTaskDir); err != nil {
		return fmt.Errorf("unsafe task directory: %w", err)
	}
	return nil
}

// addWorktreeForBranch creates the git worktree, trying the checkout branch directly first
// and falling back to a suffixed branch if the checkout branch is already in use.
// When a checkout branch is specified, it sets the upstream tracking branch to
// origin/<checkout-branch> so ahead/behind counts are relative to the PR's remote branch.
func (m *Manager) addWorktreeForBranch(ctx context.Context, req CreateRequest, worktreePath, fallbackBranch, startPoint, baseRef string) (string, string, error) {
	if req.CheckoutBranch == "" {
		id, err := m.gitAddWorktree(ctx, req.RepositoryPath, fallbackBranch, worktreePath, baseRef)
		return id, fallbackBranch, err
	}

	// Try checking out the PR branch directly (common case: single task per PR).
	var id string
	var err error
	if req.RemoteSyncHandled && startPoint != req.CheckoutBranch {
		id, err = m.gitAddWorktreeExistingAtRef(ctx, req.RepositoryPath, req.CheckoutBranch, worktreePath, startPoint)
	} else {
		id, err = m.gitAddWorktreeExisting(ctx, req.RepositoryPath, req.CheckoutBranch, worktreePath)
	}
	if err == nil {
		m.setUpstreamIfExists(ctx, worktreePath, req.CheckoutBranch, req.CheckoutBranch)
		return id, req.CheckoutBranch, nil
	}
	if !errors.Is(err, ErrBranchCheckedOut) {
		return "", "", err
	}

	// Branch is in use by another worktree — create a unique fallback branch
	// using the original branch name with a random suffix.
	suffixed := req.CheckoutBranch + "-" + SmallSuffix(3)
	id, err = m.gitAddWorktree(ctx, req.RepositoryPath, suffixed, worktreePath, startPoint)
	if err == nil {
		m.setUpstreamIfExists(ctx, worktreePath, suffixed, req.CheckoutBranch)
	}
	return id, suffixed, err
}

// setUpstreamIfExists sets the upstream tracking branch for a worktree branch
// to origin/<remoteBranch> if the remote-tracking ref exists. Non-fatal on failure.
func (m *Manager) setUpstreamIfExists(ctx context.Context, worktreePath, localBranch, remoteBranch string) {
	upstream := "origin/" + remoteBranch
	// Verify the remote-tracking ref exists. Use the non-interactive helper so
	// this cannot hang on a credential prompt while Create holds repoLock.
	verifyCmd := m.newNonInteractiveGitCmd(ctx, worktreePath, "rev-parse", "--verify", upstream)
	if err := runGitCmd(ctx, verifyCmd); err != nil {
		return
	}
	cmd := m.newNonInteractiveGitCmd(ctx, worktreePath, "branch", "--set-upstream-to="+upstream, localBranch)
	if out, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		m.logger.Debug("failed to set upstream (non-fatal)",
			zap.String("branch", localBranch),
			zap.String("upstream", upstream),
			zap.String("output", string(out)),
			zap.Error(err))
	}
}

// FetchBranchResult holds the outcome of a fetchBranchToLocal call.
type FetchBranchResult struct {
	StartPoint    string // Ref to use as worktree start point (e.g., "origin/branch"); empty = use local branch
	Warning       string // User-friendly warning (non-empty when fell back to local)
	WarningDetail string // Raw git command output for debugging, or the probe that produced the warning when no git command failed
}

// fetchBranchToLocal ensures a branch exists locally and is up-to-date.
// It first tries to fetch from origin to get the latest commits. If the fetch
// fails (no remote, auth issue, offline), it falls back to the local branch.
// Returns a FetchBranchResult with warning info and an error if the branch
// doesn't exist anywhere.
//
// When prNumber > 0, the fetch uses the refs/pull/<N>/head refspec instead of
// fetching the branch by name. GitHub mirrors every PR head under that ref on
// the base repo, so this is the only way to materialize a fork PR's head
// without adding the fork as a remote.
func (m *Manager) fetchBranchToLocal(ctx context.Context, repoPath, branch string, prNumber int) (*FetchBranchResult, error) {
	return m.fetchBranchToLocalWithPolicy(ctx, repoPath, branch, prNumber, false)
}

func (m *Manager) fetchBranchToLocalWithPolicy(
	ctx context.Context, repoPath, branch string, prNumber int, required bool,
) (*FetchBranchResult, error) {
	m.logger.Info("syncing checkout branch",
		zap.String("branch", branch),
		zap.Int("pr_number", prNumber),
		zap.Bool("required", required),
		zap.String("repo_path", repoPath))

	// Acquire the throttle slot FIRST, then build fetchCtx with the
	// full m.fetchTimeout budget. The previous shape (build fetchCtx,
	// then call runGitCmdCombinedOutput which Acquires internally) let
	// throttle queue time burn the fetch budget — same failure mode as
	// the manager_git.go probes fixed in PR #1216 (70s lock-held trace,
	// signal:killed). With this ordering the budget only counts actual
	// git execution time.
	refspec := branch + ":" + branch
	if prNumber > 0 {
		refspec = fmt.Sprintf("pull/%d/head:%s", prNumber, branch)
	}
	output, err, fetchCtxErr := m.runGitCombinedAfterAcquire(ctx, m.fetchTimeout, repoPath, "fetch", gitNoTags, "origin", refspec)
	if err == nil {
		return &FetchBranchResult{}, nil
	}
	outputStr := string(output)

	// If the branch is checked out in another worktree, git refuses to update
	// the local ref. Retry by fetching only the remote-tracking ref (origin/branch),
	// which is always safe regardless of worktree state.
	if isFetchRefusedCheckedOut(outputStr) {
		if result := m.retryFetchAsRemoteTrackingRef(ctx, repoPath, branch, prNumber, required); result != nil {
			if required && result.Warning != "" {
				return nil, fmt.Errorf(
					"required refresh of checkout branch %q could not verify the refreshed ref: %w",
					branch, ErrGitCommandFailed,
				)
			}
			return result, nil
		}
	}

	if required {
		if isRemoteBranchMissingError(outputStr) {
			return nil, fmt.Errorf(
				"required refresh of checkout branch %q found no remote ref: %w",
				branch, ErrInvalidBaseBranch,
			)
		}
		reason := classifyGitFallbackReason(err, outputStr, fetchCtxErr)
		return nil, fmt.Errorf(
			"required refresh of checkout branch %q failed (%s): %w",
			branch, reason, syncFailureCause(reason, err, fetchCtxErr),
		)
	}

	m.logger.Warn("fetch from origin failed, checking local branch",
		zap.String("branch", branch),
		zap.String("output", outputStr),
		zap.Error(err))

	// Fall back to local branch if it exists.
	exists, existsErr := m.branchExists(ctx, repoPath, branch)
	if existsErr != nil {
		return nil, fmt.Errorf("could not verify local branch %q after fetch failure (%s): %w", branch, strings.TrimSpace(outputStr), existsErr)
	}
	if !exists {
		return nil, fmt.Errorf("%w: branch %q not found locally or on remote: %s", ErrInvalidBaseBranch, branch, outputStr)
	}

	reason := classifyGitFallbackReason(err, outputStr, fetchCtxErr)
	warning := fmt.Sprintf("Could not fetch latest from origin (%s). Using local version of branch %q which may be outdated.", reason, branch)
	m.logger.Info("using local branch (fetch failed)",
		zap.String("branch", branch),
		zap.String("warning", warning))
	return &FetchBranchResult{
		Warning:       warning,
		WarningDetail: strings.TrimSpace(outputStr),
	}, nil
}

// retryFetchAsRemoteTrackingRef retries the fetch with just the remote-tracking
// ref (or pull/N/head) so it doesn't try to update the local branch ref, which
// fails when that branch is already checked out in another worktree. Returns
// nil if the retry also failed and the caller should fall back to the local
// branch path.
//
// Uses ctx (not the original fetchCtx) and a fresh m.fetchTimeout budget via
// runGitCombinedAfterAcquire — the parent's fetchCtx is already consumed by
// the first attempt, so reusing it would leave no room for the retry.
func (m *Manager) retryFetchAsRemoteTrackingRef(
	ctx context.Context, repoPath, branch string, prNumber int, required bool,
) *FetchBranchResult {
	retryRef := branch
	if prNumber > 0 {
		retryRef = fmt.Sprintf("pull/%d/head", prNumber)
	}
	if _, retryErr, _ := m.runGitCombinedAfterAcquire(ctx, m.fetchTimeout, repoPath, "fetch", gitNoTags, "origin", retryRef); retryErr != nil {
		return nil
	}
	m.logger.Info("fetched via remote-tracking ref (branch checked out elsewhere)",
		zap.String("branch", branch))
	if prNumber > 0 {
		if required {
			// A fork PR has no origin/<branch> ref. FETCH_HEAD is the only
			// ref produced by the successful pull/<N>/head retry, and is
			// therefore the safe start point for a required refresh.
			return &FetchBranchResult{StartPoint: "FETCH_HEAD"}
		}
		// Fork PRs have no origin/<branch> ref — the bare
		// pull/<N>/head retry only updates FETCH_HEAD, so the local
		// `branch` (already populated by the prior worktree) is the
		// only valid start point. Empty StartPoint signals the
		// caller to fall back to req.CheckoutBranch.
		return &FetchBranchResult{}
	}
	return m.resolveRetryStartPoint(ctx, repoPath, branch)
}

// resolveRetryStartPoint picks the start point after the remote-tracking retry
// succeeded. The retry refreshed origin/<branch> but deliberately left the
// local branch alone — that is the whole point of dropping the
// `<branch>:<branch>` refspec — so origin/<branch> is the better start point
// only while it already contains every local commit.
//
// The first fetch is refused whenever the branch is checked out, which for a
// user's own repository is the normal state of the branch they work on. If it
// carries commits they have not pushed, adopting the remote ref drops them from
// the new worktree, and Create still succeeds so nothing surfaces the loss.
// Being slightly stale is recoverable; losing unpushed work is not. A probe
// that cannot answer keeps the local branch for the same reason.
//
// This also matches the sibling fallbacks: the direct-checkout path in
// addWorktreeForBranch checks out the local branch, and so do
// handleFetchFallback and the non-current-branch path in resolveLocalBaseRef.
func (m *Manager) resolveRetryStartPoint(ctx context.Context, repoPath, branch string) *FetchBranchResult {
	remoteRef := "origin/" + branch
	remoteContainsLocal, remoteErr := m.refContains(ctx, repoPath, remoteRef, branch)
	if remoteErr == nil && remoteContainsLocal {
		return &FetchBranchResult{StartPoint: remoteRef}
	}
	m.logger.Info("using local branch (remote-tracking ref does not contain all local commits)",
		zap.String("branch", branch),
		zap.String("remote_ref", remoteRef))

	// Being ahead of the remote ref costs the worktree nothing — the local
	// branch is then a strict superset, so there is nothing to warn about.
	// Every other shape does: the refs have diverged, or the probe could not
	// answer (origin/<branch> absent because the remote has no standard fetch
	// refspec, or the probe itself was bounded). In those cases the worktree
	// really may miss commits that are on origin, so the existing FetchWarning
	// surface has to say so rather than report a clean success. The wording
	// covers both, because this branch cannot tell them apart.
	localContainsRemote, localErr := m.refContains(ctx, repoPath, branch, remoteRef)
	if localErr == nil && localContainsRemote {
		return &FetchBranchResult{}
	}
	warningDetail := fmt.Sprintf("git merge-base --is-ancestor %s %s reported non-zero", branch, remoteRef)
	if remoteErr != nil || localErr != nil {
		warningDetail = "git merge-base ancestry could not be verified"
	}
	return &FetchBranchResult{
		Warning: fmt.Sprintf(
			"Could not confirm that %s contains every commit on local branch %q. Using the local version so unpushed commits are kept; it may not include the latest changes from origin.",
			remoteRef, branch),
		// No git command failed here, so there is no raw output to carry: the
		// probe ran and answered "no" (or could not answer). Name the probe
		// rather than inventing a fake error string.
		WarningDetail: warningDetail,
	}
}

// gitAddWorktreeExisting creates a worktree that checks out an existing local branch.
// If the branch is already checked out in a stale worktree (directory no longer exists),
// it automatically prunes and retries. If the repository uses git-crypt, it creates
// the worktree without checkout, then unlocks git-crypt and performs the checkout.
func (m *Manager) gitAddWorktreeExisting(ctx context.Context, repoPath, branchName, worktreePath string) (string, error) {
	return m.gitAddWorktreeExistingAtRef(ctx, repoPath, branchName, worktreePath, "")
}

// gitAddWorktreeExistingAtRef creates a worktree for an existing branch at a
// selected start point. A non-empty start point resets the branch with git's
// -B worktree option, which is safe here because the caller has already proved
// that the refreshed remote contains the old local branch.
func (m *Manager) gitAddWorktreeExistingAtRef(ctx context.Context, repoPath, branchName, worktreePath, startPoint string) (string, error) {
	release, err := acquireWorktreeTargetPath(ctx, worktreePath)
	if err != nil {
		return "", fmt.Errorf("acquire worktree target path: %w", err)
	}
	defer release()
	return m.gitAddWorktreeExistingLocked(ctx, repoPath, branchName, worktreePath, startPoint)
}

func (m *Manager) gitAddWorktreeExistingLocked(ctx context.Context, repoPath, branchName, worktreePath, startPoint string) (string, error) {
	worktreeID := uuid.New().String()
	usesGitCrypt := m.usesGitCrypt(repoPath)

	// Build worktree add command
	args := []string{"worktree", "add"}
	if startPoint != "" {
		args = append(args, "-B", branchName)
	}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath)
	if startPoint == "" {
		args = append(args, branchName)
	} else {
		args = append(args, startPoint)
	}

	cmd := newGitCommand(ctx, args...)
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err == nil {
		if usesGitCrypt {
			if unlockErr := m.unlockGitCryptAndCheckout(ctx, worktreePath); unlockErr != nil {
				_ = m.removeWorktreeDir(ctx, worktreePath, repoPath)
				return "", unlockErr
			}
		} else {
			m.initSubmodules(ctx, worktreePath)
		}
		return worktreeID, nil
	}

	outStr := string(output)

	// Check for git-crypt smudge error and retry with --no-checkout
	if isGitCryptSmudgeError(outStr) && !usesGitCrypt {
		m.logger.Warn("git-crypt smudge error detected, retrying with --no-checkout",
			zap.String("output", outStr))
		return m.gitAddWorktreeExistingWithGitCrypt(ctx, repoPath, branchName, worktreePath, startPoint)
	}

	if !isBranchCheckedOutError(outStr) {
		m.logger.Error("git worktree add (existing branch) failed",
			zap.String("output", outStr), zap.Error(err))
		return "", ClassifyGitError(outStr, err)
	}

	if recoveryErr := m.tryRecoverCheckedOutBranch(ctx, repoPath, branchName, outStr); recoveryErr != nil {
		m.logger.Warn("branch is checked out in active worktree",
			zap.String("branch", branchName), zap.Error(recoveryErr))
		return "", ErrBranchCheckedOut
	}

	// Retry after pruning stale worktree
	return m.retryWorktreeExisting(ctx, repoPath, branchName, worktreePath, usesGitCrypt, startPoint)
}

// retryWorktreeExisting retries worktree creation after pruning stale worktrees.
func (m *Manager) retryWorktreeExisting(ctx context.Context, repoPath, branchName, worktreePath string, usesGitCrypt bool, startPoint string) (string, error) {
	worktreeID := uuid.New().String()

	args := []string{"worktree", "add"}
	if startPoint != "" {
		args = append(args, "-B", branchName)
	}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath)
	if startPoint == "" {
		args = append(args, branchName)
	} else {
		args = append(args, startPoint)
	}

	retryCmd := newGitCommand(ctx, args...)
	retryCmd.Dir = repoPath
	retryOutput, retryErr := runGitCmdCombinedOutput(ctx, retryCmd)
	if retryErr != nil {
		retryOutStr := string(retryOutput)
		if isBranchCheckedOutError(retryOutStr) {
			return "", ErrBranchCheckedOut
		}
		m.logger.Error("git worktree add retry failed",
			zap.String("output", retryOutStr), zap.Error(retryErr))
		return "", ClassifyGitError(retryOutStr, retryErr)
	}

	if usesGitCrypt {
		if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
			_ = m.removeWorktreeDir(ctx, worktreePath, repoPath)
			return "", err
		}
	} else {
		m.initSubmodules(ctx, worktreePath)
	}

	m.logger.Info("recovered from stale worktree checkout", zap.String("branch", branchName))
	return worktreeID, nil
}

// gitAddWorktreeExistingWithGitCrypt creates a worktree for an existing branch
// using --no-checkout, then unlocks git-crypt. Used as fallback when smudge error detected.
func (m *Manager) gitAddWorktreeExistingWithGitCrypt(ctx context.Context, repoPath, branchName, worktreePath, startPoint string) (string, error) {
	worktreeID := uuid.New().String()

	args := []string{"worktree", "add"}
	if startPoint != "" {
		args = append(args, "-B", branchName)
	}
	args = append(args, "--no-checkout", worktreePath)
	if startPoint == "" {
		args = append(args, branchName)
	} else {
		args = append(args, startPoint)
	}
	cmd := newGitCommand(ctx, args...)
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		outStr := string(output)
		if isBranchCheckedOutError(outStr) {
			return "", ErrBranchCheckedOut
		}
		m.logger.Error("git worktree add (--no-checkout, existing) failed",
			zap.String("output", outStr), zap.Error(err))
		return "", ClassifyGitError(outStr, err)
	}

	if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
		_ = m.removeWorktreeDir(ctx, worktreePath, repoPath)
		return "", err
	}

	return worktreeID, nil
}

// tryRecoverCheckedOutBranch attempts to recover from "branch is already checked out"
// by pruning stale worktrees. Returns nil if recovery succeeded, error otherwise.
func (m *Manager) tryRecoverCheckedOutBranch(ctx context.Context, repoPath, branchName, gitOutput string) error {
	// Parse the path from git output: "fatal: 'branch' is already checked out at '/path/to/worktree'"
	checkedOutPath := parseCheckedOutPath(gitOutput)
	if checkedOutPath == "" {
		return fmt.Errorf("could not parse worktree path from git output")
	}

	// Check if the worktree directory still exists on disk.
	if _, err := os.Stat(checkedOutPath); err == nil {
		// Directory exists — worktree is genuinely in use, can't recover.
		m.logger.Warn("branch checked out in active worktree, cannot recover",
			zap.String("branch", branchName),
			zap.String("worktree_path", checkedOutPath))
		return fmt.Errorf("worktree at %s is still active", checkedOutPath)
	}

	// Directory is gone — prune stale worktree references.
	m.logger.Info("pruning stale worktree reference",
		zap.String("branch", branchName),
		zap.String("stale_path", checkedOutPath))

	pruneCmd := newGitCommand(ctx, "worktree", "prune")
	pruneCmd.Dir = repoPath
	if output, err := runGitCmdCombinedOutput(ctx, pruneCmd); err != nil {
		m.logger.Error("git worktree prune failed",
			zap.String("output", string(output)),
			zap.Error(err))
		return fmt.Errorf("worktree prune failed: %w", err)
	}
	return nil
}

// parseCheckedOutPath extracts the worktree path from git's error message.
// Handles both "is already checked out at '/path'" and "is already used by worktree at '/path'".
func parseCheckedOutPath(gitOutput string) string {
	for _, marker := range []string{"checked out at '", "used by worktree at '"} {
		_, after, found := strings.Cut(gitOutput, marker)
		if found {
			path, _, ok := strings.Cut(after, "'")
			if ok {
				return path
			}
		}
	}
	return ""
}

// buildWorktreeNames derives the filesystem directory name and git branch name for a new worktree.
func (m *Manager) buildWorktreeNames(req CreateRequest) (dirName, branchName string) {
	dirSuffix := uuid.New().String()[:8] // Use first 8 chars of UUID for worktree dir uniqueness
	branchSuffix := SmallSuffix(3)

	if req.TaskTitle != "" {
		// Use semantic naming: {sanitized-title}_{suffix}
		dirName = SemanticWorktreeName(req.TaskTitle, dirSuffix)
	} else {
		// Fallback to task ID based naming
		dirName = req.TaskID + "_" + dirSuffix
	}
	branchName = TaskBranchNameWithSuffix(req.TaskTitle, req.TaskID, req.WorktreeBranchPrefix, branchSuffix)
	if req.WorktreeBranchTemplate != "" {
		if rendered, err := RenderTaskBranchName(BranchNameTemplateInput{
			Template: req.WorktreeBranchTemplate,
			TaskID:   req.TaskID,
			Title:    req.TaskTitle,
			Ticket:   req.WorktreeBranchTicket,
			Suffix:   branchSuffix,
		}); err == nil {
			branchName = rendered
		} else {
			m.logger.Warn("worktree branch template render failed; using fallback branch name",
				zap.String("task_id", req.TaskID),
				zap.String("template", req.WorktreeBranchTemplate),
				zap.Error(err))
		}
	}
	return dirName, branchName
}

type newBranchAddSnapshot struct {
	branchRef string
	branchOID string
}

func (m *Manager) createNewBranchRef(
	ctx context.Context, repoPath, branchName, baseRef string,
) (newBranchAddSnapshot, error) {
	resolveCmd := newGitCommand(ctx, "rev-parse", "--verify", baseRef+"^{commit}")
	resolveCmd.Dir = repoPath
	output, err := runGitCmdOutput(ctx, resolveCmd)
	if err != nil {
		return newBranchAddSnapshot{}, ClassifyGitError(string(output), err)
	}
	oid := strings.TrimSpace(string(output))
	if oid == "" {
		return newBranchAddSnapshot{}, fmt.Errorf("%w: base ref %q resolved to an empty object ID", ErrGitCommandFailed, baseRef)
	}
	ownership := newBranchAddSnapshot{branchRef: "refs/heads/" + branchName}
	zeroOID := strings.Repeat("0", len(oid))
	createCmd := newGitCommand(ctx, "update-ref", ownership.branchRef, oid, zeroOID)
	createCmd.Dir = repoPath
	createOutput, createErr := runGitCmdCombinedOutput(ctx, createCmd)
	if createErr != nil {
		if strings.Contains(strings.ToLower(string(createOutput)), "reference already exists") {
			return newBranchAddSnapshot{}, fmt.Errorf("%w: %s", ErrBranchExists, strings.TrimSpace(string(createOutput)))
		}
		return newBranchAddSnapshot{}, ClassifyGitError(string(createOutput), createErr)
	}
	ownership.branchOID = oid
	return ownership, nil
}

func deleteBranchRefIfOwned(ctx context.Context, repoPath string, snapshot newBranchAddSnapshot) ([]byte, error) {
	cmd := newGitCommand(ctx, "update-ref", "-d", snapshot.branchRef, snapshot.branchOID)
	cmd.Dir = repoPath
	return runGitCmdCombinedOutput(ctx, cmd)
}

func restoreBranchRefIfDeleted(ctx context.Context, repoPath string, snapshot newBranchAddSnapshot) ([]byte, error) {
	cmd := newGitCommand(ctx, "update-ref", snapshot.branchRef, snapshot.branchOID, strings.Repeat("0", len(snapshot.branchOID)))
	cmd.Dir = repoPath
	return runGitCmdCombinedOutput(ctx, cmd)
}

func (m *Manager) rollbackFailedNewBranchAdd(
	ctx context.Context,
	repoPath, branchName, worktreePath string,
	snapshot newBranchAddSnapshot,
) {
	if snapshot.branchRef == "" || snapshot.branchOID == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.inspectTimeout)
	defer cancel()
	registrationOwnership, inspectErr := inspectWorktreeRegistrationOwnership(
		cleanupCtx, repoPath, worktreePath, snapshot.branchRef, snapshot.branchOID,
	)
	if inspectErr != nil {
		m.logger.Warn("failed to verify partial worktree ownership; leaving path untouched",
			zap.String("path", worktreePath), zap.Error(inspectErr))
		return
	}
	switch registrationOwnership {
	case worktreeRegistrationAbsent:
		// git may fail before it registers a worktree (for example when the
		// target is non-empty). The branch was created with a zero-OID CAS, so
		// this matching delete only removes our still-unmodified ref. Never
		// touch the target path: it was not registered as ours.
		if output, err := deleteBranchRefIfOwned(cleanupCtx, repoPath, snapshot); err != nil {
			m.logger.Warn("failed to roll back unregistered worktree branch",
				zap.String("branch", branchName),
				zap.String("output", string(output)),
				zap.Error(err))
		}
		return
	case worktreeRegistrationCompeting:
		return
	case worktreeRegistrationOwned:
		// Continue below.
	default:
		return
	}
	// Delete the exact ref while the owned worktree registration still blocks
	// ordinary competing checkouts. The expected old OID preserves an advanced
	// or replaced ref; failure leaves both the ref and worktree untouched.
	if output, err := deleteBranchRefIfOwned(cleanupCtx, repoPath, snapshot); err != nil {
		m.logger.Warn("failed to roll back worktree branch",
			zap.String("branch", branchName),
			zap.String("output", string(output)),
			zap.Error(err))
		return
	}
	removeErr := m.removeWorktreeDir(cleanupCtx, worktreePath, repoPath)
	registrationOwnership, inspectErr = inspectWorktreeRegistrationOwnership(
		cleanupCtx, repoPath, worktreePath, snapshot.branchRef, snapshot.branchOID,
	)
	// Git's removal command can report an error even when its filesystem/prune
	// fallback completed the registration cleanup. The post-cleanup ownership
	// inspection is authoritative for whether deleting the ref remains safe.
	if inspectErr == nil && registrationOwnership == worktreeRegistrationAbsent {
		return
	}
	if removeErr != nil {
		m.logger.Warn("failed to roll back partial worktree",
			zap.String("path", worktreePath), zap.Error(removeErr))
	}
	if inspectErr != nil {
		m.logger.Warn("failed to verify worktree registration after partial cleanup",
			zap.String("path", worktreePath), zap.Error(inspectErr))
	} else {
		m.logger.Warn("partial worktree registration remains after cleanup",
			zap.String("path", worktreePath), zap.Uint8("registration_ownership", uint8(registrationOwnership)))
	}
	// A branch registration prevents ordinary worktree creation from adopting
	// this ref while cleanup runs. If cleanup did not prove that registration
	// is gone, restore the exact OID with a zero-OID CAS before returning so a
	// registered worktree can never be left pointing at a missing branch.
	if output, err := restoreBranchRefIfDeleted(cleanupCtx, repoPath, snapshot); err != nil {
		m.logger.Warn("failed to restore branch after partial worktree cleanup",
			zap.String("branch", branchName),
			zap.String("output", string(output)),
			zap.Error(err))
	}
}

// gitAddWorktree runs "git worktree add" and returns the new worktree UUID.
// If the repository uses git-crypt, it creates the worktree without checkout,
// then unlocks git-crypt and performs the checkout separately.
func (m *Manager) gitAddWorktree(ctx context.Context, repoPath, branchName, worktreePath, baseRef string) (string, error) {
	release, err := acquireWorktreeTargetPath(ctx, worktreePath)
	if err != nil {
		return "", fmt.Errorf("acquire worktree target path: %w", err)
	}
	defer release()
	return m.gitAddWorktreeLocked(ctx, repoPath, branchName, worktreePath, baseRef)
}

func (m *Manager) gitAddWorktreeLocked(ctx context.Context, repoPath, branchName, worktreePath, baseRef string) (string, error) {
	worktreeID := uuid.New().String()
	usesGitCrypt := m.usesGitCrypt(repoPath)
	addSnapshot, err := m.createNewBranchRef(ctx, repoPath, branchName, baseRef)
	if err != nil {
		return "", err
	}

	args := []string{"worktree", "add"}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, branchName)

	cmd := newGitCommand(ctx, args...)
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		outStr := string(output)
		m.rollbackFailedNewBranchAdd(ctx, repoPath, branchName, worktreePath, addSnapshot)
		// Check if this is a git-crypt error we didn't anticipate
		if isGitCryptSmudgeError(outStr) {
			m.logger.Warn("git-crypt smudge error detected, retrying with --no-checkout",
				zap.String("output", outStr))
			return m.gitAddWorktreeWithGitCryptLocked(ctx, repoPath, branchName, worktreePath, baseRef)
		}
		m.logger.Error("git worktree add failed",
			zap.String("output", outStr),
			zap.Error(err))
		return "", ClassifyGitError(outStr, err)
	}

	// If we used --no-checkout, we need to unlock git-crypt and checkout
	if usesGitCrypt {
		if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
			m.rollbackFailedNewBranchAdd(ctx, repoPath, branchName, worktreePath, addSnapshot)
			return "", err
		}
	} else {
		m.initSubmodules(ctx, worktreePath)
	}

	return worktreeID, nil
}

// gitAddWorktreeWithGitCrypt creates a worktree using --no-checkout and then
// unlocks git-crypt. This is used as a fallback when we detect a git-crypt
// smudge filter error.
func (m *Manager) gitAddWorktreeWithGitCryptLocked(ctx context.Context, repoPath, branchName, worktreePath, baseRef string) (string, error) {
	worktreeID := uuid.New().String()
	addSnapshot, err := m.createNewBranchRef(ctx, repoPath, branchName, baseRef)
	if err != nil {
		return "", err
	}

	cmd := newGitCommand(ctx,
		"worktree", "add",
		"--no-checkout",
		worktreePath,
		branchName)
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		m.rollbackFailedNewBranchAdd(ctx, repoPath, branchName, worktreePath, addSnapshot)
		m.logger.Error("git worktree add (--no-checkout) failed",
			zap.String("output", string(output)),
			zap.Error(err))
		return "", ClassifyGitError(string(output), err)
	}

	// Unlock git-crypt and checkout
	if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
		m.rollbackFailedNewBranchAdd(ctx, repoPath, branchName, worktreePath, addSnapshot)
		return "", err
	}

	return worktreeID, nil
}

// gitAddWorktreeForRecreate runs "git worktree add" against an existing branch
// for the recreate path, retrying with --no-checkout when a git-crypt smudge
// error is detected. Returns the effective usesGitCrypt flag (forced to true
// if the retry path was taken so the caller knows to unlock+checkout).
func (m *Manager) gitAddWorktreeForRecreate(ctx context.Context, repoPath, branch, worktreePath, startPoint string) (bool, error) {
	release, err := acquireWorktreeTargetPath(ctx, worktreePath)
	if err != nil {
		return false, fmt.Errorf("acquire worktree target path: %w", err)
	}
	defer release()
	return m.gitAddWorktreeForRecreateLocked(ctx, repoPath, branch, worktreePath, startPoint)
}

func (m *Manager) gitAddWorktreeForRecreateLocked(ctx context.Context, repoPath, branch, worktreePath, startPoint string) (bool, error) {
	usesGitCrypt := m.usesGitCrypt(repoPath)
	args := []string{"worktree", "add"}
	if startPoint != "" {
		args = append(args, "-B", branch)
	}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath)
	if startPoint == "" {
		args = append(args, branch)
	} else {
		args = append(args, startPoint)
	}

	cmd := newGitCommand(ctx, args...)
	cmd.Dir = repoPath
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err == nil {
		return usesGitCrypt, nil
	}
	outStr := string(output)
	if isGitCryptSmudgeError(outStr) && !usesGitCrypt {
		m.logger.Warn("git-crypt smudge error during recreate, retrying with --no-checkout",
			zap.String("output", outStr))
		retryArgs := []string{"worktree", "add"}
		if startPoint != "" {
			retryArgs = append(retryArgs, "-B", branch)
		}
		retryArgs = append(retryArgs, "--no-checkout", worktreePath)
		if startPoint == "" {
			retryArgs = append(retryArgs, branch)
		} else {
			retryArgs = append(retryArgs, startPoint)
		}
		retryCmd := newGitCommand(ctx, retryArgs...)
		retryCmd.Dir = repoPath
		if retryOutput, retryErr := runGitCmdCombinedOutput(ctx, retryCmd); retryErr != nil {
			m.logger.Error("failed to recreate worktree (--no-checkout)",
				zap.String("output", string(retryOutput)),
				zap.Error(retryErr))
			return false, ClassifyGitError(string(retryOutput), retryErr)
		}
		return true, nil // Force unlock/checkout
	}
	m.logger.Error("failed to recreate worktree",
		zap.String("output", outStr),
		zap.Error(err))
	return false, ClassifyGitError(outStr, err)
}

// copyConfiguredFiles copies user-specified files from the source repo into
// the freshly created worktree, recording the resulting file list and
// warnings on wt for the env preparer to surface. Failures are logged but
// never propagated — worktree creation must succeed even if file seeding
// partially fails.
func (m *Manager) copyConfiguredFiles(ctx context.Context, req CreateRequest, wt *Worktree) {
	if m.repoProvider == nil || req.RepositoryID == "" {
		return
	}
	repo, err := m.repoProvider.GetRepository(ctx, req.RepositoryID)
	if err != nil {
		m.logger.Warn("copy-files: failed to fetch repository",
			zap.String("repository_id", req.RepositoryID),
			zap.Error(err))
		return
	}
	if repo == nil || repo.CopyFiles == "" {
		return
	}
	specs := copyfiles.ParseSpecs(repo.CopyFiles)
	if len(specs) == 0 {
		return
	}
	copied, warnings, err := copyfiles.Copy(ctx, req.RepositoryPath, wt.Path, specs, m.logger.Zap())
	if err != nil {
		m.logger.Warn("worktree copy-files failed",
			zap.String("session_id", req.SessionID),
			zap.String("repo_id", req.RepositoryID),
			zap.Error(err))
	}
	for _, w := range warnings {
		m.logger.Warn("worktree copy-files warning",
			zap.String("repo_id", req.RepositoryID),
			zap.String("path", wt.Path),
			zap.String("warning", w))
	}
	wt.CopiedFiles = copied
	wt.CopyFilesWarnings = warnings
}

// removeRecreatedWorktreePath deletes only a managed worktree path through
// no-follow directory handles. Legacy paths outside tasksBase retain their
// existing behavior because they are not controlled by the managed root.
func (m *Manager) removeRecreatedWorktreePath(ctx context.Context, worktreePath string) error {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return fmt.Errorf("resolve tasks base path for recreate removal: %w", err)
	}
	tasksBase = filepath.Clean(tasksBase)
	relativePath, err := filepath.Rel(tasksBase, worktreePath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("remove existing worktree path: %w", err)
		}
		return nil
	}
	if err := storageworkspaces.RemoveDirectoryNoFollow(ctx, tasksBase, worktreePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing worktree path without following links: %w", err)
	}
	return nil
}

// restoreMissingTasksBaseForRecreate rebuilds the task-root directory only when
// the configured managed base disappeared. The persisted task directory name
// fixes the root identity, while a fresh symlink validation protects the newly
// created path before Git is allowed to populate it.
func (m *Manager) restoreMissingTasksBaseForRecreate(worktreePath string, existing *Worktree) error {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return fmt.Errorf("resolve tasks base path for recreate: %w", err)
	}
	tasksBase = filepath.Clean(tasksBase)
	relativePath, err := filepath.Rel(tasksBase, worktreePath)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return nil
	}
	if _, err := os.Lstat(tasksBase); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return fmt.Errorf("inspect tasks base path for recreate: %w", err)
		}
		return nil
	}

	taskDirName := strings.SplitN(relativePath, string(filepath.Separator), 2)[0]
	if existing.TaskDirName != "" && existing.TaskDirName != taskDirName {
		return fmt.Errorf("cannot recreate worktree: persisted task root %q does not match path root %q", existing.TaskDirName, taskDirName)
	}
	taskRoot := filepath.Join(tasksBase, taskDirName)
	// Build the missing base and task root through no-follow directory handles.
	// This keeps a concurrent replacement of tasksBase from redirecting marker
	// creation into an external directory.
	taskRootHandle, err := storageworkspaces.CreateDirectoryNoFollow(tasksBase, taskRoot, 0o755)
	if err != nil {
		return fmt.Errorf("recreate missing tasks base: %w", err)
	}
	defer func() { _ = taskRootHandle.Close() }()
	if err := taskRootHandle.VerifyPath(taskRoot); err != nil {
		return fmt.Errorf("recreated task root changed during recovery: %w", err)
	}
	markerTaskDirName := taskDirName
	if err := storageworkspaces.WriteOwnershipMarkerNoFollow(taskRootHandle, storageworkspaces.OwnershipMarker{
		TaskID: existing.TaskID, TaskDirName: markerTaskDirName,
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		return fmt.Errorf("mark recreated task directory ownership: %w", err)
	}
	return nil
}

// recreate recreates a worktree from stored metadata.
func (m *Manager) recreate(ctx context.Context, existing *Worktree, req CreateRequest) (*Worktree, error) {
	if err := m.refreshRepositoryForMaterialization(ctx, &req); err != nil {
		return nil, err
	}
	// Recreate bypasses the new-worktree path, so perform the same required
	// base refresh before touching the existing worktree path. A failed refresh
	// must leave the retryable on-disk state intact.
	if req.PullBeforeWorktree && !req.RemoteSyncHandled {
		refreshReq := req
		if _, _, _, err := m.resolveBaseRefWithFallback(ctx, &refreshReq); err != nil {
			return nil, err
		}
	}

	// Clean up the recorded directory through no-follow descriptors. Reuse
	// validation can race with a local rename, so path-based RemoveAll here
	// could otherwise be redirected into another task root.
	if existing.Path != "" {
		if err := m.removeRecreatedWorktreePath(ctx, existing.Path); err != nil {
			return nil, err
		}
	}

	// Remove from git worktree list
	cmd := newGitCommand(ctx, "worktree", "prune")
	cmd.Dir = req.RepositoryPath
	if err := runGitCmd(ctx, cmd); err != nil {
		m.logger.Debug("git worktree prune failed", zap.Error(err))
	}

	// Get repository lock
	repoLock := m.getRepoLock(req.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(req.RepositoryPath)
	}()
	emptyRemoteBaseRef, err := m.ensureEmptyRemoteBaseline(ctx, &req)
	if err != nil {
		return nil, err
	}

	// Reuse the original on-disk path so the worktree is recreated in the
	// same task-dir slot it was first created in.
	worktreePath := existing.Path
	if worktreePath == "" {
		return nil, fmt.Errorf("cannot recreate worktree: existing record has no path")
	}
	if err := m.restoreMissingTasksBaseForRecreate(worktreePath, existing); err != nil {
		return nil, err
	}

	// Archive deletes the local branch (removeWorktree runs `git branch -D`),
	// so a recreate after unarchive must restore it first. fetchBranchToLocal
	// fetches origin <branch>:<branch> — or pull/<N>/head for fork PRs, whose
	// head branch never exists on origin by name — and only errors when the
	// branch exists neither locally nor on the remote.
	exists, probeErr := m.branchExists(ctx, req.RepositoryPath, existing.Branch)
	if probeErr != nil {
		// "Could not tell" (timeout / fs stall) is not "missing" — reporting
		// ErrBranchUnrecoverable here would misclassify recoverable work as
		// gone. Propagate so the caller can retry the recreate.
		return nil, fmt.Errorf("cannot verify worktree branch %q: %w", existing.Branch, probeErr)
	}
	contributionRemote := ""
	contributionRef := ""
	if req.RemoteContribution != nil {
		var materializeErr error
		contributionRemote, contributionRef, materializeErr = m.materializeRemoteContribution(ctx, req.RepositoryPath, req.RemoteContribution)
		if materializeErr != nil {
			return nil, materializeErr
		}
	}
	refreshedStartPoint := ""
	if req.RemoteSyncHandled && req.RemoteContribution == nil && emptyRemoteBaseRef == "" {
		sourceBranch := existing.Branch
		if req.CheckoutBranch != "" {
			sourceBranch = req.CheckoutBranch
		}
		selectedRef, prepareErr := m.prepareBranchFromRefreshedOrigin(
			ctx, req.RepositoryPath, existing.Branch, sourceBranch, req.PRNumber,
		)
		if prepareErr != nil {
			return nil, prepareErr
		}
		if selectedRef == "" {
			return nil, fmt.Errorf("%w: refreshed branch %q was not materialized", ErrBranchUnrecoverable, sourceBranch)
		}
		if selectedRef != existing.Branch {
			refreshedStartPoint = selectedRef
		}
		exists = true
	}
	if !exists {
		if req.RemoteContribution != nil {
			branchCmd := m.newNonInteractiveGitCmd(ctx, req.RepositoryPath, "branch", existing.Branch, contributionRef)
			if output, branchErr := runGitCmdCombinedOutput(ctx, branchCmd); branchErr != nil {
				return nil, fmt.Errorf("restore contribution branch: %s: %w", strings.TrimSpace(string(output)), branchErr)
			}
		} else if emptyRemoteBaseRef != "" {
			branchCmd := m.newNonInteractiveGitCmd(ctx, req.RepositoryPath, "branch", existing.Branch, emptyRemoteBaseRef)
			if output, branchErr := runGitCmdCombinedOutput(ctx, branchCmd); branchErr != nil {
				return nil, fmt.Errorf("restore empty-remote worktree branch: %s: %w", strings.TrimSpace(string(output)), branchErr)
			}
		} else {
			if _, fetchErr := m.fetchBranchToLocalWithPolicy(
				ctx, req.RepositoryPath, existing.Branch, req.PRNumber,
				req.PullBeforeWorktree && !req.RemoteSyncHandled,
			); fetchErr != nil {
				m.logger.Warn("failed to restore worktree branch during recreate",
					zap.String("worktree_id", existing.ID),
					zap.String("branch", existing.Branch),
					zap.Error(fetchErr))
				// Only a confirmed-missing remote ref means the work is gone;
				// transient fetch failures (network, auth) keep their own error
				// so callers don't treat a reachable branch as unrecoverable.
				if isRemoteRefMissingError(fetchErr) {
					return nil, fmt.Errorf("%w: %q", ErrBranchUnrecoverable, existing.Branch)
				}
				return nil, fetchErr
			}
		}
	}
	if exists && req.RemoteContribution != nil {
		branchRef := "refs/heads/" + existing.Branch
		if err := m.validateContributionAncestor(ctx, req.RepositoryPath, req.RemoteContribution.HeadSHA, branchRef); err != nil {
			return nil, fmt.Errorf("existing contribution branch is not based on the validated source: %w", err)
		}
	}

	// Try to add worktree using existing branch
	usesGitCrypt, err := m.gitAddWorktreeForRecreate(ctx, req.RepositoryPath, existing.Branch, worktreePath, refreshedStartPoint)
	if err != nil {
		return nil, err
	}

	// If using git-crypt, unlock and checkout
	if usesGitCrypt {
		if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
			_ = m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath)
			return nil, err
		}
	} else {
		m.initSubmodules(ctx, worktreePath)
	}
	if contributionRemote != "" {
		if err := m.setUpstreamIfExistsRemote(ctx, worktreePath, existing.Branch, contributionRemote, req.RemoteContribution.HeadBranch); err != nil {
			_ = m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath)
			return nil, err
		}
	}

	// Update record. DeletedAt must be cleared alongside the status: archive
	// cleanup releases the reference (status=deleted + deleted_at), and
	// UpdateWorktree persists deleted_at verbatim — leaving it set would keep
	// the freshly recreated worktree invisible to every lookup that filters on
	// `deleted_at IS NULL`, so the next launch would mint a brand-new worktree
	// beside the one we just restored.
	now := time.Now()
	existing.Path = worktreePath
	existing.Status = StatusActive
	existing.DeletedAt = nil
	existing.UpdatedAt = now

	if m.store != nil {
		if err := m.store.UpdateWorktree(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update worktree record: %w", err)
		}
	}

	// Update cache keyed by (sessionID, repositoryID, branchSlug).
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(req.SessionID, req.RepositoryID, existing.BranchSlug)] = existing
		m.mu.Unlock()
	}

	m.logger.Info("recreated worktree",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath))

	return existing, nil
}
