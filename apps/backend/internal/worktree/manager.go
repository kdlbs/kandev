package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	defaultGitFetchTimeout   = 90 * time.Second
	defaultGitPullTimeout    = 60 * time.Second
	defaultGitInspectTimeout = 10 * time.Second
	gitNoTags                = "--no-tags"
)

// repoLockEntry tracks a repository lock and its reference count.
type repoLockEntry struct {
	mu       *sync.Mutex
	refCount int
}

// Manager handles Git worktree operations for concurrent agent execution.
type Manager struct {
	config Config
	logger *logger.Logger
	store  Store
	// worktrees is the in-memory cache keyed by cacheKey(sessionID, repositoryID).
	// For legacy single-repo writes the repositoryID may be empty, in which
	// case the cache key collapses to "{sessionID}|" — still distinct from
	// any per-repo entry the same session might gain later.
	worktrees  map[string]*Worktree
	mu         sync.RWMutex // Protects worktrees map
	repoLocks  map[string]*repoLockEntry
	repoLockMu sync.Mutex

	// Optional dependencies for script execution
	repoProvider     RepositoryProvider
	scriptMsgHandler ScriptMessageHandler

	// Timeouts for best-effort remote sync before creating a worktree.
	fetchTimeout time.Duration
	pullTimeout  time.Duration
	// Bound for cheap git ref-inspection commands (branchExists, currentBranch).
	inspectTimeout time.Duration
}

// ScriptMessageHandler provides script execution and message streaming.
type ScriptMessageHandler interface {
	ExecuteSetupScript(ctx context.Context, req ScriptExecutionRequest) error
	ExecuteCleanupScript(ctx context.Context, req ScriptExecutionRequest) error
}

// Store is the interface for worktree persistence.
type Store interface {
	// CreateWorktree persists a new worktree record.
	CreateWorktree(ctx context.Context, wt *Worktree) error
	// GetWorktreeByID retrieves a worktree by its unique ID.
	GetWorktreeByID(ctx context.Context, id string) (*Worktree, error)
	// GetWorktreeBySessionID retrieves a single active worktree by session ID.
	// For multi-repo sessions, returns the first one found (typically the
	// primary/first-created). Use GetWorktreesBySessionID when the caller
	// needs all of them.
	GetWorktreeBySessionID(ctx context.Context, sessionID string) (*Worktree, error)
	// GetWorktreesByTaskID retrieves all worktrees for a task (used for cleanup on task deletion).
	GetWorktreesByTaskID(ctx context.Context, taskID string) ([]*Worktree, error)
	// GetWorktreesByRepositoryID retrieves all worktrees for a repository.
	GetWorktreesByRepositoryID(ctx context.Context, repoID string) ([]*Worktree, error)
	// UpdateWorktree updates an existing worktree record.
	UpdateWorktree(ctx context.Context, wt *Worktree) error
	// DeleteWorktree removes a worktree record.
	DeleteWorktree(ctx context.Context, id string) error
	// ListActiveWorktrees returns all active worktrees.
	ListActiveWorktrees(ctx context.Context) ([]*Worktree, error)
}

// MultiRepoStore is an optional capability some stores implement to support
// multi-repo task sessions (one worktree per repository per session). The
// Manager checks at runtime whether its Store satisfies this interface and
// uses the multi-repo lookups when available.
type MultiRepoStore interface {
	// GetWorktreesBySessionID returns all active worktrees for the session.
	GetWorktreesBySessionID(ctx context.Context, sessionID string) ([]*Worktree, error)
	// GetWorktreeBySessionAndRepository returns the active worktree for the
	// given (session, repository) pair, or nil if none exists.
	GetWorktreeBySessionAndRepository(ctx context.Context, sessionID, repositoryID string) (*Worktree, error)
}

// NewManager creates a new worktree manager.
func NewManager(cfg Config, store Store, log *logger.Logger) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.Default()
	}

	// Ensure tasks base directory exists (if configured)
	if cfg.TasksBasePath != "" {
		tasksBase, err := cfg.ExpandedTasksBasePath()
		if err != nil {
			return nil, fmt.Errorf("failed to expand tasks base path: %w", err)
		}
		if err := os.MkdirAll(tasksBase, 0755); err != nil {
			return nil, fmt.Errorf("failed to create tasks base directory: %w", err)
		}
	}

	fetchTimeout := defaultGitFetchTimeout
	if cfg.FetchTimeoutSeconds > 0 {
		fetchTimeout = time.Duration(cfg.FetchTimeoutSeconds) * time.Second
	}

	pullTimeout := defaultGitPullTimeout
	if cfg.PullTimeoutSeconds > 0 {
		pullTimeout = time.Duration(cfg.PullTimeoutSeconds) * time.Second
	}

	return &Manager{
		config:         cfg,
		logger:         log.WithFields(zap.String("component", "worktree-manager")),
		store:          store,
		worktrees:      make(map[string]*Worktree),
		repoLocks:      make(map[string]*repoLockEntry),
		fetchTimeout:   fetchTimeout,
		pullTimeout:    pullTimeout,
		inspectTimeout: defaultGitInspectTimeout,
	}, nil
}

// SetRepositoryProvider sets the repository provider for fetching repository information.
func (m *Manager) SetRepositoryProvider(provider RepositoryProvider) {
	m.repoProvider = provider
}

