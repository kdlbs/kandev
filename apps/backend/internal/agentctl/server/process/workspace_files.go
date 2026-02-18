package process

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

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
		// Skip specific directories that should be ignored
		name := entry.Name()
		if name == ".git" || name == "node_modules" || name == ".next" || name == "dist" || name == "build" {
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

// resolveSafePath resolves reqPath to an absolute path within workDir,
// rejecting any path traversal attempts.
func (wt *WorkspaceTracker) resolveSafePath(reqPath string) (string, error) {
	cleanWorkDir := filepath.Clean(wt.workDir)
	cleanReqPath := filepath.Clean(reqPath)

	var fullPath string
	if filepath.IsAbs(cleanReqPath) && strings.HasPrefix(cleanReqPath, cleanWorkDir+string(os.PathSeparator)) {
		fullPath = cleanReqPath
	} else {
		fullPath = filepath.Join(wt.workDir, cleanReqPath)
	}

	if !strings.HasPrefix(fullPath, cleanWorkDir+string(os.PathSeparator)) && fullPath != cleanWorkDir {
		return "", fmt.Errorf("path traversal detected")
	}

	return fullPath, nil
}

// GetFileContent returns the content of a file.
// If the file is not valid UTF-8, it is base64-encoded and isBinary is true.
func (wt *WorkspaceTracker) GetFileContent(reqPath string) (string, int64, bool, error) {
	fullPath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return "", 0, false, err
	}

	// Check if file exists and is a regular file
	info, err := os.Stat(fullPath)
	if err != nil {
		return "", 0, false, fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return "", 0, false, fmt.Errorf("path is a directory, not a file")
	}

	// Check file size (limit to 10MB)
	const maxFileSize = 10 * 1024 * 1024
	if info.Size() > maxFileSize {
		return "", info.Size(), false, fmt.Errorf("file too large (max 10MB)")
	}

	// Read file content
	file, err := os.Open(fullPath)
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to read file: %w", err)
	}

	// Detect binary: if content is not valid UTF-8, base64-encode it
	if !utf8.Valid(content) {
		encoded := base64.StdEncoding.EncodeToString(content)
		return encoded, info.Size(), true, nil
	}

	return string(content), info.Size(), false, nil
}

// ApplyFileDiff applies a unified diff to a file with conflict detection
// Uses git apply for reliable, battle-tested patch application
func (wt *WorkspaceTracker) ApplyFileDiff(reqPath string, unifiedDiff string, originalHash string) (string, error) {
	fullPath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return "", err
	}

	cleanWorkDir := filepath.Clean(wt.workDir)

	// Read current file content
	currentContent, _, _, err := wt.GetFileContent(reqPath)
	if err != nil {
		return "", fmt.Errorf("failed to read current file: %w", err)
	}

	// Calculate hash of current content for conflict detection
	currentHash := calculateSHA256(currentContent)
	if originalHash != "" && currentHash != originalHash {
		return "", fmt.Errorf("conflict detected: file has been modified (expected hash %s, got %s)", originalHash, currentHash)
	}

	// Write diff to a temporary patch file
	patchFile := filepath.Join(wt.workDir, ".kandev-patch.tmp")
	err = os.WriteFile(patchFile, []byte(unifiedDiff), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write patch file: %w", err)
	}
	defer func() {
		_ = os.Remove(patchFile) // Best effort cleanup
	}()

	// Use git apply to apply the patch directly to the file
	// This is much more reliable than custom diff application
	cmd := exec.Command("git", "apply", "-p0", "--unidiff-zero", "--whitespace=nowarn", patchFile)
	cmd.Dir = wt.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git apply failed: %w\nOutput: %s", err, string(output))
	}

	// Read the updated content
	updatedContent, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read updated file: %w", err)
	}

	// Calculate new hash
	newHash := calculateSHA256(string(updatedContent))

	// Notify with the relative path
	relPath := strings.TrimPrefix(fullPath, cleanWorkDir+string(os.PathSeparator))
	wt.addPendingChange(relPath, types.FileOpWrite)

	wt.logger.Debug("applied file diff using git apply",
		zap.String("path", relPath),
		zap.String("old_hash", currentHash),
		zap.String("new_hash", newHash),
	)

	return newHash, nil
}

// CreateFile creates a new file in the workspace
func (wt *WorkspaceTracker) CreateFile(reqPath string) error {
	fullPath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return err
	}

	// Create intermediate directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Atomically create the file, failing if it already exists
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("file already exists: %s", reqPath)
		}
		return fmt.Errorf("failed to create file: %w", err)
	}
	_ = f.Close()

	// Notify with the relative path
	cleanWorkDir := filepath.Clean(wt.workDir)
	relPath := strings.TrimPrefix(fullPath, cleanWorkDir+string(os.PathSeparator))
	wt.addPendingChange(relPath, types.FileOpCreate)

	return nil
}

// DeleteFile deletes a file from the workspace
func (wt *WorkspaceTracker) DeleteFile(reqPath string) error {
	fullPath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return err
	}

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", reqPath)
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Don't allow deleting directories
	if info.IsDir() {
		return fmt.Errorf("cannot delete directory: %s", reqPath)
	}

	// Delete the file
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Notify with the relative path
	cleanWorkDir := filepath.Clean(wt.workDir)
	relPath := strings.TrimPrefix(fullPath, cleanWorkDir+string(os.PathSeparator))
	wt.addPendingChange(relPath, types.FileOpRemove)

	return nil
}

// calculateSHA256 calculates the SHA256 hash of a string
func calculateSHA256(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// scoredMatch holds a file path and its match score for sorting
type scoredMatch struct {
	path  string
	score int
}

// SearchFiles searches for files matching the query string.
// It uses fuzzy matching with scoring based on how well the query matches.
func (wt *WorkspaceTracker) SearchFiles(query string, limit int) []string {
	if query == "" {
		return []string{}
	}
	if limit <= 0 {
		limit = 20
	}

	query = strings.ToLower(query)
	var matches []scoredMatch

	wt.mu.RLock()
	files := wt.currentFiles.Files
	wt.mu.RUnlock()

	for _, file := range files {
		path := file.Path
		lowerPath := strings.ToLower(path)
		name := filepath.Base(lowerPath)

		score := 0
		switch {
		case name == query:
			score = 100 // Exact filename match
		case strings.HasPrefix(name, query):
			score = 75 // Filename starts with query
		case strings.Contains(name, query):
			score = 50 // Filename contains query
		case strings.Contains(lowerPath, query):
			score = 25 // Path contains query
		}

		if score > 0 {
			matches = append(matches, scoredMatch{path: path, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		// Secondary sort by path length (shorter paths first)
		return len(matches[i].path) < len(matches[j].path)
	})

	// Return top limit results
	result := make([]string, 0, limit)
	for i := 0; i < len(matches) && i < limit; i++ {
		result = append(result, matches[i].path)
	}

	return result
}
