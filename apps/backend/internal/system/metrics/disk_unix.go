//go:build !windows

package metrics

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
)

const diskStatfsTimeout = 2 * time.Second

var statfs = syscall.Statfs

type diskStatfsResult struct {
	stat syscall.Statfs_t
	err  error
}

func diskPercent(ctx context.Context, path string) (float64, error) {
	result := make(chan diskStatfsResult, 1)
	go func() {
		var stat syscall.Statfs_t
		err := statfs(path, &stat)
		result <- diskStatfsResult{stat: stat, err: err}
	}()

	timer := time.NewTimer(diskStatfsTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return 0, fmt.Errorf("disk usage timed out for %q", path)
	case res := <-result:
		if res.err != nil {
			return 0, res.err
		}
		return diskPercentFromStatfs(res.stat)
	}
}

func diskPercentFromStatfs(stat syscall.Statfs_t) (float64, error) {
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 0, errors.New("disk total is zero")
	}
	return (1 - float64(free)/float64(total)) * 100, nil
}
