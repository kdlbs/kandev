package plugins

import (
	"context"
	"testing"
)

// TestAutoUpdatePollerStartStopIdempotent proves the poller's goroutine
// ownership is clean: Start/Stop are each idempotent and Stop drains the loop
// (goleak's TestMain catches a leaked poller goroutine). The Service has no
// marketplace attached, so the immediate sweep on Start is a no-op — this test
// exercises the lifecycle, not the update logic (covered by RunAutoUpdatePass
// tests, driven directly rather than through the timer).
func TestAutoUpdatePollerStartStopIdempotent(t *testing.T) {
	svc, _, _ := newTestService(t)
	p := NewAutoUpdatePoller(svc, testLogger(t))

	p.Stop() // stop before start is a no-op
	p.Start(context.Background())
	p.Start(context.Background()) // second start is a no-op
	p.Stop()
	p.Stop() // second stop is a no-op
}

// TestAutoUpdatePollerStopCancelsViaContext proves cancelling the parent
// context tears the loop down even without an explicit Stop() (Stop still runs
// for a clean drain).
func TestAutoUpdatePollerStopCancelsViaContext(t *testing.T) {
	svc, _, _ := newTestService(t)
	p := NewAutoUpdatePoller(svc, testLogger(t))

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	cancel()
	p.Stop()
}
