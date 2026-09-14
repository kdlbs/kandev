package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// HandoffCleaner is the office task-handoffs cleanup adapter that
// satisfies task/service.WorkspaceCleaner. It threads the managed-root
// guard before any destructive operation, refuses paths outside the
// configured kandev-managed roots, and delegates the actual disk
// removal to the existing Manager.
//
// Wired in cmd/kandev: handoffSvc.SetWorkspaceCleaner(NewHandoffCleaner(worktreeMgr, log)).
type HandoffCleaner struct {
	manager *Manager
	logger  *logger.Logger
	// extraRoots is the optional list of additional directories the
	// cleaner is allowed to remove (e.g. the office multi-repo root,
	// plain folder root). Worktree paths are validated separately
	// against the manager's TasksBasePath.
	extraRoots []string
}

// NewHandoffCleaner constructs the cleaner. extraRoots is appended to
// the manager's tasks base path; pass office-specific managed roots
// (multi-repo / plain-folder) when wiring.
func NewHandoffCleaner(mgr *Manager, log *logger.Logger, extraRoots ...string) *HandoffCleaner {
	return &HandoffCleaner{
		manager:    mgr,
		logger:     log.WithFields(zap.String("component", "handoff-cleaner")),
		extraRoots: extraRoots,
	}
}

func (c *HandoffCleaner) ValidateManagedRoot(path string) error {
	if err := c.requireManagedRoot(path); err != nil {
		return err
	}
	root, err := c.managedRootFor(path)
	if err != nil {
		return err
	}
	return rejectSymlinkComponents(root, path)
}

// CreateManagedDirectory creates a restore path through no-follow directory
// descriptors, so a component replacement cannot redirect MkdirAll.
func (c *HandoffCleaner) CreateManagedDirectory(path string, mode os.FileMode) error {
	if err := c.requireManagedRoot(path); err != nil {
		return err
	}
	root, err := c.managedRootFor(path)
	if err != nil {
		return err
	}
	handle, err := storageworkspaces.CreateDirectoryNoFollow(root, path, mode)
	if err != nil {
		return err
	}
	return handle.Close()
}

func (c *HandoffCleaner) removeManagedDirectory(ctx context.Context, path string) error {
	root, err := c.managedRootFor(path)
	if err != nil {
		return err
	}
	handle, err := storageworkspaces.OpenDirectoryNoFollow(root, path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = handle.Close()
	}()
	return handle.RemoveDirectory(ctx)
}

func (c *HandoffCleaner) managedRootFor(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for _, root := range c.managedRoots() {
		rootAbs, rootErr := filepath.Abs(root)
		if rootErr == nil && isDescendant(rootAbs, abs) {
			return rootAbs, nil
		}
	}
	return "", fmt.Errorf("managed-root guard: %s is not inside a managed root", path)
}

// CleanupPlainFolder removes a Kandev-owned plain folder. The path
// MUST resolve to a location under one of the configured managed
// roots; anything else is rejected up front so a corrupted
// materialized_path can never delete arbitrary user files.
func (c *HandoffCleaner) CleanupPlainFolder(ctx context.Context, path string) error {
	if err := c.requireManagedRoot(path); err != nil {
		return err
	}
	root, err := c.managedRootFor(path)
	if err != nil {
		return err
	}
	if err := rejectSymlinkComponents(root, path); err != nil {
		return err
	}
	c.logger.Info("cleanup plain folder", zap.String("path", path))
	if err := c.removeManagedDirectory(ctx, path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// removeBranch is FALSE: handoffs cleanup releases the materialized
// workspace; the branch the agent created is left intact so any
// pushed PR / remote ref survives. Operators clean up branches via
// the existing branch-cleanup tooling.
func (c *HandoffCleaner) CleanupSingleRepoWorktree(ctx context.Context, worktreeID string) error {
	if c.manager == nil {
		return errors.New("worktree manager not configured")
	}
	if worktreeID == "" {
		return errors.New("worktree id is required")
	}
	c.logger.Info("cleanup single-repo worktree", zap.String("worktree_id", worktreeID))
	return c.manager.RemoveByID(ctx, worktreeID, false)
}

// CleanupMultiRepoRoot removes every per-repo worktree under a
// multi-repo task root, then removes the root directory itself. The
// root path is validated against the managed-roots set first; if the
// guard fails the per-repo removals are also skipped.
func (c *HandoffCleaner) CleanupMultiRepoRoot(ctx context.Context, rootPath string, worktreeIDs []string) error {
	if c.manager == nil {
		return errors.New("worktree manager not configured")
	}
	if err := c.requireManagedRoot(rootPath); err != nil {
		return err
	}
	root, err := c.managedRootFor(rootPath)
	if err != nil {
		return err
	}
	if err := rejectSymlinkComponents(root, rootPath); err != nil {
		return err
	}
	if len(worktreeIDs) == 0 {
		return errors.New("multi-repo worktree inventory is empty")
	}
	var removalErrors []error
	for _, id := range worktreeIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("multi-repo worktree inventory contains an empty ID")
		}
		if err := c.manager.RemoveByID(ctx, id, false); err != nil {
			c.logger.Warn("multi-repo worktree remove failed",
				zap.String("worktree_id", id), zap.Error(err))
			removalErrors = append(removalErrors, fmt.Errorf("remove worktree %s: %w", id, err))
		}
	}
	if err := errors.Join(removalErrors...); err != nil {
		return err
	}
	if err := c.removeManagedDirectory(ctx, rootPath); err != nil {
		return fmt.Errorf("remove multi-repo root %s: %w", rootPath, err)
	}
	return nil
}

