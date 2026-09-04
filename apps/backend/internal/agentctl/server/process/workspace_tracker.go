package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// DefaultGitPollInterval is the default interval for polling git status
const DefaultGitPollInterval = 3 * time.Second

// workspaceGitStatusObserveTimeout bounds a shared live status observation so
// a wedged Git process cannot retain the tracker flight indefinitely.
const workspaceGitStatusObserveTimeout = 60 * time.Second

// pollModeGracePeriod is how long a tracker stays at its fast construction
// default before demoting itself to slow when no explicit mode ever arrives.
// Long enough to cover a freshly-spawned instance being opened by the user
// (the gateway pushes within milliseconds of a focus), short enough that a
// workspace nobody watches stops scanning almost immediately.
const pollModeGracePeriod = 60 * time.Second

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
	gitEnv       []string
	gitEnvMu     sync.RWMutex
	// allowedSourceRoots are canonical roots of durable sources explicitly
	// attached to this workspace. They are deliberately separate from workDir:
	// a link may point outside the workspace, but only to one of these roots.
	allowedSourceRoots []string
	// repositoryName identifies the repository this tracker covers when the
	// agent's workspace is a multi-repo task root. Stamped onto every emitted
	// GitStatusUpdate / FileListUpdate so the frontend can key per-repo state.
	// Empty for the single-repo case.
	repositoryName string

	// baseBranch is the task-specific base branch used to compute diff stats
	// (BaseCommit, Ahead/Behind). When set, it takes precedence over the
	// hardcoded origin/main → master fallback list in workspace_git_status.go.
	// Sourced from task_repositories.base_branch on the kandev backend.
	// Empty for legacy tasks or external branches with no recorded base.
	baseBranch string
	// comparisonTarget is the provider-qualified comparison binding. Its
	// state is independent from baseBranch: an unavailable explicit target must
	// never resolve through origin/<same-name>.
	comparisonTarget          *models.ComparisonTarget
	comparisonTargetRef       string
	comparisonTargetStatus    string
	comparisonTargetErrorCode string
	// comparisonAnchor is set for an initialized submodule. Unlike a branch
	// name, it must remain pinned to the gitlink commit recorded by the parent
	// comparison tree, even when the submodule's own default branch moves.
	comparisonAnchor    string
	comparisonAnchorSet bool

	// lastBaseBranchLog dedupes the diff-base resolution log. resolveBaseBranch
	// runs on every status poll (as often as every couple of seconds), so
	// logging each resolution would bury the signal it exists to provide.
	// Holds the previously logged "reason|stored|ref" so only a *change* is reported.
	lastBaseBranchLog string
	baseBranchLogMu   sync.Mutex

	// Current state
	currentStatus types.GitStatusUpdate
	currentFiles  types.FileListUpdate
	mu            sync.RWMutex

	// Cached git state for detecting manual operations
	cachedHeadSHA     string
	cachedBranchName  string // Current branch name for detecting branch switches
	cachedIndexHash   string // Hash of git status porcelain output to detect staging changes
	cachedUpstreamSHA string // SHA of @{upstream}, "" when no upstream — detects push/fetch
	gitStateMu        sync.RWMutex

	// Unified workspace stream subscribers
	workspaceStreamSubscribers map[types.WorkspaceStreamSubscriber]struct{}
	workspaceSubMu             sync.RWMutex

	// Polling intervals (used in PollModeFast)
	filePollInterval time.Duration
	gitPollInterval  time.Duration

	// pollMode is the current rate at which the polling loops scan git state.
	// Default at construction is PollModeFast so a freshly-spawned instance has
	// no blind window, then pollModeGraceTimer demotes it to slow unless the
	// gateway pushes a mode first. Mutate via SetPollMode (never directly) so
	// loops receive the wake-up notification on transitions.
	pollMode   PollMode
	pollModeMu sync.RWMutex
	// pollModePushed records that an explicit SetPollMode arrived, which
	// disarms the grace demotion for good — from then on the gateway owns the
	// mode. Guarded by pollModeMu.
	pollModePushed     bool
	pollModeGraceTimer *time.Timer
	// pollModeGrace is pollModeGracePeriod in production; tests shorten it the
	// same way they shorten filePollInterval / gitPollInterval.
	pollModeGrace time.Duration
	// One channel per loop because we need both loops to wake up on a mode
	// change. A single shared channel would let the first reader steal the
	// signal so only one loop wakes.
	monitorModeChanged chan struct{} // buffered(1); read by monitorLoop
	gitPollModeChanged chan struct{} // buffered(1); read by pollGitChanges

	// Overlap guards: prevent tick pile-up when git commands take longer than the poll interval.
	monitorRunning int32 // atomic; 1 if monitorLoop tick is in progress
	gitPollRunning int32 // atomic; 1 if pollGitChanges tick is in progress

	// gitPollTickCount and gitPollTickTotalNanos accumulate the number and
	// summed duration of completed pollGitChanges ticks since this tracker
	// started (never reset — only Stop() ends accumulation). Backs the
	// diagnostic agentctl_git_poll_ms metric read via GitPollTickStats; see
	// api.handleSystemMetrics.
	gitPollTickCount      atomic.Int64
	gitPollTickTotalNanos atomic.Int64
	gitPollStatsMu        sync.Mutex

	// monitorTickCount and monitorTickTotalNanos accumulate the number and
	// summed duration of completed workspace monitor scans since this tracker
	// started. This includes the initial scan and each later monitor tick. It
	// backs the diagnostic agentctl_monitor_poll_ms metric.
	monitorTickCount      atomic.Int64
	monitorTickTotalNanos atomic.Int64
	monitorStatsMu        sync.Mutex

	// updateMu prevents concurrent updateGitStatus calls from the two polling loops.
	// Polling loops use TryLock (skip if busy); RefreshGitStatus uses Lock (always completes).
	updateMu sync.Mutex
	// mutationMu serializes final upload target selection and placement. Upload
	// bytes are staged before this lock, so slow readers do not block other
	// workspace mutations while the target invariant remains atomic.
	mutationMu sync.Mutex

	// gitStatusObserver is the expensive live repository observation. Keeping it
	// as a dependency makes the concurrency contract deterministic to test while
	// production uses computeGitStatus.
	gitStatusObserver       func(context.Context) (types.GitStatusUpdate, error)
	gitStatusObserveTimeout time.Duration
	gitStatusGroup          singleflight.Group
	gitStatusObserveMu      sync.Mutex
	gitStatusObserveWG      sync.WaitGroup
	gitStatusWaiterJoined   func() // Optional test synchronization hook; nil in production.
	// gitStatusBetweenQueries is an optional test hook invoked between the
	// tracked and untracked queries. It is nil in production.
	gitStatusBetweenQueries func()

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

