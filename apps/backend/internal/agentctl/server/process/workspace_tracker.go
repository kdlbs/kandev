package process

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// WorkspaceTracker monitors workspace changes and provides real-time updates
type WorkspaceTracker struct {
	workDir string
	logger  *logger.Logger

	// Current state
	currentStatus types.GitStatusUpdate
	currentFiles  types.FileListUpdate
	mu            sync.RWMutex

	// Unified workspace stream subscribers
	workspaceStreamSubscribers map[types.WorkspaceStreamSubscriber]struct{}
	workspaceSubMu             sync.RWMutex

	// Filesystem watcher
	watcher *fsnotify.Watcher

	// Debounce channel for filesystem change events
	fsChangeTrigger chan struct{}

	// Control
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
}

// NewWorkspaceTracker creates a new workspace tracker
func NewWorkspaceTracker(workDir string, log *logger.Logger) *WorkspaceTracker {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("failed to create filesystem watcher", zap.Error(err))
		watcher = nil
	}

	return &WorkspaceTracker{
		workDir:                    workDir,
		logger:                     log.WithFields(zap.String("component", "workspace-tracker")),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		watcher:                    watcher,
		fsChangeTrigger:            make(chan struct{}, 1), // Buffered to avoid blocking
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
				// Notify workspace stream subscribers that workspace changed
				wt.notifyWorkspaceStreamFileChange(types.FileChangeNotification{
					Timestamp: time.Now(),
					Path:      "",
					Operation: "refresh",
				})
				pendingUpdate = false
			}
			debounceTimer = nil
		}
	}
}

// updateGitStatus updates the git status
func (wt *WorkspaceTracker) updateGitStatus(ctx context.Context) {
	status, err := wt.getGitStatus(ctx)
	if err != nil {
		return
	}

	wt.mu.Lock()
	wt.currentStatus = status
	wt.mu.Unlock()

	// Notify workspace stream subscribers
	wt.notifyWorkspaceStreamGitStatus(status)
}

