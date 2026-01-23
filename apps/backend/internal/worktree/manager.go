package worktree

import (
	"context"
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

// Manager handles Git worktree operations for concurrent agent execution.
type Manager struct {
	config     Config
	logger     *logger.Logger
	store      Store
	worktrees  map[string]*Worktree // sessionID -> worktree (in-memory cache)
	mu         sync.RWMutex         // Protects worktrees map
	repoLocks  map[string]*sync.Mutex
	repoLockMu sync.Mutex

	// Optional dependencies for script execution
	repoProvider     RepositoryProvider
	scriptMsgHandler ScriptMessageHandler
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
		config:    cfg,
		logger:    log.WithFields(zap.String("component", "worktree-manager")),
		store:     store,
		worktrees: make(map[string]*Worktree),
		repoLocks: make(map[string]*sync.Mutex),
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

// getRepoLock returns a mutex for the given repository path.
func (m *Manager) getRepoLock(repoPath string) *sync.Mutex {
	m.repoLockMu.Lock()
	defer m.repoLockMu.Unlock()

	if lock, exists := m.repoLocks[repoPath]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	m.repoLocks[repoPath] = lock
	return lock
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
				m.logger.Info("reusing existing worktree by session ID",
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

	// Check base branch exists
	if !m.branchExists(req.RepositoryPath, req.BaseBranch) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidBaseBranch, req.BaseBranch)
	}

	// Check worktree limit
	count, err := m.countWorktreesForRepo(ctx, req.RepositoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to count worktrees: %w", err)
	}
	if count >= m.config.MaxPerRepo {
		return nil, fmt.Errorf("%w: %d", ErrMaxWorktrees, m.config.MaxPerRepo)
	}

	// Get repository lock for safe concurrent access
	repoLock := m.getRepoLock(req.RepositoryPath)
	repoLock.Lock()
	defer repoLock.Unlock()

	return m.createWorktree(ctx, req)
}

// createWorktree performs the actual git worktree creation.
func (m *Manager) createWorktree(ctx context.Context, req CreateRequest) (*Worktree, error) {
	// Generate a unique worktree ID and random suffix
	worktreeID := uuid.New().String()
	dirSuffix := worktreeID[:8] // Use first 8 chars of UUID for worktree dir uniqueness
	branchSuffix := SmallSuffix(3)
	prefix := NormalizeBranchPrefix(req.WorktreeBranchPrefix)

	// Generate semantic name from task title if available
	var worktreeDirName, branchName string
	if req.TaskTitle != "" {
		// Use semantic naming: {sanitized-title}_{suffix}
		semanticName := SemanticWorktreeName(req.TaskTitle, dirSuffix)
		worktreeDirName = semanticName
		// For branch, use sanitized title (20 chars max) + suffix
		sanitizedTitle := SanitizeForBranch(req.TaskTitle, 20)
		if sanitizedTitle == "" {
			sanitizedTitle = SanitizeForBranch(req.TaskID, 20)
		}
		branchName = prefix + sanitizedTitle + "-" + branchSuffix
	} else {
		// Fallback to task ID based naming
		worktreeDirName = req.TaskID + "_" + dirSuffix
		branchName = prefix + SanitizeForBranch(req.TaskID, 20) + "-" + branchSuffix
	}

	worktreePath, err := m.config.WorktreePath(worktreeDirName)
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree path: %w", err)
	}

	// Create worktree with new branch
	// git worktree add -b <branch> <path> <base-branch>
	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"-b", branchName,
		worktreePath,
		req.BaseBranch)
	cmd.Dir = req.RepositoryPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Error("git worktree add failed",
			zap.String("output", string(output)),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %s", ErrGitCommandFailed, string(output))
	}

	now := time.Now()
	wt := &Worktree{
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

	// Persist to store
	if m.store != nil {
		if req.SessionID == "" {
			m.logger.Warn("skipping worktree persistence: missing session_id",
				zap.String("task_id", req.TaskID),
				zap.String("worktree_id", wt.ID))
		} else {
			if err := m.store.CreateWorktree(ctx, wt); err != nil {
				// Cleanup on failure
				if cleanupErr := m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath); cleanupErr != nil {
					m.logger.Warn("failed to cleanup worktree after persist failure", zap.Error(cleanupErr))
				}
				return nil, fmt.Errorf("failed to persist worktree: %w", err)
			}
		}
	}

	// Update cache (keyed by sessionID)
	if req.SessionID != "" {
		m.mu.Lock()
		m.worktrees[req.SessionID] = wt
		m.mu.Unlock()
	}

	// Execute setup script if configured
	if m.scriptMsgHandler != nil && m.repoProvider != nil {
		repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
		if err != nil {
			m.logger.Warn("failed to fetch repository for setup script",
				zap.String("repository_id", wt.RepositoryID),
				zap.Error(err))
		} else if strings.TrimSpace(repo.SetupScript) != "" {
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
				// Setup failed - cleanup worktree
				m.logger.Error("setup script failed, cleaning up worktree",
					zap.String("worktree_id", wt.ID),
					zap.Error(err))

				// Remove from cache
				if wt.SessionID != "" {
					m.mu.Lock()
					delete(m.worktrees, wt.SessionID)
					m.mu.Unlock()
				}

				// Remove directory
				if cleanupErr := m.removeWorktreeDir(ctx, wt.Path, req.RepositoryPath); cleanupErr != nil {
					m.logger.Warn("failed to cleanup worktree after setup failure",
						zap.Error(cleanupErr))
				}

				// Mark as deleted in database
				if m.store != nil {
					now := time.Now()
					wt.Status = StatusDeleted
					wt.DeletedAt = &now
					wt.UpdatedAt = now
					if updateErr := m.store.UpdateWorktree(ctx, wt); updateErr != nil {
						m.logger.Warn("failed to update worktree status",
							zap.Error(updateErr))
					}
				}

				return nil, fmt.Errorf("setup script failed: %w", err)
			}

			m.logger.Info("setup script completed successfully",
				zap.String("worktree_id", wt.ID))
		}
	}

	m.logger.Info("created worktree",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath),
		zap.String("branch", branchName))

	return wt, nil
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
	defer repoLock.Unlock()

	// Execute cleanup script BEFORE removing directory
	if m.scriptMsgHandler != nil && m.repoProvider != nil {
		repo, err := m.repoProvider.GetRepository(ctx, wt.RepositoryID)
		if err != nil {
			m.logger.Warn("failed to fetch repository for cleanup script",
				zap.String("repository_id", wt.RepositoryID),
				zap.Error(err))
		} else if strings.TrimSpace(repo.CleanupScript) != "" {
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
				// Log error but continue with deletion
				m.logger.Warn("cleanup script failed, proceeding with deletion",
					zap.String("worktree_id", wt.ID),
					zap.Error(err))
			} else {
				m.logger.Info("cleanup script completed successfully",
					zap.String("worktree_id", wt.ID))
			}
		}
	}

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
			m.logger.Warn("failed to update worktree status",
				zap.Error(err))
		}
	}

	// Update cache
	m.mu.Lock()
	delete(m.worktrees, wt.TaskID)
	m.mu.Unlock()

	m.logger.Info("removed worktree",
		zap.String("task_id", wt.TaskID),
		zap.String("worktree_id", wt.ID),
		zap.String("path", wt.Path),
		zap.Bool("branch_removed", removeBranch))

	return nil
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
		delete(m.worktrees, wt.TaskID)
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

// countWorktreesForRepo counts active worktrees for a repository.
func (m *Manager) countWorktreesForRepo(ctx context.Context, repoID string) (int, error) {
	if m.store == nil {
		// Count from cache if no store
		m.mu.RLock()
		defer m.mu.RUnlock()
		count := 0
		for _, wt := range m.worktrees {
			if wt.RepositoryID == repoID && wt.Status == StatusActive {
				count++
			}
		}
		return count, nil
	}

	worktrees, err := m.store.GetWorktreesByRepositoryID(ctx, repoID)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, wt := range worktrees {
		if wt.Status == StatusActive {
			count++
		}
	}
	return count, nil
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

		// Fallback to direct removal
		if err := os.RemoveAll(worktreePath); err != nil {
			return err
		}

		// Prune stale worktree entries
		cmd = exec.CommandContext(ctx, "git", "worktree", "prune")
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			m.logger.Debug("git worktree prune failed", zap.Error(err))
		}
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
	defer repoLock.Unlock()

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
