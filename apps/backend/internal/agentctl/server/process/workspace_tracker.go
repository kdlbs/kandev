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
	workDir string
	gitDir  string // Cached git dir path (works with worktrees)
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

	// Git polling interval
	gitPollInterval time.Duration

	// Control
	stopCh   chan struct{}
	wg       sync.WaitGroup
	started  bool
	stopOnce sync.Once
}

// NewWorkspaceTracker creates a new workspace tracker
func NewWorkspaceTracker(workDir string, log *logger.Logger) *WorkspaceTracker {
	resolvedWorkDir := resolveExistingWorkDir(workDir, log.WithFields(zap.String("component", "workspace-tracker")))

	// Cache git dir path (works with worktrees where .git is a file)
	gitDir := resolveGitDir(resolvedWorkDir)

	return &WorkspaceTracker{
		workDir:                    resolvedWorkDir,
		gitDir:                     gitDir,
		logger:                     log.WithFields(zap.String("component", "workspace-tracker")),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		gitPollInterval:            DefaultGitPollInterval,
		stopCh:                     make(chan struct{}),
	}
}

// resolveGitDir returns the git directory path, handling worktrees.
// Returns empty string if the path cannot be resolved or validated.
func resolveGitDir(workDir string) string {
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
	// Clean and validate the path to prevent path traversal
	gitDir = filepath.Clean(gitDir)
	// Verify the path exists and is a directory
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return gitDir
}

// Start begins monitoring the workspace using polling (no fsnotify).
// File changes are detected via git status polling, which is efficient and
// doesn't consume file descriptors like fsnotify/kqueue does on macOS.
func (wt *WorkspaceTracker) Start(ctx context.Context) {
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
	go wt.monitorLoop(ctx)

	// Start git polling for detecting manual git operations (commits, resets, etc.)
	wt.wg.Add(1)
	go wt.pollGitChanges(ctx)
}

// Stop stops the workspace tracker
func (wt *WorkspaceTracker) Stop() {
	wt.stopOnce.Do(func() {
		close(wt.stopCh)
		wt.wg.Wait()
		wt.logger.Info("workspace tracker stopped")
	})
}