// getGitStatus retrieves the current git status
func (wt *WorkspaceTracker) getGitStatus(ctx context.Context) (types.GitStatusUpdate, error) {
	update := types.GitStatusUpdate{
		Timestamp: time.Now(),
		Modified:  []string{},
		Added:     []string{},
		Deleted:   []string{},
		Untracked: []string{},
		Renamed:   []string{},
		Files:     make(map[string]types.FileInfo),
	}

	// Get current branch
	branchCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = wt.workDir
	branchOut, err := branchCmd.Output()
	if err != nil {
		return update, err
	}
	update.Branch = strings.TrimSpace(string(branchOut))

	// Get remote branch
	remoteCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "@{upstream}")
	remoteCmd.Dir = wt.workDir
	if remoteOut, err := remoteCmd.Output(); err == nil {
		update.RemoteBranch = strings.TrimSpace(string(remoteOut))
	}

	// Get current HEAD commit SHA
	headCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	headCmd.Dir = wt.workDir
	if headOut, err := headCmd.Output(); err == nil {
		update.HeadCommit = strings.TrimSpace(string(headOut))
	}

	// Get base commit SHA (base branch HEAD)
	// Use remote branch if available, fall back to main/master
	baseBranch := update.RemoteBranch
	if baseBranch == "" {
		// Try main first, then master
		baseBranch = "main"
		checkCmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "main")
		checkCmd.Dir = wt.workDir
		if err := checkCmd.Run(); err != nil {
			baseBranch = "master"
		}
	}
	baseCmd := exec.CommandContext(ctx, "git", "rev-parse", baseBranch)
	baseCmd.Dir = wt.workDir
	if baseOut, err := baseCmd.Output(); err == nil {
		update.BaseCommit = strings.TrimSpace(string(baseOut))
	}

	// Get ahead/behind counts
	if update.RemoteBranch != "" {
		countCmd := exec.CommandContext(ctx, "git", "rev-list", "--left-right", "--count", update.Branch+"..."+update.RemoteBranch)
		countCmd.Dir = wt.workDir
		if countOut, err := countCmd.Output(); err == nil {
			parts := strings.Fields(string(countOut))
			if len(parts) == 2 {
				update.Ahead, _ = strconv.Atoi(parts[0])
				update.Behind, _ = strconv.Atoi(parts[1])
			}
		}
	}

	// Get file status using git status --porcelain
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = wt.workDir
	statusOut, err := statusCmd.Output()
	if err != nil {
		return update, err
	}

	// Parse porcelain output
	// Git status --porcelain format: XY filename
	// X = index (staged) status, Y = working tree (unstaged) status
	// ' ' = unmodified, M = modified, A = added, D = deleted, R = renamed, ? = untracked
	lines := strings.Split(string(statusOut), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		indexStatus := line[0]    // Staged status (X)
		workTreeStatus := line[1] // Unstaged status (Y)
		filePath := strings.TrimSpace(line[3:])

		fileInfo := types.FileInfo{
			Path: filePath,
		}

		// Determine staged status based on index and worktree status.
		// A file's change is "staged" if the relevant change is in the index.
		// If there's a worktree change (like deletion), that change is unstaged.
		//
		// Examples:
		// "M " = modified and staged (Staged=true)
		// " M" = modified but unstaged (Staged=false)
		// "MM" = modified staged + more unstaged modifications (Staged=false for the current state)
		// "A " = added and staged (Staged=true)
		// "AD" = added to index but deleted in worktree - deletion is unstaged (Staged=false)
		// "D " = deleted and staged (Staged=true)
		// " D" = deleted but unstaged (Staged=false)

		// Parse status - prioritize worktree changes as they represent the current state
		switch {
		case indexStatus == '?' && workTreeStatus == '?':
			fileInfo.Status = "untracked"
			fileInfo.Staged = false
			update.Untracked = append(update.Untracked, filePath)
		case workTreeStatus == 'D':
			// File deleted in worktree - this is an unstaged deletion
			// Even if there were staged changes before (A, M), the deletion is unstaged
			fileInfo.Status = "deleted"
			fileInfo.Staged = false
			update.Deleted = append(update.Deleted, filePath)
		case indexStatus == 'D':
			// File deleted and staged (no worktree status means deletion is complete)
			fileInfo.Status = "deleted"
			fileInfo.Staged = true
			update.Deleted = append(update.Deleted, filePath)
		case workTreeStatus == 'M':
			// Modified in worktree - unstaged modification
			fileInfo.Status = "modified"
			fileInfo.Staged = false
			update.Modified = append(update.Modified, filePath)
		case indexStatus == 'M':
			// Modified and staged (no worktree changes)
			fileInfo.Status = "modified"
			fileInfo.Staged = true
			update.Modified = append(update.Modified, filePath)
		case indexStatus == 'A':
			// Added and staged
			fileInfo.Status = "added"
			fileInfo.Staged = true
			update.Added = append(update.Added, filePath)
		case indexStatus == 'R':
			fileInfo.Status = "renamed"
			fileInfo.Staged = true
			// Renamed files have format "old -> new"
			if idx := strings.Index(filePath, " -> "); idx != -1 {
				fileInfo.OldPath = filePath[:idx]
				fileInfo.Path = filePath[idx+4:]
			}
			update.Renamed = append(update.Renamed, filePath)
		}

		update.Files[filePath] = fileInfo
	}

	// Enrich file info with diff data (additions, deletions, and actual diff content)
	wt.enrichWithDiffData(ctx, &update)

	return update, nil
}

