package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CommonPath returns the deepest lexical directory containing every path.
func CommonPath(paths ...string) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("at least one path is required")
	}
	common := filepath.Clean(paths[0])
	if !filepath.IsAbs(common) {
		return "", fmt.Errorf("path must be absolute: %q", paths[0])
	}
	for _, raw := range paths[1:] {
		path := filepath.Clean(raw)
		if !filepath.IsAbs(path) || filepath.VolumeName(path) != filepath.VolumeName(common) {
			return "", fmt.Errorf("paths must be absolute and on one volume: %q", raw)
		}
		for !containsPath(common, path) {
			parent := filepath.Dir(common)
			if parent == common {
				break
			}
			common = parent
		}
	}
	return common, nil
}

// ValidateNoSymlinkPath rejects symlinks in every existing component from
// anchor through path and verifies that the canonical existing prefix remains
// beneath the canonical anchor. Missing trailing components are allowed.
func ValidateNoSymlinkPath(anchor, path string) error {
	anchor = filepath.Clean(anchor)
	path = filepath.Clean(path)
	if !filepath.IsAbs(anchor) || !filepath.IsAbs(path) || !containsPath(anchor, path) {
		return fmt.Errorf("path %q is outside safety anchor %q", path, anchor)
	}
	if err := requireRealDirectory(anchor); err != nil {
		return err
	}
	canonicalAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return fmt.Errorf("resolve safety anchor %s: %w", anchor, err)
	}
	relative, err := filepath.Rel(anchor, path)
	if err != nil {
		return err
	}
	deepest, err := deepestRealDirectory(anchor, relative)
	if err != nil {
		return err
	}
	canonicalDeepest, err := filepath.EvalSymlinks(deepest)
	if err != nil {
		return fmt.Errorf("resolve path component %s: %w", deepest, err)
	}
	if !containsPath(canonicalAnchor, canonicalDeepest) {
		return fmt.Errorf("canonical path %s escapes safety anchor %s", canonicalDeepest, canonicalAnchor)
	}
	return nil
}

func deepestRealDirectory(anchor, relative string) (string, error) {
	current := anchor
	deepest := anchor
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("inspect path component %s: %w", current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path component must not be a symlink: %s", current)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("path component must be a directory: %s", current)
		}
		deepest = current
	}
	return deepest, nil
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect safety anchor %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("safety anchor must be a real directory: %s", path)
	}
	return nil
}

func containsPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
