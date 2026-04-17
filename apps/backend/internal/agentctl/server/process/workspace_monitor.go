package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// DefaultFilePollInterval is the interval for polling file modification times.
// This replaces fsnotify which on macOS (kqueue) opens a file descriptor per file,
// causing "too many open files" errors in large workspaces.
const DefaultFilePollInterval = 2 * time.Second

// gitCommandTimeout is the maximum time to wait for git commands during polling.
// This prevents the monitor loop from hanging if git is blocked (e.g., index lock).
const gitCommandTimeout = 10 * time.Second

// maxConsecutiveGitFailures is the number of consecutive git command failures
// before the polling loop gives up and stops. At 2-second intervals, 5 failures
// means ~10 seconds — enough to rule out transient issues (index lock, brief GC race)
// while stopping quickly enough to avoid log spam when git is permanently broken
// (e.g., worktree .git reference points to a deleted gitdir after task archival).
const maxConsecutiveGitFailures = 5

// monitorLoop polls for file changes using file mtimes and quick git checks.
// This is more efficient than fsnotify on macOS and avoids file descriptor exhaustion.
//
// The tick interval is driven by the current PollMode (set via SetPollMode):
// fast (default 2s) for focused sessions, slow (30s) for sidebar-visible
// sessions, paused (no-op tick) when nothing is watching.
func (wt *WorkspaceTracker) monitorLoop(ctx context.Context) {
	defer wt.wg.Done()

	// If no valid git index was found at startup, there's nothing to poll.
	// Exit immediately to avoid spamming "git diff-files: exit status 128" warnings
	// every poll cycle until the consecutive failure threshold is reached.
	if wt.gitIndexPath == "" {
		wt.logger.Warn("no valid git repository found, file monitor not polling",
			zap.String("workDir", wt.workDir))
		// Close the initialScanDone channel even on early exit so callers
		// waiting on <-wt.initialScanDone don't block forever.
		select {
		case <-wt.initialScanDone:
		default:
			close(wt.initialScanDone)
		}
		return
	}

	// Start with a stopped timer; we reset it explicitly per-iteration based
	// on the current poll mode. Resettable timer (vs fixed Ticker) lets us
	// react instantly when the mode changes — critical for the user-visible
	// lag when a task gains focus.
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	// Initial update — done unconditionally so the first focus-grab isn't
	// stuck waiting for the first tick. Skipped only in fully-paused initial
	// state? No: even paused workspaces benefit from a one-time scan so
	// transitioning to slow/fast later finds a populated cache.
	wt.updateGitStatus(ctx)
	wt.updateFiles(ctx)

	// Cache the last known state (ignore error on initial fetch)
	lastState, _ := wt.getWorkspaceState(ctx)

	// Signal that the initial scan is done — tests wait on this instead of
	// sleeping to avoid races with the goroutine's first getWorkspaceState.
	select {
	case <-wt.initialScanDone:
		// already closed (shouldn't happen, but be safe)
	default:
		close(wt.initialScanDone)
	}

	wt.logger.Info("file polling started",
		zap.Duration("fast_interval", wt.filePollInterval),
		zap.String("initial_mode", string(wt.GetPollMode())))

	var consecutiveFailures int

	for {
		filePoll, _, _ := wt.pollIntervals(wt.GetPollMode())
		timer.Reset(filePoll)

		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case <-wt.monitorModeChanged:
			if stop := wt.handleMonitorModeChange(ctx, timer, &lastState, &consecutiveFailures); stop {
				return
			}
		case <-timer.C:
			if stop := wt.handleMonitorTimerTick(ctx, &lastState, &consecutiveFailures); stop {
				return
			}
		}
	}
}

// handleMonitorModeChange responds to a SetPollMode-triggered wake-up: drains
// any pending timer and, if we just transitioned into fast mode, runs an
// immediate scan so the user sees fresh state without waiting for the next tick.
// Returns true if monitorLoop should exit.
func (wt *WorkspaceTracker) handleMonitorModeChange(ctx context.Context, timer *time.Timer, lastState *workspaceState, consecutiveFailures *int) bool {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if wt.GetPollMode() != PollModeFast {
		return false
	}
	if !atomic.CompareAndSwapInt32(&wt.monitorRunning, 0, 1) {
		return false
	}
	return wt.monitorTick(ctx, lastState, consecutiveFailures)
}