// SetScriptMessageHandler sets the script message handler for executing setup/cleanup scripts.
func (m *Manager) SetScriptMessageHandler(handler ScriptMessageHandler) {
	m.scriptMsgHandler = handler
}

// getRepoLock returns a mutex for the given repository path and increments its reference count.
func (m *Manager) getRepoLock(repoPath string) *sync.Mutex {
	m.repoLockMu.Lock()
	defer m.repoLockMu.Unlock()

	if entry, exists := m.repoLocks[repoPath]; exists {
		entry.refCount++
		return entry.mu
	}

	entry := &repoLockEntry{
		mu:       &sync.Mutex{},
		refCount: 1,
	}
	m.repoLocks[repoPath] = entry
	return entry.mu
}

// releaseRepoLock decrements the reference count for a repository lock.
// If the count reaches zero, the lock is removed from the map to prevent memory leaks.
func (m *Manager) releaseRepoLock(repoPath string) {
	m.repoLockMu.Lock()
	defer m.repoLockMu.Unlock()

	entry, exists := m.repoLocks[repoPath]
	if !exists {
		return
	}

	entry.refCount--
	if entry.refCount <= 0 {
		delete(m.repoLocks, repoPath)
		m.logger.Debug("released repository lock",
			zap.String("repository_path", repoPath))
	}
}

// IsEnabled returns whether worktree mode is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// Create creates a new worktree for a session, or returns an existing one.
// Each session gets its own worktree for isolation. Checks by SessionID first,
// then by WorktreeID if provided (for session resumption).
// Only creates a new worktree if none exists for the session.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*Worktree, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// First, check if a worktree already exists for this (session, repository) pair.
	// Multi-repo sessions can host multiple worktrees concurrently, so we must
	// scope the lookup by RepositoryID rather than session alone — otherwise
	// the second repo's Create would return the first repo's worktree.
	if req.SessionID != "" {
		existing, err := m.GetBySessionAndRepo(ctx, req.SessionID, req.RepositoryID)
		if err == nil && existing != nil {
			if m.IsValid(existing.Path) {
				m.logger.Debug("reusing existing worktree by session+repo",
					zap.String("worktree_id", existing.ID),
					zap.String("session_id", req.SessionID),
					zap.String("repository_id", req.RepositoryID),
					zap.String("task_id", req.TaskID),
					zap.String("path", existing.Path))
				return existing, nil
			}
			// Worktree record exists but directory is invalid - recreate
			m.logger.Warn("worktree directory invalid, recreating",
				zap.String("worktree_id", existing.ID),
				zap.String("session_id", req.SessionID),
				zap.String("repository_id", req.RepositoryID),
				zap.String("task_id", req.TaskID))
			return m.recreate(ctx, existing, req)
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
				return existing, nil
			}
			// Worktree record exists but directory is invalid - recreate
			m.logger.Warn("worktree directory invalid, recreating",
				zap.String("worktree_id", req.WorktreeID),
				zap.String("session_id", req.SessionID),
				zap.String("task_id", req.TaskID))
			return m.recreate(ctx, existing, req)
		}
		// WorktreeID provided but not found - fall through to create new
		m.logger.Warn("worktree ID not found, creating new worktree",
			zap.String("worktree_id", req.WorktreeID),
			zap.String("session_id", req.SessionID),
			zap.String("task_id", req.TaskID))
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

	baseRef := req.BaseBranch
	if req.PullBeforeWorktree {
		baseRef = m.pullBaseBranch(ctx, req.RepositoryPath, req.BaseBranch, req.OnSyncProgress)
	}

	// Check base branch exists. When the requested branch is missing and the
	// caller supplied a FallbackBaseBranch (typically the repo's default_branch)
	// that does exist, recover automatically and surface a non-fatal warning on
	// the resulting worktree. This handles the common case where a parent task
	// was anchored to a PR head or fresh branch that has been deleted upstream
	// or never existed in a sibling repo.
	fallbackWarning, fallbackDetail := "", ""
	if !m.branchExists(ctx, req.RepositoryPath, baseRef) {
		fallback := strings.TrimSpace(req.FallbackBaseBranch)
		if fallback == "" || fallback == baseRef {
			return nil, fmt.Errorf("%w: %s", ErrInvalidBaseBranch, baseRef)
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
		if !m.branchExists(ctx, req.RepositoryPath, resolvedFallback) {
			return nil, fmt.Errorf("%w: %s (fallback %q also not found)", ErrInvalidBaseBranch, baseRef, fallback)
		}
		m.logger.Warn("requested base branch not found, falling back",
			zap.String("repository_path", req.RepositoryPath),
			zap.String("requested_branch", baseRef),
			zap.String("fallback_branch", fallback))
		// Use req.BaseBranch (the user-supplied name) in the user-facing warning
		// rather than baseRef, which may carry an internal "origin/<x>" form
		// produced by pullBaseBranch when PullBeforeWorktree is set.
		fallbackWarning = fmt.Sprintf("Requested base branch %q not found, used %q instead", req.BaseBranch, fallback)
		fallbackDetail = fmt.Sprintf("git rev-parse --verify %s failed; recovered using fallback branch %q (typically the repository's default_branch)", baseRef, fallback)
		baseRef = resolvedFallback
		// Reflect the resolved branch in the persisted worktree record so
		// downstream consumers (UI, queries, debug logs) see the actual base
		// rather than the requested-but-missing one.
		req.BaseBranch = fallback
	}

	// Worktrees are always placed under ~/.kandev/tasks/{taskDir}/{repo}/.
	// Callers must populate TaskDirName and RepoName; the legacy flat layout
	// has been removed, so a missing field is a programming error.
	if req.TaskDirName == "" || req.RepoName == "" {
		return nil, ErrTaskDirRequired
	}
	wt, err := m.createInTaskDir(ctx, req, baseRef)
	if err != nil {
		return nil, err
	}
	if fallbackWarning != "" {
		wt.BaseBranchFallbackWarning = fallbackWarning
		wt.BaseBranchFallbackDetail = fallbackDetail
	}
	return wt, nil
}

