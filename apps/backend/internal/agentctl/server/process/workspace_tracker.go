package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

// WorkspaceTracker monitors workspace changes and provides real-time updates.
// It uses git status polling instead of fsnotify to avoid file descriptor exhaustion
// on macOS (where kqueue opens a file descriptor for every watched file).
type WorkspaceTracker struct {
	workDir      string
	gitIndexPath string // Cached, validated path to git index file (works with worktrees)
	logger       *logger.Logger

	// Current state
	currentStatus types.GitStatusUpdate
	currentFiles  types.FileListUpdate
	mu            sync.RWMutex

	// Cached git state for detecting manual operations
	cachedHeadSHA    string
	cachedBranchName string // Current branch name for detecting branch switches
	cachedIndexHash  string // Hash of git status porcelain output to detect staging changes
	gitStateMu       sync.RWMutex

	// Unified workspace stream subscribers
	workspaceStreamSubscribers map[types.WorkspaceStreamSubscriber]struct{}
	workspaceSubMu             sync.RWMutex

	// Polling intervals
	filePollInterval time.Duration
	gitPollInterval  time.Duration

	// Overlap guards: prevent tick pile-up when git commands take longer than the poll interval.
	monitorRunning int32 // atomic; 1 if monitorLoop tick is in progress
	gitPollRunning int32 // atomic; 1 if pollGitChanges tick is in progress

	// updateMu prevents concurrent updateGitStatus calls from the two polling loops.
	// Polling loops use TryLock (skip if busy); RefreshGitStatus uses Lock (always completes).
	updateMu sync.Mutex

	// Control
	stopCh     chan struct{}
	wg         sync.WaitGroup
	started    bool
	stopOnce   sync.Once
	cancelCtx  context.Context    // Cancellable context for killing in-flight git commands on Stop
	cancelFunc context.CancelFunc // Cancel function called during Stop
}

// NewWorkspaceTracker creates a new workspace tracker
func NewWorkspaceTracker(workDir string, log *logger.Logger) *WorkspaceTracker {
	resolvedWorkDir := resolveExistingWorkDir(workDir, log.WithFields(zap.String("component", "workspace-tracker")))

	// Cache validated git index path (works with worktrees where .git is a file)
	gitIndexPath := resolveGitIndexPath(resolvedWorkDir)

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkspaceTracker{
		workDir:                    resolvedWorkDir,
		gitIndexPath:               gitIndexPath,
		logger:                     log.WithFields(zap.String("component", "workspace-tracker")),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		filePollInterval:           DefaultFilePollInterval,
		gitPollInterval:            DefaultGitPollInterval,
		stopCh:                     make(chan struct{}),
		cancelCtx:                  ctx,
		cancelFunc:                 cancel,
	}
}

// resolveGitIndexPath returns the validated path to the git index file.
// Returns empty string if the path cannot be resolved or validated.
// This handles worktrees where .git is a file pointing elsewhere.
func resolveGitIndexPath(workDir string) string {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	// Clean and construct the index path
	gitDir = filepath.Clean(gitDir)
	indexPath := filepath.Join(gitDir, "index")
	// Validate the index file exists (this proves gitDir is a valid git directory)
	info, err := os.Stat(indexPath)
	if err != nil || info.IsDir() {
		return ""
	}
	return indexPath
}

// workDirExists checks whether the workspace directory still exists on disk.
// Used to detect deleted worktrees and stop polling loops gracefully.
// The workDir is validated at construction time via resolveExistingWorkDir,
// so this only checks for subsequent deletion (e.g., worktree cleanup).
func (wt *WorkspaceTracker) workDirExists() bool {
	_, err := os.Stat(wt.workDir) //nolint:gosec // workDir is validated at construction via resolveExistingWorkDir
	return !os.IsNotExist(err)
}

// Start begins monitoring the workspace using polling (no fsnotify).
// File changes are detected via git status polling, which is efficient and
// doesn't consume file descriptors like fsnotify/kqueue does on macOS.
// The passed context is ignored — the tracker uses its own cancellable context
// so that Stop() can kill in-flight git commands immediately.
func (wt *WorkspaceTracker) Start(_ context.Context) {
	wt.mu.Lock()
	if wt.started {
		wt.mu.Unlock()
		wt.logger.Debug("workspace tracker already started, skipping")
		return
	}
	wt.started = true
	wt.mu.Unlock()

	// Start file change monitoring (uses git status polling)
	wt.wg.Add(1)
	go wt.monitorLoop(wt.cancelCtx)

	// Start git polling for detecting manual git operations (commits, resets, etc.)
	wt.wg.Add(1)
	go wt.pollGitChanges(wt.cancelCtx)
}

// stopTimeout is the maximum time Stop() will wait for goroutines to exit.
const stopTimeout = 5 * time.Second

// Stop stops the workspace tracker. It cancels in-flight git commands and waits
// up to 5 seconds for goroutines to exit before proceeding.
func (wt *WorkspaceTracker) Stop() {
	wt.stopOnce.Do(func() {
		wt.cancelFunc() // Kill in-flight git commands immediately
		close(wt.stopCh)

		done := make(chan struct{})
		go func() {
			wt.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// Clean shutdown
		case <-time.After(stopTimeout):
			wt.logger.Warn("workspace tracker stop timed out, proceeding anyway",
				zap.Duration("timeout", stopTimeout))
		}
		wt.logger.Info("workspace tracker stopped")
	})
}
