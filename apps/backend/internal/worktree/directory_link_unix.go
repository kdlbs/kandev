//go:build !windows

package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

func createPlatformDirectoryLink(target, link string) error   { return os.Symlink(target, link) }
func isPlatformDirectoryLink(info os.FileInfo, _ string) bool { return info.Mode()&os.ModeSymlink != 0 }
func platformDirectoryLinkTarget(path string) (string, error) { return os.Readlink(path) }
func replacePlatformDirectoryLink(link string, inspected os.FileInfo, target, _ string) error {
	tmp, err := os.CreateTemp(filepath.Dir(link), filepath.Base(link)+".tmp-*")
	if err != nil {
		return fmt.Errorf("stage directory link replacement: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("stage directory link replacement: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("stage directory link replacement: %w", err)
	}
	if err := createPlatformDirectoryLink(target, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("create directory link: %w", err)
	}
	if err := renameInspectedDirectoryLink(tmpPath, link, inspected); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
func requirePlatformDirectoryLink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !isPlatformDirectoryLink(info, path) {
		return os.ErrInvalid
	}
	return nil
}