// createInTaskDir creates a worktree inside the task directory structure:
// ~/.kandev/tasks/{taskDirName}/{repoName}/
//
// RepoName is sanitized to a single path segment so display names like
// "owner/repo" don't produce a nested subdirectory — that would push the
// worktree one level below the task root and break agentctl's sibling-based
// multi-repo detection.
func (m *Manager) createInTaskDir(ctx context.Context, req CreateRequest, baseRef string) (*Worktree, error) {
	repoDir := SanitizeRepoDirName(req.RepoName)
	if repoDir == "" {
		return nil, ErrInvalidRepoName
	}
	worktreePath, err := m.config.TaskWorktreePath(req.TaskDirName, repoDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get task worktree path: %w", err)
	}

	// Ensure parent task directory exists
	taskDir := filepath.Dir(worktreePath)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task directory: %w", err)
	}

	_, branchName := m.buildWorktreeNames(req)
	startPoint := baseRef

	var fetchResult *FetchBranchResult
	if req.CheckoutBranch != "" {
		fetchResult, err = m.fetchBranchToLocal(ctx, req.RepositoryPath, req.CheckoutBranch)
		if err != nil {
			return nil, err
		}
		if fetchResult.StartPoint != "" {
			startPoint = fetchResult.StartPoint
		} else {
			startPoint = req.CheckoutBranch
		}
	}

	worktreeID, branchName, err := m.addWorktreeForBranch(ctx, req, worktreePath, branchName, startPoint, baseRef)
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

	if err := m.runWorktreeSetupScript(ctx, wt, req.RepositoryPath); err != nil {
		return nil, err
	}

	m.logger.Info("created worktree in task directory",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("task_dir", req.TaskDirName),
		zap.String("repo_name", req.RepoName),
		zap.String("path", worktreePath),
		zap.String("branch", wt.Branch))

	return wt, nil
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
	if err := verifyCmd.Run(); err != nil {
		return
	}
	cmd := m.newNonInteractiveGitCmd(ctx, worktreePath, "branch", "--set-upstream-to="+upstream, localBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
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
func (m *Manager) fetchBranchToLocal(ctx context.Context, repoPath, branch string) (*FetchBranchResult, error) {
	m.logger.Info("syncing checkout branch",
		zap.String("branch", branch),
		zap.String("repo_path", repoPath))

	// Try to fetch from origin to get the latest version.
	fetchCtx, cancelFetch := context.WithTimeout(ctx, m.fetchTimeout)
	defer cancelFetch()

	fetchCmd := m.newNonInteractiveGitCmd(fetchCtx, repoPath, "fetch", gitNoTags, "origin", branch+":"+branch)
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		outputStr := string(output)

		// If the branch is checked out in another worktree, git refuses to update
		// the local ref. Retry by fetching only the remote-tracking ref (origin/branch),
		// which is always safe regardless of worktree state.
		if isFetchRefusedCheckedOut(outputStr) {
			retryCmd := m.newNonInteractiveGitCmd(fetchCtx, repoPath, "fetch", gitNoTags, "origin", branch)
			if _, retryErr := retryCmd.CombinedOutput(); retryErr == nil {
				m.logger.Info("fetched via remote-tracking ref (branch checked out elsewhere)",
					zap.String("branch", branch))
				return &FetchBranchResult{StartPoint: "origin/" + branch}, nil
			}
		}

		m.logger.Warn("fetch from origin failed, checking local branch",
			zap.String("branch", branch),
			zap.String("output", outputStr),
			zap.Error(err))

		// Fall back to local branch if it exists.
		if !m.branchExists(ctx, repoPath, branch) {
			return nil, fmt.Errorf("branch %q not found locally or on remote: %s", branch, outputStr)
		}

		reason := classifyGitFallbackReason(err, outputStr, fetchCtx.Err())
		warning := fmt.Sprintf("Could not fetch latest from origin (%s). Using local version of branch %q which may be outdated.", reason, branch)
		m.logger.Info("using local branch (fetch failed)",
			zap.String("branch", branch),
			zap.String("warning", warning))
		return &FetchBranchResult{
			Warning:       warning,
			WarningDetail: strings.TrimSpace(outputStr),
		}, nil
	}

	return &FetchBranchResult{}, nil
}

