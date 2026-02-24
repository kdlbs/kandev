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
	defaultGitFetchTimeout = 8 * time.Second
	defaultGitPullTimeout  = 8 * time.Second
)

// repoLockEntry tracks a repository lock and its reference count.
type repoLockEntry struct {
	mu       *sync.Mutex
	refCount int
}

// Manager handles Git worktree operations for concurrent agent execution.
type Manager struct {
	config     Config
	logger     *logger.Logger
	store      Store
	worktrees  map[string]*Worktree // sessionID -> worktree (in-memory cache)
	mu         sync.RWMutex         // Protects worktrees map
	repoLocks  map[string]*repoLockEntry
	repoLockMu sync.Mutex

	// Optional dependencies for script execution
	repoProvider     RepositoryProvider
	scriptMsgHandler ScriptMessageHandler

	// Timeouts for best-effort remote sync before creating a worktree.
	fetchTimeout time.Duration
	pullTimeout  time.Duration
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
	// GetWorktreeBySessionID retrieves the worktree by session ID.
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

// NewManager creates a new worktree manager.
func NewManager(cfg Config, store Store, log *logger.Logger) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.Default()
	}

	// Ensure base directory exists
	basePath, err := cfg.ExpandedBasePath()
	if err != nil {
		return nil, fmt.Errorf("failed to expand base path: %w", err)
	}
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree base directory: %w", err)
	}

	return &Manager{
		config:       cfg,
		logger:       log.WithFields(zap.String("component", "worktree-manager")),
		store:        store,
		worktrees:    make(map[string]*Worktree),
		repoLocks:    make(map[string]*repoLockEntry),
		fetchTimeout: defaultGitFetchTimeout,
		pullTimeout:  defaultGitPullTimeout,
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

	// First, check if a worktree already exists for this session
	if req.SessionID != "" {
		existing, err := m.GetBySessionID(ctx, req.SessionID)
		if err == nil && existing != nil {
			if m.IsValid(existing.Path) {
				m.logger.Debug("reusing existing worktree by session ID",
					zap.String("worktree_id", existing.ID),
					zap.String("session_id", req.SessionID),
					zap.String("task_id", req.TaskID),
					zap.String("path", existing.Path))
				return existing, nil
			}
			// Worktree record exists but directory is invalid - recreate
			m.logger.Warn("worktree directory invalid, recreating",
				zap.String("worktree_id", existing.ID),
				zap.String("session_id", req.SessionID),
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
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(req.RepositoryPath)
	}()

	baseRef := req.BaseBranch
	if req.PullBeforeWorktree {
		baseRef = m.pullBaseBranch(req.RepositoryPath, req.BaseBranch)
	}

	// Check base branch exists
	if !m.branchExists(req.RepositoryPath, baseRef) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidBaseBranch, baseRef)
	}

	return m.createWorktree(ctx, req, baseRef)
}

// createWorktree performs the actual git worktree creation.
func (m *Manager) createWorktree(ctx context.Context, req CreateRequest, baseRef string) (*Worktree, error) {
	worktreeDirName, branchName := m.buildWorktreeNames(req)

	worktreePath, err := m.config.WorktreePath(worktreeDirName)
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree path: %w", err)
	}

	worktreeID, err := m.gitAddWorktree(ctx, req.RepositoryPath, branchName, worktreePath, baseRef)
	if err != nil {
		return nil, err
	}

	wt := m.buildWorktreeRecord(worktreeID, req, worktreePath, branchName)

	if err := m.persistAndCacheWorktree(ctx, wt, req, worktreePath); err != nil {
		return nil, err
	}

	if err := m.runWorktreeSetupScript(ctx, wt, req.RepositoryPath); err != nil {
		return nil, err
	}

	m.logger.Info("created worktree",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath),
		zap.String("branch", branchName))

	return wt, nil
}

// buildWorktreeNames derives the filesystem directory name and git branch name for a new worktree.
func (m *Manager) buildWorktreeNames(req CreateRequest) (dirName, branchName string) {
	worktreeID := uuid.New().String()
	dirSuffix := worktreeID[:8] // Use first 8 chars of UUID for worktree dir uniqueness
	branchSuffix := SmallSuffix(3)
	prefix := NormalizeBranchPrefix(req.WorktreeBranchPrefix)

	if req.TaskTitle != "" {
		// Use semantic naming: {sanitized-title}_{suffix}
		dirName = SemanticWorktreeName(req.TaskTitle, dirSuffix)
		sanitizedTitle := SanitizeForBranch(req.TaskTitle, 20)
		if sanitizedTitle == "" {
			sanitizedTitle = SanitizeForBranch(req.TaskID, 20)
		}
		branchName = prefix + sanitizedTitle + "-" + branchSuffix
	} else {
		// Fallback to task ID based naming
		dirName = req.TaskID + "_" + dirSuffix
		branchName = prefix + SanitizeForBranch(req.TaskID, 20) + "-" + branchSuffix
	}
	return dirName, branchName
}

// gitAddWorktree runs "git worktree add" and returns the new worktree UUID.
func (m *Manager) gitAddWorktree(ctx context.Context, repoPath, branchName, worktreePath, baseRef string) (string, error) {
	worktreeID := uuid.New().String()
	// git worktree add -b <branch> <path> <base-branch>
	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"-b", branchName,
		worktreePath,
		baseRef)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Error("git worktree add failed",
			zap.String("output", string(output)),
			zap.Error(err))
		return "", fmt.Errorf("%w: %s", ErrGitCommandFailed, string(output))
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

	// Update cache (keyed by sessionID)
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[req.SessionID] = wt
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

