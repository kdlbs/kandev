package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/task/retention"
	"os"
	"path/filepath"
	"sort"
	"strings"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

var errTaskDirectoryNotManaged = errors.New("task directory is outside the managed semantic layout")

// TaskDirectoryCleanup retains the exact root admitted before worktree removal.
type TaskDirectoryCleanup struct {
	TaskID        string                            `json:"task_id"`
	Path          string                            `json:"path"`
	Identity      string                            `json:"identity"`
	WorktreePaths []string                          `json:"worktree_paths"`
	Owner         storageworkspaces.OwnershipMarker `json:"owner"`
}

// CaptureTaskDirectoryCleanup captures a managed root through a pinned handle.
func (m *Manager) CaptureTaskDirectoryCleanup(ctx context.Context, wt *Worktree) (*TaskDirectoryCleanup, error) {
	if wt == nil || wt.TaskID == "" {
		return nil, nil
	}
	if m.store == nil {
		return nil, errors.New("task directory inventory is unavailable")
	}
	root := filepath.Dir(filepath.Clean(wt.Path))
	name := wt.TaskDirName
	if name == "" {
		name = filepath.Base(root)
	}
	base, err := m.taskDirectoryCleanupBase(root, name)
	if errors.Is(err, errTaskDirectoryNotManaged) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	release, err := acquireWorktreeTargetPath(ctx, root)
	if err != nil {
		return nil, err
	}
	defer release()
	handle, err := storageworkspaces.OpenDirectoryNoFollow(base, root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = handle.Close() }()
	return m.captureTaskDirectoryHandle(ctx, handle, wt, root, name)
}

func (m *Manager) captureTaskDirectoryHandle(ctx context.Context, handle storageworkspaces.DirectoryHandle, wt *Worktree, root, name string) (*TaskDirectoryCleanup, error) {
	owner, err := readTaskDirectoryOwner(handle)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errTaskDirectoryNotManaged) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if owner.TaskDirName != name || (wt.TaskDirName == "" && owner.TaskID != wt.TaskID) {
		return nil, errors.New("task directory owner does not match cleanup target")
	}
	identity, err := storageworkspaces.DirectoryIdentity(handle)
	if err != nil {
		return nil, err
	}
	if err := handle.VerifyPath(root); err != nil {
		return nil, err
	}
	worktrees, err := m.GetAllByTaskID(ctx, wt.TaskID)
	if err != nil {
		return nil, err
	}
	paths := []string{wt.Path}
	seen := map[string]bool{wt.Path: true}
	for _, recorded := range worktrees {
		if recorded == nil {
			return nil, errors.New("task directory inventory is incomplete")
		}
		if filepath.Dir(recorded.Path) != root {
			return nil, nil
		}
		if seen[recorded.Path] {
			continue
		}
		seen[recorded.Path] = true
		paths = append(paths, recorded.Path)
	}
	return &TaskDirectoryCleanup{TaskID: wt.TaskID, Path: root, Identity: identity, Owner: owner, WorktreePaths: paths}, nil
}

func (m *Manager) taskDirectoryCleanupBase(root, name string) (string, error) {
	base, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return "", err
	}
	base = filepath.Clean(base)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		filepath.Dir(root) != base || filepath.Base(root) != name {
		return "", errTaskDirectoryNotManaged
	}
	return base, nil
}

func readTaskDirectoryOwner(handle storageworkspaces.DirectoryHandle) (storageworkspaces.OwnershipMarker, error) {
	var owner storageworkspaces.OwnershipMarker
	raw, err := handle.ReadFile(storageworkspaces.OwnershipMarkerFilename)
	if err != nil {
		return owner, err
	}
	if err := json.Unmarshal(raw, &owner); err != nil {
		return owner, err
	}
	if owner.LayoutVersion != storageworkspaces.LayoutVersionSemantic {
		return owner, errTaskDirectoryNotManaged
	}
	if owner.TaskID == "" || owner.TaskDirName == "" || owner.CreatedAt.IsZero() {
		return owner, errors.New("invalid managed task directory ownership marker")
	}
	return owner, nil
}

