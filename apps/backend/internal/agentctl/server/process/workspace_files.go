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

	// Use git ls-files to get tracked files AND untracked files (excluding ignored)
	// --cached: include tracked files
	// --others: include untracked files
	// --exclude-standard: respect .gitignore
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard")
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
	safePath := filepath.Join(wt.workDir, filepath.Clean(reqPath))
	cleanWorkDir := filepath.Clean(wt.workDir)
	if !strings.HasPrefix(safePath, cleanWorkDir+string(os.PathSeparator)) && safePath != cleanWorkDir {
		return nil, fmt.Errorf("path traversal detected")
	}

	// Check if path exists
	info, err := os.Stat(safePath)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	// Build the tree
	node, err := wt.buildFileTreeNode(safePath, reqPath, info, depth, 0)
	if err != nil {
		return nil, err
	}

	return node, nil
}

// buildFileTreeNode recursively builds a file tree node
func (wt *WorkspaceTracker) buildFileTreeNode(safePath, relPath string, info os.FileInfo, maxDepth, currentDepth int) (*types.FileTreeNode, error) {
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
	entries, err := os.ReadDir(safePath)
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

		childFullPath := filepath.Join(safePath, name)
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

// resolvedWorkDir returns the workspace directory with symlinks resolved.
func (wt *WorkspaceTracker) resolvedWorkDir() string {
	resolved, err := filepath.EvalSymlinks(filepath.Clean(wt.workDir))
	if err != nil {
		return filepath.Clean(wt.workDir)
	}
	return resolved
}

// resolveSafePath resolves reqPath to an absolute path within workDir,
// rejecting any path traversal attempts. The returned path is always
// constructed as filepath.Join(resolvedWorkDir, validatedRelPath) so that
// static analysis tools (CodeQL) can verify it stays within the workspace.
func (wt *WorkspaceTracker) resolveSafePath(reqPath string) (string, error) {
	cleanWorkDir := filepath.Clean(wt.workDir)
	cleanReqPath := filepath.Clean(reqPath)

	// Resolve workspace directory symlinks first so that all constructed
	// paths share the same canonical prefix (e.g. /private/var on macOS
	// where /var is a symlink).
	realWorkDir, err := filepath.EvalSymlinks(cleanWorkDir)
	if err != nil {
		realWorkDir = cleanWorkDir
	}

	var safePath string
	if filepath.IsAbs(cleanReqPath) {
		safePath = cleanReqPath
	} else {
		safePath = filepath.Join(realWorkDir, cleanReqPath)
	}

	// Resolve symlinks to prevent bypassing validation
	realPath, err := filepath.EvalSymlinks(safePath)
	if err != nil {
		// If EvalSymlinks fails, the path might not exist yet (e.g., creating new file)
		// In that case, resolve the parent directory
		parentDir := filepath.Dir(safePath)
		realParent, parentErr := filepath.EvalSymlinks(parentDir)
		if parentErr != nil {
			// Parent doesn't exist either, use the cleaned full path
			realPath = safePath
		} else {
			realPath = filepath.Join(realParent, filepath.Base(safePath))
		}
	}

	// Check that the real path is within the workspace
	relPath, err := filepath.Rel(realWorkDir, realPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Ensure the relative path doesn't escape the workspace
	if strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) || relPath == ".." {
		return "", fmt.Errorf("path traversal detected: %s", reqPath)
	}

	// Reconstruct the absolute path from the trusted workspace root and the
	// validated relative path. This ensures the returned path is provably
	// inside the workspace, satisfying static-analysis taint checks.
	return filepath.Join(realWorkDir, relPath), nil
}

// GetFileContent returns the content of a file.
// If the file is not valid UTF-8, it is base64-encoded and isBinary is true.
func (wt *WorkspaceTracker) GetFileContent(reqPath string) (string, int64, bool, error) {
	safePath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return "", 0, false, err
	}

	// Check if file exists and is a regular file
	info, err := os.Stat(safePath)
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
	file, err := os.Open(safePath)
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
// Uses git apply for reliable, battle-tested patch application.
// For symlinked files, resolves to the real path and rewrites the diff header
// so that git apply operates on the actual file.
func (wt *WorkspaceTracker) ApplyFileDiff(reqPath string, unifiedDiff string, originalHash string) (string, error) {
	safePath, err := wt.resolveSafePath(reqPath)
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

	// If the file is a symlink, resolve to the real path and rewrite the diff header.
	// git apply cannot patch through symlinks — it needs the real file path.
	// Note: safePath is already resolved by resolveSafePath, so we check the
	// unresolved path to detect whether the original request targets a symlink.
	applyPath := reqPath
	cleanReqPath := filepath.Clean(reqPath)
	// Validate path doesn't attempt traversal before constructing filesystem path
	if strings.Contains(cleanReqPath, "..") || filepath.IsAbs(cleanReqPath) {
		return "", fmt.Errorf("invalid path: %s", reqPath)
	}
	unresolvedPath := filepath.Join(wt.workDir, cleanReqPath)
	if info, lErr := os.Lstat(unresolvedPath); lErr == nil && info.Mode()&os.ModeSymlink != 0 {
		// File is a symlink. safePath already points to the real target.
		realWorkDir, _ := filepath.EvalSymlinks(cleanWorkDir)
		if realWorkDir == "" {
			realWorkDir = cleanWorkDir
		}
		realRel, relErr := filepath.Rel(realWorkDir, safePath)
		if relErr == nil {
			unifiedDiff = rewriteDiffPaths(unifiedDiff, reqPath, realRel)
			applyPath = realRel
		}
	}

	// Write diff to a temporary patch file
	patchFile := filepath.Join(wt.workDir, ".kandev-patch.tmp")
	err = os.WriteFile(patchFile, []byte(unifiedDiff), 0o644)
	if err != nil {
		return "", fmt.Errorf("failed to write patch file: %w", err)
	}
	defer func() {
		_ = os.Remove(patchFile) // Best effort cleanup
	}()

	// Use git apply to apply the patch directly to the file
	cmd := exec.Command("git", "apply", "-p0", "--unidiff-zero", "--whitespace=nowarn", patchFile)
	cmd.Dir = wt.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git apply failed: %w\nOutput: %s", err, string(output))
	}

	// Read the updated content (use the real file path for symlinks)
	readPath := filepath.Join(cleanWorkDir, applyPath)
	updatedContent, err := os.ReadFile(readPath)
	if err != nil {
		return "", fmt.Errorf("failed to read updated file: %w", err)
	}

	// Calculate new hash
	newHash := calculateSHA256(string(updatedContent))

	// Notify with the original relative path (not the resolved symlink target)
	relPath := strings.TrimPrefix(safePath, cleanWorkDir+string(os.PathSeparator))
	wt.notifyFileChange(relPath, types.FileOpWrite)

	wt.logger.Debug("applied file diff using git apply",
		zap.String("path", relPath),
		zap.String("old_hash", currentHash),
		zap.String("new_hash", newHash),
	)

	return newHash, nil
}

