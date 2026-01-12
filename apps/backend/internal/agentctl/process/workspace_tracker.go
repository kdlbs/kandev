package process

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// GitStatusUpdate represents a git status update
type GitStatusUpdate struct {
	Timestamp    time.Time         `json:"timestamp"`
	Modified     []string          `json:"modified"`
	Added        []string          `json:"added"`
	Deleted      []string          `json:"deleted"`
	Untracked    []string          `json:"untracked"`
	Renamed      []string          `json:"renamed"`
	Ahead        int               `json:"ahead"`
	Behind       int               `json:"behind"`
	Branch       string            `json:"branch"`
	RemoteBranch string            `json:"remote_branch,omitempty"`
	Files        map[string]FileInfo `json:"files,omitempty"`
}

// FileInfo represents information about a file
type FileInfo struct {
	Path      string `json:"path"`
	Status    string `json:"status"` // modified, added, deleted, untracked, renamed
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
	OldPath   string `json:"old_path,omitempty"` // For renamed files
	Diff      string `json:"diff,omitempty"`
}

// FileListUpdate represents a file listing update
type FileListUpdate struct {
	Timestamp time.Time  `json:"timestamp"`
	Files     []FileEntry `json:"files"`
}

// FileEntry represents a file in the workspace
type FileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

// DiffUpdate represents a diff update
type DiffUpdate struct {
	Timestamp time.Time         `json:"timestamp"`
	Files     map[string]FileInfo `json:"files"`
}

// GitStatusSubscriber is a channel that receives git status updates
type GitStatusSubscriber chan GitStatusUpdate

// FilesSubscriber is a channel that receives file listing updates
type FilesSubscriber chan FileListUpdate

// DiffSubscriber is a channel that receives diff updates
type DiffSubscriber chan DiffUpdate

