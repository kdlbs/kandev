package process

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// DefaultGitPollInterval is the default interval for polling git status
const DefaultGitPollInterval = 3 * time.Second

// fileStatus constants for FileInfo.Status values.
const (
	fileStatusDeleted  = "deleted"
	fileStatusModified = "modified"
)

// WorkspaceTracker monitors workspace changes and provides real-time updates
type WorkspaceTracker struct {
	workDir string
	logger  *logger.Logger

	// Current state
	currentStatus types.GitStatusUpdate
	currentFiles  types.FileListUpdate
	mu            sync.RWMutex

	// Cached git state for detecting manual operations
	cachedHeadSHA   string
	cachedIndexHash string // Hash of git status porcelain output to detect staging changes
	gitStateMu      sync.RWMutex

	// Unified workspace stream subscribers
	workspaceStreamSubscribers map[types.WorkspaceStreamSubscriber]struct{}
	workspaceSubMu             sync.RWMutex

	// Filesystem watcher
	watcher *fsnotify.Watcher

	// Debounce channel for filesystem change events
	fsChangeTrigger chan struct{}

	// Pending file changes accumulated between debounce flushes
	pendingChanges   []types.FileChangeNotification
	pendingChangesMu sync.Mutex

	// Git polling interval
	gitPollInterval time.Duration

	// Control
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
}

// NewWorkspaceTracker creates a new workspace tracker
func NewWorkspaceTracker(workDir string, log *logger.Logger) *WorkspaceTracker {
	resolvedWorkDir := resolveExistingWorkDir(workDir, log.WithFields(zap.String("component", "workspace-tracker")))
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("failed to create filesystem watcher", zap.Error(err))
		watcher = nil
	}

	return &WorkspaceTracker{
		workDir:                    resolvedWorkDir,
		logger:                     log.WithFields(zap.String("component", "workspace-tracker")),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		watcher:                    watcher,
		fsChangeTrigger:            make(chan struct{}, 1), // Buffered to avoid blocking
		gitPollInterval:            DefaultGitPollInterval,
		stopCh:                     make(chan struct{}),
	}
}

// Start begins monitoring the workspace
func (wt *WorkspaceTracker) Start(ctx context.Context) {
	wt.mu.Lock()
	if wt.started {
		wt.mu.Unlock()
		wt.logger.Debug("workspace tracker already started, skipping")
		return
	}
	wt.started = true
	wt.mu.Unlock()

	wt.wg.Add(1)
	go wt.monitorLoop(ctx)

	// Start git polling for detecting manual git operations
	wt.wg.Add(1)
	go wt.pollGitChanges(ctx)

	// Start filesystem watcher if available
	if wt.watcher != nil {
		wt.wg.Add(1)
		go wt.watchFilesystem(ctx)

		// Add workspace root to watcher
		if err := wt.addDirectoryRecursive(wt.workDir); err != nil {
			wt.logger.Error("failed to watch workspace directory", zap.Error(err))
		}
	}
}

// Stop stops the workspace tracker
func (wt *WorkspaceTracker) Stop() {
	close(wt.stopCh)
	if wt.watcher != nil {
		if err := wt.watcher.Close(); err != nil {
			wt.logger.Debug("failed to close watcher", zap.Error(err))
		}
	}
	wt.wg.Wait()
	wt.logger.Info("workspace tracker stopped")
}