// rewriteDiffPaths replaces file paths in unified diff headers.
// Handles both "--- a/old" / "+++ b/new" (with strip prefix) and
// "--- old" / "+++ new" (p0 mode) formats.
func rewriteDiffPaths(diff, oldPath, newPath string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			lines[i] = replaceDiffPath(line, "--- ", oldPath, newPath)
		} else if strings.HasPrefix(line, "+++ ") {
			lines[i] = replaceDiffPath(line, "+++ ", oldPath, newPath)
		}
	}
	return strings.Join(lines, "\n")
}

// replaceDiffPath replaces oldPath with newPath in a diff header line.
func replaceDiffPath(line, prefix, oldPath, newPath string) string {
	rest := line[len(prefix):]
	// Handle "--- a/path" or "--- path" formats
	cleaned := strings.TrimPrefix(rest, "a/")
	cleaned = strings.TrimPrefix(cleaned, "b/")
	if cleaned == oldPath || filepath.Clean(cleaned) == filepath.Clean(oldPath) {
		return prefix + newPath
	}
	return line
}

// CreateFile creates a new file in the workspace
func (wt *WorkspaceTracker) CreateFile(reqPath string) error {
	safePath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return err
	}

	// Create intermediate directories
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Atomically create the file, failing if it already exists
	f, err := os.OpenFile(safePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("file already exists: %s", reqPath)
		}
		return fmt.Errorf("failed to create file: %w", err)
	}
	_ = f.Close()

	// Notify with the relative path
	cleanWorkDir := filepath.Clean(wt.workDir)
	relPath := strings.TrimPrefix(safePath, cleanWorkDir+string(os.PathSeparator))
	wt.notifyFileChange(relPath, types.FileOpCreate)

	return nil
}

