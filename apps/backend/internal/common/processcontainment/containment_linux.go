//go:build linux

package processcontainment

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const kernelRoot = "/sys/fs/cgroup"
const leafPrefix = "kandev-sandbox-"

// Create allocates only a new leaf under this host process's cgroup. It never
// moves the host, enables controllers, or touches a sibling service.
func Create() (*Identity, error) {
	base, err := currentBase()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, leafPrefix+uuid.NewString())
	if err = os.Mkdir(path, 0700); err != nil {
		return nil, ErrUnavailable
	}
	file, err := openDirectory(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var stat unix.Stat_t
	boot, err := bootID()
	if err != nil || unix.Fstat(int(file.Fd()), &stat) != nil {
		_ = os.Remove(path)
		return nil, ErrUnavailable
	}
	return &Identity{Path: path, BootID: boot, Inode: stat.Ino}, nil
}

// Configure uses clone3 to place the child in the cgroup before any executable
// can run. The caller closes the returned descriptor after Start returns.
func (i *Identity) Configure(cmd *exec.Cmd) (*os.File, error) {
	if cmd == nil || cmd.Process != nil {
		return nil, ErrUnavailable
	}
	file, err := i.open()
	if err != nil {
		return nil, err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.UseCgroupFD = true
	cmd.SysProcAttr.CgroupFD = int(file.Fd())
	return file, nil
}

func (i *Identity) Empty() (bool, error) {
	file, err := i.open()
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	return empty(file)
}

// Kill targets only the captured kernel object, including detached descendants.
func (i *Identity) Kill(ctx context.Context) error {
	file, err := i.open()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	fd, err := unix.Openat(int(file.Fd()), "cgroup.kill", unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ErrUnavailable
	}
	_, writeErr := unix.Write(fd, []byte("1"))
	closeErr := unix.Close(fd)
	if writeErr != nil || closeErr != nil {
		return ErrUnavailable
	}
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		stopped, err := empty(file)
		if err != nil {
			return err
		}
		if stopped {
			return nil
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
		}
	}
}

func (i *Identity) Remove() error {
	file, err := i.open()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	stopped, err := empty(file)
	if err != nil || !stopped {
		return ErrUnavailable
	}
	return os.Remove(i.Path)
}

func (i *Identity) open() (*os.File, error) {
	if err := i.validate(); err != nil {
		return nil, err
	}
	file, err := openDirectory(i.Path)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Ino != i.Inode {
		_ = file.Close()
		return nil, ErrUnavailable
	}
	return file, nil
}

func (i *Identity) validate() error {
	if i == nil || i.Inode == 0 || !strings.HasPrefix(filepath.Base(i.Path), leafPrefix) {
		return ErrUnavailable
	}
	suffix := strings.TrimPrefix(filepath.Base(i.Path), leafPrefix)
	if parsed, err := uuid.Parse(suffix); err != nil || parsed.String() != suffix {
		return ErrUnavailable
	}
	base, err := currentBase()
	if err != nil || filepath.Dir(i.Path) != base || filepath.Clean(i.Path) != i.Path {
		return ErrUnavailable
	}
	boot, err := bootID()
	if err != nil || boot != i.BootID {
		return ErrUnavailable
	}
	return nil
}

func currentBase() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", ErrUnavailable
	}
	for _, line := range strings.Split(string(data), "\n") {
		path, ok := strings.CutPrefix(line, "0::")
		if !ok {
			continue
		}
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
			return "", ErrUnavailable
		}
		base := filepath.Join(kernelRoot, path)
		canonical, err := filepath.EvalSymlinks(base)
		if err != nil || canonical != base {
			return "", ErrUnavailable
		}
		return base, nil
	}
	return "", ErrUnavailable
}

func openDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Statfs_t
	if unix.Fstatfs(fd, &stat) != nil || stat.Type != unix.CGROUP2_SUPER_MAGIC {
		_ = unix.Close(fd)
		return nil, ErrUnavailable
	}
	return os.NewFile(uintptr(fd), path), nil
}

func empty(file *os.File) (bool, error) {
	fd, err := unix.Openat(int(file.Fd()), "cgroup.events", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, ErrUnavailable
	}
	events := os.NewFile(uintptr(fd), "cgroup.events")
	defer func() { _ = events.Close() }()
	data := make([]byte, 4096)
	n, err := events.Read(data)
	if err != nil {
		return false, ErrUnavailable
	}
	for _, line := range strings.Split(string(data[:n]), "\n") {
		switch line {
		case "populated 0":
			return true, nil
		case "populated 1":
			return false, nil
		}
	}
	return false, ErrUnavailable
}

func bootID() (string, error) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", ErrUnavailable
	}
	return strings.TrimSpace(string(raw)), nil
}

// Contains verifies current kernel membership under this exact owned leaf.
func (i *Identity) Contains(pid int) (bool, error) {
	file, err := i.open()
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	if pid <= 0 {
		return false, ErrUnavailable
	}
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return false, ErrUnavailable
	}
	for _, line := range strings.Split(string(raw), "\n") {
		path, ok := strings.CutPrefix(line, "0::")
		if !ok {
			continue
		}
		expected := strings.TrimPrefix(i.Path, kernelRoot)
		return path == expected || strings.HasPrefix(path, expected+"/"), nil
	}
	return false, ErrUnavailable
}
