package process

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	// ResolutionApplied indicates the diff was applied normally via git apply.
	ResolutionApplied = "applied"
	// ResolutionOverwritten indicates the file was overwritten with desired content.
	ResolutionOverwritten = "overwritten"
)

const maxFileSize = 10 * 1024 * 1024 // 10MB

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

		isSymlink := entry.Type()&os.ModeSymlink != 0
		var childInfo os.FileInfo
		if isSymlink {
			// Follow symlink to get target's info (IsDir, Size).
			// os.Stat returns ELOOP for circular symlinks, which we skip.
			childInfo, err = os.Stat(childFullPath)
		} else {
			childInfo, err = entry.Info()
		}
		if err != nil {
			continue
		}

		childNode, err := wt.buildFileTreeNode(childFullPath, childRelPath, childInfo, maxDepth, currentDepth+1)
		if err != nil {
			continue
		}
		childNode.IsSymlink = isSymlink

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

	// Resolve symlinks to prevent bypassing validation.
	// When the path doesn't exist yet, walk up until we find an existing
	// ancestor so symlinks (e.g. /tmp -> /private/tmp on macOS) are resolved
	// consistently with realWorkDir below. Only fall back for not-found errors;
	// propagate permission denied, symlink loops, etc.
	realPath, err := filepath.EvalSymlinks(safePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("failed to resolve path: %w", err)
		}
		realPath, err = resolveNonExistentPath(safePath)
		if err != nil {
			return "", err
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

// resolveNonExistentPath walks up from path until it finds an existing
// ancestor, resolves its symlinks, then reattaches the non-existent tail.
// This ensures paths under symlinked directories (e.g. /tmp on macOS)
// resolve consistently even when the target doesn't exist yet.
// Returns an error if an ancestor fails with something other than ErrNotExist
// (e.g. permission denied, symlink loop).
func resolveNonExistentPath(path string) (string, error) {
	current := path
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("failed to resolve ancestor %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %q: %w", path, err)
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}

// GetFileContent returns the content of a file.
// If the file is not valid UTF-8, it is base64-encoded and isBinary is true.
// If the file is a symlink, resolvedPath contains the target path relative to the workspace root.
func (wt *WorkspaceTracker) GetFileContent(reqPath string) (string, int64, bool, string, error) {
	safePath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return "", 0, false, "", err
	}

	// Check if the original path is a symlink and compute the resolved relative path.
	resolvedPath := wt.resolveSymlinkRelPath(reqPath)

	// Check if file exists and is a regular file
	info, err := os.Stat(safePath)
	if err != nil {
		return "", 0, false, "", fmt.Errorf("file not found: %w", err)
	}

	if info.IsDir() {
		return "", 0, false, "", fmt.Errorf("path is a directory, not a file")
	}

	// Check file size
	if info.Size() > maxFileSize {
		return "", info.Size(), false, "", fmt.Errorf("file too large (max 10MB)")
	}

	// Read file content
	file, err := os.Open(safePath)
	if err != nil {
		return "", 0, false, "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", 0, false, "", fmt.Errorf("failed to read file: %w", err)
	}

	// Detect binary: if content is not valid UTF-8, base64-encode it
	if !utf8.Valid(content) {
		encoded := base64.StdEncoding.EncodeToString(content)
		return encoded, info.Size(), true, resolvedPath, nil
	}

	return string(content), info.Size(), false, resolvedPath, nil
}

