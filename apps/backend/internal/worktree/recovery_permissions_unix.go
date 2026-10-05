//go:build unix

package worktree

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const recoverySpecialMode = os.ModeSetuid | os.ModeSetgid | os.ModeSticky

func recoveryEntryMode(mode os.FileMode) (os.FileMode, error) {
	return mode.Perm() | mode&recoverySpecialMode, nil
}

func applyRecoveryFileAttributes(file *os.File, sourceInfo os.FileInfo) error {
	return applyRecoveryFileAttributesWith(file, sourceInfo, file.Chown)
}

func applyRecoveryFileAttributesWith(
	file *os.File,
	sourceInfo os.FileInfo,
	changeOwner func(int, int) error,
) error {
	wantMode, err := recoveryEntryMode(sourceInfo.Mode())
	if err != nil {
		return err
	}
	wantUID, wantGID, requireUID, requireGID, err := recoveryRequiredIdentity(sourceInfo)
	if err != nil {
		return err
	}
	currentInfo, err := file.Stat()
	if err != nil {
		return err
	}
	currentUID, currentGID, _, _, err := recoveryRequiredIdentityForMode(currentInfo, requireUID, requireGID)
	if err != nil {
		return err
	}
	uid, gid := -1, -1
	if requireUID && currentUID != wantUID {
		uid = wantUID
	}
	if requireGID && currentGID != wantGID {
		gid = wantGID
	}
	if uid != -1 || gid != -1 {
		if err := changeOwner(uid, gid); err != nil {
			return fmt.Errorf("preserve recovery set-ID identity: %w", err)
		}
	}
	if err := file.Chmod(wantMode); err != nil {
		return fmt.Errorf("apply recovery mode: %w", err)
	}
	return verifyRecoveryFileAttributes(file, wantMode, wantUID, wantGID, requireUID, requireGID)
}

func recoveryRequiredIdentity(info os.FileInfo) (int, int, bool, bool, error) {
	return recoveryRequiredIdentityForMode(info, info.Mode()&os.ModeSetuid != 0, info.Mode()&os.ModeSetgid != 0)
}

func recoveryRequiredIdentityForMode(info os.FileInfo, requireUID, requireGID bool) (int, int, bool, bool, error) {
	if !requireUID && !requireGID {
		return 0, 0, false, false, nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false, false, fmt.Errorf("recovery set-ID identity is unavailable")
	}
	return int(stat.Uid), int(stat.Gid), requireUID, requireGID, nil
}

func verifyRecoveryFileAttributes(
	file *os.File,
	wantMode os.FileMode,
	wantUID, wantGID int,
	requireUID, requireGID bool,
) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	gotMode, err := recoveryEntryMode(info.Mode())
	if err != nil {
		return err
	}
	if gotMode != wantMode {
		return fmt.Errorf("recovery mode did not persist: got %s, want %s", gotMode, wantMode)
	}
	gotUID, gotGID, _, _, err := recoveryRequiredIdentityForMode(info, requireUID, requireGID)
	if err != nil {
		return err
	}
	if requireUID && gotUID != wantUID || requireGID && gotGID != wantGID {
		return fmt.Errorf("recovery set-ID identity did not persist")
	}
	return nil
}

func verifyRecoverySourceInfo(expected, current os.FileInfo) error {
	if !os.SameFile(expected, current) {
		return fmt.Errorf("recovery source entry changed during copy")
	}
	wantMode, err := recoveryEntryMode(expected.Mode())
	if err != nil {
		return err
	}
	gotMode, err := recoveryEntryMode(current.Mode())
	if err != nil {
		return err
	}
	if gotMode != wantMode {
		return fmt.Errorf("recovery source mode changed during copy")
	}
	wantUID, wantGID, requireUID, requireGID, err := recoveryRequiredIdentity(expected)
	if err != nil {
		return err
	}
	gotUID, gotGID, _, _, err := recoveryRequiredIdentityForMode(current, requireUID, requireGID)
	if err != nil {
		return err
	}
	if requireUID && gotUID != wantUID || requireGID && gotGID != wantGID {
		return fmt.Errorf("recovery source set-ID identity changed during copy")
	}
	return nil
}

func openRecoverySourceFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open recovery source file")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("recovery source is not a regular file")
	}
	return file, nil
}

func openRecoveryDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open recovery directory")
	}
	return file, nil
}

func createRecoveryFile(path string) (*os.File, error) {
	fd, err := unix.Open(path,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("create recovery destination file")
	}
	return file, nil
}
