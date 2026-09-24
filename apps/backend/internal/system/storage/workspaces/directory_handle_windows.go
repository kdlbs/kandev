//go:build windows

package workspaces

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type windowsDirectoryHandle struct {
	rootHandle   windows.Handle
	parentHandle windows.Handle
	targetHandle windows.Handle
	target       string
	once         sync.Once
}

// OpenDirectoryNoFollow opens root and target with OBJ_DONT_REPARSE and
// FILE_OPEN_REPARSE_POINT for every component. It requests read-only access so a
// process whose current directory sits in the task root or worktree (a CWD
// handle carries no FILE_SHARE_DELETE) cannot make validation fail with a
// sharing violation. DELETE is deferred to RemoveDirectory.
func OpenDirectoryNoFollow(root, target string) (DirectoryHandle, error) {
	relative, err := dependencyRelativePath(root, target)
	if err != nil {
		return nil, err
	}
	rootHandle, err := openWindowsDependencyDirectoryPath(root, windowsDependencyReadAccess)
	if err != nil {
		return nil, fmt.Errorf("open task root: %w", err)
	}
	parentHandle, targetHandle, err := openWindowsDependencyTarget(rootHandle, relative, windowsDependencyReadAccess)
	if err != nil {
		_ = windows.CloseHandle(rootHandle)
		return nil, fmt.Errorf("open worktree target: %w", err)
	}
	return &windowsDirectoryHandle{
		rootHandle: rootHandle, parentHandle: parentHandle, targetHandle: targetHandle,
		target: filepath.Base(filepath.Clean(relative)),
	}, nil
}

// CreateDirectoryNoFollow creates every missing component below root through
// native directory handles. Reparse points and non-directories are rejected.
func CreateDirectoryNoFollow(root, target string, _ os.FileMode) (DirectoryHandle, error) {
	relative, err := dependencyRelativePath(root, target)
	if err != nil {
		return nil, err
	}
	rootHandle, err := openOrCreateWindowsDependencyDirectoryPath(root)
	if err != nil {
		return nil, err
	}
	parentHandle, targetHandle, err := openOrCreateWindowsDependencyTarget(rootHandle, relative)
	if err != nil {
		_ = windows.CloseHandle(rootHandle)
		return nil, err
	}
	return &windowsDirectoryHandle{
		rootHandle: rootHandle, parentHandle: parentHandle, targetHandle: targetHandle,
		target: filepath.Base(filepath.Clean(relative)),
	}, nil
}

