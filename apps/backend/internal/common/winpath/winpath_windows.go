//go:build windows

package winpath

import (
	"errors"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// ReadAccess validates and enumerates a pinned directory. It must never
	// request DELETE: a Windows current-directory handle is opened with
	// FILE_SHARE_READ|FILE_SHARE_WRITE but not FILE_SHARE_DELETE, so any
	// process whose current directory sits in the path makes a DELETE-requesting
	// open fail with a sharing violation and blocks resume.
	ReadAccess uint32 = windows.FILE_GENERIC_READ
	// DeleteAccess reopens a target and its descendants with DELETE, but only
	// at the moment it is about to mark them for deletion.
	DeleteAccess uint32 = windows.FILE_GENERIC_READ | windows.DELETE
	// WriteAccess writes new components; it never needs to delete the
	// already-existing managed root.
	WriteAccess uint32 = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE
	// TraverseAccess is used for ancestors of the target, which are only
	// traversed and attribute-checked. Asking for generic read would
	// additionally require FILE_LIST_DIRECTORY, which a share or folder may
	// deny even when traversal is allowed.
	TraverseAccess uint32 = windows.FILE_TRAVERSE |
		windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
)

// These handles are consumed by os.File (Readdirnames, Stat, Read, Write), which
// issues synchronous I/O. NtCreateFile defaults to asynchronous handles, and a
// synchronous directory read on one (GetFileInformationByHandleEx) waits on the
// file object forever, so every component is opened for synchronous I/O.
const openOptions uint32 = windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT

// OpenDirectoryPath opens the absolute directory path for desiredAccess: the
// volume root is resolved once, every component below it is opened relative to
// its parent without following links, and each handle is checked for a reparse
// point before it is used as a parent.
func OpenDirectoryPath(path string, desiredAccess uint32) (windows.Handle, error) {
	return OpenDirectoryPathWithDisposition(path, desiredAccess, windows.FILE_OPEN)
}

// OpenDirectoryPathWithDisposition opens an absolute directory path with an
// explicit disposition for each component below its volume root.
func OpenDirectoryPathWithDisposition(
	path string,
	desiredAccess uint32,
	disposition uint32,
) (windows.Handle, error) {
	root, parts, err := PathParts(path)
	if err != nil {
		return 0, err
	}
	rootAccess := TraverseAccess
	if len(parts) == 0 {
		rootAccess = desiredAccess
	}
	handle, err := OpenVolumeRoot(root, rootAccess)
	if err != nil {
		return 0, err
	}
	for index, part := range parts {
		// Intermediate components are only traversed; only the final component
		// carries the caller's access (DELETE for removal, read for validation).
		access := TraverseAccess
		if index == len(parts)-1 {
			access = desiredAccess
		}
		next, openErr := OpenHandleWithDisposition(handle, part, access, disposition, windows.FILE_DIRECTORY_FILE)
		_ = windows.CloseHandle(handle)
		if openErr != nil {
			return 0, openErr
		}
		handle, openErr = RejectReparse(next)
		if openErr != nil {
			return 0, openErr
		}
	}
	return handle, nil
}

// OpenVolumeRoot opens a volume or share root. Callers must open each
// user-controlled component relative to this handle and reject reparse points.
func OpenVolumeRoot(root string, desiredAccess uint32) (windows.Handle, error) {
	return createFile(
		0,
		NTPath(root),
		desiredAccess,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.OBJ_CASE_INSENSITIVE,
		windows.FILE_SYNCHRONOUS_IO_NONALERT,
	)
}

// OpenDirectoryRelative opens name below parent as a directory without
// following links.
func OpenDirectoryRelative(parent windows.Handle, name string, desiredAccess uint32) (windows.Handle, error) {
	return OpenHandle(parent, name, desiredAccess, windows.FILE_DIRECTORY_FILE)
}

// OpenEntryRelative opens name below parent as a non-directory without
// following links.
func OpenEntryRelative(parent windows.Handle, name string, desiredAccess uint32) (windows.Handle, error) {
	return OpenHandle(parent, name, desiredAccess, windows.FILE_NON_DIRECTORY_FILE)
}

