package api

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// hostProbeTimeout bounds the one-time host confinement probe.
const hostProbeTimeout = 5 * time.Second

var hostProbeOnce sync.Once

// probeSandboxHost runs the host confinement probe at most once per process and
// must complete before the first request is served. The result is immutable.
func probeSandboxHost(report func(error)) {
	hostProbeOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			return
		}
		SurvivalCapabilities = executionCapabilities(runtime.GOOS, runHostProbe(seatbeltHostProbe, report))
	})
}

// runHostProbe fails closed: an error, a timeout, or a panic is reported and
// leaves the capability absent.
func runHostProbe(probe func(context.Context) error, report func(error)) (passed bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			report(fmt.Errorf("sandbox host probe panicked: %v", recovered))
			passed = false
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), hostProbeTimeout)
	defer cancel()
	if err := probe(ctx); err != nil {
		report(err)
		return false
	}
	return true
}
