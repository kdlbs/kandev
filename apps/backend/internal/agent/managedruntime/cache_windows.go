//go:build windows

package managedruntime

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows uses native handles rooted at the cache. OBJ_DONT_REPARSE and
// FILE_OPEN_REPARSE_POINT reject junctions and symlinks while each component
// is opened; deletion is then requested on the opened handle.
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
	return openManagedRuntimeHandle(0, managedRuntimeWindowsNTPath(path), windows.FILE_DIRECTORY_FILE)
}

func openManagedRuntimeDirectoryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	return openManagedRuntimeHandle(parent, name, windows.FILE_DIRECTORY_FILE)
}

func openManagedRuntimeEntryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	return openManagedRuntimeHandle(parent, name, windows.FILE_NON_DIRECTORY_FILE)
}

func openManagedRuntimeHandle(parent windows.Handle, name string, createOption uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	objectAttributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var ioStatus windows.IO_STATUS_BLOCK
	var allocationSize int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
		objectAttributes,
		&ioStatus,
		&allocationSize,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		createOption|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

func managedRuntimeWindowsNTPath(path string) string {
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, `\\?\`) {
		return `\??\` + strings.TrimPrefix(clean, `\\?\`)
	}
	if strings.HasPrefix(clean, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(clean, `\\`)
	}
	return `\??\` + clean
}

func rejectManagedRuntimeReparse(handle windows.Handle) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("refusing to invalidate execution cache through reparse point")
	}
	return nil
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
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(
		windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, false,
		windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		return 0, err
	}
	return duplicate, nil
}

func markManagedRuntimeForDelete(handle windows.Handle) error {
	info := struct{ Flags uint32 }{
		Flags: windows.FILE_DISPOSITION_DELETE |
			windows.FILE_DISPOSITION_POSIX_SEMANTICS |
			windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE,
	}
	return windows.SetFileInformationByHandle(
		handle,
		windows.FileDispositionInfoEx,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
}
