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

	"golang.org/x/sys/unix"
)

const dependencyDirectoryOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

// removeDependencyDirectory opens every path component with O_NOFOLLOW and removes
// entries through the resulting directory descriptors. This keeps a concurrent
// rename or symlink replacement from redirecting deletion outside the workspace.
func removeDependencyDirectory(ctx context.Context, root, target string) error {
	return removeDependencyDirectoryWithHook(ctx, root, target, nil)
}

// removeDependencyDirectoryWithHook is also used by the race regression test to
// replace the validated path after its descriptors have been opened.
func removeDependencyDirectoryWithHook(
	ctx context.Context,
	root, target string,
	afterOpen func(),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	relative, err := dependencyRelativePath(root, target)
	if err != nil {
		return err
	}
	rootFD, err := openDependencyDirectoryPath(root)
	if err != nil {
		return fmt.Errorf("open dependency workspace root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()

	parentFD, targetFD, err := openDependencyTarget(rootFD, relative)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(targetFD) }()
	if parentFD != rootFD {
		defer func() { _ = unix.Close(parentFD) }()
	}
	if afterOpen != nil {
		afterOpen()
	}
	if err := removeUnixDependencyContents(ctx, targetFD); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Unlinking through the parent descriptor with AT_REMOVEDIR cannot follow a
	// replacement symlink. The target's contents were removed through targetFD.
	if err := unix.Unlinkat(parentFD, filepath.Base(filepath.Clean(relative)), unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

func dependencyRelativePath(root, target string) (string, error) {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if !filepath.IsAbs(root) || !filepath.IsAbs(target) {
		return "", fmt.Errorf("dependency paths must be absolute")
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("resolve dependency path: %w", err)
	}
	if relative == "." || filepath.IsAbs(relative) {
		return "", fmt.Errorf("dependency target must be below workspace root")
	}
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("dependency target contains an unsafe path component")
		}
	}
	return relative, nil
}

func openDependencyDirectoryPath(path string) (int, error) {
	if !filepath.IsAbs(path) {
		return -1, fmt.Errorf("dependency workspace root must be absolute")
	}
	fd, err := unix.Open(string(filepath.Separator), dependencyDirectoryOpenFlags, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range dependencyAbsolutePathComponents(path) {
		next, err := unix.Openat(fd, part, dependencyDirectoryOpenFlags, 0)
		if err != nil {
			_ = unix.Close(fd)
			return -1, err
		}
		_ = unix.Close(fd)
		fd = next
	}
	return fd, nil
}

func dependencyAbsolutePathComponents(path string) []string {
	clean := filepath.Clean(path)
	trimmed := strings.TrimPrefix(filepath.ToSlash(clean), "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "." {
			result = append(result, part)
		}
	}
	return result
}

func openDependencyTarget(rootFD int, relative string) (parentFD, targetFD int, err error) {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	parentFD = rootFD
	for index, part := range parts {
		next, openErr := unix.Openat(parentFD, part, dependencyDirectoryOpenFlags, 0)
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

func removeUnixDependencyContents(ctx context.Context, dirFD int) error {
	names, err := readUnixDependencyNames(dirFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := removeUnixDependencyEntry(ctx, dirFD, name); err != nil {
			return err
		}
	}
	return nil
}

func readUnixDependencyNames(dirFD int) ([]string, error) {
	readFD, err := unix.Dup(dirFD)
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(readFD), "dependency-directory")
	if dir == nil {
		_ = unix.Close(readFD)
		return nil, fmt.Errorf("open dependency directory descriptor")
	}
	names, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return names, nil
}

func removeUnixDependencyEntry(ctx context.Context, dirFD int, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(dirFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
		return removeUnixDependencySubdirectory(ctx, dirFD, name)
	}
	if err := unix.Unlinkat(dirFD, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

func removeUnixDependencySubdirectory(ctx context.Context, parentFD int, name string) error {
	childFD, err := unix.Openat(parentFD, name, dependencyDirectoryOpenFlags, 0)
	if err != nil {
		return err
	}
	err = removeUnixDependencyContents(ctx, childFD)
	_ = unix.Close(childFD)
	if err != nil {
		return err
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}
