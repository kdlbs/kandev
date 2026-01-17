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

	// Subscribers (legacy - kept for backward compatibility)
	gitStatusSubscribers   map[types.GitStatusSubscriber]struct{}
	filesSubscribers       map[types.FilesSubscriber]struct{}
	fileChangeSubscribers  map[types.FileChangeSubscriber]struct{}
	subMu                  sync.RWMutex

	// Unified workspace stream subscribers
	workspaceStreamSubscribers map[types.WorkspaceStreamSubscriber]struct{}
	workspaceSubMu             sync.RWMutex

	// Filesystem watcher
	watcher *fsnotify.Watcher

	// Debounce channel for filesystem change events
	fsChangeTrigger chan struct{}

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
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
		gitStatusSubscribers:       make(map[types.GitStatusSubscriber]struct{}),
		filesSubscribers:           make(map[types.FilesSubscriber]struct{}),
		fileChangeSubscribers:      make(map[types.FileChangeSubscriber]struct{}),
		workspaceStreamSubscribers: make(map[types.WorkspaceStreamSubscriber]struct{}),
		watcher:                    watcher,
		fsChangeTrigger:            make(chan struct{}, 1), // Buffered to avoid blocking
		stopCh:                     make(chan struct{}),
	}
}

// Start begins monitoring the workspace
func (wt *WorkspaceTracker) Start(ctx context.Context) {
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
				// Notify file change subscribers that workspace changed
				wt.notifyFileChangeSubscribers(types.FileChangeNotification{
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

	// Notify subscribers
	wt.notifyGitStatusSubscribers(status)
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
	lines := strings.Split(string(statusOut), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}

		statusCode := line[:2]
		filePath := strings.TrimSpace(line[3:])

		fileInfo := types.FileInfo{
			Path: filePath,
		}

		// Parse status code
		switch {
		case strings.HasPrefix(statusCode, "M ") || strings.HasPrefix(statusCode, " M"):
			fileInfo.Status = "modified"
			update.Modified = append(update.Modified, filePath)
		case strings.HasPrefix(statusCode, "A "):
			fileInfo.Status = "added"
			update.Added = append(update.Added, filePath)
		case strings.HasPrefix(statusCode, "D ") || strings.HasPrefix(statusCode, " D"):
			fileInfo.Status = "deleted"
			update.Deleted = append(update.Deleted, filePath)
		case strings.HasPrefix(statusCode, "??"):
			fileInfo.Status = "untracked"
			update.Untracked = append(update.Untracked, filePath)
		case strings.HasPrefix(statusCode, "R "):
			fileInfo.Status = "renamed"
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

		// Update existing file info if it exists
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
		} else {
			// File might be committed but not in status (because it's committed)
			// Add it to the Files map
			fileInfo := types.FileInfo{
				Path:      filePath,
				Status:    "modified", // Assume modified if it has a diff
				Additions: additions,
				Deletions: deletions,
			}

			// Get the actual diff content for this file
			diffCmd := exec.CommandContext(ctx, "git", "diff", baseRef, "--", filePath)
			diffCmd.Dir = wt.workDir
			if diffOut, err := diffCmd.Output(); err == nil {
				fileInfo.Diff = string(diffOut)
			}

			update.Files[filePath] = fileInfo
			update.Modified = append(update.Modified, filePath)
		}
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

				// Format as a diff
				var diffBuilder strings.Builder
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

	// Notify subscribers
	wt.notifyFilesSubscribers(files)
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



// Subscriber management methods

// SubscribeGitStatus creates a new git status subscriber
func (wt *WorkspaceTracker) SubscribeGitStatus() types.GitStatusSubscriber {
	sub := make(types.GitStatusSubscriber, 10)

	wt.subMu.Lock()
	wt.gitStatusSubscribers[sub] = struct{}{}
	wt.subMu.Unlock()

	// Send current status immediately
	wt.mu.RLock()
	current := wt.currentStatus
	wt.mu.RUnlock()

	if current.Timestamp.IsZero() {
		current.Timestamp = time.Now()
	}

	select {
	case sub <- current:
	default:
	}

	return sub
}

// UnsubscribeGitStatus removes a git status subscriber
func (wt *WorkspaceTracker) UnsubscribeGitStatus(sub types.GitStatusSubscriber) {
	wt.subMu.Lock()
	delete(wt.gitStatusSubscribers, sub)
	wt.subMu.Unlock()
	close(sub)
}

// SubscribeFiles creates a new files subscriber
func (wt *WorkspaceTracker) SubscribeFiles() types.FilesSubscriber {
	sub := make(types.FilesSubscriber, 10)

	wt.subMu.Lock()
	wt.filesSubscribers[sub] = struct{}{}
	wt.subMu.Unlock()

	// Send current files immediately
	wt.mu.RLock()
	current := wt.currentFiles
	wt.mu.RUnlock()

	if current.Timestamp.IsZero() {
		current.Timestamp = time.Now()
	}

	select {
	case sub <- current:
	default:
	}

	return sub
}

// UnsubscribeFiles removes a files subscriber
func (wt *WorkspaceTracker) UnsubscribeFiles(sub types.FilesSubscriber) {
	wt.subMu.Lock()
	delete(wt.filesSubscribers, sub)
	wt.subMu.Unlock()
	close(sub)
}

// Notification methods

// notifyGitStatusSubscribers notifies all git status subscribers
func (wt *WorkspaceTracker) notifyGitStatusSubscribers(update types.GitStatusUpdate) {
	wt.subMu.RLock()
	defer wt.subMu.RUnlock()

	for sub := range wt.gitStatusSubscribers {
		select {
		case sub <- update:
		default:
			// Subscriber is slow, skip
		}
	}

	// Also notify unified workspace stream subscribers
	wt.notifyWorkspaceStreamGitStatus(update)
}

// notifyFilesSubscribers notifies all files subscribers
func (wt *WorkspaceTracker) notifyFilesSubscribers(update types.FileListUpdate) {
	wt.subMu.RLock()
	defer wt.subMu.RUnlock()

	for sub := range wt.filesSubscribers {
		select {
		case sub <- update:
		default:
			// Subscriber is slow, skip
		}
	}

	// Also notify unified workspace stream subscribers
	wt.notifyWorkspaceStreamFileList(update)
}



// SubscribeFileChanges creates a new file change subscriber
func (wt *WorkspaceTracker) SubscribeFileChanges() types.FileChangeSubscriber {
	sub := make(types.FileChangeSubscriber, 100)

	wt.subMu.Lock()
	wt.fileChangeSubscribers[sub] = struct{}{}
	wt.subMu.Unlock()

	return sub
}

// UnsubscribeFileChanges removes a file change subscriber
func (wt *WorkspaceTracker) UnsubscribeFileChanges(sub types.FileChangeSubscriber) {
	wt.subMu.Lock()
	delete(wt.fileChangeSubscribers, sub)
	wt.subMu.Unlock()
	close(sub)
}

// notifyFileChangeSubscribers notifies all file change subscribers
func (wt *WorkspaceTracker) notifyFileChangeSubscribers(notification types.FileChangeNotification) {
	wt.subMu.RLock()
	defer wt.subMu.RUnlock()

	for sub := range wt.fileChangeSubscribers {
		select {
		case sub <- notification:
		default:
			// Subscriber is slow, skip
		}
	}

	// Also notify unified workspace stream subscribers
	wt.notifyWorkspaceStreamFileChange(notification)
}

// Unified Workspace Stream subscriber methods

// SubscribeWorkspaceStream creates a new unified workspace stream subscriber.
// This subscriber receives all workspace events (git status, file changes, file list)
// through a single channel.
func (wt *WorkspaceTracker) SubscribeWorkspaceStream() types.WorkspaceStreamSubscriber {
	sub := make(types.WorkspaceStreamSubscriber, 100)

	wt.workspaceSubMu.Lock()
	wt.workspaceStreamSubscribers[sub] = struct{}{}
	wt.workspaceSubMu.Unlock()

	// Send current git status immediately
	wt.mu.RLock()
	currentStatus := wt.currentStatus
	currentFiles := wt.currentFiles
	wt.mu.RUnlock()

	// Send initial git status if available
	if !currentStatus.Timestamp.IsZero() {
		select {
		case sub <- types.NewWorkspaceGitStatus(&currentStatus):
		default:
		}
	}

	// Send initial file list if available
	if !currentFiles.Timestamp.IsZero() {
		select {
		case sub <- types.NewWorkspaceFileList(&currentFiles):
		default:
		}
	}

	return sub
}

// UnsubscribeWorkspaceStream removes a unified workspace stream subscriber
func (wt *WorkspaceTracker) UnsubscribeWorkspaceStream(sub types.WorkspaceStreamSubscriber) {
	wt.workspaceSubMu.Lock()
	delete(wt.workspaceStreamSubscribers, sub)
	wt.workspaceSubMu.Unlock()
	close(sub)
}

// notifyWorkspaceStreamGitStatus notifies unified stream subscribers of git status update
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

// notifyWorkspaceStreamFileChange notifies unified stream subscribers of file change
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

// notifyWorkspaceStreamFileList notifies unified stream subscribers of file list update
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

// NotifyWorkspaceStreamMessage sends a workspace message to all unified stream subscribers.
// This is used by the API server to forward shell output to subscribers.
func (wt *WorkspaceTracker) NotifyWorkspaceStreamMessage(msg types.WorkspaceStreamMessage) {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

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
	// Resolve the full path
	fullPath := filepath.Join(wt.workDir, reqPath)

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
	// Resolve the full path
	fullPath := filepath.Join(wt.workDir, reqPath)

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