// gitAddWorktreeExisting creates a worktree that checks out an existing local branch.
// If the branch is already checked out in a stale worktree (directory no longer exists),
// it automatically prunes and retries. If the repository uses git-crypt, it creates
// the worktree without checkout, then unlocks git-crypt and performs the checkout.
func (m *Manager) gitAddWorktreeExisting(ctx context.Context, repoPath, branchName, worktreePath string) (string, error) {
	worktreeID := uuid.New().String()
	usesGitCrypt := m.usesGitCrypt(repoPath)

	// Build worktree add command
	args := []string{"worktree", "add"}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, branchName)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
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

	retryCmd := exec.CommandContext(ctx, "git", args...)
	retryCmd.Dir = repoPath
	retryOutput, retryErr := retryCmd.CombinedOutput()
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

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--no-checkout", worktreePath, branchName)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
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

	pruneCmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	pruneCmd.Dir = repoPath
	if output, err := pruneCmd.CombinedOutput(); err != nil {
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
	prefix := NormalizeBranchPrefix(req.WorktreeBranchPrefix)

	if req.TaskTitle != "" {
		// Use semantic naming: {sanitized-title}_{suffix}
		dirName = SemanticWorktreeName(req.TaskTitle, dirSuffix)
		branchName = TaskBranchNameWithSuffix(req.TaskTitle, req.TaskID, prefix, branchSuffix)
	} else {
		// Fallback to task ID based naming
		dirName = req.TaskID + "_" + dirSuffix
		branchName = TaskBranchNameWithSuffix("", req.TaskID, prefix, branchSuffix)
	}
	return dirName, branchName
}

// gitAddWorktree runs "git worktree add" and returns the new worktree UUID.
// If the repository uses git-crypt, it creates the worktree without checkout,
// then unlocks git-crypt and performs the checkout separately.
func (m *Manager) gitAddWorktree(ctx context.Context, repoPath, branchName, worktreePath, baseRef string) (string, error) {
	worktreeID := uuid.New().String()
	usesGitCrypt := m.usesGitCrypt(repoPath)

	// Build worktree add command.
	// Use -c branch.autoSetupMerge=false to prevent git from automatically
	// setting upstream tracking when the base ref is a remote-tracking branch
	// (e.g. origin/feature/foo). New task branches should start with no
	// upstream until the user explicitly pushes.
	args := []string{"-c", "branch.autoSetupMerge=false", "worktree", "add", "-b", branchName}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, baseRef)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(output)
		// Check if this is a git-crypt error we didn't anticipate
		if isGitCryptSmudgeError(outStr) {
			m.logger.Warn("git-crypt smudge error detected, retrying with --no-checkout",
				zap.String("output", outStr))
			return m.gitAddWorktreeWithGitCrypt(ctx, repoPath, branchName, worktreePath, baseRef)
		}
		m.logger.Error("git worktree add failed",
			zap.String("output", outStr),
			zap.Error(err))
		return "", fmt.Errorf("%w: %s", ErrGitCommandFailed, outStr)
	}

	// If we used --no-checkout, we need to unlock git-crypt and checkout
	if usesGitCrypt {
		if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
			// Cleanup the worktree on failure
			_ = m.removeWorktreeDir(ctx, worktreePath, repoPath)
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
func (m *Manager) gitAddWorktreeWithGitCrypt(ctx context.Context, repoPath, branchName, worktreePath, baseRef string) (string, error) {
	worktreeID := uuid.New().String()

	// Create worktree without checkout.
	// Use -c branch.autoSetupMerge=false to prevent git from automatically
	// setting upstream tracking when the base ref is a remote-tracking branch.
	cmd := exec.CommandContext(ctx, "git",
		"-c", "branch.autoSetupMerge=false",
		"worktree", "add",
		"-b", branchName,
		"--no-checkout",
		worktreePath,
		baseRef)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Error("git worktree add (--no-checkout) failed",
			zap.String("output", string(output)),
			zap.Error(err))
		return "", fmt.Errorf("%w: %s", ErrGitCommandFailed, string(output))
	}

	// Unlock git-crypt and checkout
	if err := m.unlockGitCryptAndCheckout(ctx, worktreePath); err != nil {
		_ = m.removeWorktreeDir(ctx, worktreePath, repoPath)
		return "", err
	}

	return worktreeID, nil
}

// buildWorktreeRecord constructs an in-memory Worktree value from a completed git worktree add.
func (m *Manager) buildWorktreeRecord(worktreeID string, req CreateRequest, worktreePath, branchName string) *Worktree {
	now := time.Now()
	return &Worktree{
		ID:             worktreeID,
		SessionID:      req.SessionID,
		TaskID:         req.TaskID,
		RepositoryID:   req.RepositoryID,
		RepositoryPath: req.RepositoryPath,
		Path:           worktreePath,
		Branch:         branchName,
		BaseBranch:     req.BaseBranch,
		Status:         StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// persistAndCacheWorktree saves the worktree to the store and updates the in-memory cache.
func (m *Manager) persistAndCacheWorktree(ctx context.Context, wt *Worktree, req CreateRequest, worktreePath string) error {
	if m.store != nil {
		if err := m.persistWorktree(ctx, wt, req, worktreePath); err != nil {
			return err
		}
	}

	// Update cache keyed by (sessionID, repositoryID) so multi-repo sessions
	// can hold multiple entries.
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(req.SessionID, req.RepositoryID)] = wt
		m.mu.Unlock()
	}
	return nil
}

// persistWorktree writes the worktree to persistent storage, logging a warning when
// session_id is missing and cleaning up the git worktree directory on failure.
func (m *Manager) persistWorktree(ctx context.Context, wt *Worktree, req CreateRequest, worktreePath string) error {
	if req.SessionID == "" {
		m.logger.Warn("skipping worktree persistence: missing session_id",
			zap.String("task_id", req.TaskID),
			zap.String("worktree_id", wt.ID))
		return nil
	}
	if err := m.store.CreateWorktree(ctx, wt); err != nil {
		// Cleanup git worktree on store failure
		if cleanupErr := m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath); cleanupErr != nil {
			m.logger.Warn("failed to cleanup worktree after persist failure", zap.Error(cleanupErr))
		}
		return fmt.Errorf("failed to persist worktree: %w", err)
	}
	return nil
}