// enrichWithDiffData adds diff information (additions, deletions, diff content) to file info
func (wt *WorkspaceTracker) enrichWithDiffData(ctx context.Context, update *types.GitStatusUpdate) {
	// Determine the base ref to compare against
	// Use remote branch if available, otherwise use HEAD
	baseRef := "HEAD"
	if update.RemoteBranch != "" {
		baseRef = update.RemoteBranch
	}

	// Get numstat for additions/deletions count (compare against base branch)
	numstatCmd := exec.CommandContext(ctx, "git", "diff", "--numstat", baseRef)
	numstatCmd.Dir = wt.workDir
	numstatOut, err := numstatCmd.Output()
	if err != nil {
		wt.logger.Debug("failed to get numstat", zap.Error(err))
		return
	}

	// Parse numstat output and update file info
	lines := strings.Split(string(numstatOut), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		additions, _ := strconv.Atoi(parts[0])
		deletions, _ := strconv.Atoi(parts[1])
		filePath := parts[2]

		// Only update file info if it exists in status (uncommitted changes).
		// Files that appear in diff but not in status are committed changes - we don't
		// add them to the Files map as that would make git status show already-committed files.
		if fileInfo, exists := update.Files[filePath]; exists {
			fileInfo.Additions = additions
			fileInfo.Deletions = deletions

			// Get the actual diff content for this file (compare against base branch)
			diffCmd := exec.CommandContext(ctx, "git", "diff", baseRef, "--", filePath)
			diffCmd.Dir = wt.workDir
			if diffOut, err := diffCmd.Output(); err == nil {
				fileInfo.Diff = string(diffOut)
			}

			update.Files[filePath] = fileInfo
		}
		// NOTE: We intentionally do NOT add files that are in the diff but not in git status.
		// Those are committed changes, not uncommitted changes. The UI should only show
		// uncommitted changes in the "files changed" count.
	}

	// Also check for staged changes
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--numstat")
	stagedCmd.Dir = wt.workDir
	stagedOut, err := stagedCmd.Output()
	if err != nil {
		return
	}

	lines = strings.Split(string(stagedOut), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		additions, _ := strconv.Atoi(parts[0])
		deletions, _ := strconv.Atoi(parts[1])
		filePath := parts[2]

		// Update existing file info if it exists
		if fileInfo, exists := update.Files[filePath]; exists {
			// Add staged changes to the counts
			fileInfo.Additions += additions
			fileInfo.Deletions += deletions

			// Get the staged diff content
			diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--", filePath)
			diffCmd.Dir = wt.workDir
			if diffOut, err := diffCmd.Output(); err == nil {
				// Append staged diff to existing diff
				if fileInfo.Diff != "" {
					fileInfo.Diff += "\n\n--- Staged changes ---\n" + string(diffOut)
				} else {
					fileInfo.Diff = string(diffOut)
				}
			}

			update.Files[filePath] = fileInfo
		}
	}

	// For untracked files, show the entire file content as "added"
	for filePath, fileInfo := range update.Files {
		if fileInfo.Status == "untracked" {
			// Show file content as diff (all additions)
			catCmd := exec.CommandContext(ctx, "cat", filePath)
			catCmd.Dir = wt.workDir
			if catOut, err := catCmd.Output(); err == nil {
				content := string(catOut)
				lines := strings.Split(content, "\n")
				fileInfo.Additions = len(lines)
				fileInfo.Deletions = 0

				// Format as a proper git diff with all required headers
				// The @git-diff-view/react library requires the full git diff format
				var diffBuilder strings.Builder
				diffBuilder.WriteString("diff --git a/" + filePath + " b/" + filePath + "\n")
				diffBuilder.WriteString("new file mode 100644\n")
				diffBuilder.WriteString("index 0000000..0000000\n")
				diffBuilder.WriteString("--- /dev/null\n")
				diffBuilder.WriteString("+++ b/" + filePath + "\n")
				diffBuilder.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
				for _, line := range lines {
					diffBuilder.WriteString("+" + line + "\n")
				}
				fileInfo.Diff = diffBuilder.String()

				update.Files[filePath] = fileInfo
			}
		}
	}
}

