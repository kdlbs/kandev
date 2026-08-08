package metrics

import "errors"

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
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	return DiskCapacity{
		TotalBytes: total, AvailableBytes: available, UsedBytes: used, UsedPercent: percent,
	}, nil
}
