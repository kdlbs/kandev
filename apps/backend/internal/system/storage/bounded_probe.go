package storage

import (
	"context"
	"fmt"
	"time"
)

const maxConcurrentDiskProbes = 8

var diskProbeSlots = make(chan struct{}, maxConcurrentDiskProbes)

type diskProbeResult[T any] struct {
	value T
	err   error
}

// boundedDiskProbe limits both the time a request waits and the number of
// filesystem operations that can remain blocked after their callers return.
func boundedDiskProbe[T any](ctx context.Context, timeout time.Duration, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := probeCtx.Err(); err != nil {
		return zero, err
	}
	select {
	case diskProbeSlots <- struct{}{}:
	case <-probeCtx.Done():
		return zero, probeCtx.Err()
	default:
		return zero, fmt.Errorf("disk probe limit reached")
	}
	if err := probeCtx.Err(); err != nil {
		<-diskProbeSlots
		return zero, err
	}

	resultCh := make(chan diskProbeResult[T], 1)
	go func() {
		var result diskProbeResult[T]
		defer func() {
			<-diskProbeSlots
			if recovered := recover(); recovered != nil {
				result.err = fmt.Errorf("disk probe panicked: %v", recovered)
			}
			resultCh <- result
		}()
		result.value, result.err = operation(probeCtx)
	}()

	select {
	case result := <-resultCh:
		return result.value, result.err
	case <-probeCtx.Done():
		return zero, probeCtx.Err()
	}
}