// GetBySessionID returns the worktree for a session, if it exists.
func (m *Manager) GetBySessionID(ctx context.Context, sessionID string) (*Worktree, error) {
	// Check cache first (keyed by sessionID)
	m.mu.RLock()
	if wt, ok := m.worktrees[sessionID]; ok {
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
			// Update cache
			m.mu.Lock()
			m.worktrees[sessionID] = wt
			m.mu.Unlock()
			return wt, nil
		}
	}

	return nil, ErrWorktreeNotFound
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

	// Update cache
	m.mu.Lock()
	if wt.SessionID != "" {
		delete(m.worktrees, wt.SessionID)
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
		delete(m.worktrees, wt.SessionID)
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
			delete(m.worktrees, wt.SessionID)
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

// Reconcile syncs worktree state with active tasks on startup.
func (m *Manager) Reconcile(ctx context.Context, activeTasks []string) error {
	basePath, err := m.config.ExpandedBasePath()
	if err != nil {
		return fmt.Errorf("failed to expand base path: %w", err)
	}

	// Create a set of active task IDs
	activeSet := make(map[string]bool)
	for _, taskID := range activeTasks {
		activeSet[taskID] = true
	}

	// Scan worktree directories
	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No worktrees directory yet
		}
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		taskID := entry.Name()
		worktreePath := filepath.Join(basePath, taskID)

		if !activeSet[taskID] {
			// Orphaned worktree - no matching active task
			m.logger.Info("cleaning up orphaned worktree",
				zap.String("task_id", taskID),
				zap.String("path", worktreePath))

			// Remove directory
			if err := os.RemoveAll(worktreePath); err != nil {
				m.logger.Warn("failed to remove orphaned worktree",
					zap.String("path", worktreePath),
					zap.Error(err))
			}
		}
	}

	return nil
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
func (m *Manager) branchExists(repoPath, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = repoPath
	err := cmd.Run()
	return err == nil
}

func (m *Manager) currentBranch(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
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

	out := strings.ToLower(cmdOutput)
	if strings.Contains(out, "authentication failed") ||
		strings.Contains(out, "terminal prompts disabled") ||
		strings.Contains(out, "could not read username") ||
		strings.Contains(out, "username for 'https://") ||
		strings.Contains(out, "askpass") {
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
func (m *Manager) pullBaseBranch(repoPath, baseBranch string) string {
	// Normalize branch name - strip "origin/" prefix if present
	localBranch := strings.TrimPrefix(baseBranch, "origin/")
	isRemoteRef := localBranch != baseBranch

	// Fetch branch from origin in non-interactive mode.
	fetchCtx, cancelFetch := context.WithTimeout(context.Background(), m.fetchTimeout)
	defer cancelFetch()

	fetchArgs := []string{"fetch", "origin"}
	if localBranch != "" {
		fetchArgs = append(fetchArgs, localBranch)
	}
	fetchCmd := m.newNonInteractiveGitCmd(fetchCtx, repoPath, fetchArgs...)
	if output, err := fetchCmd.CombinedOutput(); err != nil {
		m.logger.Warn("git fetch failed before worktree creation; continuing with fallback ref",
			zap.String("branch", baseBranch),
			zap.String("reason", classifyGitFallbackReason(err, string(output), fetchCtx.Err())),
			zap.String("fallback_ref", baseBranch),
			zap.String("output", string(output)),
			zap.Error(err))
		return baseBranch
	}

	// If the original ref was already a remote ref, use it directly
	if isRemoteRef {
		return "origin/" + localBranch
	}

	// For local branches: try to update if we're on that branch, otherwise use origin/<branch>
	remoteRef := "origin/" + localBranch
	currentBranch := m.currentBranch(repoPath)

	if currentBranch == baseBranch {
		// We're on the target branch - try to pull (fast-forward only)
		pullCtx, cancelPull := context.WithTimeout(context.Background(), m.pullTimeout)
		defer cancelPull()

		pullCmd := m.newNonInteractiveGitCmd(pullCtx, repoPath, "pull", "--ff-only", "origin", baseBranch)
		if output, err := pullCmd.CombinedOutput(); err != nil {
			m.logger.Warn("git pull failed before worktree creation; continuing with remote ref",
				zap.String("branch", baseBranch),
				zap.String("reason", classifyGitFallbackReason(err, string(output), pullCtx.Err())),
				zap.String("remote_ref", remoteRef),
				zap.String("output", string(output)),
				zap.Error(err))
			return remoteRef
		}
		return baseBranch
	}

	// Not on the target branch - use the remote ref if it exists
	if m.branchExists(repoPath, remoteRef) {
		return remoteRef
	}

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

	worktreePath, err := m.config.WorktreePath(req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree path: %w", err)
	}

	// Try to add worktree using existing branch
	cmd = exec.CommandContext(ctx, "git", "worktree", "add",
		worktreePath,
		existing.Branch)
	cmd.Dir = req.RepositoryPath

	if output, err := cmd.CombinedOutput(); err != nil {
		m.logger.Error("failed to recreate worktree",
			zap.String("output", string(output)),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %s", ErrGitCommandFailed, string(output))
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

	// Update cache (keyed by sessionID)
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[req.SessionID] = existing
		m.mu.Unlock()
	}

	m.logger.Info("recreated worktree",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath))

	return existing, nil
}