// cacheKey computes the cache key for a (sessionID, repositoryID) pair.
// Used by all read/write paths against m.worktrees so multi-repo sessions
// can hold multiple worktrees concurrently.
func cacheKey(sessionID, repositoryID string) string {
	return sessionID + "|" + repositoryID
}

// firstCacheEntryForSession scans the cache for any entry belonging to
// sessionID. Returns the first match or nil. Caller must hold m.mu.
func (m *Manager) firstCacheEntryForSession(sessionID string) *Worktree {
	prefix := sessionID + "|"
	for k, wt := range m.worktrees {
		if strings.HasPrefix(k, prefix) {
			return wt
		}
	}
	return nil
}

// GetBySessionID returns one worktree for the session. For multi-repo sessions
// it returns whichever worktree happens to come first; callers that need a
// specific repo's worktree should use GetBySessionAndRepo, and callers that
// need them all should use GetAllBySessionID.
func (m *Manager) GetBySessionID(ctx context.Context, sessionID string) (*Worktree, error) {
	// Check cache first
	m.mu.RLock()
	if wt := m.firstCacheEntryForSession(sessionID); wt != nil {
		m.mu.RUnlock()
		return wt, nil
	}
	m.mu.RUnlock()

	// Check store
	if m.store != nil {
		wt, err := m.store.GetWorktreeBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if wt != nil {
			// Update cache under the wt's repository key (or empty for legacy
			// records that never had one).
			m.mu.Lock()
			m.worktrees[cacheKey(sessionID, wt.RepositoryID)] = wt
			m.mu.Unlock()
			return wt, nil
		}
	}

	return nil, ErrWorktreeNotFound
}

// GetAllBySessionID returns all active worktrees for a session. Empty slice
// (not error) if the session has none.
func (m *Manager) GetAllBySessionID(ctx context.Context, sessionID string) ([]*Worktree, error) {
	if m.store == nil {
		// Cache-only path: scan our in-memory map.
		m.mu.RLock()
		defer m.mu.RUnlock()
		prefix := sessionID + "|"
		var out []*Worktree
		for k, wt := range m.worktrees {
			if strings.HasPrefix(k, prefix) {
				out = append(out, wt)
			}
		}
		return out, nil
	}
	if multi, ok := m.store.(MultiRepoStore); ok {
		wts, err := multi.GetWorktreesBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		// Refresh cache while we're at it.
		m.mu.Lock()
		for _, wt := range wts {
			m.worktrees[cacheKey(sessionID, wt.RepositoryID)] = wt
		}
		m.mu.Unlock()
		return wts, nil
	}
	// Single-repo store fallback: at most one worktree per session.
	wt, err := m.store.GetWorktreeBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if wt == nil {
		return nil, nil
	}
	return []*Worktree{wt}, nil
}

// GetBySessionAndRepo returns the active worktree for the (session, repo)
// pair, or ErrWorktreeNotFound if none exists. Falls back to GetBySessionID
// when the store does not implement MultiRepoStore (legacy single-repo).
func (m *Manager) GetBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*Worktree, error) {
	if sessionID == "" {
		return nil, ErrWorktreeNotFound
	}
	// Check cache first.
	m.mu.RLock()
	if wt, ok := m.worktrees[cacheKey(sessionID, repositoryID)]; ok {
		m.mu.RUnlock()
		return wt, nil
	}
	m.mu.RUnlock()

	if m.store == nil {
		return nil, ErrWorktreeNotFound
	}
	if multi, ok := m.store.(MultiRepoStore); ok {
		wt, err := multi.GetWorktreeBySessionAndRepository(ctx, sessionID, repositoryID)
		if err != nil {
			return nil, err
		}
		if wt == nil {
			return nil, ErrWorktreeNotFound
		}
		m.mu.Lock()
		m.worktrees[cacheKey(sessionID, repositoryID)] = wt
		m.mu.Unlock()
		return wt, nil
	}
	// Legacy single-repo store: best we can do is the session lookup.
	wt, err := m.store.GetWorktreeBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if wt == nil || (repositoryID != "" && wt.RepositoryID != repositoryID) {
		return nil, ErrWorktreeNotFound
	}
	return wt, nil
}

// GetByID returns a worktree by its unique ID.
func (m *Manager) GetByID(ctx context.Context, worktreeID string) (*Worktree, error) {
	if m.store == nil {
		return nil, ErrWorktreeNotFound
	}

	wt, err := m.store.GetWorktreeByID(ctx, worktreeID)
	if err != nil {
		return nil, err
	}
	if wt == nil {
		return nil, ErrWorktreeNotFound
	}
	return wt, nil
}

// GetAllByTaskID returns all worktrees for a task.
func (m *Manager) GetAllByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	if m.store == nil {
		return nil, nil
	}
	return m.store.GetWorktreesByTaskID(ctx, taskID)
}

