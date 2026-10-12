//go:build windows

package worktree

import (
	"fmt"
	"os"
)

const recoverySpecialMode = os.ModeSetuid | os.ModeSetgid | os.ModeSticky

func recoveryEntryMode(mode os.FileMode) (os.FileMode, error) {
	if mode&recoverySpecialMode != 0 {
		return 0, fmt.Errorf("host filesystem cannot preserve recovery special mode bits")
	}
	return mode.Perm(), nil
}

func applyRecoveryFileAttributes(file *os.File, sourceInfo os.FileInfo) error {
	wantMode, err := recoveryEntryMode(sourceInfo.Mode())
	if err != nil {
		return err
	}
	if err := file.Chmod(wantMode); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	gotMode, err := recoveryEntryMode(info.Mode())
	if err != nil {
		return err
	}
	if gotMode != wantMode {
		return fmt.Errorf("recovery mode did not persist")
	}
	return nil
}

func recoveryRequiredIdentity(info os.FileInfo) (int, int, bool, bool, error) {
	return recoveryRequiredIdentityForMode(info, info.Mode()&os.ModeSetuid != 0, info.Mode()&os.ModeSetgid != 0)
}

func recoveryRequiredIdentityForMode(_ os.FileInfo, requireUID, requireGID bool) (int, int, bool, bool, error) {
	if requireUID || requireGID {
		return 0, 0, false, false, fmt.Errorf("host filesystem cannot preserve recovery set-ID identity")
	}
	return 0, 0, false, false, nil
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
	return nil
}

func openRecoverySourceFile(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("recovery source changed during open")
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("recovery source is not a regular file")
	}
	return file, nil
}

func openRecoveryDirectory(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) || !info.IsDir() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("recovery directory changed during open")
	}
	return file, nil
}

func createRecoveryFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return file, nil
}
