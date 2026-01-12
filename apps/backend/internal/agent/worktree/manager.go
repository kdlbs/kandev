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
	worktrees  map[string]*Worktree // taskID -> worktree (in-memory cache)
	mu         sync.RWMutex         // Protects worktrees map
	repoLocks  map[string]*sync.Mutex
	repoLockMu sync.Mutex
}

// Store is the interface for worktree persistence.
type Store interface {
	// CreateWorktree persists a new worktree record.
	CreateWorktree(ctx context.Context, wt *Worktree) error
	// GetWorktreeByTaskID retrieves a worktree by task ID.
	GetWorktreeByTaskID(ctx context.Context, taskID string) (*Worktree, error)
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

// GetWorktreeInfo returns worktree path and branch for a task.
// Returns nil values if no worktree exists for the task.
// This method implements the WorktreeLookup interface for task enrichment.
func (m *Manager) GetWorktreeInfo(ctx context.Context, taskID string) (path, branch *string) {
	wt, err := m.GetByTaskID(ctx, taskID)
	if err != nil || wt == nil {
		return nil, nil
	}
	return &wt.Path, &wt.Branch
}

// Create creates a new worktree for a task.
// If a worktree already exists for the task, it returns the existing one.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (*Worktree, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check if worktree already exists
	existing, err := m.GetByTaskID(ctx, req.TaskID)
	if err == nil && existing != nil {
		if m.IsValid(existing.Path) {
			m.logger.Info("reusing existing worktree",
				zap.String("task_id", req.TaskID),
				zap.String("path", existing.Path))
			return existing, nil
		}
		// Worktree record exists but directory is invalid - recreate
		m.logger.Warn("worktree directory invalid, recreating",
			zap.String("task_id", req.TaskID))
		return m.recreate(ctx, existing, req)
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
	worktreePath, err := m.config.WorktreePath(req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree path: %w", err)
	}

	branchName := req.BranchName
	if branchName == "" {
		branchName = m.config.BranchName(req.TaskID)
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
		// Check if branch already exists
		if strings.Contains(string(output), "already exists") {
			// Try without -b flag (use existing branch)
			cmd = exec.CommandContext(ctx, "git", "worktree", "add",
				worktreePath,
				branchName)
			cmd.Dir = req.RepositoryPath
			output, err = cmd.CombinedOutput()
			if err != nil {
				m.logger.Error("git worktree add failed",
					zap.String("output", string(output)),
					zap.Error(err))
				return nil, fmt.Errorf("%w: %s", ErrGitCommandFailed, string(output))
			}
		} else {
			m.logger.Error("git worktree add failed",
				zap.String("output", string(output)),
				zap.Error(err))
			return nil, fmt.Errorf("%w: %s", ErrGitCommandFailed, string(output))
		}
	}

	now := time.Now()
	wt := &Worktree{
		ID:             uuid.New().String(),
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
		if err := m.store.CreateWorktree(ctx, wt); err != nil {
			// Cleanup on failure
			m.removeWorktreeDir(ctx, worktreePath, req.RepositoryPath)
			return nil, fmt.Errorf("failed to persist worktree: %w", err)
		}
	}

	// Update cache
	m.mu.Lock()
	m.worktrees[req.TaskID] = wt
	m.mu.Unlock()

	m.logger.Info("created worktree",
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath),
		zap.String("branch", branchName))

	return wt, nil
}

// GetByTaskID returns the worktree for a task, if it exists.
func (m *Manager) GetByTaskID(ctx context.Context, taskID string) (*Worktree, error) {
	// Check cache first
	m.mu.RLock()
	if wt, ok := m.worktrees[taskID]; ok {
		m.mu.RUnlock()
		return wt, nil
	}
	m.mu.RUnlock()

	// Check store
	if m.store != nil {
		wt, err := m.store.GetWorktreeByTaskID(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if wt != nil {
			// Update cache
			m.mu.Lock()
			m.worktrees[taskID] = wt
			m.mu.Unlock()
			return wt, nil
		}
	}

	return nil, ErrWorktreeNotFound
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

// Remove removes a worktree and optionally its branch.
func (m *Manager) Remove(ctx context.Context, taskID string, removeBranch bool) error {
	wt, err := m.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}

	// Get repository lock
	repoLock := m.getRepoLock(wt.RepositoryPath)
	repoLock.Lock()
	defer repoLock.Unlock()

	// Remove worktree directory
	if err := m.removeWorktreeDir(ctx, wt.Path, wt.RepositoryPath); err != nil {
		m.logger.Warn("failed to remove worktree directory",
			zap.String("path", wt.Path),
			zap.Error(err))
	}

	// Optionally remove the branch
	if removeBranch {
		cmd := exec.CommandContext(ctx, "git", "branch", "-D", wt.Branch)
		cmd.Dir = wt.RepositoryPath
		if output, err := cmd.CombinedOutput(); err != nil {
			m.logger.Warn("failed to delete branch",
				zap.String("branch", wt.Branch),
				zap.String("output", string(output)),
				zap.Error(err))
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
	delete(m.worktrees, taskID)
	m.mu.Unlock()

	m.logger.Info("removed worktree",
		zap.String("task_id", taskID),
		zap.String("path", wt.Path),
		zap.Bool("branch_removed", removeBranch))

	return nil
}



// OnTaskDeleted cleans up the worktree when a task is deleted.
func (m *Manager) OnTaskDeleted(ctx context.Context, taskID string) error {
	// Try to remove the worktree, ignore if not found
	err := m.Remove(ctx, taskID, true)
	if err == ErrWorktreeNotFound {
		return nil
	}
	return err
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
		cmd.Run() // Ignore errors
	}
	return nil
}

// recreate recreates a worktree from stored metadata.
func (m *Manager) recreate(ctx context.Context, existing *Worktree, req CreateRequest) (*Worktree, error) {
	// Clean up existing directory if present
	if existing.Path != "" {
		os.RemoveAll(existing.Path)
	}

	// Remove from git worktree list
	cmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	cmd.Dir = req.RepositoryPath
	cmd.Run() // Ignore errors

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

	// Update cache
	m.mu.Lock()
	m.worktrees[req.TaskID] = existing
	m.mu.Unlock()

	m.logger.Info("recreated worktree",
		zap.String("task_id", req.TaskID),
		zap.String("path", worktreePath))

	return existing, nil
}