// IsValid checks if a worktree directory is valid and usable.
func (m *Manager) IsValid(path string) bool {
	// Check directory exists
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check .git file exists (worktrees have .git file, not directory)
	gitFile := filepath.Join(path, ".git")
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return false
	}

	// .git file should contain "gitdir: <path>"
	if !strings.HasPrefix(string(content), "gitdir:") {
		return false
	}

	return true
}

// RemoveByID removes a specific worktree by its ID and optionally its branch.
func (m *Manager) RemoveByID(ctx context.Context, worktreeID string, removeBranch bool) error {
	wt, err := m.GetByID(ctx, worktreeID)
	if err != nil {
		return err
	}
	return m.removeWorktree(ctx, wt, removeBranch)
}

// removeWorktree performs the actual removal of a worktree.
func (m *Manager) removeWorktree(ctx context.Context, wt *Worktree, removeBranch bool) error {
	// Get repository lock
	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(wt.RepositoryPath)
	}()

	// Execute cleanup script BEFORE removing directory
	m.runWorktreeCleanupScript(ctx, wt)

	// Remove worktree directory
	if err := m.removeWorktreeDir(ctx, wt.Path, wt.RepositoryPath); err != nil {
		m.logger.Warn("failed to remove worktree directory",
			zap.String("path", wt.Path),
			zap.Error(err))
	}

	// Optionally remove the branch from the main repository
	if removeBranch {
		m.logger.Info("deleting branch from main repository",
			zap.String("branch", wt.Branch),
			zap.String("repository_path", wt.RepositoryPath))

		cmd := exec.CommandContext(ctx, "git", "branch", "-D", wt.Branch)
		cmd.Dir = wt.RepositoryPath
		if output, err := cmd.CombinedOutput(); err != nil {
			m.logger.Warn("failed to delete branch from main repository",
				zap.String("branch", wt.Branch),
				zap.String("repository_path", wt.RepositoryPath),
				zap.String("output", string(output)),
				zap.Error(err))
		} else {
			m.logger.Info("successfully deleted branch from main repository",
				zap.String("branch", wt.Branch),
				zap.String("repository_path", wt.RepositoryPath))
		}
	}

	// Update store
	if m.store != nil {
		now := time.Now()
		wt.Status = StatusDeleted
		wt.DeletedAt = &now
		wt.UpdatedAt = now
		if err := m.store.UpdateWorktree(ctx, wt); err != nil {
			// Record may already be deleted by another cleanup path (e.g. task deletion).
			// This is expected and harmless - only log at debug level.
			m.logger.Debug("failed to update worktree status (may already be deleted)",
				zap.String("worktree_id", wt.ID),
				zap.Error(err))
		}
	}

	// Update cache: delete the (session, repo) entry. Removing a worktree only
	// affects its own repo; siblings on other repos must remain cached.
	m.mu.Lock()
	if wt.SessionID != "" {
		delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID))
	}
	m.mu.Unlock()

	m.logger.Info("removed worktree",
		zap.String("task_id", wt.TaskID),
		zap.String("worktree_id", wt.ID),
		zap.String("path", wt.Path),
		zap.Bool("branch_removed", removeBranch))

	return nil
}

func (m *Manager) runWorktreeSetupScript(ctx context.Context, wt *Worktree, repositoryPath string) error {
	if m.scriptMsgHandler == nil || m.repoProvider == nil {
		return nil
	}
	if wt.RepositoryID == "" {
		// Nothing to set up without a linked repository; upstream may not
		// always populate this field.
		return nil
	}
	repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		m.logger.Warn("failed to fetch repository for setup script",
			zap.String("repository_id", wt.RepositoryID),
			zap.Error(err))
		return nil
	}
	if strings.TrimSpace(repo.SetupScript) == "" {
		return nil
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
	}
	if err := m.scriptMsgHandler.ExecuteSetupScript(ctx, scriptReq); err != nil {
		m.logger.Error("setup script failed, cleaning up worktree",
			zap.String("worktree_id", wt.ID),
			zap.Error(err))
		m.cleanupWorktreeOnSetupFailure(ctx, wt, repositoryPath)
		return fmt.Errorf("setup script failed: %w", err)
	}
	m.logger.Info("setup script completed successfully", zap.String("worktree_id", wt.ID))
	return nil
}

// cleanupWorktreeOnSetupFailure removes the in-memory cache entry, deletes the worktree
// directory, and marks the worktree as deleted in the store after a setup script failure.
func (m *Manager) cleanupWorktreeOnSetupFailure(ctx context.Context, wt *Worktree, repositoryPath string) {
	if wt.SessionID != "" {
		m.mu.Lock()
		delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID))
		m.mu.Unlock()
	}
	if cleanupErr := m.removeWorktreeDir(ctx, wt.Path, repositoryPath); cleanupErr != nil {
		m.logger.Warn("failed to cleanup worktree after setup failure", zap.Error(cleanupErr))
	}
	if m.store == nil {
		return
	}
	now := time.Now()
	wt.Status = StatusDeleted
	wt.DeletedAt = &now
	wt.UpdatedAt = now
	if updateErr := m.store.UpdateWorktree(ctx, wt); updateErr != nil {
		m.logger.Warn("failed to update worktree status", zap.Error(updateErr))
	}
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