// CleanupTaskDirectory removes the root only after every recorded checkout is gone.
// Archive identity, runtime stop, and environment ownership are guarded by the caller.
func (m *Manager) CleanupTaskDirectory(ctx context.Context, target *TaskDirectoryCleanup) (bool, error) {
	ctx, releaseRetention, retentionErr := retention.AcquireMutation(ctx)
	if retentionErr != nil {
		return false, retentionErr
	}
	defer releaseRetention()

	if target == nil || target.Identity == "" || target.TaskID == "" {
		return false, errors.New("task directory cleanup lacks retained identity")
	}
	if m.store == nil {
		return false, errors.New("task directory inventory is unavailable")
	}
	if err := m.checkWorkspaceRetention(ctx, target.TaskID, "", target.Path); err != nil {
		return false, err
	}
	base, err := m.taskDirectoryCleanupBase(target.Path, target.Owner.TaskDirName)
	if err != nil {
		return false, err
	}
	releaseRoot, err := acquireWorktreeTargetPath(ctx, target.Path)
	if err != nil {
		return false, err
	}
	defer releaseRoot()
	worktrees, err := m.GetAllByTaskID(ctx, target.TaskID)
	if err != nil {
		return false, err
	}
	worktrees, err = taskDirectoryCleanupPaths(target, worktrees)
	if err != nil {
		return false, err
	}

	releasePaths, err := lockTaskDirectoryWorktrees(ctx, worktrees)
	if err != nil {
		return false, err
	}
	defer releasePaths()
	// Re-read after acquiring the paths so a concurrent create/cleanup is visible.
	worktrees, err = m.GetAllByTaskID(ctx, target.TaskID)
	if err != nil {
		return false, err
	}
	eligible, err := m.taskDirectoryWorktreesRemoved(ctx, target, worktrees)
	if err != nil || !eligible {
		return false, err
	}
	return removeCapturedTaskDirectory(ctx, base, target)
}

func removeCapturedTaskDirectory(ctx context.Context, base string, target *TaskDirectoryCleanup) (bool, error) {
	handle, err := storageworkspaces.OpenDirectoryNoFollow(base, target.Path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = handle.Close() }()
	identity, err := storageworkspaces.DirectoryIdentity(handle)
	if err != nil {
		return false, err
	}
	if identity != target.Identity {
		return false, errors.New("task directory filesystem identity changed")
	}
	owner, err := readTaskDirectoryOwner(handle)
	// Partial recursive removal may already have removed the marker. The retained
	// filesystem identity still pins that exact admitted directory for retry.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil && owner != target.Owner {
		return false, errors.New("task directory ownership marker changed")
	}
	if err := handle.VerifyPath(target.Path); err != nil {
		return false, err
	}
	if err := handle.RemoveDirectory(ctx); err != nil {
		return false, fmt.Errorf("remove archived task directory: %w", err)
	}
	return true, nil
}

func lockTaskDirectoryWorktrees(ctx context.Context, worktrees []*Worktree) (func(), error) {
	paths := make(map[string]struct{})
	for _, wt := range worktrees {
		if wt != nil && wt.Path != "" {
			paths[filepath.Clean(wt.Path)] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	releases := make([]func(), 0, len(ordered))
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	for _, path := range ordered {
		unlock, err := acquireWorktreeTargetPath(ctx, path)
		if err != nil {
			release()
			return nil, err
		}
		releases = append(releases, unlock)
	}
	return release, nil
}

func (m *Manager) taskDirectoryWorktreesRemoved(ctx context.Context, target *TaskDirectoryCleanup, worktrees []*Worktree) (bool, error) {
	// SQL inventory omits deleted history; other stores can retain it.
	for _, wt := range worktrees {
		if wt == nil || wt.TaskID != target.TaskID || filepath.Dir(wt.Path) != target.Path {
			return false, errors.New("task directory inventory identity mismatch")
		}
		if wt.Status != StatusDeleted {
			return false, nil
		}
	}
	activePaths, err := m.store.ListActiveWorktreePaths(ctx)
	if err != nil {
		return false, err
	}
	for _, path := range activePaths {
		relative, err := filepath.Rel(target.Path, path)
		if err != nil {
			return false, err
		}
		if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return false, nil
		}
	}
	for _, path := range target.WorktreePaths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return false, errors.New("removed worktree path reappeared")
			}
			return false, err
		}
	}
	return true, nil
}

func taskDirectoryCleanupPaths(target *TaskDirectoryCleanup, worktrees []*Worktree) ([]*Worktree, error) {
	for _, wt := range worktrees {
		if wt == nil || wt.TaskID != target.TaskID || filepath.Dir(filepath.Clean(wt.Path)) != target.Path {
			return nil, errors.New("task directory inventory path mismatch")
		}
	}
	for _, path := range target.WorktreePaths {
		if filepath.Dir(filepath.Clean(path)) != target.Path {
			return nil, errors.New("retained task directory path mismatch")
		}
		worktrees = append(worktrees, &Worktree{Path: path})
	}
	return worktrees, nil
}
