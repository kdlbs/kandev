//go:build windows

package managedruntime

import (
	"os"
	"os/user"
	"strings"

	"golang.org/x/sys/windows"
)

func repairLocalUserIdentity() string {
	if current, err := user.Current(); err == nil && current.Uid != "" {
		return "uid:" + current.Uid
	}
	if username := strings.TrimSpace(os.Getenv("USERNAME")); username != "" {
		return "user:" + strings.ToLower(username)
	}
	return "user:unknown"
}

func validatePrivateRepairPermissions(os.FileInfo) error {
	return nil
}

func validatePrivateRepairOwnership(os.FileInfo) error {
	return nil
}

func syncRepairDirectory(string) {}

func openRepairContextLock(path string) (*os.File, error) {
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(path),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errRepairLockHandle
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		_ = file.Close()
		return nil, errRepairLockHandle
	}
	return file, nil
}

func tryLockRepairContext(file *os.File) (bool, error) {
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &windows.Overlapped{},
	)
	if err == windows.ERROR_LOCK_VIOLATION {
		return false, nil
	}
	return err == nil, err
}

func unlockRepairContextLock(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
