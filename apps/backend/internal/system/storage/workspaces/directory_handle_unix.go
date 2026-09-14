//go:build !windows

package workspaces

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

type unixDirectoryHandle struct {
	rootFD   int
	parentFD int
	targetFD int
	target   string
	once     sync.Once
}

// OpenDirectoryNoFollow opens root and target with O_NOFOLLOW for every
// component. The returned handle remains attached to the original directory
// even if its lexical path is renamed or replaced later.
func OpenDirectoryNoFollow(root, target string) (DirectoryHandle, error) {
	relative, err := dependencyRelativePath(root, target)
	if err != nil {
		return nil, err
	}
	rootFD, err := openDependencyDirectoryPath(root)
	if err != nil {
		return nil, err
	}
	parentFD, targetFD, err := openDependencyTarget(rootFD, relative)
	if err != nil {
		_ = unix.Close(rootFD)
		return nil, err
	}
	return &unixDirectoryHandle{
		rootFD: rootFD, parentFD: parentFD, targetFD: targetFD,
		target: filepath.Base(filepath.Clean(relative)),
	}, nil
}

// CreateDirectoryNoFollow creates every missing component below root through
// directory descriptors. Existing symlinks and non-directories are rejected.
func CreateDirectoryNoFollow(root, target string, mode os.FileMode) (DirectoryHandle, error) {
	relative, err := dependencyRelativePath(root, target)
	if err != nil {
		return nil, err
	}
	rootFD, err := openOrCreateDependencyDirectoryPath(root, mode)
	if err != nil {
		return nil, err
	}
	parentFD, targetFD, err := openOrCreateDependencyTarget(rootFD, relative, mode)
	if err != nil {
		_ = unix.Close(rootFD)
		return nil, err
	}
	return &unixDirectoryHandle{
		rootFD: rootFD, parentFD: parentFD, targetFD: targetFD,
		target: filepath.Base(filepath.Clean(relative)),
	}, nil
}

func (h *unixDirectoryHandle) Close() error {
	var closeErr error
	h.once.Do(func() {
		if h.targetFD >= 0 {
			if err := unix.Close(h.targetFD); err != nil {
				closeErr = err
			}
		}
		if h.parentFD >= 0 && h.parentFD != h.rootFD {
			if err := unix.Close(h.parentFD); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		if h.rootFD >= 0 {
			if err := unix.Close(h.rootFD); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

func (h *unixDirectoryHandle) VerifyPath(path string) error {
	if h == nil || h.targetFD < 0 {
		return errors.New("directory handle is closed")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	dup, err := unix.Dup(h.targetFD)
	if err != nil {
		return fmt.Errorf("duplicate directory handle: %w", err)
	}
	file := os.NewFile(uintptr(dup), "worktree-directory")
	if file == nil {
		_ = unix.Close(dup)
		return errors.New("create directory file from handle")
	}
	handleInfo, statErr := file.Stat()
	_ = file.Close()
	if statErr != nil {
		return fmt.Errorf("stat directory handle: %w", statErr)
	}
	if !os.SameFile(pathInfo, handleInfo) {
		return fmt.Errorf("directory path changed: %s", path)
	}
	return nil
}

func (h *unixDirectoryHandle) IsValidWorktree() bool {
	content, err := h.ReadFile(".git")
	return err == nil && strings.HasPrefix(string(content), "gitdir:")
}

// RemoveDirectory removes the pinned directory through its open descriptors.
// It never resolves the directory again from its lexical path, so a rename or
// replacement cannot redirect deletion to a different workspace.
func (h *unixDirectoryHandle) RemoveDirectory(ctx context.Context) error {
	if h == nil || h.parentFD < 0 || h.targetFD < 0 || h.target == "" {
		return errors.New("directory handle is closed")
	}
	if err := removeUnixDependencyContents(ctx, h.targetFD); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var targetInfo unix.Stat_t
	if err := unix.Fstat(h.targetFD, &targetInfo); err != nil {
		return fmt.Errorf("stat pinned directory: %w", err)
	}
	var currentInfo unix.Stat_t
	if err := unix.Fstatat(h.parentFD, h.target, &currentInfo, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			// The pinned directory was renamed. Its contents are safe to clear,
			// but there is no longer a path entry that this handle can unlink.
			return nil
		}
		return fmt.Errorf("inspect pinned directory entry: %w", err)
	}
	if targetInfo.Dev != currentInfo.Dev || targetInfo.Ino != currentInfo.Ino {
		return errors.New("pinned directory path changed during cleanup")
	}
	if err := unix.Unlinkat(h.parentFD, h.target, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

func (h *unixDirectoryHandle) ReadFile(name string) ([]byte, error) {
	if h == nil || h.targetFD < 0 {
		return nil, errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(h.targetFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filepath.Join("worktree-directory", name))
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create file from directory entry handle")
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, statErr
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("directory entry is not a regular file: %s", name)
	}
	content, readErr := io.ReadAll(file)
	_ = file.Close()
	return content, readErr
}

func (h *unixDirectoryHandle) WriteFile(name string, data []byte, mode os.FileMode) error {
	if h == nil || h.targetFD < 0 {
		return errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return err
	}
	fd, err := unix.Openat(h.targetFD, name,
		unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		uint32(mode.Perm()))
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), filepath.Join("worktree-directory", name))
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("create writable file from directory entry handle")
	}
	if err := unix.Fchmod(fd, uint32(mode.Perm())); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func openOrCreateDependencyDirectoryPath(path string, mode os.FileMode) (int, error) {
	if !filepath.IsAbs(path) {
		return -1, fmt.Errorf("dependency workspace root must be absolute")
	}
	fd, err := unix.Open(string(filepath.Separator), dependencyDirectoryOpenFlags, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range dependencyAbsolutePathComponents(path) {
		next, openErr := unix.Openat(fd, part, dependencyDirectoryOpenFlags, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, part, uint32(mode.Perm())); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return -1, mkdirErr
			}
			next, openErr = unix.Openat(fd, part, dependencyDirectoryOpenFlags, 0)
		}
		if openErr != nil {
			_ = unix.Close(fd)
			return -1, openErr
		}
		_ = unix.Close(fd)
		fd = next
	}
	return fd, nil
}

func openOrCreateDependencyTarget(rootFD int, relative string, mode os.FileMode) (parentFD, targetFD int, err error) {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	parentFD = rootFD
	for index, part := range parts {
		next, openErr := unix.Openat(parentFD, part, dependencyDirectoryOpenFlags, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(parentFD, part, uint32(mode.Perm())); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				if parentFD != rootFD {
					_ = unix.Close(parentFD)
				}
				return -1, -1, mkdirErr
			}
			next, openErr = unix.Openat(parentFD, part, dependencyDirectoryOpenFlags, 0)
		}
		if openErr != nil {
			if parentFD != rootFD {
				_ = unix.Close(parentFD)
			}
			return -1, -1, openErr
		}
		if index == len(parts)-1 {
			return parentFD, next, nil
		}
		if parentFD != rootFD {
			_ = unix.Close(parentFD)
		}
		parentFD = next
	}
	return -1, -1, fmt.Errorf("dependency target is empty")
}
