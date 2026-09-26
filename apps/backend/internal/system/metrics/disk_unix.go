//go:build !windows

package metrics

import (
	"context"
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
	capacity, err := DiskUsage(ctx, path)
	if err != nil {
		return 0, err
	}
	return capacity.UsedPercent, nil
}

func DiskUsage(ctx context.Context, path string) (DiskCapacity, error) {
	release, err := acquireDiskUsageCall(ctx)
	if err != nil {
		return DiskCapacity{}, err
	}
	result := make(chan diskStatfsResult, 1)
	go func() {
		defer release()
		var stat syscall.Statfs_t
		callErr := statfs(path, &stat)
		result <- diskStatfsResult{stat: stat, err: callErr}
	}()

	timer := time.NewTimer(diskStatfsTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return DiskCapacity{}, ctx.Err()
	case <-timer.C:
		return DiskCapacity{}, fmt.Errorf("disk usage timed out for %q", path)
	case res := <-result:
		if res.err != nil {
			return DiskCapacity{}, res.err
		}
		return diskCapacityFromStatfs(res.stat)
	}
}

func diskPercentFromStatfs(stat syscall.Statfs_t) (float64, error) {
	capacity, err := diskCapacityFromStatfs(stat)
	if err != nil {
		return 0, err
	}
	return capacity.UsedPercent, nil
}

func diskCapacityFromStatfs(stat syscall.Statfs_t) (DiskCapacity, error) {
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	return diskCapacityFromBytes(total, available)
}