// WorkspaceTracker monitors workspace changes and provides real-time updates
type WorkspaceTracker struct {
	workDir string
	logger  *logger.Logger

	// Current state
	currentStatus GitStatusUpdate
	currentFiles  FileListUpdate
	currentDiff   DiffUpdate
	mu            sync.RWMutex

	// Subscribers
	gitStatusSubscribers map[GitStatusSubscriber]struct{}
	filesSubscribers     map[FilesSubscriber]struct{}
	diffSubscribers      map[DiffSubscriber]struct{}
	subMu                sync.RWMutex

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewWorkspaceTracker creates a new workspace tracker
func NewWorkspaceTracker(workDir string, log *logger.Logger) *WorkspaceTracker {
	return &WorkspaceTracker{
		workDir:              workDir,
		logger:               log.WithFields(zap.String("component", "workspace-tracker")),
		gitStatusSubscribers: make(map[GitStatusSubscriber]struct{}),
		filesSubscribers:     make(map[FilesSubscriber]struct{}),
		diffSubscribers:      make(map[DiffSubscriber]struct{}),
		stopCh:               make(chan struct{}),
	}
}

// Start begins monitoring the workspace
func (wt *WorkspaceTracker) Start(ctx context.Context) {
	wt.wg.Add(1)
	go wt.monitorLoop(ctx)
}

// Stop stops the workspace tracker
func (wt *WorkspaceTracker) Stop() {
	close(wt.stopCh)
	wt.wg.Wait()
	wt.logger.Info("workspace tracker stopped")
}

// monitorLoop periodically checks for workspace changes
func (wt *WorkspaceTracker) monitorLoop(ctx context.Context) {
	defer wt.wg.Done()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Initial update
	wt.updateGitStatus(ctx)
	wt.updateFiles(ctx)
	wt.updateDiff(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-wt.stopCh:
			return
		case <-ticker.C:
			wt.updateGitStatus(ctx)
			wt.updateFiles(ctx)
			wt.updateDiff(ctx)
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
func (wt *WorkspaceTracker) getGitStatus(ctx context.Context) (GitStatusUpdate, error) {
	update := GitStatusUpdate{
		Timestamp: time.Now(),
		Modified:  []string{},
		Added:     []string{},
		Deleted:   []string{},
		Untracked: []string{},
		Renamed:   []string{},
		Files:     make(map[string]FileInfo),
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

		fileInfo := FileInfo{
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
func (wt *WorkspaceTracker) enrichWithDiffData(ctx context.Context, update *GitStatusUpdate) {
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
			fileInfo := FileInfo{
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
func (wt *WorkspaceTracker) getFileList(ctx context.Context) (FileListUpdate, error) {
	update := FileListUpdate{
		Timestamp: time.Now(),
		Files:     []FileEntry{},
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
		update.Files = append(update.Files, FileEntry{
			Path:  line,
			IsDir: false,
		})
	}

	return update, nil
}

// updateDiff updates the diff information
func (wt *WorkspaceTracker) updateDiff(ctx context.Context) {
	diff, err := wt.getDiff(ctx)
	if err != nil {
		wt.logger.Debug("failed to get diff", zap.Error(err))
		return
	}

	wt.mu.Lock()
	wt.currentDiff = diff
	wt.mu.Unlock()

	// Notify subscribers
	wt.notifyDiffSubscribers(diff)
}

// getDiff retrieves diff information for changed files
func (wt *WorkspaceTracker) getDiff(ctx context.Context) (DiffUpdate, error) {
	update := DiffUpdate{
		Timestamp: time.Now(),
		Files:     make(map[string]FileInfo),
	}

	// Get current branch info to determine base ref
	status, err := wt.getGitStatus(ctx)
	if err != nil {
		return update, err
	}

	// Determine the base ref to compare against
	baseRef := "HEAD"
	if status.RemoteBranch != "" {
		baseRef = status.RemoteBranch
	}

	// Get diff with numstat for additions/deletions (compare against base branch)
	cmd := exec.CommandContext(ctx, "git", "diff", "--numstat", baseRef)
	cmd.Dir = wt.workDir
	out, err := cmd.Output()
	if err != nil {
		return update, err
	}

	// Parse numstat output
	lines := strings.Split(string(out), "\n")
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

		fileInfo := FileInfo{
			Path:      filePath,
			Status:    "modified",
			Additions: additions,
			Deletions: deletions,
		}

		// Get the actual diff for this file (compare against base branch)
		diffCmd := exec.CommandContext(ctx, "git", "diff", baseRef, "--", filePath)
		diffCmd.Dir = wt.workDir
		if diffOut, err := diffCmd.Output(); err == nil {
			fileInfo.Diff = string(diffOut)
		}

		update.Files[filePath] = fileInfo
	}

	// Also check for staged changes
	stagedCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--numstat")
	stagedCmd.Dir = wt.workDir
	if stagedOut, err := stagedCmd.Output(); err == nil {
		lines := strings.Split(string(stagedOut), "\n")
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

			fileInfo := FileInfo{
				Path:      filePath,
				Status:    "staged",
				Additions: additions,
				Deletions: deletions,
			}

			// Get the actual diff for this file
			diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--", filePath)
			diffCmd.Dir = wt.workDir
			if diffOut, err := diffCmd.Output(); err == nil {
				fileInfo.Diff = string(diffOut)
			}

			update.Files[filePath] = fileInfo
		}
	}

	return update, nil
}

// Subscriber management methods

// SubscribeGitStatus creates a new git status subscriber
func (wt *WorkspaceTracker) SubscribeGitStatus() GitStatusSubscriber {
	sub := make(GitStatusSubscriber, 10)

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
func (wt *WorkspaceTracker) UnsubscribeGitStatus(sub GitStatusSubscriber) {
	wt.subMu.Lock()
	delete(wt.gitStatusSubscribers, sub)
	wt.subMu.Unlock()
	close(sub)
}

// SubscribeFiles creates a new files subscriber
func (wt *WorkspaceTracker) SubscribeFiles() FilesSubscriber {
	sub := make(FilesSubscriber, 10)

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
func (wt *WorkspaceTracker) UnsubscribeFiles(sub FilesSubscriber) {
	wt.subMu.Lock()
	delete(wt.filesSubscribers, sub)
	wt.subMu.Unlock()
	close(sub)
}

// SubscribeDiff creates a new diff subscriber
func (wt *WorkspaceTracker) SubscribeDiff() DiffSubscriber {
	sub := make(DiffSubscriber, 10)

	wt.subMu.Lock()
	wt.diffSubscribers[sub] = struct{}{}
	wt.subMu.Unlock()

	// Send current diff immediately
	wt.mu.RLock()
	current := wt.currentDiff
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

// UnsubscribeDiff removes a diff subscriber
func (wt *WorkspaceTracker) UnsubscribeDiff(sub DiffSubscriber) {
	wt.subMu.Lock()
	delete(wt.diffSubscribers, sub)
	wt.subMu.Unlock()
	close(sub)
}

// Notification methods

// notifyGitStatusSubscribers notifies all git status subscribers
func (wt *WorkspaceTracker) notifyGitStatusSubscribers(update GitStatusUpdate) {
	wt.subMu.RLock()
	defer wt.subMu.RUnlock()

	for sub := range wt.gitStatusSubscribers {
		select {
		case sub <- update:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyFilesSubscribers notifies all files subscribers
func (wt *WorkspaceTracker) notifyFilesSubscribers(update FileListUpdate) {
	wt.subMu.RLock()
	defer wt.subMu.RUnlock()

	for sub := range wt.filesSubscribers {
		select {
		case sub <- update:
		default:
			// Subscriber is slow, skip
		}
	}
}

// notifyDiffSubscribers notifies all diff subscribers
func (wt *WorkspaceTracker) notifyDiffSubscribers(update DiffUpdate) {
	wt.subMu.RLock()
	defer wt.subMu.RUnlock()

	for sub := range wt.diffSubscribers {
		select {
		case sub <- update:
		default:
			// Subscriber is slow, skip
		}
	}
}

// GetCurrentGitStatus returns the current git status
func (wt *WorkspaceTracker) GetCurrentGitStatus() GitStatusUpdate {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return wt.currentStatus
}

// GetCurrentFiles returns the current file list
func (wt *WorkspaceTracker) GetCurrentFiles() FileListUpdate {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return wt.currentFiles
}

// GetCurrentDiff returns the current diff
func (wt *WorkspaceTracker) GetCurrentDiff() DiffUpdate {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return wt.currentDiff
}




