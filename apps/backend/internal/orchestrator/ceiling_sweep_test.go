package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCeilingSweeper_TicksOnIntervalAndStops(t *testing.T) {
	r := newCeilingSweeper()
	r.interval = 5 * time.Millisecond
	var ticks atomic.Int32
	require.True(t, r.start(context.Background(), func(context.Context) { ticks.Add(1) }))

	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	require.GreaterOrEqual(t, ticks.Load(), int32(2))

	r.stop()
	afterStop := ticks.Load()
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, afterStop, ticks.Load(), "no further ticks after stop")
}

func TestCeilingSweeper_SignalNowTriggersAPromptTick(t *testing.T) {
	// A long interval means only a signal, not the ticker, can produce a tick
	// within the test's deadline (AC-15).
	r := newCeilingSweeper()
	r.interval = time.Hour
	var ticks atomic.Int32
	require.True(t, r.start(context.Background(), func(context.Context) { ticks.Add(1) }))
	defer r.stop()

	r.signalNow()
	deadline := time.Now().Add(500 * time.Millisecond)
	for ticks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, int32(1), ticks.Load())
}

func TestCeilingSweeper_SignalCoalescesWhilePassIsInFlight(t *testing.T) {
	// AC-15e: a signal received while a pass is already running or pending
	// coalesces into exactly one further pass, not one per signal.
	r := newCeilingSweeper()
	r.interval = time.Hour
	tickStarted := make(chan struct{}, 8)
	release := make(chan struct{})
	var ticks atomic.Int32
	require.True(t, r.start(context.Background(), func(context.Context) {
		ticks.Add(1)
		tickStarted <- struct{}{}
		<-release
	}))
	defer func() {
		close(release)
		r.stop()
	}()

	r.signalNow()
	<-tickStarted // first tick is now blocked inside release

	// Every one of these must coalesce into at most one further tick.
	r.signalNow()
	r.signalNow()
	r.signalNow()

	release <- struct{}{} // let the first tick finish
	<-tickStarted         // the one coalesced pass
	require.Equal(t, int32(2), ticks.Load())

	// No third tick should follow from the extra signals.
	select {
	case <-tickStarted:
		t.Fatal("a third tick fired; signals must coalesce into one further pass")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestService_SignalCeilingSweep_NilSweeperIsANoOp(t *testing.T) {
	svc := &Service{}
	require.NotPanics(t, svc.signalCeilingSweep)
}

func TestService_StartStopCeilingSweeper_NilSweeperIsANoOp(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	require.NotPanics(t, func() { svc.startCeilingSweeper(context.Background()) })
	require.NotPanics(t, svc.stopCeilingSweeper)
}

func TestCeilingSweepTick_ExpiresStaleReservationsAndDrains(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	fixedStart := time.Now()
	svc.sessionCeiling.now = func() time.Time { return fixedStart }

	decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
		taskID: "stale-task", sessionID: "", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted)
	require.NotEmpty(t, decision.reservationKey)

	// Advance the controller's clock well past the launch budget, so the tick's
	// AC-7 backstop finds this reservation stale.
	svc.sessionCeiling.now = func() time.Time { return fixedStart.Add(24 * time.Hour) }
	svc.ceilingSweepTick(context.Background())

	svc.sessionCeiling.mu.Lock()
	_, held := svc.sessionCeiling.reservations[decision.reservationKey]
	svc.sessionCeiling.mu.Unlock()
	require.False(t, held, "a tick must run the AC-7 backstop and expire a stale reservation")
}
