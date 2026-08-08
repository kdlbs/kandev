package metrics

import (
	"context"
	"errors"
)

// DiskCapacity describes the space available on a filesystem.
type DiskCapacity struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedBytes      uint64
	UsedPercent    float64
}

func diskCapacityFromBytes(total, available uint64) (DiskCapacity, error) {
	if total == 0 {
		return DiskCapacity{}, errors.New("disk total is zero")
	}
	if available > total {
		available = total
	}
	used := total - available
	percent := float64(used) / float64(total) * 100
	return DiskCapacity{
		TotalBytes: total, AvailableBytes: available, UsedBytes: used, UsedPercent: percent,
	}, nil
}

// Only one potentially blocking platform call is admitted at a time. Context
// cancellation can return the request before the OS call does, so this guard
// prevents repeated refreshes from accumulating unbounded goroutines.
var diskUsageCall = make(chan struct{}, 1)

func acquireDiskUsageCall(ctx context.Context) (func(), error) {
	select {
	case diskUsageCall <- struct{}{}:
		return func() { <-diskUsageCall }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