// resolveSymlinkRelPath checks if reqPath is a symlink and returns the resolved
// target path relative to the workspace root. Returns "" if not a symlink.
func (wt *WorkspaceTracker) resolveSymlinkRelPath(reqPath string) string {
	cleanReqPath := filepath.Clean(reqPath)
	if strings.Contains(cleanReqPath, "..") || filepath.IsAbs(cleanReqPath) {
		return ""
	}
	unresolvedPath := filepath.Join(wt.workDir, cleanReqPath)
	info, err := os.Lstat(unresolvedPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return ""
	}
	// Resolve the symlink target
	realTarget, err := filepath.EvalSymlinks(unresolvedPath)
	if err != nil {
		return ""
	}
	realWorkDir, _ := filepath.EvalSymlinks(filepath.Clean(wt.workDir))
	if realWorkDir == "" {
		realWorkDir = filepath.Clean(wt.workDir)
	}
	rel, err := filepath.Rel(realWorkDir, realTarget)
	if err != nil {
		return ""
	}
	return rel
}

// ApplyFileDiff applies a unified diff to a file with conflict detection.
// Uses git apply for reliable, battle-tested patch application.
// For symlinked files, resolves to the real path and rewrites the diff header
// so that git apply operates on the actual file.
// When desiredContent is provided and the diff cannot be applied (hash conflict),
// the file is overwritten with the desired content as a fallback.
// Returns the new hash and a resolution string ("applied" or "overwritten").
func (wt *WorkspaceTracker) ApplyFileDiff(reqPath, unifiedDiff, originalHash string, desiredContent *string) (string, string, error) {
	safePath, err := wt.resolveSafePath(reqPath)
	if err != nil {
		return "", "", err
	}

	cleanWorkDir := filepath.Clean(wt.workDir)

	// Read current file content
	currentContent, _, _, _, err := wt.GetFileContent(reqPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read current file: %w", err)
	}

	// Calculate hash of current content for conflict detection
	currentHash := calculateSHA256(currentContent)
	if originalHash != "" && currentHash != originalHash {
		if desiredContent != nil {
			return wt.writeDesiredContent(safePath, cleanWorkDir, reqPath, *desiredContent, currentHash)
		}
		return "", "", fmt.Errorf("conflict detected: file has been modified (expected hash %s, got %s)", originalHash, currentHash)
	}

	// If the file is a symlink, resolve to the real path and rewrite the diff header.
	// git apply cannot patch through symlinks — it needs the real file path.
	applyPath, unifiedDiff := wt.resolveSymlinkForDiff(reqPath, safePath, cleanWorkDir, unifiedDiff)

	// Write diff to a temporary patch file
	patchFile := filepath.Join(wt.workDir, ".kandev-patch.tmp")
	err = os.WriteFile(patchFile, []byte(unifiedDiff), 0o644)
	if err != nil {
		return "", "", fmt.Errorf("failed to write patch file: %w", err)
	}
	defer func() {
		_ = os.Remove(patchFile) // Best effort cleanup
	}()

	// Use git apply to apply the patch directly to the file
	cmd := exec.Command("git", "apply", "-p0", "--unidiff-zero", "--whitespace=nowarn", patchFile)
	cmd.Dir = wt.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		if desiredContent != nil {
			return wt.writeDesiredContent(safePath, cleanWorkDir, reqPath, *desiredContent, currentHash)
		}
		return "", "", fmt.Errorf("git apply failed: %w\nOutput: %s", err, string(output))
	}

	// Read the updated content (use the real file path for symlinks)
	readPath := filepath.Join(cleanWorkDir, applyPath)
	updatedContent, err := os.ReadFile(readPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read updated file: %w", err)
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

	return newHash, ResolutionApplied, nil
}

// resolveSymlinkForDiff checks if reqPath is a symlink and, if so, rewrites the diff
// paths to target the real file. Returns the path to use for git apply and the
// (possibly rewritten) diff.
func (wt *WorkspaceTracker) resolveSymlinkForDiff(
	reqPath, safePath, cleanWorkDir, unifiedDiff string,
) (string, string) {
	cleanReqPath := filepath.Clean(reqPath)
	if strings.Contains(cleanReqPath, "..") || filepath.IsAbs(cleanReqPath) {
		return reqPath, unifiedDiff
	}
	unresolvedPath := filepath.Join(wt.workDir, cleanReqPath)
	info, lErr := os.Lstat(unresolvedPath)
	if lErr != nil || info.Mode()&os.ModeSymlink == 0 {
		return reqPath, unifiedDiff
	}
	// File is a symlink. safePath already points to the real target.
	realWorkDir, _ := filepath.EvalSymlinks(cleanWorkDir)
	if realWorkDir == "" {
		realWorkDir = cleanWorkDir
	}
	realRel, relErr := filepath.Rel(realWorkDir, safePath)
	if relErr != nil {
		return reqPath, unifiedDiff
	}
	return realRel, rewriteDiffPaths(unifiedDiff, reqPath, realRel)
}

