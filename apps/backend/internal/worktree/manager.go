package worktree

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

const (
	defaultGitFetchTimeout = 90 * time.Second
	defaultGitPullTimeout  = 60 * time.Second
	// defaultGitInspectTimeout bounds cheap local git ref-inspection
	// commands (rev-parse --verify, rev-parse --abbrev-ref HEAD).
	// Normally <100ms, but on CrowdStrike-instrumented macOS every
	// fork+exec is intercepted by syspolicyd so a single spawn can take
	// 1–3s under load. 30s gives ~100x headroom over normal operation
	// while still surfacing true hangs (credential prompts, stuck
	// filters, filesystem stalls) in a reasonable window.
	defaultGitInspectTimeout = 30 * time.Second
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
	repoProvider      RepositoryProvider
	scriptMsgHandler  ScriptMessageHandler
	scriptEnvProvider ScriptEnvironmentProvider

	// Timeouts for best-effort remote sync before creating a worktree.
	fetchTimeout time.Duration
	pullTimeout  time.Duration
	// Bound for cheap git ref-inspection commands (branchExists, currentBranch).
	inspectTimeout time.Duration
}

// ScriptEnvironmentProvider supplies install-managed environment variables to
// repository setup and cleanup scripts.
type ScriptEnvironmentProvider interface {
	ExecutionEnvironment(ctx context.Context) (map[string]string, error)
}

// SetScriptEnvironmentProvider wires managed script environment settings.
func (m *Manager) SetScriptEnvironmentProvider(provider ScriptEnvironmentProvider) {
	m.scriptEnvProvider = provider
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
	// ListActiveWorktreePaths returns the worktree_path of every active,
	// non-deleted worktree row that has a non-empty path. Used by the
	// office GC to identify live worktrees that must not be swept.
	ListActiveWorktreePaths(ctx context.Context) ([]string, error)
}

// MultiRepoStore is an optional capability some stores implement to support
// multi-repo task sessions (one worktree per repository per session). The
// Manager checks at runtime whether its Store satisfies this interface and
// uses the multi-repo lookups when available.
type MultiRepoStore interface {
	// GetWorktreesBySessionID returns all active worktrees for the session.
	GetWorktreesBySessionID(ctx context.Context, sessionID string) ([]*Worktree, error)
	// GetWorktreeBySessionAndRepository returns the active worktree for the
	// given (session, repository, branchSlug) triple, or nil if none exists.
	// branchSlug scopes the lookup for multi-branch tasks; empty matches the
	// legacy single-branch persistence shape.
	GetWorktreeBySessionAndRepository(ctx context.Context, sessionID, repositoryID, branchSlug string) (*Worktree, error)
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

// ListActiveWorktreePaths returns the absolute on-disk paths of all
// currently active, non-deleted worktrees. Used by the office GC as the
// authoritative inventory of paths that must not be swept.
func (m *Manager) ListActiveWorktreePaths(ctx context.Context) ([]string, error) {
	return m.store.ListActiveWorktreePaths(ctx)
}

// IsEnabled returns whether worktree mode is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
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
