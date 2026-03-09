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

	// Cache validated git index path (works with worktrees where .git is a file)
	gitIndexPath := resolveGitIndexPath(resolvedWorkDir)

	return &WorkspaceTracker{
		workDir:                    resolvedWorkDir,
		gitIndexPath:               gitIndexPath,
		logger:                     log.WithFields(zap.String("component", "workspace-tracker")),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		gitPollInterval:            DefaultGitPollInterval,
		stopCh:                     make(chan struct{}),
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