// CleanupWorktrees removes provided worktrees without re-fetching from the store.
func (m *Manager) CleanupWorktrees(ctx context.Context, worktrees []*Worktree) error {
	if len(worktrees) == 0 {
		return nil
	}

	var lastErr error
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if err := m.removeWorktree(ctx, wt, true); err != nil {
			m.logger.Warn("failed to remove worktree on task deletion",
				zap.String("task_id", wt.TaskID),
				zap.String("worktree_id", wt.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	m.mu.Lock()
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if wt.SessionID != "" {
			delete(m.worktrees, cacheKey(wt.SessionID, wt.RepositoryID))
		}
	}
	m.mu.Unlock()

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

// isGitRepo checks if a path is a Git repository.
func (m *Manager) isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	// .git can be either a directory (regular repo) or a file (worktree)
	return info.IsDir() || info.Mode().IsRegular()
}

// branchExists checks if a branch exists in the repository.
// Bounded by m.inspectTimeout so a hung git (credential prompt, stuck filter,
// filesystem stall) cannot deadlock the caller while holding repoLock. When
// the bound fires, the ctx error is logged so the root cause is visible in
// logs rather than surfacing only as a misleading "branch not found".
func (m *Manager) branchExists(ctx context.Context, repoPath, branch string) bool {
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, "rev-parse", "--verify", branch)
	if err := cmd.Run(); err != nil {
		if ctxErr := inspectCtx.Err(); ctxErr != nil {
			m.logger.Warn("branchExists bounded by context",
				zap.String("repository_path", repoPath),
				zap.String("branch", branch),
				zap.Error(ctxErr))
		}
		return false
	}
	return true
}

func (m *Manager) currentBranch(ctx context.Context, repoPath string) string {
	inspectCtx, cancel := context.WithTimeout(ctx, m.inspectTimeout)
	defer cancel()
	cmd := m.newNonInteractiveGitCmd(inspectCtx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		if ctxErr := inspectCtx.Err(); ctxErr != nil {
			m.logger.Warn("currentBranch bounded by context",
				zap.String("repository_path", repoPath),
				zap.Error(ctxErr))
		}
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (m *Manager) newNonInteractiveGitCmd(ctx context.Context, repoPath string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
		"GIT_ASKPASS=echo",
		"SSH_ASKPASS=/bin/false",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
	)
	// After the context cancels and the process is killed, child processes
	// (e.g. credential helpers) may still hold stdout/stderr pipes open.
	// WaitDelay bounds how long CombinedOutput waits for those pipes to close.
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd
}

func classifyGitFallbackReason(cmdErr error, cmdOutput string, ctxErr error) string {
	if errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(cmdErr, context.DeadlineExceeded) {
		return "timeout"
	}

	if containsAuthFailure(strings.ToLower(cmdOutput)) {
		return "non_interactive_auth_failed"
	}

	return "git_command_failed"
}

// pullBaseBranch fetches the latest changes from origin and returns the best ref to use
// for creating a new worktree. The function handles three scenarios:
//
//  1. baseBranch is already a remote ref (e.g., "origin/main"): fetch and use it directly
//  2. baseBranch is a local branch and we're currently on it: pull --ff-only to update
//  3. baseBranch is a local branch but we're on a different branch: use origin/<branch> instead
//
// On fetch/pull failure, errors are logged but the function continues with the best available ref.
func (m *Manager) pullBaseBranch(ctx context.Context, repoPath, baseBranch string, onProgress SyncProgressCallback) string {
	localBranch := strings.TrimPrefix(baseBranch, "origin/")
	isRemoteRef := localBranch != baseBranch
	stepName := "Sync base branch"

	m.reportSyncProgress(onProgress, SyncProgressEvent{
		StepName: stepName,
		Status:   SyncProgressRunning,
		Output:   fmt.Sprintf("Fetching latest changes for %s", baseBranch),
	})

	// Fetch branch from origin in non-interactive mode.
	fetchCtx, cancelFetch := context.WithTimeout(ctx, m.fetchTimeout)
	defer cancelFetch()

	fetchArgs := []string{"fetch", gitNoTags, "origin"}
	if localBranch != "" {
		fetchArgs = append(fetchArgs, localBranch)
	}
	fetchCmd := m.newNonInteractiveGitCmd(fetchCtx, repoPath, fetchArgs...)
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		return m.handleFetchFallback(baseBranch, stepName, onProgress, fetchCtx.Err(), output, err)
	}

	if isRemoteRef {
		resolved := "origin/" + localBranch
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", resolved), "")
		return resolved
	}

	return m.resolveLocalBaseRef(ctx, repoPath, baseBranch, localBranch, stepName, onProgress)
}

func (m *Manager) reportSyncProgress(cb SyncProgressCallback, event SyncProgressEvent) {
	if cb != nil {
		cb(event)
	}
}

func (m *Manager) reportSyncCompleted(stepName string, onProgress SyncProgressCallback, output, errOutput string) {
	m.reportSyncProgress(onProgress, SyncProgressEvent{
		StepName: stepName,
		Status:   SyncProgressCompleted,
		Output:   output,
		Error:    strings.TrimSpace(errOutput),
	})
}

func (m *Manager) handleFetchFallback(baseBranch, stepName string, onProgress SyncProgressCallback, ctxErr error, output []byte, cmdErr error) string {
	reason := classifyGitFallbackReason(cmdErr, string(output), ctxErr)
	m.logger.Warn("git fetch failed before worktree creation; continuing with fallback ref",
		zap.String("branch", baseBranch),
		zap.String("reason", reason),
		zap.String("fallback_ref", baseBranch),
		zap.String("output", string(output)),
		zap.Error(cmdErr))
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Fetch %s; using fallback ref %s", reason, baseBranch), string(output))
	return baseBranch
}

func (m *Manager) resolveLocalBaseRef(
	ctx context.Context, repoPath, baseBranch, localBranch, stepName string,
	onProgress SyncProgressCallback,
) string {
	remoteRef := "origin/" + localBranch
	if m.currentBranch(ctx, repoPath) == baseBranch {
		return m.pullCurrentBranchOrFallback(ctx, repoPath, baseBranch, remoteRef, stepName, onProgress)
	}
	if m.branchExists(ctx, repoPath, remoteRef) {
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", remoteRef), "")
		return remoteRef
	}
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Remote ref not found; using %s", baseBranch), "")
	return baseBranch
}

func (m *Manager) pullCurrentBranchOrFallback(
	ctx context.Context, repoPath, baseBranch, remoteRef, stepName string,
	onProgress SyncProgressCallback,
) string {
	pullCtx, cancelPull := context.WithTimeout(ctx, m.pullTimeout)
	defer cancelPull()

	pullCmd := m.newNonInteractiveGitCmd(pullCtx, repoPath, "pull", "--ff-only", "origin", baseBranch)
	if output, err := pullCmd.CombinedOutput(); err != nil {
		reason := classifyGitFallbackReason(err, string(output), pullCtx.Err())
		m.logger.Warn("git pull failed before worktree creation; continuing with remote ref",
			zap.String("branch", baseBranch),
			zap.String("reason", reason),
			zap.String("remote_ref", remoteRef),
			zap.String("output", string(output)),
			zap.Error(err))
		m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Pull %s; using %s", reason, remoteRef), string(output))
		return remoteRef
	}
	m.reportSyncCompleted(stepName, onProgress, fmt.Sprintf("Synced and using %s", baseBranch), "")
	return baseBranch
}

// removeWorktreeDir removes a worktree directory using git worktree remove.
func (m *Manager) removeWorktreeDir(ctx context.Context, worktreePath, repoPath string) error {
	// First try git worktree remove
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		m.logger.Debug("git worktree remove failed, falling back to rm",
			zap.String("output", string(output)),
			zap.Error(err))

		if err := m.forceRemoveDir(ctx, worktreePath); err != nil {
			return err
		}

		// Prune stale worktree entries
		pruneCmd := exec.CommandContext(ctx, "git", "worktree", "prune")
		pruneCmd.Dir = repoPath
		if err := pruneCmd.Run(); err != nil {
			m.logger.Debug("git worktree prune failed", zap.Error(err))
		}
	}
	return nil
}