func (h *windowsDirectoryHandle) Close() error {
	var closeErr error
	h.once.Do(func() {
		if h.targetHandle != 0 {
			if err := windows.CloseHandle(h.targetHandle); err != nil {
				closeErr = err
			}
		}
		if h.parentHandle != 0 && h.parentHandle != h.rootHandle {
			if err := windows.CloseHandle(h.parentHandle); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		if h.rootHandle != 0 {
			if err := windows.CloseHandle(h.rootHandle); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

func (h *windowsDirectoryHandle) VerifyPath(path string) error {
	if h == nil || h.targetHandle == 0 {
		return errors.New("directory handle is closed")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	dup, err := duplicateWindowsDependencyHandle(h.targetHandle)
	if err != nil {
		return fmt.Errorf("duplicate directory handle: %w", err)
	}
	file := os.NewFile(uintptr(dup), "worktree-directory")
	if file == nil {
		_ = windows.CloseHandle(dup)
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

func (h *windowsDirectoryHandle) IsValidWorktree() bool {
	content, err := h.ReadFile(".git")
	return err == nil && strings.HasPrefix(string(content), "gitdir:")
}

// RemoveDirectory removes the pinned directory through its native handle. It
// never resolves the directory again from its lexical path, so a rename or
// replacement cannot redirect deletion to a different workspace.
func (h *windowsDirectoryHandle) RemoveDirectory(ctx context.Context) error {
	if h == nil || h.targetHandle == 0 || h.parentHandle == 0 || h.target == "" {
		return errors.New("directory handle is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	deletable, same, err := h.openPinnedTargetForDelete()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// A sharing violation must be reported before any child entry is
			// removed. The caller can retry after the holder releases the target.
			return err
		}
		// The pinned directory was renamed and has no lexical entry to mark.
		// Its handle still identifies the original worktree, so clean it through
		// that handle and preserve the existing no-follow behavior.
		return removeWindowsDependencyContents(ctx, h.targetHandle)
	}
	if !same {
		// The lexical path now names another directory. Continue through the
		// pinned handle so the replacement cannot be touched, but report the
		// path change to the caller after the original contents are cleared.
		if err := removeWindowsDependencyContents(ctx, h.targetHandle); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.New("pinned directory path changed during cleanup")
	}
	defer func() { _ = windows.CloseHandle(deletable) }()
	if err := removeWindowsDependencyContents(ctx, deletable); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return markWindowsDependencyForDelete(deletable)
}

// openPinnedTargetForDelete reopens the target relative to its parent with
// DELETE access before cleanup mutates any child entry. The validation open
// deliberately omitted DELETE so a process whose current directory sits in
// the task root could not block resume with a sharing violation. The reopened
// handle is confirmed to be the same file as the pinned target before it is
// used for deletion, so a lexical replacement cannot redirect the cleanup.
func (h *windowsDirectoryHandle) openPinnedTargetForDelete() (windows.Handle, bool, error) {
	deletable, err := openWindowsDependencyDirectoryRelative(h.parentHandle, h.target, windowsDependencyDeleteAccess)
	if err != nil {
		return 0, false, err
	}
	same, err := windowsDependencyHandlesSameFile(h.targetHandle, deletable)
	if err != nil {
		_ = windows.CloseHandle(deletable)
		return 0, false, err
	}
	if !same {
		_ = windows.CloseHandle(deletable)
		return 0, false, nil
	}
	return deletable, true, nil
}

func windowsDependencyHandlesSameFile(a, b windows.Handle) (bool, error) {
	var infoA, infoB windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(a, &infoA); err != nil {
		return false, err
	}
	if err := windows.GetFileInformationByHandle(b, &infoB); err != nil {
		return false, err
	}
	return infoA.VolumeSerialNumber == infoB.VolumeSerialNumber &&
		infoA.FileIndexHigh == infoB.FileIndexHigh &&
		infoA.FileIndexLow == infoB.FileIndexLow, nil
}

func (h *windowsDirectoryHandle) ReadFile(name string) ([]byte, error) {
	file, err := h.OpenFile(name)
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	return content, closeErr
}

func (h *windowsDirectoryHandle) OpenFile(name string) (io.ReadCloser, error) {
	if h == nil || h.targetHandle == 0 {
		return nil, errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return nil, err
	}
	handle, err := openWindowsDependencyEntryRelative(h.targetHandle, name, windowsDependencyReadAccess)
	if err != nil {
		return nil, err
	}
	if reparse, checkErr := windowsDependencyHandleIsReparsePoint(handle); checkErr != nil {
		_ = windows.CloseHandle(handle)
		return nil, checkErr
	} else if reparse {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("directory entry is a reparse point: %s", name)
	}
	file := os.NewFile(uintptr(handle), filepath.Join("worktree-directory", name))
	if file == nil {
		_ = windows.CloseHandle(handle)
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
	return file, nil
}

func (h *windowsDirectoryHandle) OpenSubdirectory(name string) (DirectoryHandle, error) {
	if h == nil || h.targetHandle == 0 {
		return nil, errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return nil, err
	}
	targetHandle, err := openWindowsDependencyDirectoryRelative(h.targetHandle, name, windowsDependencyReadAccess)
	if err != nil {
		return nil, err
	}
	if reparse, err := windowsDependencyHandleIsReparsePoint(targetHandle); err != nil {
		_ = windows.CloseHandle(targetHandle)
		return nil, err
	} else if reparse {
		_ = windows.CloseHandle(targetHandle)
		return nil, fmt.Errorf("directory entry is a reparse point: %s", name)
	}
	rootHandle, err := duplicateWindowsDependencyHandle(h.targetHandle)
	if err != nil {
		_ = windows.CloseHandle(targetHandle)
		return nil, err
	}
	parentHandle, err := duplicateWindowsDependencyHandle(h.targetHandle)
	if err != nil {
		_ = windows.CloseHandle(targetHandle)
		_ = windows.CloseHandle(rootHandle)
		return nil, err
	}
	return &windowsDirectoryHandle{
		rootHandle: rootHandle, parentHandle: parentHandle, targetHandle: targetHandle, target: name,
	}, nil
}

func (h *windowsDirectoryHandle) LstatEntry(name string) (os.FileMode, error) {
	if h == nil || h.targetHandle == 0 {
		return 0, errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return 0, err
	}
	handle, err := openWindowsDependencyHandle(h.targetHandle, name, windowsDependencyReadAccess, 0)
	if err != nil {
		return 0, err
	}
	if reparse, err := windowsDependencyHandleIsReparsePoint(handle); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	} else if reparse {
		_ = windows.CloseHandle(handle)
		return os.ModeSymlink, nil
	}
	file := os.NewFile(uintptr(handle), filepath.Join("worktree-directory", name))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("create file info from directory entry handle")
	}
	info, err := file.Stat()
	closeErr := file.Close()
	if err != nil {
		return 0, err
	}
	return info.Mode(), closeErr
}

func (h *windowsDirectoryHandle) ReadLink(name string) (string, error) {
	if h == nil || h.targetHandle == 0 {
		return "", errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return "", err
	}
	handle, err := openWindowsDependencyHandle(h.targetHandle, name, windowsDependencyReadAccess, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	buffer := make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var returned uint32
	if err := windows.DeviceIoControl(
		handle, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &buffer[0], uint32(len(buffer)), &returned, nil,
	); err != nil {
		return "", err
	}
	if returned < 16 {
		return "", errors.New("reparse point data is truncated")
	}
	tag := binary.LittleEndian.Uint32(buffer[:4])
	pathOffset := 16
	switch tag {
	case windows.IO_REPARSE_TAG_SYMLINK:
		if returned < 20 {
			return "", errors.New("symbolic link data is truncated")
		}
		pathOffset = 20
	case windows.IO_REPARSE_TAG_MOUNT_POINT:
		pathOffset = 16
	default:
		return "", errors.New("reparse point is not a symbolic link or junction")
	}
	nameOffset := binary.LittleEndian.Uint16(buffer[12:14])
	nameLength := binary.LittleEndian.Uint16(buffer[14:16])
	if nameLength == 0 {
		nameOffset = binary.LittleEndian.Uint16(buffer[8:10])
		nameLength = binary.LittleEndian.Uint16(buffer[10:12])
	}
	start := pathOffset + int(nameOffset)
	end := start + int(nameLength)
	if start < pathOffset || end > int(returned) || nameLength%2 != 0 {
		return "", errors.New("reparse point path is invalid")
	}
	units := make([]uint16, nameLength/2)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(buffer[start+index*2 : start+index*2+2])
	}
	return string(utf16.Decode(units)), nil
}

func (h *windowsDirectoryHandle) ReadDir() ([]os.DirEntry, error) {
	if h == nil || h.targetHandle == 0 {
		return nil, errors.New("directory handle is closed")
	}
	handle, err := duplicateWindowsDependencyHandle(h.targetHandle)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(handle), "worktree-directory")
	if directory == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("create directory reader from handle")
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return nil, readErr
	}
	return entries, closeErr
}

func (h *windowsDirectoryHandle) WriteFile(name string, data []byte, _ os.FileMode) error {
	if h == nil || h.targetHandle == 0 {
		return errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return err
	}
	handle, err := openWindowsDependencyHandleWithDisposition(
		h.targetHandle, name, windowsDependencyWriteAccess, windows.FILE_OVERWRITE_IF, windows.FILE_NON_DIRECTORY_FILE,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), filepath.Join("worktree-directory", name))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("create writable file from directory entry handle")
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func openOrCreateWindowsDependencyDirectoryPath(path string) (windows.Handle, error) {
	root, parts, err := windowsDependencyPathParts(path)
	if err != nil {
		return 0, err
	}
	rootHandle, err := openWindowsDependencyHandle(0, windowsDependencyNTPath(root), windowsDependencyWriteAccess, windows.FILE_DIRECTORY_FILE)
	if err != nil {
		return 0, err
	}
	for _, part := range parts {
		next, openErr := openWindowsDependencyHandleWithDisposition(
			rootHandle, part, windowsDependencyWriteAccess, windows.FILE_OPEN_IF, windows.FILE_DIRECTORY_FILE,
		)
		if openErr != nil {
			_ = windows.CloseHandle(rootHandle)
			return 0, openErr
		}
		if reparse, checkErr := windowsDependencyHandleIsReparsePoint(next); checkErr != nil {
			_ = windows.CloseHandle(next)
			_ = windows.CloseHandle(rootHandle)
			return 0, checkErr
		} else if reparse {
			_ = windows.CloseHandle(next)
			_ = windows.CloseHandle(rootHandle)
			return 0, fmt.Errorf("dependency path is a reparse point")
		}
		_ = windows.CloseHandle(rootHandle)
		rootHandle = next
	}
	return rootHandle, nil
}

func openOrCreateWindowsDependencyTarget(rootHandle windows.Handle, relative string) (parent, target windows.Handle, err error) {
	parts := strings.FieldsFunc(filepath.Clean(relative), func(r rune) bool { return r == '\\' || r == '/' })
	parent = rootHandle
	for index, part := range parts {
		next, openErr := openWindowsDependencyHandleWithDisposition(
			parent, part, windowsDependencyWriteAccess, windows.FILE_OPEN_IF, windows.FILE_DIRECTORY_FILE,
		)
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

func windowsDependencyPathParts(path string) (string, []string, error) {
	if !filepath.IsAbs(path) {
		return "", nil, fmt.Errorf("dependency workspace root must be absolute")
	}
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if volume == "" {
		return "", nil, fmt.Errorf("dependency workspace root has no volume")
	}
	remainder := strings.TrimPrefix(clean, volume)
	remainder = strings.TrimLeft(remainder, `\`+`/`)
	root := volume + `\`
	if remainder == "" {
		return root, nil, nil
	}
	return root, strings.FieldsFunc(remainder, func(r rune) bool { return r == '\\' || r == '/' }), nil
}
