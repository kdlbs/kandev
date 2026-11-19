package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const maxWorkspaceRootFiles = 500
const maxWorkspaceRootEntriesScanned = 2048
const workspaceRootReadDirBatchSize = 128

type workspaceRootFileObserverState struct {
	lastFiles     []types.WorkspaceRootFile
	lastTruncated bool
	publishedOnce bool
}

type workspaceRootFilesWalk struct {
	rootPath       string
	checkoutRoots  []string
	exclusions     []string
	files          []types.WorkspaceRootFile
	truncated      bool
	entriesScanned int
}

// SetCheckoutRoots sets repository directories excluded from root-file scans.
func (wt *WorkspaceTracker) SetCheckoutRoots(roots []string) {
	root := filepath.Clean(wt.workDir)
	canonical := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, candidate := range roots {
		if candidate == "" {
			continue
		}
		path := filepath.Clean(candidate)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		canonical = append(canonical, path)
	}
	sort.Strings(canonical)
	wt.mu.Lock()
	wt.checkoutRoots = canonical
	wt.mu.Unlock()
}

func (wt *WorkspaceTracker) checkoutRootSnapshot() []string {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return append([]string(nil), wt.checkoutRoots...)
}

// ListWorkspaceRootFiles returns regular files outside configured checkouts.
func (wt *WorkspaceTracker) ListWorkspaceRootFiles(ctx context.Context) (types.WorkspaceRootFiles, error) {
	exclusions, _ := wt.recoveryExclusionSnapshot()
	files, truncated, err := walkWorkspaceRootFiles(ctx, wt.workDir, wt.checkoutRootSnapshot(), exclusions)
	if err != nil {
		return types.WorkspaceRootFiles{}, err
	}
	return types.WorkspaceRootFiles{
		Supported:    true,
		Files:        files,
		Truncated:    truncated,
		Destinations: []types.WorkspaceRootDestination{},
	}, nil
}

func (m *Manager) WorkspaceRootFiles(ctx context.Context) (types.WorkspaceRootFiles, error) {
	unsupported := types.WorkspaceRootFiles{
		Files:        []types.WorkspaceRootFile{},
		Destinations: []types.WorkspaceRootDestination{},
	}
	root, trackers := m.snapshotTrackers()
	if root == nil || !root.suppressGitPublication {
		return unsupported, nil
	}
	listing, err := root.ListWorkspaceRootFiles(ctx)
	if err != nil {
		return types.WorkspaceRootFiles{}, err
	}
	listing.Supported = true
	listing.Destinations = make([]types.WorkspaceRootDestination, 0, len(trackers))
	rootPath := filepath.Clean(root.workDir)
	for _, tracker := range trackers {
		if tracker == nil || tracker.IsSubmodule() || tracker.RepositoryName() == "" {
			continue
		}
		rel, err := filepath.Rel(rootPath, filepath.Clean(tracker.workDir))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if !isWorkspaceRootCheckoutDirectory(rootPath, rel) {
			continue
		}
		listing.Destinations = append(listing.Destinations, types.WorkspaceRootDestination{
			RepositoryName: tracker.RepositoryName(),
			Path:           filepath.ToSlash(rel),
		})
	}
	return listing, nil
}

func workspaceCheckoutRoots(root string, trackers []*WorkspaceTracker) []string {
	paths := make([]string, 0, len(trackers))
	root = filepath.Clean(root)
	for _, tracker := range trackers {
		if tracker == nil || tracker.IsSubmodule() || tracker.RepositoryName() == "" {
			continue
		}
		path := filepath.Clean(tracker.workDir)
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			paths = append(paths, path)
		}
	}
	return paths
}

func walkWorkspaceRootFiles(
	ctx context.Context,
	rootPath string,
	checkoutRoots []string,
	exclusions []string,
) ([]types.WorkspaceRootFile, bool, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, false, fmt.Errorf("open workspace root: %w", err)
	}
	defer func() { _ = root.Close() }()

	walk := &workspaceRootFilesWalk{
		rootPath:      filepath.Clean(rootPath),
		checkoutRoots: checkoutRoots,
		exclusions:    exclusions,
		files:         make([]types.WorkspaceRootFile, 0),
	}
	err = walkWorkspaceRootFilesDirectory(ctx, root, "", walk)
	sort.Slice(walk.files, func(left, right int) bool {
		return walk.files[left].Path < walk.files[right].Path
	})
	return walk.files, walk.truncated, err
}

