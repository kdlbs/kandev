//go:build windows

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeFileWithinRoot(root, path string, data []byte, mode os.FileMode) error {
	cleanPath, err := containedPath(root, path)
	if err != nil {
		return err
	}
	if err := rejectExistingSymlinkComponents(root, cleanPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(cleanPath), err)
	}
	file, err := os.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("open session file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := writeAllFile(file, data); err != nil {
		return fmt.Errorf("write session file %q: %w", path, err)
	}
	return nil
}

func rejectExistingSymlinkComponents(root, path string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return err
	}
	current := filepath.Clean(root)
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("session path component %q is a symlink", current)
		}
	}
	return nil
}
