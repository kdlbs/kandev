//go:build !windows

package managedruntime

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const managedRuntimeDirectoryOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

// removeNpxExecutionTree walks the cache using directory descriptors. Every
// component is opened with O_NOFOLLOW and the final unlink is relative to the
// still-open _npx descriptor, so a concurrent replacement cannot redirect the
// deletion through a symlink.
func removeNpxExecutionTree(cacheRoot, key string) error {
	rootFD, err := openManagedRuntimeDirectoryPath(cacheRoot)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open npm cache root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()

	npxFD, err := unix.Openat(rootFD, "_npx", managedRuntimeDirectoryOpenFlags, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open npm execution cache: %w", err)
	}
	defer func() { _ = unix.Close(npxFD) }()

	targetFD, err := unix.Openat(npxFD, key, managedRuntimeDirectoryOpenFlags, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open npm execution tree: %w", err)
	}
	defer func() { _ = unix.Close(targetFD) }()

	if err := removeManagedRuntimeDirectoryContents(targetFD); err != nil {
		return fmt.Errorf("remove npm execution tree contents: %w", err)
	}
	if err := unix.Unlinkat(npxFD, key, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("remove npm execution tree: %w", err)
	}
	return nil
}

func openManagedRuntimeDirectoryPath(path string) (int, error) {
	if !filepath.IsAbs(path) {
		return -1, fmt.Errorf("npm cache root must be absolute")
	}
	fd, err := unix.Open(string(filepath.Separator), managedRuntimeDirectoryOpenFlags, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range managedRuntimeAbsolutePathComponents(path) {
		next, openErr := unix.Openat(fd, part, managedRuntimeDirectoryOpenFlags, 0)
		if openErr != nil {
			_ = unix.Close(fd)
			return -1, openErr
		}
		_ = unix.Close(fd)
		fd = next
	}
	return fd, nil
}

func managedRuntimeAbsolutePathComponents(path string) []string {
	trimmed := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	components := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "." {
			components = append(components, part)
		}
	}
	return components
}

func removeManagedRuntimeDirectoryContents(dirFD int) error {
	names, err := readManagedRuntimeDirectoryNames(dirFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		var stat unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			childFD, err := unix.Openat(dirFD, name, managedRuntimeDirectoryOpenFlags, 0)
			if err != nil {
				return err
			}
			err = removeManagedRuntimeDirectoryContents(childFD)
			_ = unix.Close(childFD)
			if err != nil {
				return err
			}
			if err := unix.Unlinkat(dirFD, name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
				return err
			}
			continue
		}
		if err := unix.Unlinkat(dirFD, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
	}
	return nil
}

func readManagedRuntimeDirectoryNames(dirFD int) ([]string, error) {
	readFD, err := unix.Dup(dirFD)
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(readFD), "managed-runtime-cache")
	if dir == nil {
		_ = unix.Close(readFD)
		return nil, fmt.Errorf("open npm execution directory descriptor")
	}
	names, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return names, nil
}
