package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// DefaultFilePollInterval is the interval for polling file modification times.
// This replaces fsnotify which on macOS (kqueue) opens a file descriptor per file,
// causing "too many open files" errors in large workspaces.
const DefaultFilePollInterval = 2 * time.Second

// monitorLoop polls for file changes using file mtimes and quick git checks.
// This is more efficient than fsnotify on macOS and avoids file descriptor exhaustion.
func (wt *WorkspaceTracker) monitorLoop(ctx context.Context) {
	defer wt.wg.Done()

	ticker := time.NewTicker(DefaultFilePollInterval)
	defer ticker.Stop()

	// Initial update
	wt.updateGitStatus(ctx)
	wt.updateFiles(ctx)

	// Cache the last known state
	lastState := wt.getWorkspaceState(ctx)

	wt.logger.Info("file polling started", zap.Duration("interval", DefaultFilePollInterval))

	for {
		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case <-ticker.C:
			// Quick state check using mtime + diff-files
			currentState := wt.getWorkspaceState(ctx)
			if currentState.changed(lastState) {
				lastState = currentState
				wt.logger.Debug("workspace state changed, updating")

				// Update file list and notify subscribers
				wt.updateFiles(ctx)
				wt.notifyWorkspaceStreamFileChange(types.FileChangeNotification{
					Timestamp: time.Now(),
					Operation: types.FileOpRefresh,
				})
			}
		}
	}
}

// workspaceState holds quick-check state for detecting workspace changes.
type workspaceState struct {
	indexMtime  time.Time // index mtime - changes on stage/unstage/commit
	diffFilesID string    // hash of git diff-files output - changes on any file modification
}

// getWorkspaceState returns the current workspace state using fast checks.
// Uses index mtime + git diff-files output which includes file content hashes.
func (wt *WorkspaceTracker) getWorkspaceState(ctx context.Context) workspaceState {
	var state workspaceState

	// Check index mtime (gitIndexPath is pre-validated at startup)
	if wt.gitIndexPath != "" {
		if info, err := os.Stat(wt.gitIndexPath); err == nil {
			state.indexMtime = info.ModTime()
		}
	}

	// git diff-files shows changed files with their blob hashes
	// Output changes whenever file content changes (not just dirty/clean boolean)
	cmd := exec.CommandContext(ctx, "git", "diff-files")
	cmd.Dir = wt.workDir
	out, _ := cmd.Output()
	// Use length + first/last bytes as cheap identity check
	// (full hash would work too but this is faster for our use case)
	if len(out) > 0 {
		state.diffFilesID = fmt.Sprintf("%d:%x:%x", len(out), out[0], out[len(out)-1])
	}

	return state
}

// changed returns true if the workspace state has changed.
func (s workspaceState) changed(other workspaceState) bool {
	return s.indexMtime != other.indexMtime || s.diffFilesID != other.diffFilesID
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