// updateFiles updates the file listing
func (wt *WorkspaceTracker) updateFiles(ctx context.Context) {
	files, err := wt.getFileList(ctx)
	if err != nil {
		wt.logger.Debug("failed to get file list", zap.Error(err))
		return
	}

	wt.mu.Lock()
	wt.currentFiles = files
	wt.mu.Unlock()

	// Notify workspace stream subscribers
	wt.notifyWorkspaceStreamFileList(files)
}

// getFileList retrieves the list of files in the workspace
func (wt *WorkspaceTracker) getFileList(ctx context.Context) (types.FileListUpdate, error) {
	update := types.FileListUpdate{
		Timestamp: time.Now(),
		Files:     []types.FileEntry{},
	}

	// Use git ls-files to get tracked files
	cmd := exec.CommandContext(ctx, "git", "ls-files")
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		return update, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		update.Files = append(update.Files, types.FileEntry{
			Path:  line,
			IsDir: false,
		})
	}

	return update, nil
}

// Unified workspace stream subscriber management methods

// SubscribeWorkspaceStream creates a new unified workspace stream subscriber
// and sends current git status and file list immediately
func (wt *WorkspaceTracker) SubscribeWorkspaceStream() types.WorkspaceStreamSubscriber {
	sub := make(types.WorkspaceStreamSubscriber, 100)

	wt.workspaceSubMu.Lock()
	wt.workspaceStreamSubscribers[sub] = struct{}{}
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.Unlock()
	wt.logger.Info("workspace stream subscriber added", zap.Int("subscribers", count))

	// Send current git status immediately
	wt.mu.RLock()
	currentStatus := wt.currentStatus
	currentFiles := wt.currentFiles
	wt.mu.RUnlock()

	if currentStatus.Timestamp.IsZero() {
		currentStatus.Timestamp = time.Now()
	}
	if currentFiles.Timestamp.IsZero() {
		currentFiles.Timestamp = time.Now()
	}

	// Send git status
	select {
	case sub <- types.NewWorkspaceGitStatus(&currentStatus):
	default:
	}

	// Send file list
	select {
	case sub <- types.NewWorkspaceFileList(&currentFiles):
	default:
	}

	return sub
}

// UnsubscribeWorkspaceStream removes and closes a workspace stream subscriber
func (wt *WorkspaceTracker) UnsubscribeWorkspaceStream(sub types.WorkspaceStreamSubscriber) {
	wt.workspaceSubMu.Lock()
	delete(wt.workspaceStreamSubscribers, sub)
	count := len(wt.workspaceStreamSubscribers)
	wt.workspaceSubMu.Unlock()
	close(sub)
	wt.logger.Info("workspace stream subscriber removed", zap.Int("subscribers", count))
}

// Notification methods for unified workspace stream

// notifyWorkspaceStreamGitStatus sends git status to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamGitStatus(update types.GitStatusUpdate) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceGitStatus(&update)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamGitCommit sends git commit notification to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamGitCommit(commit *types.GitCommitNotification) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceGitCommit(commit)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// NotifyGitCommit notifies all subscribers about a new git commit
func (wt *WorkspaceTracker) NotifyGitCommit(commit *types.GitCommitNotification) {
	wt.notifyWorkspaceStreamGitCommit(commit)
}

// notifyWorkspaceStreamFileChange sends file change notification to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamFileChange(notification types.FileChangeNotification) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceFileChange(&notification)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamFileList sends file list update to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamFileList(update types.FileListUpdate) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceFileList(&update)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamProcessOutput sends process output to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamProcessOutput(output *types.ProcessOutput) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceProcessOutput(output)
	wt.logger.Debug("broadcast process output",
		zap.String("session_id", output.SessionID),
		zap.String("process_id", output.ProcessID),
		zap.String("kind", string(output.Kind)),
		zap.Int("subscribers", len(wt.workspaceStreamSubscribers)),
	)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyWorkspaceStreamProcessStatus sends process status updates to all workspace stream subscribers
