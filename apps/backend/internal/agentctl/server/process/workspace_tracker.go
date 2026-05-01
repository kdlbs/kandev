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
	fileStatusDeleted   = "deleted"
	fileStatusModified  = "modified"
	fileStatusUntracked = "untracked"
)

// WorkspaceTracker monitors workspace changes and provides real-time updates.
// It uses git status polling instead of fsnotify to avoid file descriptor exhaustion
// on macOS (where kqueue opens a file descriptor for every watched file).
type WorkspaceTracker struct {
	workDir      string
	gitIndexPath string // Cached, validated path to git index file (works with worktrees)
	logger       *logger.Logger
	// repositoryName identifies the repository this tracker covers when the
	// agent's workspace is a multi-repo task root. Stamped onto every emitted
	// GitStatusUpdate / FileListUpdate so the frontend can key per-repo state.
	// Empty for the single-repo case.
	repositoryName string

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

	// Polling intervals (used in PollModeFast)
	filePollInterval time.Duration
	gitPollInterval  time.Duration

	// pollMode is the current rate at which the polling loops scan git state.
	// Default at construction is PollModeSlow — safe fallback before the gateway
	// pushes a focus signal. Mutate via SetPollMode (never directly) so loops
	// receive the wake-up notification on transitions.
	pollMode   PollMode
	pollModeMu sync.RWMutex
	// One channel per loop because we need both loops to wake up on a mode
	// change. A single shared channel would let the first reader steal the
	// signal so only one loop wakes.
	monitorModeChanged chan struct{} // buffered(1); read by monitorLoop
	gitPollModeChanged chan struct{} // buffered(1); read by pollGitChanges

	// Overlap guards: prevent tick pile-up when git commands take longer than the poll interval.
	monitorRunning int32 // atomic; 1 if monitorLoop tick is in progress
	gitPollRunning int32 // atomic; 1 if pollGitChanges tick is in progress

	// updateMu prevents concurrent updateGitStatus calls from the two polling loops.
	// Polling loops use TryLock (skip if busy); RefreshGitStatus uses Lock (always completes).
	updateMu sync.Mutex

	// Control
	stopCh          chan struct{}
	wg              sync.WaitGroup
	started         bool
	stopOnce        sync.Once
	initialScanDone chan struct{}      // closed after monitorLoop's first getWorkspaceState; tests wait on it
	tickDone        chan struct{}      // buffered(1); signalled after each monitorTick completes; tests wait on it
	cancelCtx       context.Context    // Cancellable context for killing in-flight git commands on Stop
	cancelFunc      context.CancelFunc // Cancel function called during Stop
}

// NewWorkspaceTracker creates a new workspace tracker for a single git
// repository (or a single workspace root that may or may not be a git repo).
// For multi-repo task roots use NewWorkspaceTrackerForRepo per repo subdir.
func NewWorkspaceTracker(workDir string, log *logger.Logger) *WorkspaceTracker {
	tlog := log.WithFields(zap.String("component", "workspace-tracker"))
	resolvedWorkDir := resolveExistingWorkDir(workDir, tlog)
	// Multi-repo task roots are plain directories that hold one git worktree
	// per repository as siblings. The git poller can't monitor a non-git path,
	// so when the configured workDir isn't a git repo, fall back to the first
	// child subdirectory that is. This preserves single-repo behavior for
	// callers using NewWorkspaceTracker; multi-repo callers should use
	// NewWorkspaceTrackerForRepo per repo subdir to get per-repo events.
	resolvedWorkDir = preferGitRepoChildIfRootIsBare(resolvedWorkDir, tlog)
	return newWorkspaceTracker(resolvedWorkDir, "", log)
}

// NewWorkspaceTrackerForRepo creates a tracker scoped to a specific repository
// subdirectory. The repositoryName is stamped onto every emitted event so the
// frontend can route updates per repo for multi-repo task roots.
func NewWorkspaceTrackerForRepo(workDir, repositoryName string, log *logger.Logger) *WorkspaceTracker {
	tlog := log.WithFields(
		zap.String("component", "workspace-tracker"),
		zap.String("repository_name", repositoryName),
	)
	resolvedWorkDir := resolveExistingWorkDir(workDir, tlog)
	return newWorkspaceTracker(resolvedWorkDir, repositoryName, log)
}