// forceRemoveDir removes a directory, retrying on transient failures.
// On macOS, os.RemoveAll can fail with "directory not empty" when files
// have special attributes or were recently released by other processes
// (e.g. .next/dev build cache). Falls back to rm -rf as a last resort.
func (m *Manager) forceRemoveDir(ctx context.Context, dir string) error {
	const maxRetries = 3
	const retryDelay = 200 * time.Millisecond

	for i := range maxRetries {
		err := os.RemoveAll(dir)
		if err == nil {
			return nil
		}
		if i < maxRetries-1 {
			m.logger.Debug("os.RemoveAll failed, retrying",
				zap.String("path", dir),
				zap.Int("attempt", i+1),
				zap.Error(err))
			time.Sleep(retryDelay)
		}
	}

	// Last resort: shell out to rm -rf which handles macOS edge cases better
	cmd := exec.CommandContext(ctx, "rm", "-rf", dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rm -rf failed: %w (output: %s)", err, string(output))
	}
	return nil
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
	cmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	cmd.Dir = req.RepositoryPath
	if err := cmd.Run(); err != nil {
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

	// Try to add worktree using existing branch
	usesGitCrypt := m.usesGitCrypt(req.RepositoryPath)
	args := []string{"worktree", "add"}
	if usesGitCrypt {
		args = append(args, "--no-checkout")
	}
	args = append(args, worktreePath, existing.Branch)

	cmd = exec.CommandContext(ctx, "git", args...)
	cmd.Dir = req.RepositoryPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(output)
		// If git-crypt smudge error and we didn't detect git-crypt, retry with --no-checkout
		if isGitCryptSmudgeError(outStr) && !usesGitCrypt {
			m.logger.Warn("git-crypt smudge error during recreate, retrying with --no-checkout",
				zap.String("output", outStr))
			retryCmd := exec.CommandContext(ctx, "git", "worktree", "add", "--no-checkout", worktreePath, existing.Branch)
			retryCmd.Dir = req.RepositoryPath
			if retryOutput, retryErr := retryCmd.CombinedOutput(); retryErr != nil {
				m.logger.Error("failed to recreate worktree (--no-checkout)",
					zap.String("output", string(retryOutput)),
					zap.Error(retryErr))
				return nil, fmt.Errorf("%w: %s", ErrGitCommandFailed, string(retryOutput))
			}
			usesGitCrypt = true // Force unlock/checkout
		} else {
			m.logger.Error("failed to recreate worktree",
				zap.String("output", outStr),
				zap.Error(err))
			return nil, fmt.Errorf("%w: %s", ErrGitCommandFailed, outStr)
		}
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

	// Update cache keyed by (sessionID, repositoryID).
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(req.SessionID, req.RepositoryID)] = existing
		m.mu.Unlock()
	}

	m.logger.Info("recreated worktree",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath))

	return existing, nil
}
