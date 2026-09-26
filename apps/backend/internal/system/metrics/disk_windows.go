//go:build windows

package metrics

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

const diskQueryTimeout = 2 * time.Second

// getDiskFreeSpaceEx is a package-level seam so tests can substitute a fake.
var getDiskFreeSpaceEx = windows.GetDiskFreeSpaceEx

type diskQueryResult struct {
	totalBytes uint64
	freeBytes  uint64
	err        error
}

func diskPercent(ctx context.Context, path string) (float64, error) {
	capacity, err := DiskUsage(ctx, path)
	if err != nil {
		return 0, err
	}
	return capacity.UsedPercent, nil
}

func DiskUsage(ctx context.Context, path string) (DiskCapacity, error) {
	dir, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return DiskCapacity{}, fmt.Errorf("convert path %q: %w", path, err)
	}
	release, err := acquireDiskUsageCall(ctx)
	if err != nil {
		return DiskCapacity{}, err
	}

	result := make(chan diskQueryResult, 1)
	go func() {
		defer release()
		var freeToCaller, totalBytes, totalFree uint64
		callErr := getDiskFreeSpaceEx(dir, &freeToCaller, &totalBytes, &totalFree)
		result <- diskQueryResult{totalBytes: totalBytes, freeBytes: freeToCaller, err: callErr}
	}()

	timer := time.NewTimer(diskQueryTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return DiskCapacity{}, ctx.Err()
	case <-timer.C:
		return DiskCapacity{}, fmt.Errorf("disk usage timed out for %q", path)
	case res := <-result:
		if res.err != nil {
			return DiskCapacity{}, fmt.Errorf("GetDiskFreeSpaceEx %q: %w", path, res.err)
		}
		return diskCapacityFromBytes(res.totalBytes, res.freeBytes)
	}
}

func diskPercentFromBytes(total, free uint64) (float64, error) {
	capacity, err := diskCapacityFromBytes(total, free)
	if err != nil {
		return 0, err
	}
	return capacity.UsedPercent, nil
}