// handleMonitorTimerTick is the body of a regular timer-driven tick. In paused
// mode the body is a no-op (we still wake periodically to check the mode in
// case the change-signal was missed). Returns true if monitorLoop should exit.
func (wt *WorkspaceTracker) handleMonitorTimerTick(ctx context.Context, lastState *workspaceState, consecutiveFailures *int) bool {
	if _, _, paused := wt.pollIntervals(wt.GetPollMode()); paused {
		return false
	}
	// Skip this tick if the previous cycle is still running (prevents process
	// pile-up when git commands take longer than the poll interval on large repos).
	if !atomic.CompareAndSwapInt32(&wt.monitorRunning, 0, 1) {
		return false
	}
	return wt.monitorTick(ctx, lastState, consecutiveFailures)
}

// monitorTick runs a single monitor cycle: checks workspace state and triggers
// updates if changes are detected. Returns true if the loop should stop.
// The deferred flag reset ensures monitorRunning is cleared even on panic.
func (wt *WorkspaceTracker) monitorTick(ctx context.Context, lastState *workspaceState, consecutiveFailures *int) bool {
	defer func() {
		atomic.StoreInt32(&wt.monitorRunning, 0)
		// Signal that the tick completed — tests wait on this instead of
		// spinning on the monitorRunning atomic flag.
		select {
		case wt.tickDone <- struct{}{}:
		default:
		}
	}()

	currentState, err := wt.getWorkspaceState(ctx)
	if err != nil {
		if !wt.workDirExists() {
			wt.logger.Warn("work directory no longer exists, stopping workspace tracker",
				zap.String("workDir", wt.workDir))
			return true
		}

		*consecutiveFailures++
		if *consecutiveFailures >= maxConsecutiveGitFailures {
			wt.logger.Error("git commands failing repeatedly, stopping workspace monitor",
				zap.String("workDir", wt.workDir),
				zap.Int("consecutiveFailures", *consecutiveFailures),
				zap.Error(err))
			return true
		}
		wt.logger.Warn("failed to get workspace state, will retry next cycle",
			zap.String("workDir", wt.workDir),
			zap.Int("consecutiveFailures", *consecutiveFailures),
			zap.Error(err))
		return false
	}
	*consecutiveFailures = 0
	if currentState.changed(*lastState) {
		*lastState = currentState
		wt.logger.Debug("workspace state changed, updating")

		wt.tryUpdateGitStatus(ctx)
		wt.updateFiles(ctx)
		wt.notifyWorkspaceStreamFileChange(types.FileChangeNotification{
			Timestamp: time.Now(),
			Operation: types.FileOpRefresh,
		})
	}
	return false
}

// workspaceState holds quick-check state for detecting workspace changes.
type workspaceState struct {
	indexMtime  time.Time // index mtime - changes on stage/unstage/commit
	diffFilesID string    // hash of git diff-files output - changes on tracked file modification
	untrackedID string    // hash of untracked files list + mtimes - changes on untracked file changes
}

// getWorkspaceState returns the current workspace state using fast checks.
// Uses index mtime + git diff-files output + file mtimes.
// Returns an error if any git command fails, so the caller can skip the update
// and retry on the next poll cycle.
func (wt *WorkspaceTracker) getWorkspaceState(ctx context.Context) (workspaceState, error) {
	var state workspaceState

	// Check index mtime (gitIndexPath is pre-validated at startup)
	if wt.gitIndexPath != "" {
		if info, err := os.Stat(wt.gitIndexPath); err == nil {
			state.indexMtime = info.ModTime()
		}
	}

	// Create a timeout context for git commands to prevent hanging
	gitCtx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()

	// git diff-files shows changed files with their blob hashes.
	// However, the output doesn't change when an already-dirty file is modified
	// again because the index blob hash stays constant and the worktree hash
	// is always shown as 0000000 (not computed). To detect subsequent changes
	// to dirty files, we also include the mtime of each dirty file.
	cmd := wt.pollingGitCommand(gitCtx, "diff-files", "--name-only")
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	out, err := cmd.Output()
	if err != nil {
		return state, fmt.Errorf("git diff-files in %s: %w (stderr: %s)",
			wt.workDir, err, strings.TrimSpace(stderrBuf.String()))
	}
	state.diffFilesID = wt.buildDirtyFilesID(string(out))

	// Check untracked files - git diff-files doesn't include them
	// Use git ls-files to get untracked files, then check their mtimes
	untrackedID, err := wt.getUntrackedFilesID(gitCtx)
	if err != nil {
		return state, err
	}
	state.untrackedID = untrackedID

	return state, nil
}

