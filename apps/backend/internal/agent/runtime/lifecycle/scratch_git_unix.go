//go:build linux || darwin

package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openGitExcludeNoFollow(gitDir string) (*os.File, error) {
	gitFD, err := unix.Open(gitDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open git directory: %w", err)
	}
	defer func() { _ = unix.Close(gitFD) }()

	infoFD, err := unix.Openat(gitFD, "info", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open git info directory: %w", err)
	}
	defer func() { _ = unix.Close(infoFD) }()

	fd, err := unix.Openat(
		infoFD,
		"exclude",
		unix.O_RDWR|unix.O_APPEND|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0644,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filepath.Join(gitDir, "info", "exclude"))
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("create file from git exclude descriptor")
	}
	return file, nil
}
