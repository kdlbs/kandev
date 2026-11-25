//go:build windows

package managedruntime

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"

	"github.com/kandev/kandev/internal/common/winpath"
)

// Windows uses native handles rooted at the cache. Absolute paths are opened
// through the shared no-follow walk, which resolves the object-manager volume
// link once and then opens each component relative to its parent with
// OBJ_DONT_REPARSE and FILE_OPEN_REPARSE_POINT, rejecting junctions and
// symlinks; deletion is then requested on the opened handle.
func removeNpxExecutionTree(cacheRoot, key string) error {
	rootHandle, err := openManagedRuntimeDirectoryPath(cacheRoot)
	if isManagedRuntimeNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open npm cache root: %w", err)
	}
	defer windows.CloseHandle(rootHandle)

	npxHandle, err := openManagedRuntimeDirectoryRelative(rootHandle, "_npx")
	if isManagedRuntimeNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open npm execution cache: %w", err)
	}
	if err := rejectManagedRuntimeReparse(npxHandle); err != nil {
		return err
	}
	defer windows.CloseHandle(npxHandle)

	targetHandle, err := openManagedRuntimeDirectoryRelative(npxHandle, key)
	if isManagedRuntimeNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open npm execution tree: %w", err)
	}
	if err := rejectManagedRuntimeReparse(targetHandle); err != nil {
		return err
	}
	defer windows.CloseHandle(targetHandle)

	if err := removeManagedRuntimeWindowsContents(targetHandle); err != nil {
		return fmt.Errorf("remove npm execution tree contents: %w", err)
	}
	return markManagedRuntimeForDelete(targetHandle)
}

func isManagedRuntimeNotFound(err error) bool {
	if errors.Is(err, windows.ERROR_PATH_NOT_FOUND) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return true
	}
	var status windows.NTStatus
	if !errors.As(err, &status) {
		return false
	}
	errno := status.Errno()
	return errors.Is(errno, windows.ERROR_PATH_NOT_FOUND) || errors.Is(errno, windows.ERROR_FILE_NOT_FOUND)
}

func openManagedRuntimeDirectoryPath(path string) (windows.Handle, error) {
	return winpath.OpenDirectoryPath(path,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE)
}

func openManagedRuntimeDirectoryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	return openManagedRuntimeHandle(parent, name, windows.FILE_DIRECTORY_FILE)
}

func openManagedRuntimeEntryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	return openManagedRuntimeHandle(parent, name, windows.FILE_NON_DIRECTORY_FILE)
}

func openManagedRuntimeHandle(parent windows.Handle, name string, createOption uint32) (windows.Handle, error) {
	return openManagedRuntimeHandleWithAccess(parent, name, createOption,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE)
}

func openManagedRuntimeReadHandle(parent windows.Handle, name string, createOption uint32) (windows.Handle, error) {
	return openManagedRuntimeHandleWithAccess(parent, name, createOption, windows.FILE_GENERIC_READ)
}

func openManagedRuntimeHandleWithAccess(parent windows.Handle, name string, createOption, access uint32) (windows.Handle, error) {
	return winpath.OpenHandleWithSharing(
		parent, name, access, windows.FILE_OPEN, createOption,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
	)
}

func openNpxInventoryDirectoryPath(path string) (*os.File, error) {
	handle, err := winpath.OpenDirectoryPath(path, windows.FILE_GENERIC_READ)
	if err != nil {
		return nil, normalizeNpxInventoryOpenError(err)
	}
	if err := rejectManagedRuntimeReparse(handle); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), "npm-cache"), nil
}

func openNpxInventoryDirectory(parent *os.File, name string) (*os.File, error) {
	if !validInventoryEntryName(name) {
		return nil, errors.New("invalid npm execution entry name")
	}
	handle, err := openManagedRuntimeReadHandle(windows.Handle(parent.Fd()), name, windows.FILE_DIRECTORY_FILE)
	if err != nil {
		return nil, normalizeNpxInventoryOpenError(err)
	}
	if err := rejectManagedRuntimeReparse(handle); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}

func openNpxInventoryFile(parent *os.File, name string) (*os.File, error) {
	if !validInventoryEntryName(name) {
		return nil, errors.New("invalid npm execution entry name")
	}
	handle, err := openManagedRuntimeReadHandle(windows.Handle(parent.Fd()), name, windows.FILE_NON_DIRECTORY_FILE)
	if err != nil {
		return nil, normalizeNpxInventoryOpenError(err)
	}
	if err := rejectManagedRuntimeReparse(handle); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}

func normalizeNpxInventoryOpenError(err error) error {
	if isManagedRuntimeNotFound(err) {
		return os.ErrNotExist
	}
	return err
}

func rejectManagedRuntimeReparse(handle windows.Handle) error {
	_, err := winpath.RejectReparse(handle)
	return err
}

func removeManagedRuntimeWindowsContents(dirHandle windows.Handle) error {
	readHandle, err := duplicateManagedRuntimeHandle(dirHandle)
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(readHandle), "managed-runtime-cache")
	if dir == nil {
		_ = windows.CloseHandle(readHandle)
		return fmt.Errorf("open npm execution directory handle")
	}
	names, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	for _, name := range names {
		child, dirErr := openManagedRuntimeDirectoryRelative(dirHandle, name)
		if dirErr == nil {
			if err := rejectManagedRuntimeReparse(child); err != nil {
				return err
			}
			if err := removeManagedRuntimeWindowsContents(child); err != nil {
				_ = windows.CloseHandle(child)
				return err
			}
			err := markManagedRuntimeForDelete(child)
			_ = windows.CloseHandle(child)
			if err != nil {
				return err
			}
			continue
		}

		file, fileErr := openManagedRuntimeEntryRelative(dirHandle, name)
		if fileErr != nil {
			return fmt.Errorf("open npm execution entry %s: %w", name, fileErr)
		}
		if err := rejectManagedRuntimeReparse(file); err != nil {
			return err
		}
		err = markManagedRuntimeForDelete(file)
		_ = windows.CloseHandle(file)
		if err != nil {
			return err
		}
	}
	return nil
}

func duplicateManagedRuntimeHandle(handle windows.Handle) (windows.Handle, error) {
	return winpath.DuplicateHandle(handle)
}

func markManagedRuntimeForDelete(handle windows.Handle) error {
	return winpath.MarkForDelete(handle)
}
