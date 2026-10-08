//go:build windows

package workspaces

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (h *windowsDirectoryHandle) ReadFileLimit(name string, maxBytes int64) ([]byte, error) {
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

func (h *windowsDirectoryHandle) ReadContextEntries() ([]DirectoryEntry, error) {
	entries, err := h.ReadDir()
	if err != nil {
		return nil, err
	}
	result := make([]DirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		mode, err := h.LstatEntry(entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var size int64
		if mode.IsRegular() {
			file, openErr := h.OpenFile(entry.Name())
			if errors.Is(openErr, os.ErrNotExist) {
				continue
			}
			if openErr != nil {
				return nil, openErr
			}
			statter, ok := file.(interface{ Stat() (os.FileInfo, error) })
			if !ok {
				_ = file.Close()
				return nil, errors.New("opened directory entry does not expose file metadata")
			}
			info, statErr := statter.Stat()
			closeErr := file.Close()
			if statErr != nil {
				return nil, statErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			size = info.Size()
		}
		result = append(result, DirectoryEntry{Name: entry.Name(), Mode: mode, Size: size})
	}
	return result, nil
}

func (h *windowsDirectoryHandle) CreateFile(name string, data []byte, _ os.FileMode) error {
	if h == nil || h.targetHandle == 0 {
		return errors.New("directory handle is closed")
	}
	if err := validateDirectoryEntryName(name); err != nil {
		return err
	}
	handle, err := openWindowsDependencyHandleWithDisposition(
		h.targetHandle, name, windowsDependencyWriteAccess, windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), filepath.Join("worktree-directory", name))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("create file from directory handle")
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

func (h *windowsDirectoryHandle) WriteFileAtomic(name string, data []byte, _ os.FileMode) error {
	if h == nil || h.targetHandle == 0 {
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
	handle, err := openWindowsDependencyHandleWithDisposition(
		h.targetHandle, tempName, windowsDependencyWriteAccess|windows.DELETE,
		windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), tempName)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("create temporary file from directory handle")
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = markWindowsDependencyForDelete(handle)
		}
		_ = file.Close()
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := renameWindowsContextDirectoryEntry(handle, h.targetHandle, name); err != nil {
		return err
	}
	renamed = true
	return nil
}

func renameWindowsContextDirectoryEntry(file, parent windows.Handle, name string) error {
	encoded, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	encoded = encoded[:len(encoded)-1]
	var info windowsContextFileRenameInformation
	bufferSize := int(unsafe.Offsetof(info.FileName)) + len(encoded)*2
	buffer := make([]byte, bufferSize)
	infoPtr := (*windowsContextFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	infoPtr.ReplaceIfExists = 1
	infoPtr.RootDirectory = parent
	infoPtr.FileNameLength = uint32(len(encoded) * 2)
	copy(unsafe.Slice(&infoPtr.FileName[0], len(encoded)), encoded)
	return windows.SetFileInformationByHandle(
		file,
		windows.FileRenameInfo,
		(*byte)(unsafe.Pointer(&buffer[0])),
		uint32(len(buffer)),
	)
}

type windowsContextFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}
