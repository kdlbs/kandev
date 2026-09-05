//go:build !linux && !darwin && !windows

package process

import (
	"context"
	"time"
)

// walkProcessTree returns "unknown" on unsupported platforms.
func walkProcessTree(_ context.Context, _ rootIdentity, _ turnStartMarker) string {
	return probeResultUnknown
}

// captureRootIdentity always fails on unsupported platforms.
func captureRootIdentity(_ int) (rootIdentity, bool) {
	return rootIdentity{}, false
}

// newTurnStartMarker records the wall-clock stamp only; unsupported
// platforms never populate bootTicks since walkProcessTree always answers
// unknown here regardless.
func newTurnStartMarker(t time.Time) turnStartMarker {
	return turnStartMarker{wallTime: t}
}
