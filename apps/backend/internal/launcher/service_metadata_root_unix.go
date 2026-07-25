//go:build unix

package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

const nativeMetadataDirectoryOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

type unixNativeMetadataRoot struct {
	dir *os.File
}

func openSystemNativeMetadataHome(homeDir string) (nativeMetadataRoot, error) {
	if !filepath.IsAbs(homeDir) {
		return nil, fmt.Errorf("system service metadata home must be absolute")
	}
	clean := canonicalSystemMetadataHome(runtime.GOOS, homeDir)
	rootFD, err := unix.Open(string(filepath.Separator), nativeMetadataDirectoryOpenFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("open filesystem root: %w", err)
	}
	current := os.NewFile(uintptr(rootFD), string(filepath.Separator))
	if current == nil {
		_ = unix.Close(rootFD)
		return nil, fmt.Errorf("open filesystem root: invalid directory handle")
	}
	for _, component := range systemMetadataPathComponents(clean) {
		next, err := openSystemMetadataDir(current, component)
		if err != nil {
			_ = current.Close()
			return nil, err
		}
		_ = current.Close()
		current = next
	}
	return &unixNativeMetadataRoot{dir: current}, nil
}

func canonicalSystemMetadataHome(goos, homeDir string) string {
	clean := filepath.Clean(homeDir)
	if goos == goosDarwin && (clean == "/var" || strings.HasPrefix(clean, "/var/")) {
		return "/private" + clean
	}
	return clean
}

func systemMetadataPathComponents(path string) []string {
	trimmed := strings.TrimPrefix(filepath.ToSlash(path), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func openSystemMetadataDir(parent *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, nativeMetadataDirectoryOpenFlags, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, fmt.Errorf(
			"system metadata home component %q does not exist; pre-create and chown the system home before installing",
			name,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("open system metadata path component %q without symlinks: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open system metadata path component %q: invalid directory handle", name)
	}
	return file, nil
}

func (r *unixNativeMetadataRoot) Close() error {
	return r.dir.Close()
}

func (r *unixNativeMetadataRoot) EntryMode(name string) (os.FileMode, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(r.dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, &os.PathError{Op: "fstatat", Path: name, Err: err}
	}
	return unixStatMode(stat.Mode), nil
}

func unixStatMode[T ~uint16 | ~uint32](rawMode T) os.FileMode {
	modeBits := uint32(rawMode)
	mode := os.FileMode(modeBits & 0o777)
	switch modeBits & unix.S_IFMT {
	case unix.S_IFDIR:
		mode |= os.ModeDir
	case unix.S_IFLNK:
		mode |= os.ModeSymlink
	}
	return mode
}

func (r *unixNativeMetadataRoot) Mkdir(name string, perm os.FileMode) error {
	return unix.Mkdirat(int(r.dir.Fd()), name, uint32(perm.Perm()))
}

func (r *unixNativeMetadataRoot) OpenRoot(name string) (nativeMetadataRoot, error) {
	fd, err := unix.Openat(int(r.dir.Fd()), name, nativeMetadataDirectoryOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open metadata directory %q: invalid directory handle", name)
	}
	return &unixNativeMetadataRoot{dir: file}, nil
}

func (r *unixNativeMetadataRoot) OpenSelf() (*os.File, error) {
	fd, err := unix.Openat(int(r.dir.Fd()), ".", nativeMetadataDirectoryOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), r.dir.Name())
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("duplicate metadata directory: invalid directory handle")
	}
	return file, nil
}

func (r *unixNativeMetadataRoot) CreateExclusive(name string, perm os.FileMode) (*os.File, error) {
	flags := unix.O_CREAT | unix.O_EXCL | unix.O_WRONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	fd, err := unix.Openat(int(r.dir.Fd()), name, flags, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("create metadata file %q: invalid file handle", name)
	}
	return file, nil
}

func (r *unixNativeMetadataRoot) Rename(oldName, newName string) error {
	fd := int(r.dir.Fd())
	return unix.Renameat(fd, oldName, fd, newName)
}

func (r *unixNativeMetadataRoot) Remove(name string) error {
	return unix.Unlinkat(int(r.dir.Fd()), name, 0)
}