// DeleteFile deletes a file or directory from the workspace.
func (wt *WorkspaceTracker) DeleteFile(reqPath string) error {
	safePath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return err
	}

	cleanWorkDir := wt.resolvedWorkDir()
	if safePath == cleanWorkDir {
		return fmt.Errorf("cannot delete workspace root")
	}
	if err := wt.validateWorkspacePaths(safePath); err != nil {
		return err
	}

	// Check if file exists
	info, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", reqPath)
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		if err := os.RemoveAll(safePath); err != nil {
			return fmt.Errorf("failed to delete directory: %w", err)
		}
	} else {
		if err := os.Remove(safePath); err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}

	relPath := strings.TrimPrefix(safePath, cleanWorkDir+string(os.PathSeparator))
	wt.notifyFileChange(relPath, types.FileOpRemove)

	return nil
}

// RenameFile renames/moves a file or directory in the workspace.
func (wt *WorkspaceTracker) RenameFile(oldPath, newPath string) error {
	if oldPath == "" || newPath == "" {
		return fmt.Errorf("old_path and new_path are required")
	}

	oldSafePath, err := wt.resolveSafePath(oldPath)
	if err != nil {
		return err
	}
	newSafePath, err := wt.resolveSafePath(newPath)
	if err != nil {
		return err
	}

	if err := wt.validateWorkspacePaths(oldSafePath, newSafePath); err != nil {
		return err
	}
	if oldSafePath == newSafePath {
		return nil
	}

	if err := validateSourceExists(oldSafePath, oldPath); err != nil {
		return err
	}
	if err := validateTargetAvailable(newSafePath, newPath); err != nil {
		return err
	}

	parentDir := filepath.Dir(newSafePath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target parent directories: %w", err)
	}

	if err := os.Rename(oldSafePath, newSafePath); err != nil {
		return fmt.Errorf("failed to rename path: %w", err)
	}

	cleanWorkDir := wt.resolvedWorkDir()
	oldRelPath := strings.TrimPrefix(oldSafePath, cleanWorkDir+string(os.PathSeparator))
	newRelPath := strings.TrimPrefix(newSafePath, cleanWorkDir+string(os.PathSeparator))
	wt.notifyFileChange(oldRelPath, types.FileOpRename)
	if newRelPath != oldRelPath {
		wt.notifyFileChange(newRelPath, types.FileOpRename)
	}

	return nil
}

// validateWorkspacePaths checks that all provided paths are strictly inside the workspace.
func (wt *WorkspaceTracker) validateWorkspacePaths(paths ...string) error {
	cleanWorkDir := wt.resolvedWorkDir()
	workDirPrefix := cleanWorkDir + string(os.PathSeparator)
	for _, p := range paths {
		if !strings.HasPrefix(p, workDirPrefix) {
			return fmt.Errorf("path outside workspace")
		}
	}
	return nil
}

func validateSourceExists(safePath, reqPath string) error {
	_, err := os.Stat(safePath)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", reqPath)
	}
	return fmt.Errorf("failed to stat path: %w", err)
}

func validateTargetAvailable(safePath, reqPath string) error {
	_, err := os.Stat(safePath)
	if err == nil {
		return fmt.Errorf("target already exists: %s", reqPath)
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("failed to stat target: %w", err)
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
