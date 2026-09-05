package worktree

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// DirtyWorktree describes local changes that would be lost by task deletion.
// Paths are derived from the persisted worktree record and Git status; callers
// never provide a filesystem path for this inspection.
type DirtyWorktree struct {
	WorktreeID   string   `json:"worktree_id"`
	RepositoryID string   `json:"repository_id"`
	Path         string   `json:"path"`
	DirtyFiles   []string `json:"dirty_files"`
}

// WorktreeCleanupOptions controls the destructive part of worktree cleanup.
// All identity, registration, path ownership, and branch safety checks remain
// active regardless of this option.
type WorktreeCleanupOptions struct {
	DiscardWorktreeChanges bool
}

// InspectDirtyWorktrees audits the recorded paths and reports local Git
// changes without mutating the checkout. Inspection errors fail closed so a
// caller cannot treat an unknown checkout state as clean.
func (m *Manager) InspectDirtyWorktrees(
	ctx context.Context, worktrees []*Worktree,
) ([]DirtyWorktree, error) {
	dirty := make([]DirtyWorktree, 0)
	seen := make(map[string]struct{}, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil || strings.TrimSpace(wt.ID) == "" {
			continue
		}
		m.enrichCleanupWorktreeFromCache(wt)
		if strings.TrimSpace(wt.RepositoryPath) == "" || strings.TrimSpace(wt.Path) == "" {
			continue
		}
		key := wt.RepositoryPath + "\x00" + filepath.Clean(wt.Path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
			return nil, fmt.Errorf("validate worktree cleanup path %s: %w", wt.ID, err)
		}
		pathPresent, err := cleanupPathPresent(wt.Path)
		if err != nil {
			return nil, fmt.Errorf("inspect worktree cleanup path %s: %w", wt.ID, err)
		}
		if !pathPresent {
			continue
		}
		pathHandle, err := m.openCleanupPathHandle(wt.Path, true)
		if err != nil {
			return nil, fmt.Errorf("pin worktree cleanup path %s: %w", wt.ID, err)
		}
		status, statusErr := m.runBoundedGitInspect(
			ctx, wt.Path, "status", "--porcelain=v1", "--untracked-files=normal", "-z",
		)
		if pathHandle != nil {
			_ = pathHandle.Close()
		}
		if statusErr != nil {
			return nil, fmt.Errorf("inspect worktree changes for %s: %w", wt.ID, statusErr)
		}
		files := parseDirtyWorktreeFiles(status)
		if len(files) == 0 {
			continue
		}
		dirty = append(dirty, DirtyWorktree{
			WorktreeID:   wt.ID,
			RepositoryID: wt.RepositoryID,
			Path:         filepath.Clean(wt.Path),
			DirtyFiles:   files,
		})
	}
	return dirty, nil
}

func parseDirtyWorktreeFiles(status string) []string {
	seen := make(map[string]struct{})
	files := make([]string, 0)
	skipNext := false
	for _, record := range strings.Split(status, "\x00") {
		if skipNext {
			skipNext = false
			continue
		}
		if len(record) < 4 || record[2] != ' ' {
			continue
		}
		if record[0] == 'R' || record[0] == 'C' {
			// With -z, Git emits the rename/copy origin path as the next
			// NUL-delimited field without an XY status prefix. The destination
			// above is the path users need to review before discarding.
			skipNext = true
		}
		path := record[3:]
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}