func (wt *WorkspaceTracker) notifyWorkspaceStreamProcessStatus(status *types.ProcessStatusUpdate) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceProcessStatus(status)
	wt.logger.Debug("broadcast process status",
		zap.String("session_id", status.SessionID),
		zap.String("process_id", status.ProcessID),
		zap.String("status", string(status.Status)),
		zap.Int("subscribers", len(wt.workspaceStreamSubscribers)),
	)
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			// Subscriber is slow, skip
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

			// If a directory was created, watch it
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := wt.addDirectoryRecursive(event.Name); err != nil {
						wt.logger.Debug("failed to watch new directory", zap.Error(err))
					}
				}
			}

			// Trigger debounced update (non-blocking)
			select {
			case wt.fsChangeTrigger <- struct{}{}:
			default:
				// Channel full, update already pending
			}

		case err, ok := <-wt.watcher.Errors:
			if !ok {
				return
			}
			wt.logger.Debug("filesystem watcher error", zap.Error(err))
		}
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

// GetFileTree returns the file tree for a given path and depth
func (wt *WorkspaceTracker) GetFileTree(reqPath string, depth int) (*types.FileTreeNode, error) {
	// Resolve the full path with path traversal protection
	fullPath := filepath.Join(wt.workDir, filepath.Clean(reqPath))
	cleanWorkDir := filepath.Clean(wt.workDir)
	if !strings.HasPrefix(fullPath, cleanWorkDir+string(os.PathSeparator)) && fullPath != cleanWorkDir {
		return nil, fmt.Errorf("path traversal detected")
	}

	// Check if path exists
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	// Build the tree
	node, err := wt.buildFileTreeNode(fullPath, reqPath, info, depth, 0)
	if err != nil {
		return nil, err
	}

	return node, nil
}

// buildFileTreeNode recursively builds a file tree node
func (wt *WorkspaceTracker) buildFileTreeNode(fullPath, relPath string, info os.FileInfo, maxDepth, currentDepth int) (*types.FileTreeNode, error) {
	node := &types.FileTreeNode{
		Name:  info.Name(),
		Path:  relPath,
		IsDir: info.IsDir(),
		Size:  info.Size(),
	}

	// If it's a file or we've reached max depth, return
	if !info.IsDir() || (maxDepth > 0 && currentDepth >= maxDepth) {
		return node, nil
	}

	// Read directory contents
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return node, nil // Return node without children on error
	}

	// Build children
	node.Children = make([]*types.FileTreeNode, 0, len(entries))
	for _, entry := range entries {
		// Skip hidden files and common ignore patterns
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != "." && name != ".." {
			continue
		}
		if name == "node_modules" || name == ".next" || name == "dist" || name == "build" {
			continue
		}

		childFullPath := filepath.Join(fullPath, name)
		childRelPath := filepath.Join(relPath, name)

		childInfo, err := entry.Info()
		if err != nil {
			continue
		}

		childNode, err := wt.buildFileTreeNode(childFullPath, childRelPath, childInfo, maxDepth, currentDepth+1)
		if err != nil {
			continue
		}

		node.Children = append(node.Children, childNode)
	}

	return node, nil
}

// GetFileContent returns the content of a file
func (wt *WorkspaceTracker) GetFileContent(reqPath string) (string, int64, error) {
	// Resolve the full path with path traversal protection
	fullPath := filepath.Join(wt.workDir, filepath.Clean(reqPath))
	cleanWorkDir := filepath.Clean(wt.workDir)
	if !strings.HasPrefix(fullPath, cleanWorkDir+string(os.PathSeparator)) && fullPath != cleanWorkDir {
		return "", 0, fmt.Errorf("path traversal detected")
	}

	// Check if file exists and is a regular file
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return "", 0, fmt.Errorf("path is a directory, not a file")
	}

	// Check file size (limit to 10MB)
	const maxFileSize = 10 * 1024 * 1024
	if info.Size() > maxFileSize {
		return "", info.Size(), fmt.Errorf("file too large (max 10MB)")
	}

	// Read file content
	file, err := os.Open(fullPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), info.Size(), nil
}
