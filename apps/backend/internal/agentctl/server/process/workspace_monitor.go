package process

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

// monitorLoop handles debounced filesystem change events
// When files change, we wait for activity to settle, then update everything at once
func (wt *WorkspaceTracker) monitorLoop(ctx context.Context) {
	defer wt.wg.Done()

	// Debounce duration - wait this long after last file change before updating
	const debounceDuration = 300 * time.Millisecond

	var debounceTimer *time.Timer
	var pendingUpdate bool

	// Initial update
	wt.updateGitStatus(ctx)
	wt.updateFiles(ctx)

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case <-wt.stopCh:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case <-wt.fsChangeTrigger:
			// File change detected - start or reset debounce timer
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(debounceDuration)
			} else {
				// Reset the timer if already running
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(debounceDuration)
			}
			pendingUpdate = true
		case <-func() <-chan time.Time {
			if debounceTimer != nil {
				return debounceTimer.C
			}
			return nil
		}():
			// Debounce timer fired - update everything
			if pendingUpdate {
				wt.updateGitStatus(ctx)
				wt.updateFiles(ctx)

				// Drain pending changes and emit specific notifications
				wt.pendingChangesMu.Lock()
				changes := wt.pendingChanges
				wt.pendingChanges = nil
				wt.pendingChangesMu.Unlock()

				wt.emitFileChanges(changes)
				pendingUpdate = false
			}
			debounceTimer = nil
		}
	}
}

// watchFilesystem watches for filesystem changes and triggers debounced updates
func (wt *WorkspaceTracker) watchFilesystem(ctx context.Context) {
	defer wt.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case event, ok := <-wt.watcher.Events:
			if !ok {
				return
			}

			// Ignore CHMOD events - permission changes don't affect file content
			// and shouldn't trigger UI refreshes. This prevents loops from filesystem
			// scanners, git operations, or other tools that touch file permissions.
			if event.Op == fsnotify.Chmod {
				continue
			}

			// If a directory was created, watch it
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := wt.addDirectoryRecursive(event.Name); err != nil {
						wt.logger.Debug("failed to watch new directory", zap.Error(err))
					}
				}
			}

			// Record specific change with relative path
			relPath, relErr := filepath.Rel(wt.workDir, event.Name)
			if relErr != nil {
				relPath = event.Name
			}
			wt.addPendingChange(relPath, fsOpToString(event.Op))

		case err, ok := <-wt.watcher.Errors:
			if !ok {
				return
			}
			wt.logger.Debug("filesystem watcher error", zap.Error(err))
		}
	}
}

// fsOpToString converts an fsnotify operation to a FileOp string constant.
func fsOpToString(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create != 0:
		return types.FileOpCreate
	case op&fsnotify.Write != 0:
		return types.FileOpWrite
	case op&fsnotify.Remove != 0:
		return types.FileOpRemove
	case op&fsnotify.Rename != 0:
		return types.FileOpRename
	default:
		return types.FileOpRefresh
	}
}

// addDirectoryRecursive adds a directory and all its subdirectories to the watcher
func (wt *WorkspaceTracker) addDirectoryRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and common ignore patterns
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".next" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}

			// Add directory to watcher
			if err := wt.watcher.Add(path); err != nil {
				wt.logger.Debug("failed to watch directory", zap.String("path", path), zap.Error(err))
			}
		}

		return nil
	})
}

// addPendingChange records a specific file change and triggers a debounced update.
func (wt *WorkspaceTracker) addPendingChange(relPath, operation string) {
	wt.pendingChangesMu.Lock()
	wt.pendingChanges = append(wt.pendingChanges, types.FileChangeNotification{
		Timestamp: time.Now(),
		Path:      relPath,
		Operation: operation,
	})
	wt.pendingChangesMu.Unlock()

	// Trigger debounced update (non-blocking)
	select {
	case wt.fsChangeTrigger <- struct{}{}:
	default:
	}
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
