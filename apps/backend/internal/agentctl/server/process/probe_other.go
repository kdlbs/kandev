//go:build !linux && !darwin && !windows

package process

import (
	"context"
	"time"
)

// walkProcessTree returns "unknown" on unsupported platforms.
func walkProcessTree(_ context.Context, _ int, _ time.Time) string {
	return probeResultUnknown
}
