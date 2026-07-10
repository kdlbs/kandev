//go:build windows

package metrics

import (
	"context"
	"errors"
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
	dir, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("convert path %q: %w", path, err)
	}

	result := make(chan diskQueryResult, 1)
	go func() {
		var freeToCaller, totalBytes, totalFree uint64
		callErr := getDiskFreeSpaceEx(dir, &freeToCaller, &totalBytes, &totalFree)
		result <- diskQueryResult{totalBytes: totalBytes, freeBytes: totalFree, err: callErr}
	}()

	timer := time.NewTimer(diskQueryTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return 0, fmt.Errorf("disk usage timed out for %q", path)
	case res := <-result:
		if res.err != nil {
			return 0, fmt.Errorf("GetDiskFreeSpaceEx %q: %w", path, res.err)
		}
		return diskPercentFromBytes(res.totalBytes, res.freeBytes)
	}
}

func diskPercentFromBytes(total, free uint64) (float64, error) {
	if total == 0 {
		return 0, errors.New("disk total is zero")
	}
	return (1 - float64(free)/float64(total)) * 100, nil
}
