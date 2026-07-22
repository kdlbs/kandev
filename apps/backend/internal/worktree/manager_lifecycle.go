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
	"github.com/kandev/kandev/internal/system/storage"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

// Create creates a new worktree for a session, or returns an existing one.
// Each session gets its own worktree for isolation. Checks by SessionID first,
// then by WorktreeID if provided (for session resumption).
// Only creates a new worktree if none exists for the session.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*Worktree, error) {
	if err := req.Validate(); err != nil {
		return nil, err
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
		return wt, err
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
			if m.IsValid(existing.Path) {
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
			if m.IsValid(existing.Path) {
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
	if req.PullBeforeWorktree {
		baseRef = m.pullBaseBranch(ctx, req.RepositoryPath, req.BaseBranch, req.OnSyncProgress)
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
	if fallback == "" || fallback == baseRef {
		return "", "", "", fmt.Errorf("%w: %s", ErrInvalidBaseBranch, baseRef)
	}
	// Best-effort fetch of the fallback so it is available locally in
	// containerized / shallow-clone environments where the fallback may
	// only exist on the remote. pullBaseBranch may resolve the name to a
	// remote-tracking ref (e.g. "main" -> "origin/main") which we must use
	// for the existence check and downstream git operations.
	resolvedFallback := fallback
	if req.PullBeforeWorktree {
		resolvedFallback = m.pullBaseBranch(ctx, req.RepositoryPath, fallback, nil)
	}
	fallbackExists, fallbackErr := m.branchExists(ctx, req.RepositoryPath, resolvedFallback)
	if fallbackErr != nil {
		return "", "", "", fmt.Errorf("could not verify fallback base branch %q: %w", resolvedFallback, fallbackErr)
	}
	if !fallbackExists {
		return "", "", "", fmt.Errorf("%w: %s (fallback %q also not found)", ErrInvalidBaseBranch, baseRef, fallback)
	}
	m.logger.Warn("requested base branch not found, falling back",
		zap.String("repository_path", req.RepositoryPath),
		zap.String("requested_branch", baseRef),
		zap.String("fallback_branch", fallback))
	// Use req.BaseBranch (the user-supplied name) in the user-facing warning
	// rather than baseRef, which may carry an internal "origin/<x>" form
	// produced by pullBaseBranch when PullBeforeWorktree is set.
	warning = fmt.Sprintf("Requested base branch %q not found, used %q instead", req.BaseBranch, fallback)
	detail = fmt.Sprintf("git rev-parse --verify %s failed; recovered using fallback branch %q (typically the repository's default_branch)", baseRef, fallback)
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
	if req.CheckoutBranch != "" {
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
			fetchResult, err = m.fetchBranchToLocal(ctx, req.RepositoryPath, req.CheckoutBranch, req.PRNumber)
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
	m.runWorktreeSetupScript(ctx, wt)

	m.logger.Info("created worktree in task directory",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("task_dir", req.TaskDirName),
		zap.String("repo_name", req.RepoName),
		zap.String("path", worktreePath),
		zap.String("branch", wt.Branch))

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
	id, err := m.gitAddWorktreeExisting(ctx, req.RepositoryPath, req.CheckoutBranch, worktreePath)
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
	WarningDetail string // Raw git command output for debugging
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
	m.logger.Info("syncing checkout branch",
		zap.String("branch", branch),
		zap.Int("pr_number", prNumber),
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
		if result := m.retryFetchAsRemoteTrackingRef(ctx, repoPath, branch, prNumber); result != nil {
			return result, nil
		}
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
		return nil, fmt.Errorf("branch %q not found locally or on remote: %s", branch, outputStr)
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
func (m *Manager) retryFetchAsRemoteTrackingRef(ctx context.Context, repoPath, branch string, prNumber int) *FetchBranchResult {
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
		// Fork PRs have no origin/<branch> ref — the bare
		// pull/<N>/head retry only updates FETCH_HEAD, so the local
		// `branch` (already populated by the prior worktree) is the
		// only valid start point. Empty StartPoint signals the
		// caller to fall back to req.CheckoutBranch.
		return &FetchBranchResult{}
	}
	return &FetchBranchResult{StartPoint: "origin/" + branch}
}

// gitAddWorktreeExisting creates a worktree that checks out an existing local branch.
// If the branch is already checked out in a stale worktree (directory no longer exists),
// it automatically prunes and retries. If the repository uses git-crypt, it creates
// the worktree without checkout, then unlocks git-crypt and performs the checkout.
func (m *Manager) gitAddWorktreeExisting(ctx context.Context, repoPath, branchName, worktreePath string) (string, error) {
	release, err := acquireWorktreeTargetPath(ctx, worktreePath)
	if err != nil {
		return "", fmt.Errorf("acquire worktree target path: %w", err)
	}
	defer release()
	return m.gitAddWorktreeExistingLocked(ctx, repoPath, branchName, worktreePath)
}

func (m *Manager) gitAddWorktreeExistingLocked(ctx context.Context, repoPath, branchName, worktreePath string) (string, error) {
	worktreeID := uuid.New().String()
	usesGitCrypt := m.usesGitCrypt(repoPath)

	// Build worktree add command
	args := []string{"worktree", "add"}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, branchName)

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
		return m.gitAddWorktreeExistingWithGitCrypt(ctx, repoPath, branchName, worktreePath)
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
	return m.retryWorktreeExisting(ctx, repoPath, branchName, worktreePath, usesGitCrypt)
}

// retryWorktreeExisting retries worktree creation after pruning stale worktrees.
func (m *Manager) retryWorktreeExisting(ctx context.Context, repoPath, branchName, worktreePath string, usesGitCrypt bool) (string, error) {
	worktreeID := uuid.New().String()

	args := []string{"worktree", "add"}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, branchName)

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
func (m *Manager) gitAddWorktreeExistingWithGitCrypt(ctx context.Context, repoPath, branchName, worktreePath string) (string, error) {
	worktreeID := uuid.New().String()

	cmd := newGitCommand(ctx, "worktree", "add", "--no-checkout", worktreePath, branchName)
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
	registrationOwned, inspectErr := inspectWorktreeRegistrationOwnership(
		cleanupCtx, repoPath, worktreePath, snapshot.branchRef, snapshot.branchOID,
	)
	if inspectErr != nil {
		m.logger.Warn("failed to verify partial worktree ownership; leaving path untouched",
			zap.String("path", worktreePath), zap.Error(inspectErr))
		return
	}
	if !registrationOwned {
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
	if err := m.removeWorktreeDir(cleanupCtx, worktreePath, repoPath); err != nil {
		m.logger.Warn("failed to roll back partial worktree",
			zap.String("path", worktreePath), zap.Error(err))
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
func (m *Manager) gitAddWorktreeForRecreate(ctx context.Context, repoPath, branch, worktreePath string) (bool, error) {
	release, err := acquireWorktreeTargetPath(ctx, worktreePath)
	if err != nil {
		return false, fmt.Errorf("acquire worktree target path: %w", err)
	}
	defer release()
	return m.gitAddWorktreeForRecreateLocked(ctx, repoPath, branch, worktreePath)
}

func (m *Manager) gitAddWorktreeForRecreateLocked(ctx context.Context, repoPath, branch, worktreePath string) (bool, error) {
	usesGitCrypt := m.usesGitCrypt(repoPath)
	args := []string{"worktree", "add"}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, branch)

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
		retryCmd := newGitCommand(ctx, "worktree", "add", "--no-checkout", worktreePath, branch)
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

// recreate recreates a worktree from stored metadata.
func (m *Manager) recreate(ctx context.Context, existing *Worktree, req CreateRequest) (*Worktree, error) {
	// Clean up existing directory if present
	if existing.Path != "" {
		if err := os.RemoveAll(existing.Path); err != nil {
			m.logger.Debug("failed to remove existing worktree path", zap.Error(err))
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

	// Reuse the original on-disk path so the worktree is recreated in the
	// same task-dir slot it was first created in.
	worktreePath := existing.Path
	if worktreePath == "" {
		return nil, fmt.Errorf("cannot recreate worktree: existing record has no path")
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
	if !exists {
		if _, fetchErr := m.fetchBranchToLocal(ctx, req.RepositoryPath, existing.Branch, req.PRNumber); fetchErr != nil {
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

	// Try to add worktree using existing branch
	usesGitCrypt, err := m.gitAddWorktreeForRecreate(ctx, req.RepositoryPath, existing.Branch, worktreePath)
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

	// Update record
	now := time.Now()
	existing.Path = worktreePath
	existing.Status = StatusActive
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
