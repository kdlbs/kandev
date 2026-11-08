//go:build !windows

package managedruntime

import (
	"errors"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func repairLocalUserIdentity() string {
	return "uid:" + strconv.FormatUint(uint64(os.Getuid()), 10)
}

func validatePrivateRepairPermissions(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return errRepairDirectoryPermissions
	}
	return nil
}

func validatePrivateRepairOwnership(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("managed runtime repair ownership is unavailable")
	}
	if int(stat.Uid) != os.Geteuid() {
		return errors.New("managed runtime repair owner does not match the execution user")
	}
	return nil
}

func syncRepairDirectory(directory string) {
	fd, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	_ = unix.Fsync(fd)
	_ = unix.Close(fd)
}

func openRepairContextLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errRepairLockHandle
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errRepairLockHandle
	}
	return file, nil
}

func tryLockRepairContext(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
		return false, nil
	}
	return err == nil, err
}

func unlockRepairContextLock(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