func newWorkspaceTracker(resolvedWorkDir, repositoryName string, log *logger.Logger) *WorkspaceTracker {
	logFields := []zap.Field{zap.String("component", "workspace-tracker")}
	if repositoryName != "" {
		logFields = append(logFields, zap.String("repository_name", repositoryName))
	}

	// Cache validated git index path (works with worktrees where .git is a file)
	gitIndexPath := resolveGitIndexPath(resolvedWorkDir)

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkspaceTracker{
		workDir:                    resolvedWorkDir,
		gitIndexPath:               gitIndexPath,
		repositoryName:             repositoryName,
		logger:                     log.WithFields(logFields...),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		filePollInterval:           DefaultFilePollInterval,
		gitPollInterval:            DefaultGitPollInterval,
		// Default to fast polling — matches pre-PR behavior so newly-created
		// agentctl instances don't have a startup window where changes go
		// undetected for up to 30s. The gateway pushes slow/paused once it
		// knows no client is actively watching this workspace; until then,
		// fast is the safe default (a freshly-spawned instance was always
		// about to be used by someone, historically). Retained-task CPU
		// savings still apply because those instances eventually receive a
		// slow or paused mode push.
		pollMode:           PollModeFast,
		monitorModeChanged: make(chan struct{}, 1),
		gitPollModeChanged: make(chan struct{}, 1),
		stopCh:             make(chan struct{}),
		initialScanDone:    make(chan struct{}),
		tickDone:           make(chan struct{}, 1),
		cancelCtx:          ctx,
		cancelFunc:         cancel,
	}
}

// preferGitRepoChildIfRootIsBare returns workDir unchanged when it's already a
// git repo, otherwise returns the path of the first immediate child directory
// that is a git repo (or worktree). Used to make the workspace tracker work
// for multi-repo task roots, which are plain holder directories with each
// repo's worktree as a child.
//
// Returns workDir unchanged when no git child is found — the tracker will
// then run with no git data, matching the pre-multi-repo behavior for
// repo-less tasks (e.g. quick chat).
func preferGitRepoChildIfRootIsBare(workDir string, log *logger.Logger) string {
	if workDir == "" || resolveGitIndexPath(workDir) != "" {
		return workDir
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return workDir
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate := filepath.Join(workDir, entry.Name())
		if resolveGitIndexPath(candidate) != "" {
			log.Info("workspace tracker falling back to repo subdirectory (multi-repo task root is not itself a git repo)",
				zap.String("task_root", workDir),
				zap.String("repo_subdir", candidate))
			return candidate
		}
	}
	return workDir
}

// resolveGitIndexPath returns the validated path to the git index file.
// Returns empty string if the path cannot be resolved or validated.
// This handles worktrees where .git is a file pointing elsewhere.
//
// Multi-repo task roots are NOT git repos themselves (they hold per-repo
// child worktrees as siblings), but `git rev-parse` ascends until it finds
// a `.git` — for tasks nested under a developer's own kandev checkout this
// would land on the OUTER worktree and silently emit its status as if it
// were the task. We guard against that by requiring the resolved git
// top-level to be the same path as workDir (or, for repo subdirs called
// here from `scanRepositorySubdirs`, an absolute git-dir reachable from the
// dir's own `.git` file). In practice this means: workDir must contain its
// own `.git` entry — file or directory — for the path to be considered a
// valid git repo for tracking.
func resolveGitIndexPath(workDir string) string {
	if !workDirHasOwnGitEntry(workDir) {
		return ""
	}
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

// workDirHasOwnGitEntry returns true when workDir contains a `.git` entry of
// its own (file for worktrees, directory for plain repos). This is what
// makes a directory "a git repo" from a tracker's perspective — without it,
// git would ascend up the tree to the nearest ancestor `.git`, which is the
// wrong scope for nested layouts (e.g. a multi-repo task root sitting
// inside the developer's kandev checkout).
func workDirHasOwnGitEntry(workDir string) bool {
	if workDir == "" {
		return false
	}
	_, err := os.Lstat(filepath.Join(workDir, ".git"))
	return err == nil
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