// recordGitPollTick accumulates one pollGitChanges tick's duration into the
// tracker's running count/total, regardless of whether the tick succeeded or
// failed — see gitPollTick's deferred call site.
func (wt *WorkspaceTracker) recordGitPollTick(d time.Duration) {
	wt.gitPollStatsMu.Lock()
	defer wt.gitPollStatsMu.Unlock()
	wt.gitPollTickTotalNanos.Add(d.Nanoseconds())
	wt.gitPollTickCount.Add(1)
}

// GitPollTickStats returns the number of completed git poll ticks and their
// summed duration in nanoseconds since this tracker started. count == 0
// means no tick has completed yet (the diagnostic metric is unavailable).
func (wt *WorkspaceTracker) GitPollTickStats() (count int64, totalNanos int64) {
	wt.gitPollStatsMu.Lock()
	defer wt.gitPollStatsMu.Unlock()
	return wt.gitPollTickCount.Load(), wt.gitPollTickTotalNanos.Load()
}

func (wt *WorkspaceTracker) recordMonitorTick(d time.Duration) {
	wt.monitorStatsMu.Lock()
	defer wt.monitorStatsMu.Unlock()
	wt.monitorTickTotalNanos.Add(d.Nanoseconds())
	wt.monitorTickCount.Add(1)
}

// MonitorTickStats returns the number of completed workspace monitor scans
// and their summed duration in nanoseconds since this tracker started.
func (wt *WorkspaceTracker) MonitorTickStats() (count int64, totalNanos int64) {
	wt.monitorStatsMu.Lock()
	defer wt.monitorStatsMu.Unlock()
	return wt.monitorTickCount.Load(), wt.monitorTickTotalNanos.Load()
}

// RepositoryName returns the repository name tag applied to events emitted by
// this tracker. Empty for the bare task-root / single-repo tracker; non-empty
// for per-repo trackers built via NewWorkspaceTrackerForRepo. Used by the
// rescan path to decide whether a discovered subdir already has a tracker.
func (wt *WorkspaceTracker) RepositoryName() string {
	return wt.repositoryName
}

