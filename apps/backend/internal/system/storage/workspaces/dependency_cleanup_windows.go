//go:build windows

package workspaces

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows uses native handles rooted at the workspace. OBJ_DONT_REPARSE and
// FILE_OPEN_REPARSE_POINT make each component no-follow; deletion is then
// requested on the opened handle rather than by path.
func removeDependencyDirectory(ctx context.Context, root, target string) error {
	return removeDependencyDirectoryWithHook(ctx, root, target, nil)
}

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
	rootHandle, err := openWindowsDependencyDirectoryPath(root)
	if err != nil {
		return fmt.Errorf("open dependency workspace root: %w", err)
	}
	defer windows.CloseHandle(rootHandle)

	parentHandle, targetHandle, err := openWindowsDependencyTarget(rootHandle, relative)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(targetHandle)
	if parentHandle != rootHandle {
		defer windows.CloseHandle(parentHandle)
	}
	if afterOpen != nil {
		afterOpen()
	}
	if err := removeWindowsDependencyContents(ctx, targetHandle); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return markWindowsDependencyForDelete(targetHandle)
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
	for _, part := range strings.FieldsFunc(filepath.Clean(relative), func(r rune) bool { return r == '\\' || r == '/' }) {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("dependency target contains an unsafe path component")
		}
	}
	return relative, nil
}

func openWindowsDependencyDirectoryPath(path string) (windows.Handle, error) {
	name := windowsDependencyNTPath(path)
	handle, err := openWindowsDependencyHandle(0, name, windows.FILE_DIRECTORY_FILE)
	if err != nil {
		return 0, err
	}
	return rejectWindowsDependencyReparse(handle)
}

func openWindowsDependencyDirectoryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	return openWindowsDependencyHandle(parent, name, windows.FILE_DIRECTORY_FILE)
}

func openWindowsDependencyEntryRelative(parent windows.Handle, name string) (windows.Handle, error) {
	return openWindowsDependencyHandle(parent, name, windows.FILE_NON_DIRECTORY_FILE)
}

func openWindowsDependencyHandle(
	parent windows.Handle,
	name string,
	createOption uint32,
) (windows.Handle, error) {
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

func rejectWindowsDependencyReparse(handle windows.Handle) (windows.Handle, error) {
	reparse, err := windowsDependencyHandleIsReparsePoint(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	if reparse {
		_ = windows.CloseHandle(handle)
		return 0, fmt.Errorf("dependency path is a reparse point")
	}
	return handle, nil
}

func windowsDependencyNTPath(path string) string {
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, `\\?\`) {
		return `\??\` + strings.TrimPrefix(clean, `\\?\`)
	}
	if strings.HasPrefix(clean, `\\`) {
		return `\??\UNC\` + strings.TrimPrefix(clean, `\\`)
	}
	return `\??\` + clean
}

func openWindowsDependencyTarget(rootHandle windows.Handle, relative string) (parent, target windows.Handle, err error) {
	parts := strings.FieldsFunc(filepath.Clean(relative), func(r rune) bool { return r == '\\' || r == '/' })
	parent = rootHandle
	for index, part := range parts {
		next, openErr := openWindowsDependencyDirectoryRelative(parent, part)
		if openErr != nil {
			if parent != rootHandle {
				_ = windows.CloseHandle(parent)
			}
			return 0, 0, openErr
		}
		if reparse, checkErr := windowsDependencyHandleIsReparsePoint(next); checkErr != nil {
			_ = windows.CloseHandle(next)
			if parent != rootHandle {
				_ = windows.CloseHandle(parent)
			}
			return 0, 0, checkErr
		} else if reparse {
			_ = windows.CloseHandle(next)
			if parent != rootHandle {
				_ = windows.CloseHandle(parent)
			}
			return 0, 0, fmt.Errorf("dependency path is a reparse point")
		}
		if index == len(parts)-1 {
			return parent, next, nil
		}
		if parent != rootHandle {
			_ = windows.CloseHandle(parent)
		}
		parent = next
	}
	return 0, 0, fmt.Errorf("dependency target is empty")
}

func windowsDependencyHandleIsReparsePoint(handle windows.Handle) (bool, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return false, err
	}
	return info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func removeWindowsDependencyContents(ctx context.Context, dirHandle windows.Handle) error {
	readHandle, err := duplicateWindowsDependencyHandle(dirHandle)
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(readHandle), "dependency-directory")
	if dir == nil {
		_ = windows.CloseHandle(readHandle)
		return fmt.Errorf("open dependency directory handle")
	}
	names, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		child, dirErr := openWindowsDependencyDirectoryRelative(dirHandle, name)
		if dirErr == nil {
			reparse, checkErr := windowsDependencyHandleIsReparsePoint(child)
			if checkErr != nil {
				_ = windows.CloseHandle(child)
				return checkErr
			}
			if reparse {
				_ = windows.CloseHandle(child)
				continue
			}
			if err := removeWindowsDependencyContents(ctx, child); err != nil {
				_ = windows.CloseHandle(child)
				return err
			}
			err := markWindowsDependencyForDelete(child)
			_ = windows.CloseHandle(child)
			if err != nil {
				return err
			}
			continue
		}
		file, fileErr := openWindowsDependencyEntryRelative(dirHandle, name)
		if fileErr != nil {
			return fmt.Errorf("open dependency entry %s: %w", name, fileErr)
		}
		reparse, checkErr := windowsDependencyHandleIsReparsePoint(file)
		if checkErr != nil {
			_ = windows.CloseHandle(file)
			return checkErr
		}
		if reparse {
			_ = windows.CloseHandle(file)
			continue
		}
		err = markWindowsDependencyForDelete(file)
		_ = windows.CloseHandle(file)
		if err != nil {
			return err
		}
	}
	return nil
}

func duplicateWindowsDependencyHandle(handle windows.Handle) (windows.Handle, error) {
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(
		windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, false,
		windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		return 0, err
	}
	return duplicate, nil
}

func markWindowsDependencyForDelete(handle windows.Handle) error {
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