func walkWorkspaceRootFilesDirectory(
	ctx context.Context,
	directory *os.Root,
	relDir string,
	walk *workspaceRootFilesWalk,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := readWorkspaceRootDirectoryEntries(directory, relDir, walk)
	if err != nil {
		if relDir != "" && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission)) {
			return nil
		}
		return fmt.Errorf("read workspace root directory %q: %w", relDir, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := processWorkspaceRootFileEntry(ctx, directory, relDir, entry.Name(), walk); err != nil {
			return err
		}
		if walk.truncated && len(walk.files) == maxWorkspaceRootFiles {
			return nil
		}
	}
	return nil
}

func readWorkspaceRootDirectoryEntries(
	directory *os.Root,
	relDir string,
	walk *workspaceRootFilesWalk,
) ([]os.DirEntry, error) {
	dirFile, err := directory.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = dirFile.Close() }()

	remaining := maxWorkspaceRootEntriesScanned - walk.entriesScanned
	limit := remaining + 1
	entries := make([]os.DirEntry, 0, min(limit, workspaceRootReadDirBatchSize))
	for len(entries) < limit {
		batchSize := min(workspaceRootReadDirBatchSize, limit-len(entries))
		batch, readErr := dirFile.ReadDir(batchSize)
		entries = append(entries, batch...)
		if len(entries) > remaining {
			entries = entries[:remaining]
			walk.truncated = true
			break
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	walk.entriesScanned += len(entries)
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})
	return entries, nil
}

func processWorkspaceRootFileEntry(
	ctx context.Context,
	directory *os.Root,
	relDir, name string,
	walk *workspaceRootFilesWalk,
) error {
	relPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(relDir), name))
	info, err := directory.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || (relPath != "" && errors.Is(err, fs.ErrPermission)) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat workspace root path %q: %w", relPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if info.IsDir() {
		return walkWorkspaceRootSubdirectory(ctx, directory, relPath, name, info, walk)
	}
	appendWorkspaceRootFile(relPath, info, walk)
	return nil
}

func walkWorkspaceRootSubdirectory(
	ctx context.Context,
	parent *os.Root,
	relPath, name string,
	info os.FileInfo,
	walk *workspaceRootFilesWalk,
) error {
	fullPath := filepath.Join(walk.rootPath, filepath.FromSlash(relPath))
	if skipWorkspaceRootDirectory(name, fullPath, walk.checkoutRoots) {
		return nil
	}
	child, err := openWorkspaceRootSubdirectory(parent, name, info)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil
		}
		if errors.Is(err, errWorkspaceRootDirectoryChanged) {
			walk.truncated = true
			return nil
		}
		postOpenInfo, statErr := parent.Lstat(name)
		if statErr == nil && postOpenInfo.Mode()&os.ModeSymlink != 0 {
			walk.truncated = true
			return nil
		}
		return fmt.Errorf("open workspace directory %q: %w", relPath, err)
	}
	defer func() { _ = child.Close() }()
	hasGitEntry, err := hasWorkspaceGitEntry(child)
	if err != nil {
		return fmt.Errorf("inspect workspace directory %q: %w", relPath, err)
	}
	if hasGitEntry {
		return nil
	}
	return walkWorkspaceRootFilesDirectory(ctx, child, relPath, walk)
}

var errWorkspaceRootDirectoryChanged = errors.New("workspace root directory changed while opening")

func openWorkspaceRootSubdirectory(parent *os.Root, name string, expected os.FileInfo) (*os.Root, error) {
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := child.Stat(".")
	if err != nil {
		_ = child.Close()
		return nil, err
	}
	if !os.SameFile(expected, opened) {
		_ = child.Close()
		return nil, errWorkspaceRootDirectoryChanged
	}
	return child, nil
}