// buildDirtyFilesID builds an identifier string for dirty tracked files.
// Includes file paths and their mtimes so subsequent changes are detected.
func (wt *WorkspaceTracker) buildDirtyFilesID(diffFilesOutput string) string {
	if diffFilesOutput == "" {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(diffFilesOutput), "\n")
	var hashInput strings.Builder
	for _, file := range lines {
		if file == "" {
			continue
		}
		hashInput.WriteString(file)
		// Include mtime so we detect content changes to already-dirty files
		safePath, err := wt.sanitizePath(file)
		if err != nil {
			continue // Skip files with invalid paths
		}
		if info, err := os.Stat(safePath); err == nil {
			hashInput.WriteString(fmt.Sprintf(":%d", info.ModTime().UnixNano()))
		}
		hashInput.WriteString(";")
	}
	return hashInput.String()
}

// getUntrackedFilesID returns a string identifying the current state of untracked files.
// Uses file list + mtimes to detect when untracked files are added, removed, or modified.
// Returns an error if the git command fails (e.g., timeout, index lock).
func (wt *WorkspaceTracker) getUntrackedFilesID(ctx context.Context) (string, error) {
	// Get list of untracked files (excluding ignored)
	cmd := wt.pollingGitCommand(ctx, "ls-files", "--others", "--exclude-standard")
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-files in %s: %w (stderr: %s)",
			wt.workDir, err, strings.TrimSpace(stderrBuf.String()))
	}
	if len(out) == 0 {
		return "", nil // No untracked files is not an error
	}

	// Build a string from file paths + mtimes to detect any changes
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var hashInput strings.Builder
	for _, file := range lines {
		if file == "" {
			continue
		}
		hashInput.WriteString(file)
		// Include mtime so we detect content changes
		// Sanitize path to prevent directory traversal attacks (CodeQL security fix)
		safePath, err := wt.sanitizePath(file)
		if err != nil {
			continue // Skip files with invalid paths
		}
		if info, err := os.Stat(safePath); err == nil {
			hashInput.WriteString(fmt.Sprintf(":%d", info.ModTime().UnixNano()))
		}
		hashInput.WriteString(";")
	}

	// Return the full string - any mtime change will produce a different string
	return hashInput.String(), nil
}

// sanitizePath validates that the given relative file path stays within the workspace directory.
// Returns the absolute path if valid, or an error if the path escapes the workspace.
func (wt *WorkspaceTracker) sanitizePath(relPath string) (string, error) {
	// Clean the path to resolve any . or .. components
	joined := filepath.Join(wt.workDir, relPath)
	absPath, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	// Ensure the resolved path is within the workspace directory
	workDirAbs, err := filepath.Abs(wt.workDir)
	if err != nil {
		return "", err
	}

	// Check that absPath starts with workDirAbs (with proper separator handling)
	if !strings.HasPrefix(absPath, workDirAbs+string(os.PathSeparator)) && absPath != workDirAbs {
		return "", fmt.Errorf("path escapes workspace: %s", relPath)
	}

	return absPath, nil
}

// changed returns true if the workspace state has changed.
func (s workspaceState) changed(other workspaceState) bool {
	return s.indexMtime != other.indexMtime ||
		s.diffFilesID != other.diffFilesID ||
		s.untrackedID != other.untrackedID
}

// emitFileChanges sends accumulated file changes to subscribers.
// Falls back to a single generic refresh when there are too many changes or none.
func (wt *WorkspaceTracker) emitFileChanges(changes []types.FileChangeNotification) {
	const maxSpecificChanges = 50
	if len(changes) == 0 || len(changes) > maxSpecificChanges {
		wt.notifyWorkspaceStreamFileChange(types.FileChangeNotification{
			Timestamp: time.Now(),
			Operation: types.FileOpRefresh,
		})
		return
	}
	for i := range changes {
		wt.notifyWorkspaceStreamFileChange(changes[i])
	}
}

// notifyFileChange sends a file change notification immediately.
// Used by direct file operations (create, delete, rename, apply diff) to notify
// subscribers without waiting for the next poll cycle.
func (wt *WorkspaceTracker) notifyFileChange(relPath, operation string) {
	wt.notifyWorkspaceStreamFileChange(types.FileChangeNotification{
		Timestamp: time.Now(),
		Path:      relPath,
		Operation: operation,
	})
}
