//go:build !windows

package workspaces

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func (h *unixDirectoryHandle) ReadFileLimit(name string, maxBytes int64) ([]byte, error) {
	file, err := h.OpenFile(name)
	if err != nil {
		return nil, err
	}
	reader := io.Reader(file)
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes+1)
	}
	content, readErr := io.ReadAll(reader)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if maxBytes > 0 && int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("directory entry exceeds %d bytes: %s", maxBytes, name)
	}
	return content, nil
}

func (h *unixDirectoryHandle) ReadContextEntries() ([]DirectoryEntry, error) {
	entries, err := h.ReadDir()
	if err != nil {
		return nil, err
	}
	result := make([]DirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		var info unix.Stat_t
		if err := unix.Fstatat(h.targetFD, entry.Name(), &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return nil, err
		}
		result = append(result, DirectoryEntry{
			Name: entry.Name(), Mode: unixFileMode(info.Mode), Size: info.Size,
		})
	}
	return result, nil
}

type unixModeBits interface {
	~uint16 | ~uint32
}

func unixFileMode[T unixModeBits](rawMode T) os.FileMode {
	mode := uint32(rawMode)
	result := os.FileMode(mode & 0o777)
	switch mode & unix.S_IFMT {
	case unix.S_IFDIR:
		result |= os.ModeDir
	case unix.S_IFLNK:
		result |= os.ModeSymlink
	case unix.S_IFREG:
	default:
		result |= os.ModeDevice
		if mode&unix.S_IFIFO != 0 {
			result |= os.ModeNamedPipe
		}
		if mode&unix.S_IFSOCK != 0 {
			result |= os.ModeSocket
		}
	}
	if mode&unix.S_ISUID != 0 {
		result |= os.ModeSetuid
	}
	if mode&unix.S_ISGID != 0 {
		result |= os.ModeSetgid
	}
	if mode&unix.S_ISVTX != 0 {
		result |= os.ModeSticky
	}
	return result
}

func (h *unixDirectoryHandle) CreateFile(name string, data []byte, mode os.FileMode) error {
	if h == nil || h.targetFD < 0 {
		return errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return err
	}
	fd, err := unix.Openat(h.targetFD, name,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		uint32(mode.Perm()),
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("create file from directory handle")
	}
	if err := file.Chmod(mode.Perm()); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (h *unixDirectoryHandle) WriteFileAtomic(name string, data []byte, mode os.FileMode) error {
	if h == nil || h.targetFD < 0 {
		return errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("generate temporary file name: %w", err)
	}
	tempName := ".context-write-" + hex.EncodeToString(nonce[:])
	tempFD, err := unix.Openat(h.targetFD, tempName,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		uint32(mode.Perm()),
	)
	if err != nil {
		return err
	}
	tempFile := os.NewFile(uintptr(tempFD), tempName)
	if tempFile == nil {
		_ = unix.Close(tempFD)
		_ = unix.Unlinkat(h.targetFD, tempName, 0)
		return errors.New("create temporary file from directory handle")
	}
	renamed := false
	defer func() {
		_ = tempFile.Close()
		if !renamed {
			_ = unix.Unlinkat(h.targetFD, tempName, 0)
		}
	}()
	if err := tempFile.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := tempFile.Write(data); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := unix.Renameat(h.targetFD, tempName, h.targetFD, name); err != nil {
		return err
	}
	renamed = true
	return unix.Fsync(h.targetFD)
}