// SetGitEnvironment stores a detached copy of the instance environment used
// for every Git command started by this tracker. Tracker-owned copies prevent
// a later manager or sibling-tracker update from changing an in-flight command.
func (wt *WorkspaceTracker) SetGitEnvironment(env []string) {
	detached := append([]string(nil), env...)
	wt.gitEnvMu.Lock()
	wt.gitEnv = detached
	wt.gitEnvMu.Unlock()
}

func (wt *WorkspaceTracker) gitEnvironmentSnapshot() []string {
	wt.gitEnvMu.RLock()
	defer wt.gitEnvMu.RUnlock()
	return append([]string(nil), wt.gitEnv...)
}

// SetBaseBranch records the task's stored base branch for this repository.
// Called once after construction by the process manager; subsequent git
// status updates use this value as the first candidate when resolving
// BaseCommit / Ahead / Behind. Empty disables the override and falls back
// to the hardcoded origin/main → master priority list.
//
// Unsafe ref names (leading "-", whitespace, shell metacharacters, ".." or
// other format violations) are rejected at this boundary because the value
// is interpolated into `exec.Command("git", …, baseBranch)` downstream and
// git itself interprets leading "-" as a flag. Rejection downgrades to the
// no-override fallback rather than erroring — keeps the tracker functional
// when an untrusted caller supplies garbage.
func (wt *WorkspaceTracker) SetBaseBranch(baseBranch string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	wt.setBaseBranchLocked(baseBranch)
}

// SetBaseBranchIfNotSubmodule updates the branch override only while holding
// the same lock used to publish a comparison anchor. This keeps a concurrent
// rescan from replacing a submodule anchor with a task-level base branch.
func (wt *WorkspaceTracker) SetBaseBranchIfNotSubmodule(baseBranch string) bool {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if wt.comparisonAnchorSet {
		return false
	}
	wt.setBaseBranchLocked(baseBranch)
	return true
}

func (wt *WorkspaceTracker) setBaseBranchLocked(baseBranch string) {
	wt.comparisonAnchor = ""
	wt.comparisonAnchorSet = false
	if !IsSafeGitRef(baseBranch) {
		wt.baseBranch = ""
		return
	}
	wt.baseBranch = baseBranch
}

// SetComparisonAnchor pins this tracker to the commit recorded by its parent
// repository at the parent's comparison point. An empty anchor is still
// meaningful: it records that this is a submodule whose committed base could
// not be resolved, so later base-branch updates must not substitute an
// unrelated branch from the task configuration.
func (wt *WorkspaceTracker) SetComparisonAnchor(anchor string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	wt.comparisonAnchorSet = true
	if !sha1HexPattern.MatchString(anchor) {
		wt.comparisonAnchor = ""
		wt.baseBranch = ""
		return
	}
	wt.comparisonAnchor = anchor
	wt.baseBranch = anchor
}

// IsSubmodule reports whether this tracker represents a declared submodule,
// including a child whose parent comparison anchor could not be resolved.
func (wt *WorkspaceTracker) IsSubmodule() bool {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return wt.comparisonAnchorSet
}

// ComparisonAnchor returns the exact parent-recorded gitlink commit when one
// was discovered. It is empty when the tracker is not a submodule or when the
// parent anchor could not be resolved locally.
func (wt *WorkspaceTracker) ComparisonAnchor() string {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return wt.comparisonAnchor
}

// IsSafeGitRef reports whether ref is safe to splice into a `git`
// subprocess argument list. Empty input returns true so callers can treat
// "" as "no override" without an extra branch. Delegates to the
// `securityutil.IsValidBranchName` sanitiser that the rest of the agentctl
// git operations (Rebase, Merge, RenameBranch, …) already use — keeps one
// canonical allowlist across the package and inherits the CodeQL
// taint-tracking recognition that pattern carries.
//
// `origin/<name>` refs are split before the underlying check because the
// shared validator rejects "/" in the first character class.
func IsSafeGitRef(ref string) bool {
	if ref == "" {
		return true
	}
	return SanitizeGitRef(ref) != ""
}

// SanitizeGitRef returns ref when securityutil.IsValidBranchName accepts
// it (after handling the `origin/<name>` prefix), else the empty string.
// Use this at the call site immediately before passing a user-controlled
// ref name into a `git` subprocess argument so the sanitiser barrier sits
// inline with the sink.
func SanitizeGitRef(ref string) string {
	if ref == "" || ref[len(ref)-1] == '/' {
		return ""
	}
	if rest, ok := strings.CutPrefix(ref, "origin/"); ok {
		if securityutil.IsValidBranchName(rest) {
			return ref
		}
		return ""
	}
	if securityutil.IsValidBranchName(ref) {
		return ref
	}
	return ""
}