// writeDesiredContent writes the desired content directly to the file as a fallback
// when the diff cannot be applied. Returns the new hash and "overwritten" resolution.
func (wt *WorkspaceTracker) writeDesiredContent(
	safePath, cleanWorkDir, reqPath, desiredContent, oldHash string,
) (string, string, error) {
	if err := os.WriteFile(safePath, []byte(desiredContent), 0o644); err != nil {
		return "", "", fmt.Errorf("failed to write desired content: %w", err)
	}

	newHash := calculateSHA256(desiredContent)
	relPath := strings.TrimPrefix(safePath, cleanWorkDir+string(os.PathSeparator))
	wt.notifyFileChange(relPath, types.FileOpWrite)

	wt.logger.Warn("overwrote file with desired content (conflict fallback)",
		zap.String("path", reqPath),
		zap.String("old_hash", oldHash),
		zap.String("new_hash", newHash),
	)

	return newHash, ResolutionOverwritten, nil
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

// GetFileContentAtRef returns the content of a file at a specific git ref (branch, commit, HEAD, etc).
// If the file is not valid UTF-8, it is base64-encoded and isBinary is true.
func (wt *WorkspaceTracker) GetFileContentAtRef(ctx context.Context, reqPath string, ref string) (string, int64, bool, error) {
	// Validate path to prevent directory traversal
	cleanPath := filepath.Clean(reqPath)
	if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
		return "", 0, false, fmt.Errorf("invalid path: path traversal detected")
	}

	// Use git show to get the file content at the specified ref
	// Format: git show <ref>:<path>
	gitRef := fmt.Sprintf("%s:%s", ref, cleanPath)

	// Preflight: check blob size before materializing content to avoid memory spikes.
	// Use CombinedOutput to capture stderr for error detection (git writes errors to stderr).
	// Set LC_ALL=C to ensure English error messages for reliable parsing.
	sizeCmd := exec.CommandContext(ctx, "git", "cat-file", "-s", gitRef)
	sizeCmd.Dir = wt.workDir
	sizeCmd.Env = append(os.Environ(), "LC_ALL=C")
	sizeOut, err := sizeCmd.CombinedOutput()
	if err != nil {
		output := string(sizeOut)
		if strings.Contains(output, "does not exist") ||
			strings.Contains(output, "Not a valid object") ||
			strings.Contains(output, "fatal: path") ||
			strings.Contains(output, "fatal: not a valid") {
			return "", 0, false, fmt.Errorf("file not found at ref %s: %s", ref, cleanPath)
		}
		return "", 0, false, fmt.Errorf("failed to stat file at ref: %w", err)
	}
	blobSize, err := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64)
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to parse file size at ref: %w", err)
	}
	if blobSize > maxFileSize {
		return "", blobSize, false, fmt.Errorf("file too large (max 10MB)")
	}

	cmd := exec.CommandContext(ctx, "git", "show", gitRef)
	cmd.Dir = wt.workDir

	content, err := cmd.Output()
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to get file at ref: %w", err)
	}

	size := int64(len(content))

	// Detect binary: if content is not valid UTF-8, base64-encode it
	if !utf8.Valid(content) {
		encoded := base64.StdEncoding.EncodeToString(content)
		return encoded, size, true, nil
	}

	return string(content), size, false, nil
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
