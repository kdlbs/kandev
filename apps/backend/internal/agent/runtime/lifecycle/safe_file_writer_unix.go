//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const safeFileDirectoryFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW

// writeFileWithinRoot walks the destination from an opened root directory.
// O_NOFOLLOW is applied to every directory and the final file so an executor
// cannot redirect a managed session write through a stale symlink.
func writeFileWithinRoot(root, path string, data []byte, mode os.FileMode) error {
	_, parts, err := safeFilePathParts(root, path)
	if err != nil {
		return err
	}
	currentFD, directoryFDs, err := openSafeFileDirectories(root, parts)
	if err != nil {
		return err
	}
	defer func() {
		for index := len(directoryFDs) - 1; index >= 0; index-- {
			_ = unix.Close(directoryFDs[index])
		}
	}()
	return writeSafeFile(currentFD, path, parts[len(parts)-1], data, mode)
}

func safeFilePathParts(root, path string) (string, []string, error) {
	cleanPath, err := containedPath(root, path)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(filepath.Clean(root), cleanPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("path %q is not a file below session root %q", path, root)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", nil, fmt.Errorf("path %q has no file name", path)
	}
	return cleanPath, parts, nil
}

func openSafeFileDirectories(root string, parts []string) (currentFD int, directoryFDs []int, err error) {
	rootFD, err := unix.Open(filepath.Clean(root), safeFileDirectoryFlags, 0)
	if err != nil {
		return 0, nil, fmt.Errorf("open session root %q: %w", root, err)
	}
	directoryFDs = []int{rootFD}
	currentFD = rootFD
	defer func() {
		if err == nil {
			return
		}
		for index := len(directoryFDs) - 1; index >= 0; index-- {
			_ = unix.Close(directoryFDs[index])
		}
	}()
	for _, part := range parts[:len(parts)-1] {
		nextFD, openErr := openSafeFileDirectory(currentFD, part)
		if openErr != nil {
			return 0, nil, openErr
		}
		directoryFDs = append(directoryFDs, nextFD)
		currentFD = nextFD
	}
	return currentFD, directoryFDs, nil
}

func openSafeFileDirectory(parentFD int, name string) (int, error) {
	nextFD, err := unix.Openat(parentFD, name, safeFileDirectoryFlags, 0)
	if errors.Is(err, unix.ENOENT) {
		if mkdirErr := unix.Mkdirat(parentFD, name, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
			return 0, fmt.Errorf("create session directory %q: %w", name, mkdirErr)
		}
		nextFD, err = unix.Openat(parentFD, name, safeFileDirectoryFlags, 0)
	}
	if err != nil {
		return 0, fmt.Errorf("open session directory %q: %w", name, err)
	}
	return nextFD, nil
}

func writeSafeFile(parentFD int, path, name string, data []byte, mode os.FileMode) error {
	fileFD, err := unix.Openat(
		parentFD,
		name,
		unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		uint32(mode.Perm()),
	)
	if err != nil {
		return fmt.Errorf("open session file %q: %w", path, err)
	}
	file := os.NewFile(uintptr(fileFD), "")
	if file == nil {
		_ = unix.Close(fileFD)
		return fmt.Errorf("open session file %q: invalid file", path)
	}
	if err := unix.Fchmod(fileFD, uint32(mode.Perm())); err != nil {
		_ = file.Close()
		return fmt.Errorf("chmod session file %q: %w", path, err)
	}
	if err := writeAllFile(file, data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write session file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close session file %q: %w", path, err)
	}
	return nil
}