// BaseBranch returns the recorded base branch override, if any. Exposed for
// tests; production callers read it indirectly through the git-status loops.
func (wt *WorkspaceTracker) BaseBranch() string {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return wt.baseBranch
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

	tracker := &WorkspaceTracker{
		workDir:                    resolvedWorkDir,
		gitIndexPath:               gitIndexPath,
		repositoryName:             repositoryName,
		logger:                     log.WithFields(logFields...),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		filePollInterval:           DefaultFilePollInterval,
		gitPollInterval:            DefaultGitPollInterval,
		// Start fast so a freshly-spawned instance — which is usually about to
		// be opened by someone — has no window where changes go unnoticed.
		// Start() then arms pollModeGraceTimer: the gateway only pushes a mode
		// for workspaces a client focuses or subscribes to, and it deliberately
		// skips the paused push for a workspace it has never pushed to
		// (workspacePollAggregator.plan). Without the timer, a tracker the
		// gateway never speaks about scans at 2s/3s for the life of the
		// process. Trackers created after launch inherit the workspace's
		// current mode instead — see Manager.configurePollMode.
		pollMode:                PollModeFast,
		pollModeGrace:           pollModeGracePeriod,
		monitorModeChanged:      make(chan struct{}, 1),
		gitPollModeChanged:      make(chan struct{}, 1),
		stopCh:                  make(chan struct{}),
		initialScanDone:         make(chan struct{}),
		tickDone:                make(chan struct{}, 1),
		cancelCtx:               ctx,
		cancelFunc:              cancel,
		gitStatusObserveTimeout: workspaceGitStatusObserveTimeout,
	}
	tracker.gitStatusObserver = tracker.computeGitStatus
	return tracker
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
	// One-shot probe at workspace setup. The after-admission helper starts the
	// 5s exec timer only after the lifecycle slot is granted; otherwise a busy
	// git pool would burn through the exec budget queueing and return "", which
	// the caller treats as "not a git repo" and permanently disables polling.
	acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelAcquire()
	out, runErr, execCtxErr := subproc.RunGitOutputAfterAcquire(
		acquireCtx,
		subproc.GitLifecycle,
		5*time.Second,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "rev-parse", "--git-dir")
			cmd.Dir = workDir
			return cmd
		},
	)
	if runErr != nil || execCtxErr != nil {
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

	wt.armPollModeGrace()
}

// armPollModeGrace schedules the fallback demotion to slow. Armed here rather
// than in the constructor so a tracker that is built but never started leaves
// no timer behind. A SetPollMode that lands first disarms it permanently.
func (wt *WorkspaceTracker) armPollModeGrace() {
	wt.pollModeMu.Lock()
	defer wt.pollModeMu.Unlock()
	if wt.pollModePushed || wt.pollModeGraceTimer != nil || wt.pollModeGrace <= 0 {
		return
	}
	wt.pollModeGraceTimer = time.AfterFunc(wt.pollModeGrace, wt.demoteUnpushedPollMode)
}

// disarmPollModeGraceLocked stops the fallback timer. Caller must hold pollModeMu.
func (wt *WorkspaceTracker) disarmPollModeGraceLocked() {
	if wt.pollModeGraceTimer != nil {
		wt.pollModeGraceTimer.Stop()
		wt.pollModeGraceTimer = nil
	}
}

// stopTimeout is the maximum time Stop() will wait for goroutines to exit.
const stopTimeout = 5 * time.Second

// Stop stops the workspace tracker. It cancels in-flight git commands and waits
// up to 5 seconds for goroutines to exit before proceeding.
func (wt *WorkspaceTracker) Stop() {
	wt.stopOnce.Do(func() {
		wt.pollModeMu.Lock()
		wt.disarmPollModeGraceLocked()
		wt.pollModeMu.Unlock()

		// Synchronize cancellation with shared-observation admission. Once this
		// lock is released, no observation can Add to gitStatusObserveWG.
		wt.gitStatusObserveMu.Lock()
		if wt.cancelFunc != nil {
			wt.cancelFunc() // Kill in-flight git commands immediately
		}
		wt.gitStatusObserveMu.Unlock()
		close(wt.stopCh)

		done := make(chan struct{})
		go func() {
			wt.wg.Wait()
			wt.gitStatusObserveWG.Wait()
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
