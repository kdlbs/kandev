package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateOwnedDirectoryLink creates a live directory reference below root.
// root is created only through real (non-symlink) ancestors, and name must be
// a single path segment. The caller owns root; this function never alters the
// target or an existing entry.
func CreateOwnedDirectoryLink(root, name, target string) (string, error) {
	if !isOwnedDirectoryLinkPath(root, name) {
		return "", fmt.Errorf("invalid owned link path")
	}
	canonicalTarget, err := canonicalDirectoryLinkTarget(target)
	if err != nil {
		return "", err
	}
	if err := mkdirOwned(root); err != nil {
		return "", err
	}
	link, err := ownedDirectoryLinkPath(root, name)
	if err != nil {
		return "", err
	}
	_, err = os.Lstat(link)
	if err == nil {
		return "", fmt.Errorf("owned link entry already exists: %s", name)
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect owned link entry: %w", err)
	}
	if err := createPlatformDirectoryLink(canonicalTarget, link); err != nil {
		return "", fmt.Errorf("create directory link: %w", err)
	}
	if err := verifyCreatedOwnedDirectoryLink(root, link); err != nil {
		return "", err
	}
	return link, nil
}

func isOwnedDirectoryLinkPath(root, name string) bool {
	return root != "" && filepath.IsAbs(root) && name != "" && filepath.Base(name) == name && name != "." && name != ".."
}

func canonicalDirectoryLinkTarget(target string) (string, error) {
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("canonicalize link target: %w", err)
	}
	info, err := os.Stat(canonicalTarget)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("link target is not a directory: %w", err)
	}
	return canonicalTarget, nil
}

func ownedDirectoryLinkPath(root, name string) (string, error) {
	link := filepath.Join(root, name)
	if filepath.Dir(link) != filepath.Clean(root) {
		return "", fmt.Errorf("link escapes owned root")
	}
	return link, nil
}

func verifyCreatedOwnedDirectoryLink(root, link string) error {
	if err := requireRealDir(root); err != nil {
		_ = os.Remove(link)
		return err
	}
	if err := requirePlatformDirectoryLink(link); err != nil {
		_ = os.Remove(link)
		return fmt.Errorf("owned link changed during creation")
	}
	return nil
}

// EnsureOwnedDirectoryLink returns an existing matching live link or creates
// it. A non-link/collision, or a link to another directory, fails closed and is
// never replaced.
//
// The existing link is matched by filesystem identity, not by resolved path.
// filepath.EvalSymlinks does not traverse a Windows junction — it returns the
// link's own normalized path — so a path comparison rejected every unchanged
// junction as a target mismatch, and each relaunch or resume of a task with
// workspace source links failed. os.Stat follows a junction and a Unix symlink
// alike, and os.SameFile compares volume and file index, which is also immune
// to 8.3 short paths and path case.
func EnsureOwnedDirectoryLink(root, name, target string) (string, bool, error) {
	link := filepath.Join(root, name)
	info, err := os.Lstat(link)
	if err == nil {
		if !isPlatformDirectoryLink(info, link) {
			return "", false, fmt.Errorf("owned link entry already exists: %s", name)
		}
		actual, err := os.Stat(link)
		if err != nil {
			return "", false, fmt.Errorf("resolve owned link: %w", err)
		}
		expected, err := os.Stat(target)
		if err != nil {
			return "", false, fmt.Errorf("inspect link target: %w", err)
		}
		if !os.SameFile(actual, expected) {
			return "", false, fmt.Errorf("owned link target mismatch: %s", name)
		}
		return link, false, nil
	}
	if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("inspect owned link entry: %w", err)
	}
	created, err := CreateOwnedDirectoryLink(root, name, target)
	return created, err == nil, err
}

// IsSelfReferentialDirectoryLink reports whether root/name is a platform
// directory link whose target is root itself — the shape an earlier release
// planted inside a user's own repository for local-executor tasks, and which
// EnsureOwnedDirectoryLink then accepts as valid forever.
//
// It deliberately only reports. Such an entry cannot be shown to be
// Kandev-owned: Kandev writes no ownership marker into user-owned sources, and
// a user (or the repository itself) may keep a link of the same name and target
// on purpose, so removing it could destroy content that is not ours. Nor could
// a stat-then-remove sequence close the window between the check and the
// unlink. Callers surface it and leave removal to the user.
func IsSelfReferentialDirectoryLink(root, name string) (bool, error) {
	if !isOwnedDirectoryLinkPath(root, name) {
		return false, fmt.Errorf("invalid owned link path")
	}
	link, err := ownedDirectoryLinkPath(root, name)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect owned link entry: %w", err)
	}
	if !isPlatformDirectoryLink(info, link) {
		return false, nil
	}
	return linkTargetsRoot(root, link)
}

// linkTargetsRoot compares filesystem identity, not resolved path text.
// filepath.EvalSymlinks does not traverse a Windows junction — it returns the
// link's own normalized path — so a path comparison never matches. os.Stat
// follows a junction and a Unix symlink alike, and os.SameFile then compares
// volume and file index, which also absorbs 8.3 short paths and case.
func linkTargetsRoot(root, link string) (bool, error) {
	linkInfo, err := os.Stat(link)
	if err != nil {
		return false, fmt.Errorf("resolve owned link: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false, fmt.Errorf("resolve owned root: %w", err)
	}
	return os.SameFile(linkInfo, rootInfo), nil
}

func mkdirOwned(root string) error {
	parent := filepath.Dir(root)
	if err := requireNoSymlinkAncestors(parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create owned parent: %w", err)
	}
	if err := requireRealDir(parent); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0o755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create owned root: %w", err)
	}
	return requireRealDir(root)
}

// requireNoSymlinkAncestors checks every existing control-path component
// before MkdirAll can traverse it. It deliberately uses Lstat so a junction
// or symlink is rejected rather than resolved.
func requireNoSymlinkAncestors(path string) error {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, volume)
	rest = strings.TrimPrefix(rest, string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect owned ancestor: %w", err)
		}
		if isPlatformDirectoryLink(info, current) {
			return fmt.Errorf("owned control ancestor is symlink: %s", current)
		}
	}
	return nil
}

func requireRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect owned path: %w", err)
	}
	if isPlatformDirectoryLink(info, path) || !info.IsDir() {
		return fmt.Errorf("owned path is not a real directory: %s", path)
	}
	return nil
}

// TaskRoot returns the canonical Kandev-owned root for a task directory name.
// It deliberately accepts only the same single-segment names used by worktree
// creation, preventing a persisted environment row from selecting another
// directory beneath (or outside) the configured tasks base.
func (m *Manager) TaskRoot(taskDirName string) (string, error) {
	if taskDirName == "" || filepath.Base(taskDirName) != taskDirName || taskDirName == "." || taskDirName == ".." {
		return "", fmt.Errorf("invalid task directory name")
	}
	base, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return "", err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", err
	}
	if err := requireRealDir(base); err != nil {
		return "", err
	}
	if err := requireNoSymlinkAncestors(base); err != nil {
		return "", err
	}
	return filepath.Join(base, taskDirName), nil
}