func appendWorkspaceRootFile(
	relPath string,
	info os.FileInfo,
	walk *workspaceRootFilesWalk,
) {
	if !info.Mode().IsRegular() || isRootOwnershipMarkerPath(relPath) || relativePathExcluded(relPath, walk.exclusions) {
		return
	}
	if len(walk.files) == maxWorkspaceRootFiles {
		walk.truncated = true
		return
	}
	walk.files = append(walk.files, types.WorkspaceRootFile{
		Path:       relPath,
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	})
}

func isWorkspaceRootCheckoutDirectory(rootPath, relPath string) bool {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return false
	}
	current := root
	defer func() { _ = current.Close() }()
	for _, part := range strings.Split(filepath.Clean(relPath), string(filepath.Separator)) {
		if part == "" || part == "." || part == string(filepath.Separator) {
			continue
		}
		info, err := current.Lstat(part)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		child, err := openWorkspaceRootSubdirectory(current, part, info)
		if err != nil {
			return false
		}
		_ = current.Close()
		current = child
	}
	return true
}

func skipWorkspaceRootDirectory(name, fullPath string, checkoutRoots []string) bool {
	if strings.HasPrefix(name, ".") || name == "node_modules" {
		return true
	}
	for _, root := range checkoutRoots {
		if filepath.Clean(fullPath) == root || pathWithinRoot(filepath.Clean(fullPath), root) {
			return true
		}
	}
	return false
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func hasWorkspaceGitEntry(dir *os.Root) (bool, error) {
	_, err := dir.Lstat(".git")
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return false, nil
	}
	return err == nil, err
}

// observeWorkspaceRootFiles publishes deltas from the last successful root
// listing. The first observation seeds state without emitting notifications.
func (wt *WorkspaceTracker) observeWorkspaceRootFiles(ctx context.Context) {
	started := time.Now()
	listing, err := wt.ListWorkspaceRootFiles(ctx)
	if err != nil {
		wt.logger.Debug("workspace root file scan failed", zap.Error(err))
		return
	}
	state := &wt.workspaceRootFileObserver
	if !state.publishedOnce {
		state.lastFiles = listing.Files
		state.lastTruncated = listing.Truncated
		state.publishedOnce = true
		return
	}
	changes := diffWorkspaceRootFiles(state.lastFiles, listing.Files)
	truncationChanged := state.lastTruncated != listing.Truncated
	if len(changes) == 0 && !truncationChanged {
		return
	}
	state.lastFiles = listing.Files
	state.lastTruncated = listing.Truncated
	wt.logger.Debug("workspace.root_files",
		zap.Int("count", len(listing.Files)),
		zap.Bool("truncated", listing.Truncated),
		zap.Duration("walk_duration", time.Since(started)))
	if len(changes) == 0 {
		wt.notifyFileChange("", types.FileOpRefresh)
		return
	}
	for _, change := range changes {
		wt.notifyFileChange(change.Path, change.Operation)
	}
}

func diffWorkspaceRootFiles(previous, current []types.WorkspaceRootFile) []types.FileChangeNotification {
	currentByPath := make(map[string]types.WorkspaceRootFile, len(current))
	for _, file := range current {
		currentByPath[file.Path] = file
	}
	changes := make([]types.FileChangeNotification, 0)
	for _, old := range previous {
		if _, exists := currentByPath[old.Path]; !exists {
			changes = append(changes, types.FileChangeNotification{Path: old.Path, Operation: types.FileOpRemove})
		}
	}
	previousByPath := make(map[string]types.WorkspaceRootFile, len(previous))
	for _, file := range previous {
		previousByPath[file.Path] = file
	}
	for _, file := range current {
		old, exists := previousByPath[file.Path]
		if !exists {
			changes = append(changes, types.FileChangeNotification{Path: file.Path, Operation: types.FileOpCreate})
		} else if old.Size != file.Size || !old.ModifiedAt.Equal(file.ModifiedAt) {
			changes = append(changes, types.FileChangeNotification{Path: file.Path, Operation: types.FileOpWrite})
		}
	}
	return changes
}
