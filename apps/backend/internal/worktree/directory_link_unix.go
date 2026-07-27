//go:build !windows

package worktree

import "os"

func createPlatformDirectoryLink(target, link string) error   { return os.Symlink(target, link) }
func isPlatformDirectoryLink(info os.FileInfo, _ string) bool { return info.Mode()&os.ModeSymlink != 0 }
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
