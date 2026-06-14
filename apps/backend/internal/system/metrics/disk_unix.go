//go:build !windows

package metrics

import (
	"errors"
	"syscall"
)

func diskPercent(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 0, errors.New("disk total is zero")
	}
	return (1 - float64(free)/float64(total)) * 100, nil
}