// OpenHandle opens name below parent without following links.
func OpenHandle(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	createOption uint32,
) (windows.Handle, error) {
	return OpenHandleWithDisposition(parent, name, desiredAccess, windows.FILE_OPEN, createOption)
}

// OpenHandleWithDisposition opens name below parent with an explicit create
// disposition and no-follow attributes.
func OpenHandleWithDisposition(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	disposition uint32,
	createOption uint32,
) (windows.Handle, error) {
	return OpenHandleWithSharing(
		parent, name, desiredAccess, disposition, createOption,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
	)
}

// OpenHandleWithSharing opens name below parent with an explicit share mode and
// no-follow attributes.
func OpenHandleWithSharing(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	disposition uint32,
	createOption uint32,
	shareMode uint32,
) (windows.Handle, error) {
	return createFile(
		parent,
		name,
		desiredAccess,
		disposition,
		createOption,
		shareMode,
		windows.OBJ_CASE_INSENSITIVE|windows.OBJ_DONT_REPARSE,
		openOptions,
	)
}

// OpenTarget walks relative below rootHandle without following links and
// returns the parent handle and the opened target. Intermediate components are
// only traversed; the final component carries targetAccess.
func OpenTarget(rootHandle windows.Handle, relative string, targetAccess uint32) (parent, target windows.Handle, err error) {
	parts := strings.FieldsFunc(filepath.Clean(relative), func(r rune) bool { return r == '\\' || r == '/' })
	parent = rootHandle
	for index, part := range parts {
		access := TraverseAccess
		if index == len(parts)-1 {
			access = targetAccess
		}
		next, openErr := OpenDirectoryRelative(parent, part, access)
		if openErr != nil {
			if parent != rootHandle {
				_ = windows.CloseHandle(parent)
			}
			return 0, 0, openErr
		}
		if reparse, checkErr := HandleIsReparsePoint(next); checkErr != nil {
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
			return 0, 0, errors.New("path is a reparse point")
		}
		if index == len(parts)-1 {
			return parent, next, nil
		}
		if parent != rootHandle {
			_ = windows.CloseHandle(parent)
		}
		parent = next
	}
	return 0, 0, errors.New("relative path is empty")
}

// HandleIsReparsePoint reports whether the opened handle names a reparse point.
func HandleIsReparsePoint(handle windows.Handle) (bool, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return false, err
	}
	return info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

// RejectReparse closes the handle and returns an error when it names a reparse
// point.
func RejectReparse(handle windows.Handle) (windows.Handle, error) {
	reparse, err := HandleIsReparsePoint(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	if reparse {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("path is a reparse point")
	}
	return handle, nil
}

// DuplicateHandle duplicates handle into the current process.
func DuplicateHandle(handle windows.Handle) (windows.Handle, error) {
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(
		windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, false,
		windows.DUPLICATE_SAME_ACCESS,
	); err != nil {
		return 0, err
	}
	return duplicate, nil
}

// MarkForDelete marks the opened handle for deletion.
func MarkForDelete(handle windows.Handle) error {
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

// createFile issues the native open with explicit object attributes and open
// options. Only the volume-root open relaxes the no-reparse attributes; every
// other caller keeps them.
func createFile(
	parent windows.Handle,
	name string,
	desiredAccess uint32,
	disposition uint32,
	createOption uint32,
	shareMode uint32,
	attributes uint32,
	options uint32,
) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	objectAttributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    attributes,
	}
	var ioStatus windows.IO_STATUS_BLOCK
	var allocationSize int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		desiredAccess,
		objectAttributes,
		&ioStatus,
		&allocationSize,
		0,
		shareMode,
		disposition,
		createOption|options,
		0,
		0,
	)
	if err != nil {
		return 0, openError(err)
	}
	return handle, nil
}

// openError converts the raw NTSTATUS returned by NtCreateFile into its Win32
// errno equivalent so errors.Is(err, os.ErrNotExist) and a readable message
// survive the fmt wrap chain the callers add.
func openError(err error) error {
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status.Errno()
	}
	return err
}
