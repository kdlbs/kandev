//go:build windows

package worktree

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"unsafe"
)

const ioReparseTagMountPoint = 0xA0000003

func createPlatformDirectoryLink(target, link string) error {
	if err := os.Mkdir(link, 0o755); err != nil {
		return err
	}
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(link), windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		_ = os.Remove(link)
		return err
	}
	defer windows.CloseHandle(h)
	b := mountPointBuffer(target)
	var n uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &b[0], uint32(len(b)), nil, 0, &n, nil); err != nil {
		_ = os.Remove(link)
		return err
	}
	return nil
}

func mountPointBuffer(target string) []byte {
	target = strings.TrimPrefix(target, `\\?\`)
	sub := `\??\` + target
	su, pr := windows.StringToUTF16(sub), windows.StringToUTF16(target)
	dataLen := 8 + len(su)*2 + len(pr)*2
	b := make([]byte, 8+dataLen)
	binary.LittleEndian.PutUint32(b, ioReparseTagMountPoint)
	binary.LittleEndian.PutUint16(b[4:], uint16(dataLen))
	binary.LittleEndian.PutUint16(b[8:], 0)
	binary.LittleEndian.PutUint16(b[10:], uint16(len(su)*2-2))
	binary.LittleEndian.PutUint16(b[12:], uint16(len(su)*2))
	binary.LittleEndian.PutUint16(b[14:], uint16(len(pr)*2-2))
	off := 16
	for _, v := range su {
		binary.LittleEndian.PutUint16(b[off:], v)
		off += 2
	}
	for _, v := range pr {
		binary.LittleEndian.PutUint16(b[off:], v)
		off += 2
	}
	return b
}
func isPlatformDirectoryLink(info os.FileInfo, path string) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return hasMountPoint(path)
}
func requirePlatformDirectoryLink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !isPlatformDirectoryLink(info, path) {
		return fmt.Errorf("not directory link")
	}
	return nil
}
func hasMountPoint(path string) bool {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	b := make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var n uint32
	if windows.DeviceIoControl(h, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &b[0], uint32(len(b)), &n, nil) != nil {
		return false
	}
	return binary.LittleEndian.Uint32(b) == ioReparseTagMountPoint
}

var _ = unsafe.Sizeof(0)
