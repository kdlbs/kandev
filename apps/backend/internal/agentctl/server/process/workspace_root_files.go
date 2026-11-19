package process

import (
	"context"
	"errors"
	"fmt"
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

type workspaceRootFileObserverState struct {
	lastFiles     []types.WorkspaceRootFile
	publishedOnce bool
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
	root string,
	checkoutRoots []string,
	exclusions []string,
) ([]types.WorkspaceRootFile, bool, error) {
	files := make([]types.WorkspaceRootFile, 0)
	truncated := false
	err := walkWorkspaceRootFilesDirectory(ctx, root, "", checkoutRoots, exclusions, &files, &truncated)
	return files, truncated, err
}

func walkWorkspaceRootFilesDirectory(
	ctx context.Context,
	root string,
	relDir string,
	checkoutRoots []string,
	exclusions []string,
	files *[]types.WorkspaceRootFile,
	truncated *bool,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := root
	if relDir != "" {
		dir = filepath.Join(root, filepath.FromSlash(relDir))
	}
	entries, err := os.ReadDir(dir)
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
		relPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(relDir), entry.Name()))
		fullPath := filepath.Join(dir, entry.Name())
		done, err := processWorkspaceRootFileEntry(ctx, root, relPath, fullPath, entry.Name(), checkoutRoots, exclusions, files, truncated)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return nil
}

func processWorkspaceRootFileEntry(
	ctx context.Context,
	root, relPath, fullPath, name string,
	checkoutRoots, exclusions []string,
	files *[]types.WorkspaceRootFile,
	truncated *bool,
) (bool, error) {
	info, err := os.Lstat(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat workspace root path %q: %w", relPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	if info.IsDir() {
		return walkWorkspaceRootSubdirectory(ctx, root, relPath, fullPath, name, checkoutRoots, exclusions, files, truncated)
	}
	return appendWorkspaceRootFile(relPath, info, exclusions, files, truncated)
}

func walkWorkspaceRootSubdirectory(
	ctx context.Context,
	root, relPath, fullPath, name string,
	checkoutRoots, exclusions []string,
	files *[]types.WorkspaceRootFile,
	truncated *bool,
) (bool, error) {
	if skipWorkspaceRootDirectory(name, fullPath, checkoutRoots) {
		return false, nil
	}
	hasGitEntry, err := hasWorkspaceGitEntry(fullPath)
	if err != nil {
		return false, fmt.Errorf("inspect workspace directory %q: %w", relPath, err)
	}
	if hasGitEntry {
		return false, nil
	}
	if err := walkWorkspaceRootFilesDirectory(ctx, root, relPath, checkoutRoots, exclusions, files, truncated); err != nil {
		return false, err
	}
	return *truncated, nil
}

func appendWorkspaceRootFile(
	relPath string,
	info os.FileInfo,
	exclusions []string,
	files *[]types.WorkspaceRootFile,
	truncated *bool,
) (bool, error) {
	if !info.Mode().IsRegular() || isRootOwnershipMarkerPath(relPath) || relativePathExcluded(relPath, exclusions) {
		return false, nil
	}
	if len(*files) == maxWorkspaceRootFiles {
		*truncated = true
		return true, nil
	}
	*files = append(*files, types.WorkspaceRootFile{
		Path:       relPath,
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	})
	return false, nil
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

func hasWorkspaceGitEntry(dir string) (bool, error) {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
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
		state.publishedOnce = true
		return
	}
	changes := diffWorkspaceRootFiles(state.lastFiles, listing.Files)
	if len(changes) == 0 {
		return
	}
	state.lastFiles = listing.Files
	wt.logger.Debug("workspace.root_files",
		zap.Int("count", len(listing.Files)),
		zap.Bool("truncated", listing.Truncated),
		zap.Duration("walk_duration", time.Since(started)))
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
