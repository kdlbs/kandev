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

	"golang.org/x/sys/windows"

	"github.com/kandev/kandev/internal/system/winpath"
)

const (
	// Validation and reuse only enumerate and read the pinned directory. They
	// must never request DELETE: a Windows current-directory handle is opened
	// with FILE_SHARE_READ|FILE_SHARE_WRITE but not FILE_SHARE_DELETE, so any
	// process whose current directory sits in the task root or worktree makes a
	// DELETE-requesting open fail with a sharing violation and blocks resume.
	windowsDependencyReadAccess uint32 = winpath.ReadAccess
	// Removal reopens the target and its descendants with DELETE, but only at the
	// moment it is about to mark them for deletion.
	windowsDependencyDeleteAccess uint32 = winpath.DeleteAccess
	// Creation writes new components and the ownership marker; it never needs to
	// delete the already-existing managed root.
	windowsDependencyWriteAccess uint32 = winpath.WriteAccess
	// Ancestors of the target are only traversed and attribute-checked. Asking
	// for generic read would additionally require FILE_LIST_DIRECTORY, which a
	// share or folder may deny even when traversal is allowed.
	windowsDependencyTraverseAccess uint32 = winpath.TraverseAccess
)

// RemoveDirectoryNoFollow opens every path component without following links and
// removes the target through the resulting native handles.
func RemoveDirectoryNoFollow(ctx context.Context, root, target string) error {
	return removeDependencyDirectory(ctx, root, target)
}

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
	rootHandle, err := openWindowsDependencyDirectoryPath(root, windowsDependencyReadAccess)
	if err != nil {
		return fmt.Errorf("open dependency workspace root: %w", err)
	}
	defer windows.CloseHandle(rootHandle)

	parentHandle, targetHandle, err := openWindowsDependencyTarget(rootHandle, relative, windowsDependencyDeleteAccess)
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

func openWindowsDependencyDirectoryPath(path string, desiredAccess uint32) (windows.Handle, error) {
	return winpath.OpenDirectoryPath(path, desiredAccess)
}

func openWindowsDependencyVolumeRoot(root string, desiredAccess uint32) (windows.Handle, error) {
	return winpath.OpenVolumeRoot(root, desiredAccess)
}

func openWindowsDependencyDirectoryRelative(parent windows.Handle, name string, desiredAccess uint32) (windows.Handle, error) {
	return winpath.OpenDirectoryRelative(parent, name, desiredAccess)
}

func openWindowsDependencyEntryRelative(parent windows.Handle, name string, desiredAccess uint32) (windows.Handle, error) {
	return winpath.OpenEntryRelative(parent, name, desiredAccess)
}

func openWindowsDependencyHandle(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	createOption uint32,
) (windows.Handle, error) {
	return winpath.OpenHandle(parent, name, desiredAccess, createOption)
}

func openWindowsDependencyHandleWithDisposition(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	disposition uint32,
	createOption uint32,
) (windows.Handle, error) {
	return winpath.OpenHandleWithDisposition(parent, name, desiredAccess, disposition, createOption)
}

func openWindowsDependencyHandleWithSharing(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	disposition uint32,
	createOption uint32,
	shareMode uint32,
) (windows.Handle, error) {
	return winpath.OpenHandleWithSharing(parent, name, desiredAccess, disposition, createOption, shareMode)
}

func openWindowsDependencyTarget(rootHandle windows.Handle, relative string, targetAccess uint32) (parent, target windows.Handle, err error) {
	return winpath.OpenTarget(rootHandle, relative, targetAccess)
}

func windowsDependencyPathParts(path string) (string, []string, error) {
	return winpath.PathParts(path)
}

func rejectWindowsDependencyReparse(handle windows.Handle) (windows.Handle, error) {
	return winpath.RejectReparse(handle)
}

func windowsDependencyHandleIsReparsePoint(handle windows.Handle) (bool, error) {
	return winpath.HandleIsReparsePoint(handle)
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
		child, dirErr := openWindowsDependencyDirectoryRelative(dirHandle, name, windowsDependencyDeleteAccess)
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
		file, fileErr := openWindowsDependencyEntryRelative(dirHandle, name, windowsDependencyDeleteAccess)
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
	return winpath.DuplicateHandle(handle)
}

func markWindowsDependencyForDelete(handle windows.Handle) error {
	return winpath.MarkForDelete(handle)
}