// CleanupRemoteEnvironment cannot safely claim success without a provider
// deletion implementation. Returning an error keeps the group in
// cleanup_failed so the environment is visible for retry or operator action.
func (c *HandoffCleaner) CleanupRemoteEnvironment(_ context.Context, provider, environmentID string) error {
	c.logger.Error("remote environment cleanup is not configured",
		zap.String("provider", provider),
		zap.String("environment_id", environmentID))
	return fmt.Errorf("remote environment cleanup is not configured for provider %q", provider)
}

// requireManagedRoot rejects paths that do not resolve to a location
// under one of the configured managed roots. This is the belt-and-
// braces guard the spec calls out as critical: even if owned_by_kandev
// were ever wrongly set, the cleanup still cannot escape the
// kandev-managed directory tree.
//
// Symlinks are resolved on both the input path and each managed root
// so the guard accepts identities like /var/folders → /private/var/folders
// (macOS tmpdirs) consistently. When the input path no longer exists
// (e.g. cleanup is running after a partial removal), we resolve
// symlinks on the deepest parent that does exist, then re-attach the
// missing tail before checking.
func (c *HandoffCleaner) requireManagedRoot(path string) error {
	if path == "" {
		return errors.New("managed-root guard: path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("managed-root guard: absolute path: %w", err)
	}
	resolved := resolveExistingPrefix(abs)
	roots := c.managedRoots()
	if len(roots) == 0 {
		return errors.New("managed-root guard: no managed roots configured")
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rootResolved := resolveExistingPrefix(rootAbs)
		if isDescendant(rootResolved, resolved) {
			return nil
		}
	}
	return fmt.Errorf("managed-root guard: %s is not inside a kandev-managed root", path)
}

// resolveExistingPrefix evaluates symlinks on the deepest existing
// ancestor of `path` and re-attaches the missing tail. Used so the
// guard handles "the directory is about to be created" / "was just
// removed" without losing symlink-equivalence with the managed roots.
func resolveExistingPrefix(path string) string {
	cleaned := filepath.Clean(path)
	prefix := cleaned
	suffix := ""
	for {
		if r, err := filepath.EvalSymlinks(prefix); err == nil {
			if suffix == "" {
				return filepath.Clean(r)
			}
			return filepath.Clean(filepath.Join(r, suffix))
		}
		parent := filepath.Dir(prefix)
		if parent == prefix {
			return cleaned
		}
		base := filepath.Base(prefix)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		prefix = parent
	}
}

// validated directory is replaced by a symlink before a destructive call.
// Destructive operations accept only stable, non-symlink path components;
// managed roots themselves are configured paths and are validated separately.
func rejectSymlinkComponents(root, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("managed-root guard: inspect path: %w", err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("managed-root guard: inspect root: %w", err)
	}
	for current := abs; isDescendant(rootAbs, current) && current != rootAbs; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed-root guard: symlink component %s is not allowed", current)
		}
	}
	return nil
}

func (c *HandoffCleaner) managedRoots() []string {
	roots := make([]string, 0, 1+len(c.extraRoots))
	if c.manager != nil {
		// Reuse the worktree manager's config so the guard tracks the
		// same root the manager creates worktrees in.
		base, err := c.manager.config.ExpandedTasksBasePath()
		if err == nil && base != "" {
			roots = append(roots, base)
		}
	}
	roots = append(roots, c.extraRoots...)
	return roots
}

// isDescendant reports whether `path` is `root` or a descendant of it.
// Uses filepath.Rel to detect ".." traversal — anything that resolves
// via Rel to a relative path NOT starting with ".." is a descendant.
func isDescendant(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..") && rel != ""
}
